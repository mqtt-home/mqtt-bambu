package main

import (
	"encoding/json"
	"errors"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mqtt-home/mqtt-bambu/bambu"
	"github.com/mqtt-home/mqtt-bambu/config"
	"github.com/mqtt-home/mqtt-bambu/version"
	"github.com/mqtt-home/mqtt-bambu/web"
	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/mqtt"
)

var (
	cloud     *bambu.Client
	manager   *bambu.Manager
	webServer *web.WebServer

	cloudOnce sync.Once
	stopPush  chan struct{}
)

func main() {
	logger.Init("info", logger.Logger())
	logger.Info("mqtt-bambu", "version", version.Info())
	initPprof()

	if len(os.Args) < 2 {
		logger.Error("No configuration file specified")
		os.Exit(1)
	}

	configFile := os.Args[1]
	logger.Info("Configuration file", "path", configFile)

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		return
	}
	logger.SetLevel(cfg.LogLevel)

	// Start the home MQTT broker connection first (status sink).
	mqtt.Start(cfg.MQTT, "bambu_mqtt")

	manager = bambu.NewManager()
	manager.SetStatusListener(publishStatus)
	manager.SetAvailabilityListener(publishAvailability)

	cloud = bambu.NewClient(cfg.Bambu.Region, cfg.Bambu.Email, cfg.Bambu.Password, cfg.Bambu.SessionFile)

	// Start the web server early so the login UI is reachable before auth.
	if cfg.Web.Enabled {
		webServer = web.NewWebServer(cloud, manager, startCloudBridge)
		go func() {
			if err := webServer.Start(cfg.Web.Port); err != nil {
				logger.Error("Failed to start web server", "error", err)
			}
		}()
		logger.Info("Web interface available", "url", "http://localhost:"+strconv.Itoa(cfg.Web.Port))
	} else {
		logger.Info("Web interface is disabled in the configuration")
	}

	// LAN printers need no authentication, so they connect straight away and
	// keep working regardless of the cloud's state.
	startLANBridge(cfg.Bambu.LAN)

	// Authenticate: reuse a persisted session, else log in with the configured
	// credentials. When a verification code is required, the web UI completes it.
	if cfg.Bambu.CloudEnabled() {
		authenticate()
	} else {
		logger.Info("Bambu cloud disabled (no email configured), running LAN-only")
	}

	// Reports are partial deltas, so periodically request a full snapshot. The
	// loop covers cloud printers too once they join.
	stopPush = make(chan struct{})
	go pushAllLoop(time.Duration(cfg.Bambu.PushAllInterval)*time.Second, stopPush)

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel

	logger.Info("Received quit signal")
	close(stopPush)
	shuttingDown.Store(true)
	manager.DisconnectAll()
}

// startLANBridge connects the printers configured for LAN mode.
func startLANBridge(printers []config.LANPrinter) {
	if len(printers) == 0 {
		return
	}

	lan := make([]bambu.LANPrinter, 0, len(printers))
	for _, p := range printers {
		lan = append(lan, bambu.LANPrinter{
			Name:       p.Name,
			Model:      p.Model,
			Serial:     p.Serial,
			Host:       p.Host,
			Port:       p.Port,
			AccessCode: p.AccessCode,
		})
	}

	added := manager.AddLANPrinters(lan)
	connectPrinters(added)
	logger.Info("LAN printers started", "printers", len(added))
}

// authenticate tries a cached session, then a direct login. If the account needs
// an emailed code or TOTP, it logs that and leaves the web login flow to finish.
func authenticate() {
	if cloud.LoadSession() {
		// An opaque token cannot be checked offline. Ask the cloud: a rejected
		// token would otherwise be reloaded on every restart while the printer
		// connections fail forever.
		err := cloud.DiscoverDevices()
		if !errors.Is(err, bambu.ErrUnauthorized) {
			if err != nil {
				logger.Warn("Could not verify the persisted Bambu session, using it as is", "error", err)
			} else if serr := cloud.SaveSession(); serr != nil {
				logger.Warn("Failed to save session", "error", serr)
			}
			startCloudBridge()
			return
		}
		logger.Warn("Bambu cloud rejected the persisted session, logging in again", "error", err)
		cloud.ClearSession()
	}

	err := cloud.Login()
	switch {
	case err == nil:
		if derr := cloud.DiscoverDevices(); derr != nil {
			logger.Error("Device discovery failed", "error", derr)
			return
		}
		if serr := cloud.SaveSession(); serr != nil {
			logger.Warn("Failed to save session", "error", serr)
		}
		startCloudBridge()
	case errors.Is(err, bambu.ErrVerificationRequired):
		logger.Warn("Email verification code required — complete login via the web UI", "url", webURL())
	case errors.Is(err, bambu.ErrTFARequired):
		logger.Warn("Two-factor code required — complete login via the web UI", "url", webURL())
	default:
		logger.Error("Bambu cloud login failed", "error", err)
	}
}

// startCloudBridge adds the account's printers to the manager and connects them.
// It is safe to call from both the startup path and the web login callback; it
// runs once.
func startCloudBridge() {
	cloudOnce.Do(func() {
		devices := cloud.Devices()
		if len(devices) == 0 {
			logger.Warn("No Bambu devices bound to this account")
		}

		added := manager.AddCloudDevices(devices, cloud.MQTTHost(), cloud.MQTTUsername(), cloud.AccessToken())
		connectPrinters(added)
		logger.Info("Cloud printers started", "printers", len(added))
	})
}

// connectPrinters seeds availability as offline, then opens each session so the
// home broker never shows a printer as online before it actually is.
func connectPrinters(printers []*bambu.ManagedPrinter) {
	for _, mp := range printers {
		publishAvailability(mp.Slug, false)
	}
	bambu.Connect(printers)
}

func pushAllLoop(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			manager.PushAll()
		case <-stop:
			return
		}
	}
}

func publishStatus(slug string, status bambu.PublishedStatus) {
	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/status"

	data, err := json.Marshal(status)
	if err != nil {
		logger.Error("Failed to marshal status", "error", err)
		return
	}
	mqtt.PublishAbsolute(topic, string(data), cfg.MQTT.Retain)
	logger.Debug("Published status", "topic", topic, "remaining", status.RemainingMinutes, "percent", status.Percent)

	if webServer != nil {
		webServer.BroadcastStatus(slug, status)
	}
}

// shuttingDown is set once the process got its termination signal.
var shuttingDown atomic.Bool

func publishAvailability(slug string, online bool) {
	// A bridge that is shutting down must not speak for its printers. In a
	// rolling update the replacement pod has already announced "online", and
	// the "offline" that DisconnectAll triggers would land on top of it and
	// stay until the next restart. bridge/state says whether the bridge is gone.
	if shuttingDown.Load() && !online {
		logger.Debug("Shutting down, not publishing availability", "printer", slug)
		return
	}

	cfg := config.Get()
	topic := cfg.MQTT.Topic + "/" + slug + "/availability"
	payload := "offline"
	if online {
		payload = "online"
	}
	mqtt.PublishAbsolute(topic, payload, cfg.MQTT.Retain)
	logger.Debug("Published availability", "topic", topic, "online", online)

	if webServer != nil {
		webServer.BroadcastAvailability(slug, online)
	}
}

func webURL() string {
	return "http://localhost:" + strconv.Itoa(config.Get().Web.Port)
}

func initPprof() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}

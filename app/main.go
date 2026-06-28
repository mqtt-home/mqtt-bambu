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

	bridgeOnce sync.Once
	stopPush   chan struct{}
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

	cloud = bambu.NewClient(cfg.Bambu.Region, cfg.Bambu.Email, cfg.Bambu.Password, cfg.Bambu.SessionFile)

	// Start the web server early so the login UI is reachable before auth.
	if cfg.Web.Enabled {
		webServer = web.NewWebServer(cloud, startBridge)
		go func() {
			if err := webServer.Start(cfg.Web.Port); err != nil {
				logger.Error("Failed to start web server", "error", err)
			}
		}()
		logger.Info("Web interface available", "url", "http://localhost:"+strconv.Itoa(cfg.Web.Port))
	} else {
		logger.Info("Web interface is disabled in the configuration")
	}

	// Authenticate: reuse a persisted session, else log in with the configured
	// credentials. When a verification code is required, the web UI completes it.
	authenticate()

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel

	logger.Info("Received quit signal")
	if stopPush != nil {
		close(stopPush)
	}
	if manager != nil {
		manager.DisconnectAll()
	}
}

// authenticate tries a cached session, then a direct login. If the account needs
// an emailed code or TOTP, it logs that and leaves the web login flow to finish.
func authenticate() {
	if cloud.LoadSession() {
		startBridge()
		return
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
		startBridge()
	case errors.Is(err, bambu.ErrVerificationRequired):
		logger.Warn("Email verification code required — complete login via the web UI", "url", webURL())
	case errors.Is(err, bambu.ErrTFARequired):
		logger.Warn("Two-factor code required — complete login via the web UI", "url", webURL())
	default:
		logger.Error("Bambu cloud login failed", "error", err)
	}
}

// startBridge connects to the printers and begins publishing. It is safe to call
// from both the startup path and the web login callback; it runs once.
func startBridge() {
	bridgeOnce.Do(func() {
		cfg := config.Get()
		devices := cloud.Devices()
		if len(devices) == 0 {
			logger.Warn("No Bambu devices bound to this account")
		}

		manager = bambu.NewManager(devices, cloud.MQTTHost(), cloud.MQTTUsername(), cloud.AccessToken())
		manager.SetStatusListener(publishStatus)
		manager.SetAvailabilityListener(publishAvailability)

		if webServer != nil {
			webServer.SetManager(manager)
		}

		// Seed availability as offline until each printer connects.
		for slug := range manager.ConnectionStates() {
			publishAvailability(slug, false)
		}

		manager.ConnectAll()

		// Periodically request a full snapshot, since reports are partial deltas.
		stopPush = make(chan struct{})
		interval := time.Duration(cfg.Bambu.PushAllInterval) * time.Second
		go pushAllLoop(interval, stopPush)

		logger.Info("Bridge started", "printers", len(devices))
	})
}

func pushAllLoop(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if manager != nil {
				manager.PushAll()
			}
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

func publishAvailability(slug string, online bool) {
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

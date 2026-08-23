package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/philipparndt/go-logger"
	"github.com/philipparndt/mqtt-gateway/config"
)

var cfg Config

// Region selects which Bambu Lab cloud endpoint to talk to.
const (
	RegionGlobal = "global"
	RegionChina  = "china"
)

// defaultLANPort is the MQTT port a printer in LAN mode listens on.
const defaultLANPort = 8883

type Config struct {
	MQTT     config.MQTTConfig `json:"mqtt"`
	Bambu    BambuConfig       `json:"bambu"`
	Web      WebConfig         `json:"web"`
	LogLevel string            `json:"loglevel,omitempty"`
}

type BambuConfig struct {
	// Region is "global" (api.bambulab.com) or "china" (api.bambulab.cn).
	Region string `json:"region,omitempty"`
	// Email is the Bambu Lab account login (email or phone). Leave empty to
	// disable the cloud connection entirely and run LAN-only.
	Email string `json:"email,omitempty"`
	// Password is the Bambu Lab account password. May be empty when the account
	// uses email-code login only; in that case authenticate via the web UI.
	Password string `json:"password,omitempty"`
	// PushAllInterval is how often (seconds) to request a full status snapshot
	// from each printer, since reports are otherwise partial deltas.
	PushAllInterval int `json:"pushall_interval,omitempty"`
	// SessionFile is where the cloud token/device cache is persisted so the
	// bridge survives restarts without re-authenticating.
	SessionFile string `json:"session_file,omitempty"`
	// LAN lists printers switched to LAN-only mode, which are reached directly
	// on the local network instead of through the cloud. Cloud and LAN printers
	// can be mixed freely; a LAN entry takes precedence over the cloud device
	// with the same serial.
	LAN []LANPrinter `json:"lan,omitempty"`
}

// LANPrinter describes a printer in LAN mode. Such a printer is not reachable
// through the cloud broker, so the bridge connects to the MQTT server the
// printer itself runs, authenticating with the LAN access code shown on the
// printer's display (Settings -> Network -> LAN Only Mode).
type LANPrinter struct {
	// Name is the display name; it also derives the MQTT topic slug.
	Name string `json:"name"`
	// Host is the printer's IP address or hostname on the local network.
	Host string `json:"host"`
	// Port is the printer's MQTT port, 8883 unless overridden.
	Port int `json:"port,omitempty"`
	// Serial is the printer's serial number, which forms its MQTT topics.
	Serial string `json:"serial"`
	// AccessCode is the 8-character LAN access code from the printer's display.
	AccessCode string `json:"access_code"`
	// Model is the product name shown in the UI ("H2D"); optional.
	Model string `json:"model,omitempty"`
}

// CloudEnabled reports whether the bridge should talk to the Bambu cloud at all.
// Without an account email there is nothing to authenticate, so the bridge runs
// LAN-only and the web UI skips the login flow.
func (b BambuConfig) CloudEnabled() bool { return b.Email != "" }

type WebConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
	// LivenessGraceSeconds is how long the bridge may stay unhealthy (not
	// authenticated, or no cloud printer connected) before the /livez probe
	// fails. Defaults to 240 (4 min) when unset.
	LivenessGraceSeconds int `json:"liveness_grace_seconds,omitempty"`
}

func LoadConfig(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		logger.Error("Error reading config file", "error", err)
		return Config{}, err
	}

	data = config.ReplaceEnvVariables(data)

	// Decode into a fresh value: unmarshalling into the package-level cfg would
	// leave fields absent from the new JSON holding their previous values.
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		logger.Error("Unmarshaling JSON", "error", err)
		return Config{}, err
	}
	cfg = loaded

	// Set default values
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.Bambu.Region == "" {
		cfg.Bambu.Region = RegionGlobal
	}

	if cfg.Bambu.PushAllInterval == 0 {
		cfg.Bambu.PushAllInterval = 300
	}

	if cfg.Bambu.SessionFile == "" {
		cfg.Bambu.SessionFile = "/var/lib/mqtt-bambu/session.json"
	}

	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}

	if err := normalizeLAN(cfg.Bambu.LAN); err != nil {
		logger.Error("Invalid LAN printer configuration", "error", err)
		return Config{}, err
	}

	if !cfg.Bambu.CloudEnabled() && len(cfg.Bambu.LAN) == 0 {
		return Config{}, fmt.Errorf("no printers configured: set bambu.email for cloud printers, bambu.lan for LAN printers, or both")
	}

	return cfg, nil
}

// normalizeLAN fills in LAN defaults in place and rejects incomplete entries —
// a printer missing its host, serial or access code can never connect, so it is
// better to fail loudly at startup than to retry forever.
func normalizeLAN(printers []LANPrinter) error {
	for i := range printers {
		p := &printers[i]
		if p.Host == "" {
			return fmt.Errorf("bambu.lan[%d]: host is required", i)
		}
		if p.Serial == "" {
			return fmt.Errorf("bambu.lan[%d] (%s): serial is required", i, p.Host)
		}
		if p.AccessCode == "" {
			return fmt.Errorf("bambu.lan[%d] (%s): access_code is required", i, p.Host)
		}
		if p.Port == 0 {
			p.Port = defaultLANPort
		}
		if p.Name == "" {
			p.Name = p.Serial
		}
	}
	return nil
}

func Get() Config {
	return cfg
}

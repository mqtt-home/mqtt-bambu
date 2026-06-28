package config

import (
	"encoding/json"
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

type Config struct {
	MQTT     config.MQTTConfig `json:"mqtt"`
	Bambu    BambuConfig       `json:"bambu"`
	Web      WebConfig         `json:"web"`
	LogLevel string            `json:"loglevel,omitempty"`
}

type BambuConfig struct {
	// Region is "global" (api.bambulab.com) or "china" (api.bambulab.cn).
	Region string `json:"region,omitempty"`
	// Email is the Bambu Lab account login (email or phone).
	Email string `json:"email"`
	// Password is the Bambu Lab account password. May be empty when the account
	// uses email-code login only; in that case authenticate via the web UI.
	Password string `json:"password,omitempty"`
	// PushAllInterval is how often (seconds) to request a full status snapshot
	// from each printer, since reports are otherwise partial deltas.
	PushAllInterval int `json:"pushall_interval,omitempty"`
	// SessionFile is where the cloud token/device cache is persisted so the
	// bridge survives restarts without re-authenticating.
	SessionFile string `json:"session_file,omitempty"`
}

type WebConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
	// LivenessGraceSeconds is how long the bridge may stay unhealthy (not
	// authenticated, or no printer connected) before the /livez probe fails.
	// Defaults to 240 (4 min) when unset.
	LivenessGraceSeconds int `json:"liveness_grace_seconds,omitempty"`
}

func LoadConfig(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		logger.Error("Error reading config file", "error", err)
		return Config{}, err
	}

	data = config.ReplaceEnvVariables(data)

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		logger.Error("Unmarshaling JSON", "error", err)
		return Config{}, err
	}

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

	return cfg, nil
}

func Get() Config {
	return cfg
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigLANDefaults(t *testing.T) {
	path := writeConfig(t, `{
	  "mqtt": {"url": "tcp://localhost:1883", "topic": "home/bambu"},
	  "bambu": {"lan": [{"host": "10.10.248.163", "serial": "0948DB552100477", "access_code": "81647bca"}]}
	}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Bambu.CloudEnabled() {
		t.Error("CloudEnabled: got true, want false without an email")
	}

	p := cfg.Bambu.LAN[0]
	if p.Port != defaultLANPort {
		t.Errorf("port: got %d want %d", p.Port, defaultLANPort)
	}
	// Without a name there is still a slug to publish under, so fall back to the
	// serial rather than letting every unnamed printer collide on "printer".
	if p.Name != "0948DB552100477" {
		t.Errorf("name: got %q want the serial", p.Name)
	}
}

func TestLoadConfigCloudAndLANTogether(t *testing.T) {
	path := writeConfig(t, `{
	  "mqtt": {"url": "tcp://localhost:1883", "topic": "home/bambu"},
	  "bambu": {
	    "email": "user@example.com",
	    "lan": [{"name": "H2D", "host": "10.0.0.5", "serial": "SER", "access_code": "code"}]
	  }
	}`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Bambu.CloudEnabled() {
		t.Error("CloudEnabled: got false, want true with an email set")
	}
	if len(cfg.Bambu.LAN) != 1 {
		t.Fatalf("LAN printers: got %d want 1", len(cfg.Bambu.LAN))
	}
}

func TestLoadConfigRejectsIncompleteLANPrinter(t *testing.T) {
	tests := map[string]string{
		"host":        `{"name": "H2D", "serial": "SER", "access_code": "code"}`,
		"serial":      `{"name": "H2D", "host": "10.0.0.5", "access_code": "code"}`,
		"access_code": `{"name": "H2D", "host": "10.0.0.5", "serial": "SER"}`,
	}
	for missing, printer := range tests {
		t.Run("missing_"+missing, func(t *testing.T) {
			path := writeConfig(t, `{"mqtt": {"topic": "t"}, "bambu": {"lan": [`+printer+`]}}`)
			_, err := LoadConfig(path)
			if err == nil {
				t.Fatalf("expected an error for a LAN printer without %s", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error %q does not name the missing field %q", err, missing)
			}
		})
	}
}

func TestLoadConfigRejectsNoPrintersAtAll(t *testing.T) {
	path := writeConfig(t, `{"mqtt": {"topic": "home/bambu"}, "bambu": {}}`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected an error when neither cloud nor LAN printers are configured")
	}
}

package bambu

import "testing"

func lanPrinter(name, serial string) LANPrinter {
	return LANPrinter{Name: name, Serial: serial, Host: "10.0.0.1", Port: 8883, AccessCode: "abc"}
}

func TestManagerMixesLANAndCloudPrinters(t *testing.T) {
	m := NewManager()

	lan := m.AddLANPrinters([]LANPrinter{lanPrinter("H2D", "SER-LAN")})
	if len(lan) != 1 {
		t.Fatalf("LAN printers added: got %d want 1", len(lan))
	}

	cloudDevices := []Device{{DevID: "SER-CLOUD", Name: "P1S", DevProductName: "P1S"}}
	cloudAdded := m.AddCloudDevices(cloudDevices, "us.mqtt.bambulab.com", "u_1", "token")
	if len(cloudAdded) != 1 {
		t.Fatalf("cloud printers added: got %d want 1", len(cloudAdded))
	}

	summaries := m.GetSummaries()
	if len(summaries) != 2 {
		t.Fatalf("summaries: got %d want 2", len(summaries))
	}

	modes := map[string]ConnectionMode{}
	for _, s := range summaries {
		modes[s.Slug] = s.Mode
	}
	if modes["h2d"] != ModeLAN {
		t.Errorf("h2d mode: got %q want %q", modes["h2d"], ModeLAN)
	}
	if modes["p1s"] != ModeCloud {
		t.Errorf("p1s mode: got %q want %q", modes["p1s"], ModeCloud)
	}
}

// A printer switched to LAN mode stays bound to the account, so cloud discovery
// still reports it. The LAN connection is the one that carries reports, so the
// cloud duplicate must be dropped rather than added alongside it.
func TestManagerCloudDeviceDoesNotDuplicateLANPrinter(t *testing.T) {
	m := NewManager()
	m.AddLANPrinters([]LANPrinter{lanPrinter("H2D", "SHARED-SERIAL")})

	added := m.AddCloudDevices(
		[]Device{{DevID: "SHARED-SERIAL", Name: "H2D", DevProductName: "H2D"}},
		"us.mqtt.bambulab.com", "u_1", "token",
	)
	if len(added) != 0 {
		t.Fatalf("cloud printers added: got %d want 0", len(added))
	}

	summaries := m.GetSummaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries: got %d want 1", len(summaries))
	}
	if summaries[0].Mode != ModeLAN {
		t.Errorf("kept mode: got %q want %q", summaries[0].Mode, ModeLAN)
	}
	if summaries[0].Slug != "h2d" {
		t.Errorf("slug: got %q want h2d", summaries[0].Slug)
	}
}

func TestManagerDisambiguatesSlugsAcrossModes(t *testing.T) {
	m := NewManager()
	m.AddLANPrinters([]LANPrinter{lanPrinter("Bambu", "SER-1")})
	m.AddCloudDevices([]Device{{DevID: "SER-2", Name: "Bambu"}}, "host", "u_1", "token")

	if mp := m.GetPrinter("bambu"); mp == nil || mp.Client.Serial() != "SER-1" {
		t.Fatalf("slug bambu: got %v", mp)
	}
	if mp := m.GetPrinter("bambu-2"); mp == nil || mp.Client.Serial() != "SER-2" {
		t.Fatalf("slug bambu-2: got %v", mp)
	}
}

func TestLANEndpointUsesAccessCodeAndSkipsCertVerification(t *testing.T) {
	c := NewLANPrinterClient(Device{DevID: "SER", Name: "H2D"}, "10.10.248.163", 8883, "81647bca")

	if c.Mode() != ModeLAN {
		t.Errorf("mode: got %q want %q", c.Mode(), ModeLAN)
	}
	if c.endpoint.address() != "ssl://10.10.248.163:8883" {
		t.Errorf("address: got %q", c.endpoint.address())
	}
	if c.endpoint.username != "bblp" {
		t.Errorf("username: got %q want bblp", c.endpoint.username)
	}
	if c.endpoint.password != "81647bca" {
		t.Errorf("password: got %q", c.endpoint.password)
	}
	// The printer serves a self-signed certificate for its own serial, so
	// verification can never succeed.
	if !c.endpoint.insecureTLS {
		t.Error("insecureTLS: got false, want true for LAN")
	}
	if c.reportTopic() != "device/SER/report" || c.requestTopic() != "device/SER/request" {
		t.Errorf("topics: %q / %q", c.reportTopic(), c.requestTopic())
	}
}

func TestCloudEndpointVerifiesCertificates(t *testing.T) {
	c := NewCloudPrinterClient(Device{DevID: "SER"}, "us.mqtt.bambulab.com", "u_1", "token")

	if c.Mode() != ModeCloud {
		t.Errorf("mode: got %q want %q", c.Mode(), ModeCloud)
	}
	if c.endpoint.address() != "ssl://us.mqtt.bambulab.com:8883" {
		t.Errorf("address: got %q", c.endpoint.address())
	}
	if c.endpoint.insecureTLS {
		t.Error("insecureTLS: got true, want false for cloud")
	}
}

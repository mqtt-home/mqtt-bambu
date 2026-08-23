package bambu

import (
	"sync"

	"github.com/philipparndt/go-logger"
)

// ManagedPrinter pairs a printer's client with its stable slug.
type ManagedPrinter struct {
	Slug   string
	Client *PrinterClient
}

// LANPrinter describes a printer in LAN mode, reachable directly on the local
// network. Unlike cloud printers these are not discovered — they come from the
// configuration, because the cloud API no longer reports them.
type LANPrinter struct {
	Name       string
	Model      string
	Serial     string
	Host       string
	Port       int
	AccessCode string
}

// DeviceSummary is the per-printer overview served to the web UI.
type DeviceSummary struct {
	Slug   string           `json:"slug"`
	Name   string           `json:"name"`
	Model  string           `json:"model"`
	Serial string           `json:"serial"`
	Mode   ConnectionMode   `json:"mode"`
	Online bool             `json:"online"`
	Status *PublishedStatus `json:"status"`
}

// Manager owns the set of printer clients, keyed by slug, and fans status and
// availability updates out to its listeners (MQTT bridge + web SSE).
//
// Printers are added incrementally: LAN printers are known from the
// configuration at startup, while cloud printers only appear once the account
// has authenticated. Both kinds live side by side in the same manager.
type Manager struct {
	mu       sync.RWMutex
	printers []*ManagedPrinter
	bySlug   map[string]*ManagedPrinter
	bySerial map[string]*ManagedPrinter
	slugUsed map[string]int

	onStatus       func(slug string, status PublishedStatus)
	onAvailability func(slug string, online bool)
}

func NewManager() *Manager {
	return &Manager{
		bySlug:   make(map[string]*ManagedPrinter),
		bySerial: make(map[string]*ManagedPrinter),
		slugUsed: make(map[string]int),
	}
}

// AddLANPrinters registers the configured LAN-mode printers and returns the
// ones that were newly added.
func (m *Manager) AddLANPrinters(printers []LANPrinter) []*ManagedPrinter {
	var added []*ManagedPrinter
	for _, p := range printers {
		device := Device{
			DevID:          p.Serial,
			Name:           p.Name,
			DevProductName: p.Model,
		}
		mp := m.add(device, func(d Device) *PrinterClient {
			return NewLANPrinterClient(d, p.Host, p.Port, p.AccessCode)
		})
		if mp != nil {
			added = append(added, mp)
		}
	}
	return added
}

// AddCloudDevices registers the printers discovered on the Bambu account and
// returns the ones that were newly added. A device whose serial is already
// configured as a LAN printer is skipped: in LAN mode the cloud broker no
// longer carries its reports, so the direct connection is the usable one.
func (m *Manager) AddCloudDevices(devices []Device, host, username, password string) []*ManagedPrinter {
	var added []*ManagedPrinter
	for _, d := range devices {
		mp := m.add(d, func(d Device) *PrinterClient {
			return NewCloudPrinterClient(d, host, username, password)
		})
		if mp != nil {
			added = append(added, mp)
		}
	}
	return added
}

// add registers one printer under a unique slug, wiring its listeners. It
// returns nil when a printer with the same serial is already managed.
func (m *Manager) add(device Device, newClient func(Device) *PrinterClient) *ManagedPrinter {
	m.mu.Lock()
	if existing, ok := m.bySerial[device.DevID]; ok {
		m.mu.Unlock()
		logger.Info("Skipping duplicate printer", "serial", device.DevID,
			"kept", existing.Client.Mode(), "name", existing.Client.Device().Name)
		return nil
	}

	slug := slugify(device.Name)
	if n := m.slugUsed[slug]; n > 0 {
		m.slugUsed[slug] = n + 1
		slug = slug + "-" + itoa(n+1)
	} else {
		m.slugUsed[slug] = 1
	}

	mp := &ManagedPrinter{Slug: slug, Client: newClient(device)}
	m.printers = append(m.printers, mp)
	m.bySlug[slug] = mp
	m.bySerial[device.DevID] = mp
	m.mu.Unlock()

	mp.Client.SetStatusListener(func(_ string, status PublishedStatus) {
		m.mu.RLock()
		cb := m.onStatus
		m.mu.RUnlock()
		if cb != nil {
			cb(mp.Slug, status)
		}
	})
	mp.Client.SetAvailabilityListener(func(_ string, online bool) {
		m.mu.RLock()
		cb := m.onAvailability
		m.mu.RUnlock()
		if cb != nil {
			cb(mp.Slug, online)
		}
	})

	logger.Info("Managing printer", "printer", device.Name, "serial", device.DevID,
		"slug", slug, "mode", mp.Client.Mode())
	return mp
}

func (m *Manager) SetStatusListener(cb func(slug string, status PublishedStatus)) {
	m.mu.Lock()
	m.onStatus = cb
	m.mu.Unlock()
}

func (m *Manager) SetAvailabilityListener(cb func(slug string, online bool)) {
	m.mu.Lock()
	m.onAvailability = cb
	m.mu.Unlock()
}

// snapshot returns a copy of the printer list so callers can iterate without
// holding the lock while the set may still grow.
func (m *Manager) snapshot() []*ManagedPrinter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ManagedPrinter, len(m.printers))
	copy(out, m.printers)
	return out
}

// Connect establishes the MQTT session for each of the given printers. A
// failure for one printer is logged but does not stop the others; the client
// keeps retrying in the background either way.
func Connect(printers []*ManagedPrinter) {
	for _, mp := range printers {
		if err := mp.Client.Connect(); err != nil {
			logger.Error("Failed to connect printer", "printer", mp.Client.Device().Name,
				"mode", mp.Client.Mode(), "error", err)
		}
	}
}

// PushAll requests a fresh snapshot from every connected printer.
func (m *Manager) PushAll() {
	for _, mp := range m.snapshot() {
		mp.Client.PushAll()
	}
}

// DisconnectAll tears down every printer session.
func (m *Manager) DisconnectAll() {
	for _, mp := range m.snapshot() {
		mp.Client.Disconnect()
	}
}

func (m *Manager) GetPrinter(slug string) *ManagedPrinter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bySlug[slug]
}

// ConnectedCount returns how many printers currently hold a live MQTT session.
func (m *Manager) ConnectedCount() int {
	count := 0
	for _, mp := range m.snapshot() {
		if mp.Client.IsOnline() {
			count++
		}
	}
	return count
}

// ConnectedCountMode is ConnectedCount restricted to one connection mode.
func (m *Manager) ConnectedCountMode(mode ConnectionMode) int {
	count := 0
	for _, mp := range m.snapshot() {
		if mp.Client.Mode() == mode && mp.Client.IsOnline() {
			count++
		}
	}
	return count
}

// ConnectionStates returns slug -> online for every printer.
func (m *Manager) ConnectionStates() map[string]bool {
	printers := m.snapshot()
	out := make(map[string]bool, len(printers))
	for _, mp := range printers {
		out[mp.Slug] = mp.Client.IsOnline()
	}
	return out
}

// GetSummaries returns the per-printer overview list.
func (m *Manager) GetSummaries() []DeviceSummary {
	printers := m.snapshot()
	out := make([]DeviceSummary, 0, len(printers))
	for _, mp := range printers {
		d := mp.Client.Device()
		out = append(out, DeviceSummary{
			Slug:   mp.Slug,
			Name:   d.Name,
			Model:  displayModel(d),
			Serial: d.DevID,
			Mode:   mp.Client.Mode(),
			Online: mp.Client.IsOnline(),
			Status: mp.Client.LastStatus(),
		})
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

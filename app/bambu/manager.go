package bambu

import (
	"sync"

	"github.com/philipparndt/go-logger"
)

// ManagedPrinter pairs a printer's cloud client with its stable slug.
type ManagedPrinter struct {
	Slug   string
	Client *PrinterClient
}

// DeviceSummary is the per-printer overview served to the web UI.
type DeviceSummary struct {
	Slug   string           `json:"slug"`
	Name   string           `json:"name"`
	Model  string           `json:"model"`
	Serial string           `json:"serial"`
	Online bool             `json:"online"`
	Status *PublishedStatus `json:"status"`
}

// Manager owns the set of printer clients, keyed by slug, and fans status and
// availability updates out to its listeners (MQTT bridge + web SSE).
type Manager struct {
	mu       sync.RWMutex
	printers []*ManagedPrinter
	bySlug   map[string]*ManagedPrinter
	bySerial map[string]*ManagedPrinter

	onStatus       func(slug string, status PublishedStatus)
	onAvailability func(slug string, online bool)
}

// NewManager builds a manager and one client per device, assigning each a unique
// slug derived from its name. host/username/password are the shared cloud MQTT
// credentials.
func NewManager(devices []Device, host, username, password string) *Manager {
	m := &Manager{
		bySlug:   make(map[string]*ManagedPrinter),
		bySerial: make(map[string]*ManagedPrinter),
	}

	used := make(map[string]int)
	for _, d := range devices {
		slug := slugify(d.Name)
		if n := used[slug]; n > 0 {
			used[slug] = n + 1
			slug = slug + "-" + itoa(n+1)
		} else {
			used[slug] = 1
		}

		client := NewPrinterClient(d, host, username, password)
		mp := &ManagedPrinter{Slug: slug, Client: client}

		serial := d.DevID
		client.SetStatusListener(func(_ string, status PublishedStatus) {
			m.mu.RLock()
			cb := m.onStatus
			m.mu.RUnlock()
			if cb != nil {
				cb(mp.Slug, status)
			}
		})
		client.SetAvailabilityListener(func(_ string, online bool) {
			m.mu.RLock()
			cb := m.onAvailability
			m.mu.RUnlock()
			if cb != nil {
				cb(mp.Slug, online)
			}
		})

		m.printers = append(m.printers, mp)
		m.bySlug[slug] = mp
		m.bySerial[serial] = mp
	}
	return m
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

// ConnectAll establishes every printer's cloud MQTT session. A failure for one
// printer is logged but does not stop the others.
func (m *Manager) ConnectAll() {
	for _, mp := range m.printers {
		if err := mp.Client.Connect(); err != nil {
			logger.Error("Failed to connect printer", "printer", mp.Client.Device().Name, "error", err)
		}
	}
}

// PushAll requests a fresh snapshot from every connected printer.
func (m *Manager) PushAll() {
	for _, mp := range m.printers {
		mp.Client.PushAll()
	}
}

// DisconnectAll tears down every printer session.
func (m *Manager) DisconnectAll() {
	for _, mp := range m.printers {
		mp.Client.Disconnect()
	}
}

func (m *Manager) GetPrinter(slug string) *ManagedPrinter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bySlug[slug]
}

func (m *Manager) ConnectedCount() int {
	count := 0
	for _, mp := range m.printers {
		if mp.Client.IsOnline() {
			count++
		}
	}
	return count
}

// ConnectionStates returns slug -> online for every printer.
func (m *Manager) ConnectionStates() map[string]bool {
	out := make(map[string]bool, len(m.printers))
	for _, mp := range m.printers {
		out[mp.Slug] = mp.Client.IsOnline()
	}
	return out
}

// GetSummaries returns the per-printer overview list.
func (m *Manager) GetSummaries() []DeviceSummary {
	out := make([]DeviceSummary, 0, len(m.printers))
	for _, mp := range m.printers {
		d := mp.Client.Device()
		out = append(out, DeviceSummary{
			Slug:   mp.Slug,
			Name:   d.Name,
			Model:  displayModel(d),
			Serial: d.DevID,
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

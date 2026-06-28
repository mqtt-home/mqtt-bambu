package bambu

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/philipparndt/go-logger"
)

// pushAllCommand is the request that asks a printer to emit a full status
// snapshot on its report topic. Reports are otherwise partial deltas.
const pushAllCommand = `{"pushing":{"sequence_id":"0","command":"pushall","version":1,"push_target":1}}`

// StatusListener is invoked with a freshly derived status whenever a report
// arrives for a printer.
type StatusListener func(serial string, status PublishedStatus)

// AvailabilityListener is invoked when a printer's cloud connection state
// changes.
type AvailabilityListener func(serial string, online bool)

// PrinterClient maintains the cloud MQTT connection for a single printer.
type PrinterClient struct {
	device   Device
	host     string
	username string
	password string

	client paho.Client
	cache  *stateCache

	onStatus       StatusListener
	onAvailability AvailabilityListener

	mu     sync.RWMutex
	online bool
	last   *PublishedStatus
}

func NewPrinterClient(device Device, host, username, password string) *PrinterClient {
	return &PrinterClient{
		device:   device,
		host:     host,
		username: username,
		password: password,
		cache:    newStateCache(),
	}
}

func (p *PrinterClient) SetStatusListener(l StatusListener)             { p.onStatus = l }
func (p *PrinterClient) SetAvailabilityListener(l AvailabilityListener) { p.onAvailability = l }

func (p *PrinterClient) Serial() string { return p.device.DevID }
func (p *PrinterClient) Device() Device { return p.device }

func (p *PrinterClient) IsOnline() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.online
}

func (p *PrinterClient) LastStatus() *PublishedStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.last
}

func (p *PrinterClient) reportTopic() string  { return "device/" + p.device.DevID + "/report" }
func (p *PrinterClient) requestTopic() string { return "device/" + p.device.DevID + "/request" }

// Connect establishes the cloud MQTT session and subscribes to the report topic.
func (p *PrinterClient) Connect() error {
	opts := paho.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("ssl://%s:8883", p.host))
	opts.SetClientID(fmt.Sprintf("mqtt-bambu-%s-%d", shortSerial(p.device.DevID), time.Now().UnixNano()))
	opts.SetUsername(p.username)
	opts.SetPassword(p.password)
	opts.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(10 * time.Second)
	opts.SetConnectTimeout(15 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetCleanSession(true)
	opts.SetOrderMatters(false)
	opts.SetOnConnectHandler(p.onConnect)
	opts.SetConnectionLostHandler(p.onConnectionLost)

	p.client = paho.NewClient(opts)
	token := p.client.Connect()
	if !token.WaitTimeout(20 * time.Second) {
		return fmt.Errorf("timed out connecting to Bambu cloud MQTT for %s", p.device.Name)
	}
	return token.Error()
}

func (p *PrinterClient) onConnect(c paho.Client) {
	logger.Info("Connected to Bambu cloud MQTT", "printer", p.device.Name, "serial", p.device.DevID)
	p.setOnline(true)

	token := c.Subscribe(p.reportTopic(), 0, p.handleMessage)
	if token.WaitTimeout(10*time.Second) && token.Error() != nil {
		logger.Error("Failed to subscribe to report topic", "printer", p.device.Name, "error", token.Error())
		return
	}
	// Ask for a full snapshot right away.
	p.PushAll()
}

func (p *PrinterClient) onConnectionLost(_ paho.Client, err error) {
	logger.Warn("Lost Bambu cloud MQTT connection", "printer", p.device.Name, "error", err)
	p.setOnline(false)
}

func (p *PrinterClient) handleMessage(_ paho.Client, msg paho.Message) {
	if !p.cache.merge(msg.Payload()) {
		return
	}
	report := p.cache.report()
	status := buildStatus(p.device.Name, displayModel(p.device), p.device.DevID, report, time.Now())

	p.mu.Lock()
	p.last = &status
	p.mu.Unlock()

	if p.onStatus != nil {
		p.onStatus(p.device.DevID, status)
	}
}

// PushAll requests a full status snapshot from the printer.
func (p *PrinterClient) PushAll() {
	if p.client == nil || !p.client.IsConnected() {
		return
	}
	token := p.client.Publish(p.requestTopic(), 0, false, pushAllCommand)
	if token.WaitTimeout(5*time.Second) && token.Error() != nil {
		logger.Debug("pushall publish failed", "printer", p.device.Name, "error", token.Error())
	}
}

// Disconnect tears down the cloud MQTT session.
func (p *PrinterClient) Disconnect() {
	if p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(250)
	}
	p.setOnline(false)
}

func (p *PrinterClient) setOnline(online bool) {
	p.mu.Lock()
	changed := p.online != online
	p.online = online
	p.mu.Unlock()
	if changed && p.onAvailability != nil {
		p.onAvailability(p.device.DevID, online)
	}
}

func shortSerial(s string) string {
	if len(s) > 6 {
		return s[len(s)-6:]
	}
	return s
}

// displayModel prefers the human product name, falling back to the model code.
func displayModel(d Device) string {
	if d.DevProductName != "" {
		return d.DevProductName
	}
	return d.DevModelName
}

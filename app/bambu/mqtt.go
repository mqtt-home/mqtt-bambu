package bambu

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/philipparndt/go-logger"
)

// pushAllCommand is the request that asks a printer to emit a full status
// snapshot on its report topic. Reports are otherwise partial deltas.
const pushAllCommand = `{"pushing":{"sequence_id":"0","command":"pushall","version":1,"push_target":1}}`

// lanUsername is the fixed MQTT username a printer in LAN mode expects; the
// password is the LAN access code from the printer's display.
const lanUsername = "bblp"

// ConnectionMode is how a printer is reached: through the Bambu cloud broker,
// or directly over the local network when the printer runs in LAN mode.
type ConnectionMode string

const (
	ModeCloud ConnectionMode = "cloud"
	ModeLAN   ConnectionMode = "lan"
)

// endpoint holds the MQTT broker coordinates and credentials for one printer.
// Cloud and LAN differ only here — the report/request topics and the payloads
// on them are identical.
type endpoint struct {
	mode     ConnectionMode
	host     string
	port     int
	username string
	password string
	// insecureTLS skips certificate verification. Printers in LAN mode serve a
	// self-signed certificate for their own IP, so verification cannot succeed.
	insecureTLS bool
}

func (e endpoint) address() string {
	return "ssl://" + net.JoinHostPort(e.host, strconv.Itoa(e.port))
}

// StatusListener is invoked with a freshly derived status whenever a report
// arrives for a printer.
type StatusListener func(serial string, status PublishedStatus)

// AvailabilityListener is invoked when a printer's connection state changes.
type AvailabilityListener func(serial string, online bool)

// PrinterClient maintains the MQTT connection for a single printer, over either
// the cloud broker or the printer's own LAN broker.
type PrinterClient struct {
	device   Device
	endpoint endpoint

	client paho.Client
	cache  *stateCache

	onStatus       StatusListener
	onAvailability AvailabilityListener

	mu     sync.RWMutex
	online bool
	last   *PublishedStatus
}

// NewCloudPrinterClient reaches the printer through the Bambu cloud broker,
// using the account's MQTT username and access token.
func NewCloudPrinterClient(device Device, host, username, password string) *PrinterClient {
	return newPrinterClient(device, endpoint{
		mode:     ModeCloud,
		host:     host,
		port:     8883,
		username: username,
		password: password,
	})
}

// NewLANPrinterClient reaches the printer directly on the local network, using
// the LAN access code as the password.
func NewLANPrinterClient(device Device, host string, port int, accessCode string) *PrinterClient {
	return newPrinterClient(device, endpoint{
		mode:        ModeLAN,
		host:        host,
		port:        port,
		username:    lanUsername,
		password:    accessCode,
		insecureTLS: true,
	})
}

func newPrinterClient(device Device, ep endpoint) *PrinterClient {
	return &PrinterClient{
		device:   device,
		endpoint: ep,
		cache:    newStateCache(),
	}
}

func (p *PrinterClient) SetStatusListener(l StatusListener)             { p.onStatus = l }
func (p *PrinterClient) SetAvailabilityListener(l AvailabilityListener) { p.onAvailability = l }

func (p *PrinterClient) Serial() string       { return p.device.DevID }
func (p *PrinterClient) Device() Device       { return p.device }
func (p *PrinterClient) Mode() ConnectionMode { return p.endpoint.mode }

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

// Connect establishes the MQTT session and subscribes to the report topic.
func (p *PrinterClient) Connect() error {
	opts := paho.NewClientOptions()
	opts.AddBroker(p.endpoint.address())
	opts.SetClientID(fmt.Sprintf("mqtt-bambu-%s-%d", shortSerial(p.device.DevID), time.Now().UnixNano()))
	opts.SetUsername(p.endpoint.username)
	opts.SetPassword(p.endpoint.password)
	opts.SetTLSConfig(&tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: p.endpoint.insecureTLS,
	})
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
		return fmt.Errorf("timed out connecting to %s MQTT (%s) for %s", p.endpoint.mode, p.endpoint.host, p.device.Name)
	}
	return token.Error()
}

func (p *PrinterClient) onConnect(c paho.Client) {
	logger.Info("Connected to Bambu MQTT", "printer", p.device.Name, "serial", p.device.DevID,
		"mode", p.endpoint.mode, "host", p.endpoint.host)
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
	logger.Warn("Lost Bambu MQTT connection", "printer", p.device.Name, "mode", p.endpoint.mode, "error", err)
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

// Disconnect tears down the MQTT session.
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

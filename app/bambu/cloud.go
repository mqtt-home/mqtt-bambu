package bambu

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/philipparndt/go-logger"
)

// Sentinel errors describing why a password login did not immediately yield a
// token. The web layer turns these into a "needs verification code" prompt.
var (
	// ErrVerificationRequired means the account requires an emailed login code.
	ErrVerificationRequired = errors.New("email verification code required")
	// ErrTFARequired means the account has TOTP two-factor enabled.
	ErrTFARequired = errors.New("two-factor authentication code required")
)

const (
	apiBaseGlobal  = "https://api.bambulab.com"
	apiBaseChina   = "https://api.bambulab.cn"
	mqttHostGlobal = "us.mqtt.bambulab.com"
	mqttHostChina  = "cn.mqtt.bambulab.com"
)

// session is the persisted authentication/device state.
type session struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	MQTTUsername string   `json:"mqtt_username"`
	Devices      []Device `json:"devices"`
}

// Device is a printer bound to the Bambu account.
type Device struct {
	DevID          string `json:"dev_id"` // serial number, used in MQTT topics
	Name           string `json:"name"`
	Online         bool   `json:"online"`
	DevModelName   string `json:"dev_model_name"`
	DevProductName string `json:"dev_product_name"`
}

// Client talks to the Bambu Lab cloud HTTP API: login (password / email code /
// TFA), token persistence, and device discovery.
type Client struct {
	region      string
	email       string
	password    string
	sessionFile string
	http        *http.Client

	mu     sync.RWMutex
	sess   session
	tfaKey string
	authed bool
}

func NewClient(region, email, password, sessionFile string) *Client {
	return &Client{
		region:      region,
		email:       email,
		password:    password,
		sessionFile: sessionFile,
		http:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) apiBase() string {
	if c.region == "china" {
		return apiBaseChina
	}
	return apiBaseGlobal
}

// MQTTHost returns the cloud MQTT broker hostname for the configured region.
func (c *Client) MQTTHost() string {
	if c.region == "china" {
		return mqttHostChina
	}
	return mqttHostGlobal
}

func (c *Client) Email() string { return c.email }

func (c *Client) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authed
}

func (c *Client) AccessToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sess.AccessToken
}

func (c *Client) MQTTUsername() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sess.MQTTUsername
}

func (c *Client) Devices() []Device {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Device, len(c.sess.Devices))
	copy(out, c.sess.Devices)
	return out
}

// --- login flow ---

type loginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password,omitempty"`
	Code     string `json:"code,omitempty"`
	APIError string `json:"apiError"`
}

type loginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	LoginType    string `json:"loginType"`
	TFAKey       string `json:"tfaKey"`
}

// Login performs a password login. On success the token is stored and the bridge
// can start. When the account needs an emailed code or TOTP, it returns
// ErrVerificationRequired / ErrTFARequired respectively (and, for email, also
// triggers the code to be sent).
func (c *Client) Login() error {
	if c.password == "" {
		// No password configured: go straight to the email-code flow.
		if err := c.RequestCode(); err != nil {
			return err
		}
		return ErrVerificationRequired
	}

	resp, err := c.doLogin(loginRequest{Account: c.email, Password: c.password})
	if err != nil {
		return err
	}

	switch resp.LoginType {
	case "verifyCode":
		if err := c.RequestCode(); err != nil {
			return err
		}
		return ErrVerificationRequired
	case "tfa":
		c.mu.Lock()
		c.tfaKey = resp.TFAKey
		c.mu.Unlock()
		return ErrTFARequired
	}

	if resp.AccessToken == "" {
		return fmt.Errorf("login returned no access token (loginType=%q)", resp.LoginType)
	}
	return c.storeToken(resp.AccessToken, resp.RefreshToken)
}

// RequestCode asks the cloud to email a login verification code.
func (c *Client) RequestCode() error {
	body := map[string]string{"email": c.email, "type": "codeLogin"}
	var out map[string]any
	if err := c.postJSON("/v1/user-service/user/sendemail/code", body, false, &out); err != nil {
		return fmt.Errorf("requesting verification code: %w", err)
	}
	logger.Info("Requested Bambu email verification code", "email", c.email)
	return nil
}

// CodeLogin completes login with an emailed verification code.
func (c *Client) CodeLogin(code string) error {
	resp, err := c.doLogin(loginRequest{Account: c.email, Code: code})
	if err != nil {
		return err
	}
	if resp.AccessToken == "" {
		return fmt.Errorf("verification code rejected")
	}
	return c.storeToken(resp.AccessToken, resp.RefreshToken)
}

func (c *Client) doLogin(req loginRequest) (loginResponse, error) {
	var resp loginResponse
	if err := c.postJSON("/v1/user-service/user/login", req, false, &resp); err != nil {
		return loginResponse{}, fmt.Errorf("login request: %w", err)
	}
	return resp, nil
}

func (c *Client) storeToken(accessToken, refreshToken string) error {
	claims, err := parseJWTClaims(accessToken)
	if err != nil {
		return fmt.Errorf("parsing access token: %w", err)
	}
	if claims.Username == "" {
		return fmt.Errorf("access token missing username claim")
	}

	c.mu.Lock()
	c.sess.AccessToken = accessToken
	c.sess.RefreshToken = refreshToken
	c.sess.MQTTUsername = claims.Username
	c.authed = true
	c.tfaKey = ""
	c.mu.Unlock()

	logger.Info("Bambu cloud authenticated", "user", claims.Username)
	return nil
}

// --- device discovery ---

type bindResponse struct {
	Message string   `json:"message"`
	Devices []Device `json:"devices"`
}

// DiscoverDevices fetches the list of printers bound to the account.
func (c *Client) DiscoverDevices() error {
	var resp bindResponse
	if err := c.getJSON("/v1/iot-service/api/user/bind", &resp); err != nil {
		return fmt.Errorf("discovering devices: %w", err)
	}
	c.mu.Lock()
	c.sess.Devices = resp.Devices
	c.mu.Unlock()
	logger.Info("Discovered Bambu devices", "count", len(resp.Devices))
	return nil
}

// --- session persistence ---

// LoadSession restores a previously persisted token + device list from disk and
// validates that the token has not expired. Returns true when a usable session
// was loaded.
func (c *Client) LoadSession() bool {
	data, err := os.ReadFile(c.sessionFile)
	if err != nil {
		return false
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil || s.AccessToken == "" {
		return false
	}
	claims, err := parseJWTClaims(s.AccessToken)
	if err != nil {
		return false
	}
	if claims.Exp > 0 && time.Now().After(time.Unix(claims.Exp, 0)) {
		logger.Info("Persisted Bambu session expired, re-authenticating")
		return false
	}

	c.mu.Lock()
	c.sess = s
	c.authed = true
	c.mu.Unlock()
	logger.Info("Loaded persisted Bambu session", "user", s.MQTTUsername, "devices", len(s.Devices))
	return true
}

// SaveSession persists the current token + device list to disk.
func (c *Client) SaveSession() error {
	c.mu.RLock()
	data, err := json.MarshalIndent(c.sess, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(c.sessionFile); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(c.sessionFile, data, 0o600)
}

// ClearSession wipes the in-memory and on-disk session.
func (c *Client) ClearSession() {
	c.mu.Lock()
	c.sess = session{}
	c.authed = false
	c.tfaKey = ""
	c.mu.Unlock()
	_ = os.Remove(c.sessionFile)
	logger.Info("Cleared Bambu session")
}

// --- HTTP helpers ---

func (c *Client) postJSON(path string, body any, auth bool, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.apiBase()+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, auth, out)
}

func (c *Client) getJSON(path string, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.apiBase()+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, true, out)
}

func (c *Client) do(req *http.Request, auth bool, out any) error {
	req.Header.Set("Accept", "application/json")
	// Present as the Bambu network agent; a plain client UA is sometimes rejected.
	req.Header.Set("User-Agent", "bambu_network_agent/01.09.05.01")
	req.Header.Set("X-BBL-Client-Name", "OrcaSlicer")
	req.Header.Set("X-BBL-Client-Version", "01.09.05.51")
	if auth {
		if token := c.AccessToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

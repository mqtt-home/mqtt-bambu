package bambu

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// makeJWT builds an unsigned JWT-shaped token with the given payload JSON.
func makeJWT(payloadJSON string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	return header + "." + payload + ".sig"
}

func TestResolveMQTTUsernameFromJWT(t *testing.T) {
	c := NewClient("global", "x@example.com", "pw", "")
	token := makeJWT(`{"username":"u_1234567","exp":9999999999}`)

	got, err := c.resolveMQTTUsername(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "u_1234567" {
		t.Fatalf("username: got %q want u_1234567", got)
	}
}

func TestParseJWTClaimsRejectsOpaqueToken(t *testing.T) {
	// An opaque (non-JWT) token must fail JWT parsing so the caller falls back to
	// the preference API rather than treating it as fatal.
	if _, err := parseJWTClaims("opaque-token-no-dots"); err == nil {
		t.Fatal("expected error parsing a non-JWT token")
	}
}

func TestDiscoverDevicesReportsRejectedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, `{"message":"token expired"}`, http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"message":"success","devices":[{"dev_id":"S1","name":"A1 mini"}]}`))
	}))
	defer srv.Close()

	c := NewClient("global", "x@example.com", "pw", "")
	c.apiBaseURL = srv.URL

	c.sess.AccessToken = "stale"
	if err := c.DiscoverDevices(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stale token: got %v want ErrUnauthorized", err)
	}

	c.sess.AccessToken = "good"
	if err := c.DiscoverDevices(); err != nil {
		t.Fatalf("good token: unexpected error: %v", err)
	}
	if got := c.Devices(); len(got) != 1 || got[0].DevID != "S1" {
		t.Fatalf("devices: got %+v", got)
	}
}

func TestServerErrorIsNotARejectedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream down", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewClient("global", "x@example.com", "pw", "")
	c.apiBaseURL = srv.URL
	c.sess.AccessToken = "good"

	err := c.DiscoverDevices()
	if err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want a plain error so the session is kept", err)
	}
}

func TestLoginAwaitsCodeUntilTokenStored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user-service/user/login":
			var req loginRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.Code != "" {
				w.Write([]byte(`{"accessToken":"` + makeJWT(`{"username":"u_1","exp":9999999999}`) + `"}`))
				return
			}
			w.Write([]byte(`{"loginType":"verifyCode"}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	c := NewClient("global", "x@example.com", "pw", "")
	c.apiBaseURL = srv.URL

	if err := c.Login(); !errors.Is(err, ErrVerificationRequired) {
		t.Fatalf("login: got %v want ErrVerificationRequired", err)
	}
	if !c.AwaitingCode() {
		t.Fatal("expected AwaitingCode after a verification code was requested")
	}
	if err := c.CodeLogin("123456"); err != nil {
		t.Fatalf("code login: %v", err)
	}
	if c.AwaitingCode() || !c.IsAuthenticated() {
		t.Fatal("expected authenticated and no longer awaiting a code")
	}
}

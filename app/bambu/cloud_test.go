package bambu

import (
	"encoding/base64"
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

package bambu

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// jwtClaims holds the fields we care about from the Bambu access token.
type jwtClaims struct {
	// Username already has the "u_<digits>" form used as the MQTT username.
	Username string `json:"username"`
	Exp      int64  `json:"exp"`
}

// parseJWTClaims decodes the (unverified) payload segment of a JWT. We only use
// it to read the username claim for the MQTT login and the expiry; the token's
// authenticity is enforced by the cloud, not by us.
func parseJWTClaims(token string) (jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return jwtClaims{}, fmt.Errorf("malformed JWT: expected 3 segments, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Fall back to standard encoding with padding, just in case.
		if payload, err = base64.StdEncoding.DecodeString(padBase64(parts[1])); err != nil {
			return jwtClaims{}, fmt.Errorf("decoding JWT payload: %w", err)
		}
	}

	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return jwtClaims{}, fmt.Errorf("unmarshaling JWT claims: %w", err)
	}
	return claims, nil
}

func padBase64(s string) string {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	return s
}

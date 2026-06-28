package bambu

import (
	"regexp"
	"strings"
)

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a printer name into a lowercase, MQTT-topic-safe slug.
// "Bambu X1C (Office)" -> "bambu-x1c-office".
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "printer"
	}
	return s
}

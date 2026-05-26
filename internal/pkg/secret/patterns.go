package secret

import (
	"regexp"

	"github.com/company/echo/internal/domain"
)

// patterns is a list of compiled regex patterns for secret detection.
var patterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"api-key", regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)},
	{"aws-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"github-token", regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`)},
	{"private-key", regexp.MustCompile(`-----BEGIN (RSA|EC|DSA|OPENSSH) PRIVATE KEY-----`)},
	{"inline-password", regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S{8,}`)},
	{"jwt-token", regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}`)},
	{"connection-string", regexp.MustCompile(`://[^:]+:[^<]{8,}@`)},
}

// Scan checks text for potential secrets. Returns nil if clean.
func Scan(text string) error {
	for _, p := range patterns {
		if p.pattern.MatchString(text) {
			return &domain.SecretError{
				Field:   "unknown",
				Pattern: p.name,
			}
		}
	}
	return nil
}

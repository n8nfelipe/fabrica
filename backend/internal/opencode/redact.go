package opencode

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	secretAssignment = regexp.MustCompile(`(?i)(["']?(?:password|passwd|token|secret|api[_-]?key|private[_-]?key|credential)["']?\s*[:=]\s*["']?)([^"'\s,;}]+)(["']?)`)
	bearerCredential = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/-]+=*`)
	commonToken      = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_]{16,}|sk-[A-Za-z0-9_-]{16,})\b`)
)

func Redact(text string) string {
	secrets := make([]string, 0)
	for _, pair := range os.Environ() {
		name, value, ok := strings.Cut(pair, "=")
		if !ok || len(value) < 4 {
			continue
		}
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "PASSWD") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "PRIVATE_KEY") || strings.Contains(upper, "CREDENTIAL") || upper == "DATABASE_URL" || strings.HasSuffix(upper, "_DSN") {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	text = secretAssignment.ReplaceAllString(text, "$1[REDACTED]$3")
	text = bearerCredential.ReplaceAllString(text, "${1}[REDACTED]")
	return commonToken.ReplaceAllString(text, "[REDACTED]")
}

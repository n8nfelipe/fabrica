package opencode

import (
	"strings"
	"testing"
)

func TestRedactKnownSecretsAndCredentialPatterns(t *testing.T) {
	const sentinel = "fabrica-secret-sentinel-2026"
	t.Setenv("FACTORY_TEST_TOKEN", sentinel)
	value := Redact("value=" + sentinel + " password=hunter2 Authorization: Bearer abc.def.token ghp_12345678901234567890 {\"api_key\":\"json-secret-value\"}")
	for _, secret := range []string{sentinel, "hunter2", "abc.def.token", "ghp_12345678901234567890", "json-secret-value"} {
		if strings.Contains(value, secret) {
			t.Fatalf("redaction left a credential value in the result")
		}
	}
	if value == "" {
		t.Fatal("redaction returned an empty result")
	}
}

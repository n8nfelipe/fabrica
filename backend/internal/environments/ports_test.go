package environments

import (
	"errors"
	"testing"
)

func TestIsPortBindConflict(t *testing.T) {
	for _, message := range []string{
		"Bind for 127.0.0.1:41000 failed: port is already allocated",
		"failed to bind host port 127.0.0.1:41000: address already in use",
	} {
		if !IsPortBindConflict(errors.New(message)) {
			t.Errorf("expected bind collision to match: %s", message)
		}
	}
	if IsPortBindConflict(errors.New("invalid Compose configuration")) {
		t.Fatal("non-bind error was classified as a port collision")
	}
}

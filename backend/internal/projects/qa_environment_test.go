package projects

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"fabrica/backend/internal/environments"
)

func TestWaitForQAHealthChecksDeclaredPath(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "healthy service exposes local URL", statusCode: http.StatusNoContent},
		{name: "unhealthy service remains blocked", statusCode: http.StatusServiceUnavailable, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(test.statusCode)
			}))
			defer server.Close()
			_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			workspace := t.TempDir()
			if err := os.Mkdir(filepath.Join(workspace, ".fabrica"), 0o700); err != nil {
				t.Fatal(err)
			}
			descriptor, err := json.Marshal(map[string]any{"version": 1, "composeFile": "compose.yaml", "qaService": "web", "containerPort": 80, "healthCheck": map[string]string{"path": "/healthz"}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, environments.DescriptorPath), descriptor, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
			defer cancel()
			url, err := waitForQAHealth(ctx, port, workspace)
			if test.wantErr {
				if err == nil || url != "" {
					t.Fatalf("unhealthy service should not expose a URL: url=%q err=%v", url, err)
				}
				return
			}
			if err != nil || url != server.URL+"/healthz" {
				t.Fatalf("healthy service URL = %q, err %v", url, err)
			}
		})
	}
}

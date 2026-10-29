package securitygateway

import (
	"strings"
	"testing"
)

func TestRunCLIValidatesPolicyAndServesProtocol(t *testing.T) {
	root := t.TempDir()
	var output strings.Builder
	args := []string{"--root", root, "--phase", PhaseSpecification, "--task-id", "12"}
	if err := RunCLI(args, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"result":{}`) {
		t.Fatalf("unexpected MCP response: %s", output.String())
	}
	if err := RunCLI([]string{"--root", root, "--phase", PhaseImplementation, "--task-id", "12"}, strings.NewReader(""), &strings.Builder{}); err == nil {
		t.Fatal("implementation gateway must require a write scope")
	}
	if err := RunCLI(append(args, "--unknown"), strings.NewReader(""), &strings.Builder{}); err == nil {
		t.Fatal("unexpected arguments must be rejected")
	}
}

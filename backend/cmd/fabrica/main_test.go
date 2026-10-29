package main

import (
	"strings"
	"testing"
)

func TestRunGatewayModeHandlesOnlyGatewaySubcommand(t *testing.T) {
	if handled, err := runGatewayMode([]string{"serve"}, strings.NewReader(""), &strings.Builder{}, &strings.Builder{}); handled || err != nil {
		t.Fatalf("normal command should not be handled: handled=%t err=%v", handled, err)
	}
	root := t.TempDir()
	var output, errorOutput strings.Builder
	handled, err := runGatewayMode([]string{"security-gateway", "--root", root, "--phase", "specification", "--task-id", "8"}, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &output, &errorOutput)
	if !handled || err != nil || !strings.Contains(output.String(), `"result":{}`) || errorOutput.Len() != 0 {
		t.Fatalf("gateway command failed: handled=%t err=%v stdout=%s stderr=%s", handled, err, output.String(), errorOutput.String())
	}
	output.Reset()
	handled, err = runGatewayMode([]string{"security-gateway", "--unknown"}, strings.NewReader(""), &output, &errorOutput)
	if !handled || err == nil || errorOutput.Len() == 0 {
		t.Fatalf("invalid gateway command was not reported: handled=%t err=%v stderr=%s", handled, err, errorOutput.String())
	}
}

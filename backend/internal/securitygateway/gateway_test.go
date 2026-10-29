package securitygateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func newTestGateway(t *testing.T, phase string, paths []string) *Gateway {
	t.Helper()
	root := t.TempDir()
	policy, err := NewPolicy(root, phase, 27, paths, "")
	if err != nil {
		t.Fatal(err)
	}
	return New(policy)
}

func toolArgs(values map[string]string) json.RawMessage {
	encoded, _ := json.Marshal(values)
	return encoded
}

func TestMCPServeInitializesListsToolsAndRejectsUnknownMethods(t *testing.T) {
	gateway := newTestGateway(t, PhaseSpecification, nil)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/arbitrary","params":{}}`,
	}, "\n")
	var output strings.Builder
	if err := gateway.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected three responses, got %d: %s", len(lines), output.String())
	}
	var initialized, tools, unknown map[string]json.RawMessage
	for i, target := range []*map[string]json.RawMessage{&initialized, &tools, &unknown} {
		if err := json.Unmarshal([]byte(lines[i]), target); err != nil {
			t.Fatal(err)
		}
	}
	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(initialized["result"], &initResult); err != nil || initResult.ProtocolVersion != "2024-11-05" {
		t.Fatalf("unexpected initialization response: %s", lines[0])
	}
	var toolResult struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(tools["result"], &toolResult); err != nil || len(toolResult.Tools) != 5 {
		t.Fatalf("unexpected tool list: %s", lines[1])
	}
	if !strings.Contains(string(unknown["error"]), "method not found") {
		t.Fatalf("expected method-not-found response: %s", lines[2])
	}
}

func TestGatewayAllowsSpecificationWritesOnlyInTaskArtifacts(t *testing.T) {
	gateway := newTestGateway(t, PhaseSpecification, nil)
	if _, err := gateway.call("write_file", toolArgs(map[string]string{"path": "openspec/changes/task-27/proposal.md", "content": "approved scope"})); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.call("write_file", toolArgs(map[string]string{"path": "src/main.go", "content": "package main"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected source write denial, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(gateway.policy.Root, "openspec/changes/task-27/proposal.md"))
	if err != nil || string(data) != "approved scope" {
		t.Fatalf("expected specification artifact to be written, data=%q err=%v", data, err)
	}
}

func TestGatewayEnforcesExactAndDirectoryImplementationScope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "frontend/src"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(root, PhaseImplementation, 27, []string{"frontend/src/App.tsx", "backend/internal/feature/"}, "")
	if err != nil {
		t.Fatal(err)
	}
	gateway := New(policy)
	for _, path := range []string{"frontend/src/App.tsx", "backend/internal/feature/new_test.go"} {
		if _, err := gateway.call("write_file", toolArgs(map[string]string{"path": path, "content": "content"})); err != nil {
			t.Fatalf("allowed write %q failed: %v", path, err)
		}
	}
	for _, path := range []string{"frontend/src/Other.tsx", "backend/internal/feature2/file.go", "../outside.txt", ".git/config", "openspec/changes/task-27/proposal.md"} {
		if _, err := gateway.call("write_file", toolArgs(map[string]string{"path": path, "content": "content"})); !errors.Is(err, ErrDenied) {
			t.Errorf("expected %q to be denied, got %v", path, err)
		}
	}
	if err := ValidateChangedFiles(policy, []string{"frontend/src/App.tsx", "backend/internal/feature/new_test.go"}); err != nil {
		t.Fatalf("expected changes within approved scope, got %v", err)
	}
	if err := ValidateChangedFiles(policy, []string{"frontend/src/Other.tsx"}); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected post-run scope validation to reject changes, got %v", err)
	}
}

func TestGatewayRejectsSymlinksAndSecretFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(root, PhaseImplementation, 27, []string{"**/"}, "")
	if err == nil {
		t.Fatal("unsafe wildcard-like scope should be rejected")
	}
	policy, err = NewPolicy(root, PhaseImplementation, 27, []string{"linked/", ""}, "")
	if err == nil {
		t.Fatal("empty or unsafe scope should be rejected")
	}
	policy, err = NewPolicy(root, PhaseImplementation, 27, []string{"linked/"}, "")
	if err != nil {
		t.Fatal(err)
	}
	gateway := New(policy)
	if _, err := gateway.call("read_file", toolArgs(map[string]string{"path": "linked/outside.txt"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected symlink read to be denied, got %v", err)
	}
	if _, err := gateway.call("read_file", toolArgs(map[string]string{"path": ".env"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected secret file read to be denied, got %v", err)
	}
}

func TestGatewayReadListAndSearchStayWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src/main.go"), []byte("package main\nneedle here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("SAFE=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(root, PhaseSpecification, 27, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	gateway := New(policy)
	content, err := gateway.call("read_file", toolArgs(map[string]string{"path": "src/main.go"}))
	if err != nil || !strings.Contains(content, "needle") {
		t.Fatalf("expected file contents, got %q err=%v", content, err)
	}
	listing, err := gateway.call("list_files", toolArgs(map[string]string{"path": ""}))
	if err != nil || strings.Contains(listing, `".env"`) {
		t.Fatalf("unexpected listing: %s err=%v", listing, err)
	}
	matches, err := gateway.call("search_files", toolArgs(map[string]string{"query": "needle", "path": "src"}))
	if err != nil || !strings.Contains(matches, "main.go") {
		t.Fatalf("expected search match, got %s err=%v", matches, err)
	}
}

func TestGatewayRequiresStrictScopeManifest(t *testing.T) {
	paths, err := ParseScopeManifest([]byte(`{"version":1,"writablePaths":["src/","README.md"]}`), 27)
	if err != nil || len(paths) != 2 {
		t.Fatalf("expected valid scope, paths=%v err=%v", paths, err)
	}
	for _, manifest := range []string{
		`{"version":1,"writablePaths":["../outside"]}`,
		`{"version":1,"writablePaths":["/etc/passwd"]}`,
		`{"version":1,"writablePaths":[".git/"]}`,
		`{"version":1,"writablePaths":[]}`,
		`{"version":1,"writablePaths":["src/"],"extra":true}`,
		`{"version":1,"writablePaths":["src/"]} {"version":1}`,
	} {
		if _, err := ParseScopeManifest([]byte(manifest), 27); err == nil {
			t.Errorf("expected manifest to be rejected: %s", manifest)
		}
	}
}

func TestGatewayStopsRepeatedToolCallLoopAndAuditsViolation(t *testing.T) {
	root := t.TempDir()
	audit := filepath.Join(t.TempDir(), "gateway-audit.jsonl")
	policy, err := NewPolicy(root, PhaseImplementation, 27, []string{"src/"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	gateway := New(policy)
	args := toolArgs(map[string]string{"path": "src/missing.go"})
	for i := 0; i < maxRepeatCalls; i++ {
		if _, err := gateway.call("read_file", args); err == nil {
			t.Fatal("expected missing file error")
		}
	}
	if _, err := gateway.call("read_file", args); !errors.Is(err, ErrLoopLimit) {
		t.Fatalf("expected repetitive call limit, got %v", err)
	}
	data, err := os.ReadFile(audit)
	if err != nil || !strings.Contains(string(data), "repeated_call_loop") {
		t.Fatalf("expected loop audit marker, data=%q err=%v", data, err)
	}
}

func TestGatewayAuditDoesNotPersistUntrustedToolNames(t *testing.T) {
	root := t.TempDir()
	audit := filepath.Join(t.TempDir(), "gateway-audit.jsonl")
	policy, err := NewPolicy(root, PhaseSpecification, 27, nil, audit)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = New(policy).call("unknown-private-token-value", json.RawMessage(`{}`))
	data, err := os.ReadFile(audit)
	if err != nil || strings.Contains(string(data), "private-token-value") || !strings.Contains(string(data), `"tool":"unknown"`) {
		t.Fatalf("untrusted tool name leaked to audit: data=%q err=%v", data, err)
	}
}

func TestGatewayStopsTotalToolCallBudget(t *testing.T) {
	gateway := newTestGateway(t, PhaseSpecification, nil)
	for i := 0; i < maxToolCalls; i++ {
		args := toolArgs(map[string]string{"path": filepath.ToSlash(filepath.Join("src", string(rune('a'+i%26)), "file"+string(rune('A'+i/26))))})
		_, _ = gateway.call("read_file", args)
	}
	if _, err := gateway.call("read_file", toolArgs(map[string]string{"path": "another-file"})); !errors.Is(err, ErrLoopLimit) {
		t.Fatalf("expected total tool-call ceiling, got %v", err)
	}
}

func TestGatewayDeletesOnlyApprovedRegularFiles(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "src", "remove.go")
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(root, PhaseImplementation, 27, []string{"src/"}, "")
	if err != nil {
		t.Fatal(err)
	}
	gateway := New(policy)
	if _, err := gateway.call("delete_file", toolArgs(map[string]string{"path": "src/remove.go"})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filename); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected authorized file deletion, got %v", err)
	}
	if _, err := gateway.call("delete_file", toolArgs(map[string]string{"path": "README.md"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected out-of-scope deletion to fail, got %v", err)
	}
}

func TestMCPRejectsMalformedMessages(t *testing.T) {
	gateway := newTestGateway(t, PhaseSpecification, nil)
	response := gateway.handle([]byte("not-json"))
	if response == nil || response.Error == nil {
		t.Fatalf("expected JSON-RPC parse error, got %#v", response)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gateway.Serve(ctx, strings.NewReader(""), &strings.Builder{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled serving context, got %v", err)
	}
}

func TestMCPRejectsInvalidRequestsAndToolArguments(t *testing.T) {
	gateway := newTestGateway(t, PhaseSpecification, nil)
	for _, line := range []string{
		`{"jsonrpc":"1.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`,
	} {
		response := gateway.handle([]byte(line))
		if response == nil || response.Error == nil {
			t.Errorf("expected invalid request error for %s", line)
		}
	}
	callError := gateway.handle([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"read_file","arguments":42}}`))
	if callError == nil || callError.Result == nil || !strings.Contains(string(mustJSON(callError.Result)), `"isError":true`) {
		t.Fatalf("expected malformed tool arguments to produce a tool error, got %#v", callError)
	}
	if _, err := gateway.call("read_file", json.RawMessage(`[]`)); err == nil {
		t.Fatal("expected non-object arguments to fail")
	}
	if _, err := gateway.call("read_file", json.RawMessage(`{"path":3}`)); err == nil {
		t.Fatal("expected non-string path to fail")
	}
	if _, err := gateway.call("read_file", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected required path to fail")
	}
	if _, err := gateway.call("list_files", json.RawMessage(`{"unexpected":true}`)); err == nil {
		t.Fatal("expected unknown argument to fail")
	}
	if _, err := gateway.call("no_such_tool", json.RawMessage(`{}`)); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected unknown tool denial, got %v", err)
	}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func TestServeReturnsScannerAndWriterErrors(t *testing.T) {
	gateway := newTestGateway(t, PhaseSpecification, nil)
	if err := gateway.Serve(context.Background(), strings.NewReader(strings.Repeat("x", maxRequestBytes+1)), io.Discard); err == nil {
		t.Fatal("expected oversized request line to fail")
	}
	if err := gateway.Serve(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), failingWriter{}); err == nil {
		t.Fatal("expected output write failure")
	}
}

func TestGatewayRejectsInvalidPathsAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "large.txt"), []byte(strings.Repeat("x", maxFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(root, PhaseImplementation, 27, []string{"src/"}, "")
	if err != nil {
		t.Fatal(err)
	}
	gateway := New(policy)
	if _, err := gateway.call("list_files", toolArgs(map[string]string{"path": "../"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected traversal listing to be denied, got %v", err)
	}
	if _, err := gateway.call("list_files", toolArgs(map[string]string{"path": "missing"})); err == nil {
		t.Fatal("expected missing directory to fail")
	}
	if _, err := gateway.call("read_file", toolArgs(map[string]string{"path": "src/large.txt"})); err == nil {
		t.Fatal("expected oversized file read to fail")
	}
	if _, err := gateway.call("search_files", toolArgs(map[string]string{"query": "", "path": "src"})); err == nil {
		t.Fatal("expected empty search to fail")
	}
	if _, err := gateway.call("search_files", toolArgs(map[string]string{"query": strings.Repeat("x", 1025), "path": "src"})); err == nil {
		t.Fatal("expected oversized search query to fail")
	}
	if _, err := gateway.call("write_file", toolArgs(map[string]string{"path": "src/large.txt", "content": strings.Repeat("x", maxFileBytes+1)})); err == nil {
		t.Fatal("expected oversized file write to fail")
	}
	if _, err := gateway.call("delete_file", toolArgs(map[string]string{"path": "src"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected directory deletion to be denied, got %v", err)
	}
}

func TestGatewayRejectsSymlinkWriteTarget(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(outside, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "src", "linked")); err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(root, PhaseImplementation, 27, []string{"src/"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := New(policy).writeFile("src/linked", "malicious"); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected symlink replacement to be rejected, got %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "safe" {
		t.Fatalf("outside target changed: %q err=%v", data, err)
	}
}

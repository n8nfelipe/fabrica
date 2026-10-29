package opencode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fabrica/backend/internal/securitygateway"
)

func fixture(t *testing.T, script string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "fake-opencode")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return root, project, binary
}

func testRequest(root, project, prompt string) Request {
	return Request{WorkspaceRoot: root, ProjectDir: project, TaskID: 1, Phase: securitygateway.PhaseSpecification, Prompt: prompt}
}

func TestRunCapturesPromptEventsAndStderr(t *testing.T) {
	root, project, binary := fixture(t, `read prompt
printf '{"type":"text","data":{"text":"done"}}\n'
printf 'warning\n' >&2
printf '%s|%s\n' "$PWD" "$prompt" >&2
printf '%s\n' "$OPENCODE_CONFIG_CONTENT" >&2
`)
	t.Setenv("PWD", root)
	result, err := (Executor{Binary: binary}).Run(context.Background(), testRequest(root, project, "spec prompt"))
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stderr, "warning\n"+project+"|spec prompt\n") || len(result.Events) != 1 || result.Events[0].Type != "text" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(result.Stderr, project+"|spec prompt") {
		t.Fatalf("stderr did not capture cwd and prompt: %q", result.Stderr)
	}
	if !strings.Contains(result.Stderr, `"action":"fabrica_gateway_*","effect":"allow"`) || !strings.Contains(result.Stderr, `"action":"*","effect":"deny"`) {
		t.Fatalf("OpenCode did not receive restrictive gateway config: %q", result.Stderr)
	}
}

func TestRunReportsProcessIdentity(t *testing.T) {
	root, project, binary := fixture(t, `printf '{"type":"text"}\n'`)
	seenPID := 0
	result, err := (Executor{Binary: binary}).Run(context.Background(), Request{
		WorkspaceRoot: root, ProjectDir: project, TaskID: 2, Phase: securitygateway.PhaseSpecification, Prompt: "run",
		OnStart: func(pid int, token string) error {
			seenPID = pid
			if pid <= 0 {
				t.Fatal("expected process id")
			}
			if token == "" && strings.Contains(runtime.GOOS, "linux") {
				t.Fatal("expected Linux process identity")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if seenPID <= 0 || len(result.Events) != 1 {
		t.Fatalf("process callback or event missing: pid=%d result=%#v", seenPID, result)
	}
}

func TestRunReturnsExitCode(t *testing.T) {
	root, project, binary := fixture(t, `echo '{"type":"error"}'
echo failed >&2
exit 7
`)
	result, err := (Executor{Binary: binary}).Run(context.Background(), testRequest(root, project, "run"))
	if err == nil || result.ExitCode != 7 || result.Stderr != "failed\n" {
		t.Fatalf("expected exit 7 with stderr, got result %#v, err %v", result, err)
	}
}

func TestRunHonorsCancellationAndOutputLimit(t *testing.T) {
	root, project, binary := fixture(t, `printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
exec sleep 5
`)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	request := testRequest(root, project, "run")
	request.OutputLimit = 8
	result, err := (Executor{Binary: binary}).Run(ctx, request)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if result.Stdout != "xxxxxxxx" || !result.StdoutTruncated {
		t.Fatalf("expected bounded stdout, got %#v", result)
	}
}

func TestRunRejectsProjectOutsideWorkspace(t *testing.T) {
	root, project, binary := fixture(t, `exit 0`)
	outside := t.TempDir()
	_, err := (Executor{Binary: binary}).Run(context.Background(), testRequest(root, outside, "run"))
	if err == nil {
		t.Fatal("expected outside project directory to be rejected")
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayConfigOverridesProjectAndInheritedConfiguration(t *testing.T) {
	root, project, _ := fixture(t, "exit 0")
	request := testRequest(root, project, "run")
	config, err := gatewayConfig(project, request, filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, `"default_agent":"fabrica"`) || !strings.Contains(config, `"servers":{"fabrica_gateway"`) || !strings.Contains(config, `"codemode":false`) || !strings.Contains(config, `"action":"*","effect":"deny"`) || !strings.Contains(config, `"action":"fabrica_gateway_*","effect":"allow"`) {
		t.Fatalf("gateway config is incomplete: %s", config)
	}
	environment := openCodeEnvironment([]string{"PATH=/bin", "PWD=/tmp/wrong-project", "OPENCODE_CONFIG=/tmp/project.json", "OPENCODE_CONFIG_CONTENT={}", "OPENCODE_CONFIG_DIR=/tmp/opencode"}, config, project)
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "OPENCODE_CONFIG=/tmp") || strings.Contains(joined, "OPENCODE_CONFIG_DIR") || strings.Contains(joined, "PWD=/tmp/wrong-project") || !strings.Contains(joined, "PWD="+project) || !strings.Contains(joined, "OPENCODE_DISABLE_PROJECT_CONFIG=1") || !strings.Contains(joined, "OPENCODE_PURE=1") {
		t.Fatalf("unsafe inherited/project config was not overridden: %s", joined)
	}
}

func TestRunFailsWhenGatewayReportsDeniedToolCall(t *testing.T) {
	root, project, binary := fixture(t, `audit=$(printf '%s' "$OPENCODE_CONFIG_CONTENT" | sed -E 's/.*"--audit-file","([^"]*)".*/\1/'); printf '{"category":"scope_violation"}\n' > "$audit"; printf '{"type":"text"}\n'`)
	result, err := (Executor{Binary: binary}).Run(context.Background(), testRequest(root, project, "run"))
	if err == nil || !strings.Contains(err.Error(), "security gateway blocked unauthorized agent action(s): scope_violation") || len(result.GatewayViolations) != 1 {
		t.Fatalf("expected gateway violation to fail the run, result=%#v err=%v", result, err)
	}
}

func TestRunRequiresGatewayPhaseAndTaskID(t *testing.T) {
	root, project, binary := fixture(t, "exit 0")
	if _, err := (Executor{Binary: binary}).Run(context.Background(), Request{WorkspaceRoot: root, ProjectDir: project, Prompt: "run"}); err == nil {
		t.Fatal("expected missing gateway policy to fail closed")
	}
}

func TestReadGatewayViolationsRejectsInvalidAuditRecords(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "violations.jsonl")
	if err := os.WriteFile(filename, []byte(`{"category":"scope_violation"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	violations, err := readGatewayViolations(filename)
	if err != nil || len(violations) != 1 || violations[0] != "scope_violation" {
		t.Fatalf("unexpected audit result: %v err=%v", violations, err)
	}
	if err := os.WriteFile(filename, []byte(`{"category":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readGatewayViolations(filename); err == nil {
		t.Fatal("expected malformed audit record to fail closed")
	}
}

func TestTerminateProcessGroupValidatesIdentityAndStopsChild(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process identity is read from Linux /proc")
	}
	if !errors.Is(TerminateProcessGroup(0, "token"), os.ErrProcessDone) || !errors.Is(TerminateProcessGroup(1, ""), os.ErrProcessDone) {
		t.Fatal("invalid process identity should be rejected")
	}
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "sleep 30")
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	identity := processIdentity(cmd.Process.Pid)
	if identity == "" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("could not read child process identity")
	}
	if err := TerminateProcessGroup(cmd.Process.Pid, "stale-token"); !errors.Is(err, os.ErrProcessDone) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("stale identity error=%v", err)
	}
	if err := TerminateProcessGroup(cmd.Process.Pid, identity); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("terminated process should return an exit error")
	}
}

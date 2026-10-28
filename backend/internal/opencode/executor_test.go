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

func TestRunCapturesPromptEventsAndStderr(t *testing.T) {
	root, project, binary := fixture(t, `read prompt
printf '{"type":"text","data":{"text":"done"}}\n'
printf 'warning\n' >&2
printf '%s|%s\n' "$PWD" "$prompt" >&2
`)
	result, err := (Executor{Binary: binary}).Run(context.Background(), Request{WorkspaceRoot: root, ProjectDir: project, Prompt: "spec prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stderr != "warning\n"+project+"|spec prompt\n" || len(result.Events) != 1 || result.Events[0].Type != "text" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(result.Stderr, project+"|spec prompt") {
		t.Fatalf("stderr did not capture cwd and prompt: %q", result.Stderr)
	}
}

func TestRunReportsProcessIdentity(t *testing.T) {
	root, project, binary := fixture(t, `printf '{"type":"text"}\n'`)
	seenPID := 0
	result, err := (Executor{Binary: binary}).Run(context.Background(), Request{
		WorkspaceRoot: root, ProjectDir: project, Prompt: "run",
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
	result, err := (Executor{Binary: binary}).Run(context.Background(), Request{WorkspaceRoot: root, ProjectDir: project, Prompt: "run"})
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
	result, err := (Executor{Binary: binary}).Run(ctx, Request{WorkspaceRoot: root, ProjectDir: project, Prompt: "run", OutputLimit: 8})
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
	_, err := (Executor{Binary: binary}).Run(context.Background(), Request{WorkspaceRoot: root, ProjectDir: outside, Prompt: "run"})
	if err == nil {
		t.Fatal("expected outside project directory to be rejected")
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatal(err)
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

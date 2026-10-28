package githubcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPassesArgumentsWithoutShellInterpolation(t *testing.T) {
	directory := t.TempDir()
	argsPath := filepath.Join(directory, "args")
	binary := filepath.Join(directory, "fake-gh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsPath + "\"\nprintf 'safe output'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	argument := "title $(touch " + filepath.Join(directory, "injected") + ")"
	output, err := (Client{Binary: binary}).Run(context.Background(), directory, "pr", "create", "--title", argument)
	if err != nil || output != "safe output" {
		t.Fatalf("Run() = %q, %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "injected")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("argument was interpreted by a shell: %v", err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil || !strings.Contains(string(args), argument) {
		t.Fatalf("gh did not receive the literal argument: %q, %v", args, err)
	}
}

func TestCheckAuthHidesCLIOutputOnFailure(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'token-bearing diagnostic' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := (Client{Binary: binary}).CheckAuth(context.Background(), directory)
	if !errors.Is(err, ErrUnauthenticated) || strings.Contains(err.Error(), "token-bearing") {
		t.Fatalf("auth error should be safe and typed: %v", err)
	}
}

func TestCreatePullRequestAndParseRequiredChecks(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "fake-gh")
	script := "#!/bin/sh\ncase \"$1 $2\" in\n  'pr create') echo https://github.com/acme/demo/pull/42 ;;\n  'pr checks') echo '[{\"name\":\"CI\",\"state\":\"SUCCESS\",\"link\":\"https://github.com/acme/demo/actions\",\"workflow\":\"CI\"}]' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client := Client{Binary: binary}
	pull, err := client.CreatePullRequest(context.Background(), directory, "Feature", "Body", "feature-branch")
	if err != nil || pull.Number != 42 || pull.URL != "https://github.com/acme/demo/pull/42" {
		t.Fatalf("PR = %#v, %v", pull, err)
	}
	checks, err := client.Checks(context.Background(), directory, pull.Number)
	if err != nil || len(checks) != 1 || checks[0].State != "SUCCESS" {
		t.Fatalf("checks = %#v, %v", checks, err)
	}
}

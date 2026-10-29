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

func TestFindViewAndMergePullRequest(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "fake-gh")
	script := "#!/bin/sh\ncase \"$1 $2 $3\" in\n  'pr view feature') echo '{\"number\":9,\"url\":\"https://github.com/acme/demo/pull/9\"}' ;;\n  'pr view 9') echo '{\"state\":\"MERGED\",\"url\":\"https://github.com/acme/demo/pull/9\",\"number\":9}' ;;\n  'pr merge 9') echo merged ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	client := Client{Binary: binary}
	pull, err := client.FindPullRequest(context.Background(), directory, "feature")
	if err != nil || pull.Number != 9 {
		t.Fatalf("FindPullRequest=%#v %v", pull, err)
	}
	state, err := client.ViewPullRequest(context.Background(), directory, 9)
	if err != nil || state.State != "MERGED" {
		t.Fatalf("ViewPullRequest=%#v %v", state, err)
	}
	if err := client.MergePullRequest(context.Background(), directory, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FindPullRequest(context.Background(), directory, " "); err == nil {
		t.Fatal("empty branch should fail")
	}
	if _, err := client.ViewPullRequest(context.Background(), directory, 0); err == nil {
		t.Fatal("invalid PR number should fail")
	}
	if err := client.MergePullRequest(context.Background(), directory, 0); err == nil {
		t.Fatal("invalid merge number should fail")
	}
}

func TestBoundedBufferTruncatesWithoutShortWrite(t *testing.T) {
	buffer := boundedBuffer{limit: 4}
	n, err := buffer.Write([]byte("abcdef"))
	if err != nil || n != 6 || buffer.String() != "abcd" {
		t.Fatalf("buffer write n=%d err=%v content=%q", n, err, buffer.String())
	}
}

func TestPullRequestClientRejectsMalformedResponsesAndRecoversExistingPR(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "fake-gh")
	write := func(script string) {
		t.Helper()
		if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	client := Client{Binary: binary}
	if _, err := client.Run(context.Background(), directory); err == nil {
		t.Fatal("Run should reject an empty command")
	}
	if _, err := client.CreatePullRequest(context.Background(), directory, "", "body", "branch"); err == nil {
		t.Fatal("CreatePullRequest should validate required fields")
	}
	write("#!/bin/sh\necho incomplete\n")
	if _, err := client.CreatePullRequest(context.Background(), directory, "title", "body", "branch"); err == nil {
		t.Fatal("CreatePullRequest should reject output without a PR URL")
	}
	write("#!/bin/sh\ncase \"$2\" in\n create) exit 1 ;;\n view) echo '{\"number\":12,\"url\":\"https://github.com/acme/demo/pull/12\"}' ;;\nesac\n")
	pull, err := client.CreatePullRequest(context.Background(), directory, "title", "body", "branch")
	if err != nil || pull.Number != 12 {
		t.Fatalf("existing PR recovery=%#v err=%v", pull, err)
	}
	write("#!/bin/sh\necho not-json\n")
	if _, err := client.Checks(context.Background(), directory, 12); err == nil {
		t.Fatal("Checks should reject invalid JSON")
	}
	if _, err := client.Checks(context.Background(), directory, 0); err == nil {
		t.Fatal("Checks should reject invalid PR numbers")
	}
	write("#!/bin/sh\ncase \"$1 $2\" in\n 'auth status') echo authenticated ;;\nesac\n")
	if err := client.CheckAuth(context.Background(), directory); err != nil {
		t.Fatalf("authenticated CLI returned error: %v", err)
	}
}

package projects

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareTaskBranchCommitsOnlyImplementationFiles(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	runGitTest(t, root, "config", "user.name", "Factory test")
	runGitTest(t, root, "config", "user.email", "factory-test@example.invalid")
	if err := os.WriteFile(filepath.Join(root, "base.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "base.txt")
	runGitTest(t, root, "commit", "-m", "chore: base")
	if err := os.WriteFile(filepath.Join(root, "implementation.txt"), []byte("implementation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("keep local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	branch, err := prepareTaskBranch(context.Background(), root, 27, "Add safe branch", []string{"implementation.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if branch != "fabrica/task-27-add-safe-branch" {
		t.Fatalf("branch = %q", branch)
	}
	committed := runGitTest(t, root, "show", "--pretty=format:", "--name-only", "HEAD")
	if strings.TrimSpace(committed) != "implementation.txt" {
		t.Fatalf("commit included unexpected paths: %q", committed)
	}
	status := runGitTest(t, root, "status", "--porcelain")
	if !strings.Contains(status, "unrelated.txt") {
		t.Fatalf("unrelated work should remain unstaged: %q", status)
	}
}

func runGitTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, output)
	}
	return string(output)
}

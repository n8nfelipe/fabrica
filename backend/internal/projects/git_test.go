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

func TestPrepareTaskBranchRejectsInvalidRepositoryAndFileStates(t *testing.T) {
	t.Run("workspace is not a repository", func(t *testing.T) {
		if _, err := prepareTaskBranch(context.Background(), t.TempDir(), 1, "Feature", []string{"file.go"}); err == nil || !strings.Contains(err.Error(), "Git repository") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("workspace is a nested directory", func(t *testing.T) {
		root := t.TempDir()
		runGitTest(t, root, "init", "-b", "main")
		nested := filepath.Join(root, "nested")
		if err := os.Mkdir(nested, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareTaskBranch(context.Background(), nested, 1, "Feature", []string{"file.go"}); err == nil || !strings.Contains(err.Error(), "root must match") {
			t.Fatalf("error=%v", err)
		}
	})
	newRepo := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		runGitTest(t, root, "init", "-b", "main")
		runGitTest(t, root, "config", "user.name", "Factory test")
		runGitTest(t, root, "config", "user.email", "factory-test@example.invalid")
		if err := os.WriteFile(filepath.Join(root, "base.txt"), []byte("base\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGitTest(t, root, "add", "base.txt")
		runGitTest(t, root, "commit", "-m", "base")
		return root
	}
	t.Run("no implementation files", func(t *testing.T) {
		root := newRepo(t)
		if _, err := prepareTaskBranch(context.Background(), root, 1, "Feature", nil); err == nil || !strings.Contains(err.Error(), "no implementation files") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("unsafe implementation path", func(t *testing.T) {
		root := newRepo(t)
		if _, err := prepareTaskBranch(context.Background(), root, 1, "Feature", []string{"../outside.go"}); err == nil || !strings.Contains(err.Error(), "unsafe file path") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("staged unrelated file", func(t *testing.T) {
		root := newRepo(t)
		if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("local\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGitTest(t, root, "add", "unrelated.txt")
		if _, err := prepareTaskBranch(context.Background(), root, 1, "Feature", []string{"implementation.go"}); err == nil || !strings.Contains(err.Error(), "staged changes outside") {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestPushTaskBranchReportsMissingRemote(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init", "-b", "main")
	if err := pushTaskBranch(context.Background(), root, "fabrica/task-1-feature"); err == nil || !strings.Contains(err.Error(), "verify the origin remote") {
		t.Fatalf("error=%v", err)
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

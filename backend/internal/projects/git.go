package projects

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var branchSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func prepareTaskBranch(ctx context.Context, workspace string, taskID int64, title string, files []string) (string, error) {
	root, err := gitOutput(ctx, workspace, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("project workspace must be a Git repository with a configured remote")
	}
	root, _ = filepath.EvalSymlinks(strings.TrimSpace(root))
	workspaceRoot, _ := filepath.EvalSymlinks(workspace)
	if root == "" || root != workspaceRoot {
		return "", errors.New("Git repository root must match the project workspace")
	}
	if len(files) == 0 {
		return "", errors.New("there are no implementation files to include in the pull request")
	}
	branch := fmt.Sprintf("fabrica/task-%d-%s", taskID, slugifyBranch(title))
	current, err := gitOutput(ctx, workspace, "branch", "--show-current")
	if err != nil {
		return "", errors.New("could not inspect the current Git branch")
	}
	if strings.TrimSpace(current) != branch {
		if _, err := gitOutput(ctx, workspace, "switch", "-c", branch); err != nil {
			if _, switchErr := gitOutput(ctx, workspace, "switch", branch); switchErr != nil {
				return "", errors.New("could not create or select the task branch")
			}
		}
	}
	args := []string{"add", "--"}
	allowed := make(map[string]bool, len(files))
	for _, path := range files {
		clean := filepath.Clean(filepath.FromSlash(path))
		if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", errors.New("implementation contains an unsafe file path")
		}
		allowed[clean] = true
		args = append(args, clean)
	}
	stagedNames, err := gitOutput(ctx, workspace, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return "", errors.New("could not inspect staged project files")
	}
	for _, path := range strings.Split(stagedNames, "\x00") {
		if path != "" && !allowed[filepath.Clean(path)] {
			return "", errors.New("project has staged changes outside the task")
		}
	}
	if _, err := gitOutput(ctx, workspace, args...); err != nil {
		return "", errors.New("could not stage task implementation files")
	}
	if _, err := gitOutput(ctx, workspace, "diff", "--cached", "--quiet"); err == nil {
		return branch, nil // A retry may find that the task commit is already present.
	}
	if _, err := gitOutput(ctx, workspace, "commit", "-m", fmt.Sprintf("feat: %s (#%d)", title, taskID)); err != nil {
		return "", errors.New("could not commit task implementation")
	}
	return branch, nil
}

func slugifyBranch(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = branchSlugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-")
	}
	if value == "" {
		return "task"
	}
	return value
}

func pushTaskBranch(ctx context.Context, workspace, branch string) error {
	if _, err := gitOutput(ctx, workspace, "push", "--set-upstream", "origin", branch); err != nil {
		return errors.New("could not push the task branch; verify the origin remote and gh auth setup-git")
	}
	return nil
}

func gitOutput(ctx context.Context, directory string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

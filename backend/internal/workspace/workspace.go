package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Root returns the configured workspace directory, defaulting to ~/workspace.
func Root(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		configured = filepath.Join(home, "workspace")
	} else if configured == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		configured = home
	} else if strings.HasPrefix(configured, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		configured = filepath.Join(home, strings.TrimPrefix(configured, "~/"))
	}

	absolute, err := filepath.Abs(configured)
	if err != nil {
		return "", fmt.Errorf("resolve workspace directory: %w", err)
	}
	return filepath.Clean(absolute), nil
}

// ValidateProjectName rejects names that cannot safely identify one workspace child.
func ValidateProjectName(name string) error {
	if name == "." || name == ".." || !projectNamePattern.MatchString(name) {
		return fmt.Errorf("invalid project name %q: use letters, numbers, dots, underscores, and hyphens, starting with a letter or number", name)
	}
	return nil
}

// CreateProject creates a dedicated project directory without replacing existing data.
func CreateProject(root, name string) (string, error) {
	if err := ValidateProjectName(name); err != nil {
		return "", err
	}

	resolvedRoot, err := Root(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(resolvedRoot, 0o700); err != nil {
		return "", fmt.Errorf("create workspace directory: %w", err)
	}
	resolvedRoot, err = filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace directory: %w", err)
	}

	projectPath := filepath.Join(resolvedRoot, name)
	relative, err := filepath.Rel(resolvedRoot, projectPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("project path escapes workspace")
	}
	if err := os.Mkdir(projectPath, 0o700); err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("project workspace %q already exists", name)
		}
		return "", fmt.Errorf("create project workspace: %w", err)
	}
	return projectPath, nil
}

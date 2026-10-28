package environments

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DescriptorPath = ".fabrica/project.json"

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type Descriptor struct {
	Version       int          `json:"version"`
	ComposeFile   string       `json:"composeFile"`
	QAService     string       `json:"qaService"`
	ContainerPort int          `json:"containerPort"`
	HealthCheck   *HealthCheck `json:"healthCheck,omitempty"`
}

type HealthCheck struct {
	Path string `json:"path"`
}

// LoadDescriptor reads the project's factory descriptor and validates its Compose path.
func LoadDescriptor(workspaceRoot string) (Descriptor, string, error) {
	var descriptor Descriptor
	root, err := canonicalDirectory(workspaceRoot)
	if err != nil {
		return descriptor, "", fmt.Errorf("resolve workspace root: %w", err)
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(DescriptorPath)))
	if errors.Is(err, os.ErrNotExist) {
		return descriptor, "", errors.New("project descriptor is missing from the workspace")
	}
	if err != nil {
		return descriptor, "", fmt.Errorf("resolve project descriptor: %w", err)
	}
	if !pathWithin(root, path) {
		return descriptor, "", errors.New("project descriptor resolves outside the workspace")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return descriptor, "", errors.New("project descriptor must be a regular file no larger than 64 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return descriptor, "", fmt.Errorf("read %s: %w", DescriptorPath, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return descriptor, "", fmt.Errorf("decode project descriptor: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return descriptor, "", errors.New("project descriptor must contain exactly one JSON object")
	}
	composePath, err := ValidateDescriptor(root, descriptor)
	if err != nil {
		return descriptor, "", err
	}
	return descriptor, composePath, nil
}

// ValidateDescriptor checks required descriptor values and confines Compose files to the project workspace.
func ValidateDescriptor(workspaceRoot string, descriptor Descriptor) (string, error) {
	root, err := canonicalDirectory(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	if descriptor.Version != 1 {
		return "", errors.New("descriptor version must be 1")
	}
	if strings.TrimSpace(descriptor.ComposeFile) == "" {
		return "", errors.New("composeFile is required")
	}
	if !serviceNamePattern.MatchString(descriptor.QAService) {
		return "", errors.New("qaService must be a valid Compose service name")
	}
	if descriptor.ContainerPort < 1 || descriptor.ContainerPort > 65535 {
		return "", errors.New("containerPort must be between 1 and 65535")
	}
	composeRelative := filepath.Clean(filepath.FromSlash(descriptor.ComposeFile))
	if filepath.IsAbs(composeRelative) || composeRelative == "." || composeRelative == ".." || strings.HasPrefix(composeRelative, ".."+string(filepath.Separator)) {
		return "", errors.New("composeFile must be a relative path inside the project workspace")
	}
	composePath, err := filepath.EvalSymlinks(filepath.Join(root, composeRelative))
	if err != nil {
		return "", fmt.Errorf("resolve Compose file: %w", err)
	}
	if !pathWithin(root, composePath) {
		return "", errors.New("composeFile resolves outside the project workspace")
	}
	info, err := os.Stat(composePath)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("composeFile must point to a regular file")
	}
	if descriptor.HealthCheck != nil {
		if err := validateHealthPath(descriptor.HealthCheck.Path); err != nil {
			return "", err
		}
	}
	return composePath, nil
}

func validateHealthPath(path string) error {
	parsed, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("healthCheck.path must be an absolute path on the QA service")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return errors.New("healthCheck.path cannot traverse directories")
		}
	}
	return nil
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workspace root is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("workspace root must be a directory")
	}
	return resolved, nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

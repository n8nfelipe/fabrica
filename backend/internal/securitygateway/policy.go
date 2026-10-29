package securitygateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	PhaseSpecification  = "specification"
	PhaseImplementation = "implementation"
	MaxScopePaths       = 256
)

var ErrDenied = errors.New("security gateway denied operation")

type ScopeManifest struct {
	Version       int      `json:"version"`
	WritablePaths []string `json:"writablePaths"`
}

type Policy struct {
	Root          string
	Phase         string
	TaskID        int64
	WritablePaths []string
	AuditFile     string
}

func NewPolicy(root, phase string, taskID int64, writablePaths []string, auditFile string) (Policy, error) {
	if root == "" || taskID <= 0 {
		return Policy{}, errors.New("gateway root and positive task ID are required")
	}
	if phase != PhaseSpecification && phase != PhaseImplementation {
		return Policy{}, errors.New("unsupported gateway phase")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Policy{}, fmt.Errorf("resolve gateway root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Policy{}, fmt.Errorf("resolve gateway root symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return Policy{}, errors.New("gateway root must be a directory")
	}
	cleanPaths := make([]string, 0, len(writablePaths))
	if len(writablePaths) > MaxScopePaths {
		return Policy{}, errors.New("write scope contains too many paths")
	}
	seen := make(map[string]struct{}, len(writablePaths))
	totalPathBytes := 0
	for _, value := range writablePaths {
		totalPathBytes += len(value)
		if len(value) > 1024 || totalPathBytes > 32<<10 || strings.ContainsAny(value, "*?[]{}") {
			return Policy{}, errors.New("write scope must use exact paths or directory prefixes")
		}
		clean, err := validateRelative(value, true)
		if err != nil || isReserved(clean, taskID) {
			return Policy{}, errors.New("write scope contains an unsafe path")
		}
		if _, exists := seen[clean]; exists {
			return Policy{}, errors.New("write scope contains duplicate paths")
		}
		seen[clean] = struct{}{}
		cleanPaths = append(cleanPaths, clean)
	}
	if phase == PhaseImplementation && len(cleanPaths) == 0 {
		return Policy{}, errors.New("implementation requires an approved write scope")
	}
	return Policy{Root: canonical, Phase: phase, TaskID: taskID, WritablePaths: cleanPaths, AuditFile: auditFile}, nil
}

func ParseScopeManifest(data []byte, taskID int64) ([]string, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var manifest ScopeManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, errors.New("scope manifest is invalid")
	}
	if manifest.Version != 1 || len(manifest.WritablePaths) == 0 || len(manifest.WritablePaths) > MaxScopePaths {
		return nil, errors.New("scope manifest must have version 1 and at least one writable path")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("scope manifest contains trailing data")
	}
	policy, err := NewPolicy(os.TempDir(), PhaseImplementation, taskID, manifest.WritablePaths, "")
	if err != nil {
		return nil, err
	}
	return policy.WritablePaths, nil
}

func validateRelative(value string, allowPrefix bool) (string, error) {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || path.IsAbs(value) {
		return "", ErrDenied
	}
	prefix := allowPrefix && strings.HasSuffix(value, "/")
	trimmed := strings.TrimSuffix(value, "/")
	if trimmed == "" || strings.Contains(trimmed, "//") {
		return "", ErrDenied
	}
	for _, component := range strings.Split(trimmed, "/") {
		if component == "" || component == "." || component == ".." {
			return "", ErrDenied
		}
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrDenied
	}
	if prefix {
		clean += "/"
	}
	return clean, nil
}

func isReserved(relative string, taskID int64) bool {
	clean := strings.TrimSuffix(relative, "/")
	for _, component := range strings.Split(clean, "/") {
		if component == ".git" || component == ".fabrica" || component == "node_modules" || component == "vendor" || component == ".cache" || isSensitiveName(component) {
			return true
		}
	}
	taskSpecPath := fmt.Sprintf("openspec/changes/task-%d", taskID)
	return clean == taskSpecPath || strings.HasPrefix(clean, taskSpecPath+"/")
}

func isSensitiveName(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") && lower != ".env.example" || lower == ".npmrc" || lower == ".netrc" || lower == "credentials" || lower == "credential" || lower == "secrets" || lower == "secret" || lower == "id_rsa" || lower == "id_ed25519" || lower == "id_ecdsa" || lower == "authorized_keys" || lower == ".aws" || lower == ".ssh" || lower == ".gnupg" || lower == ".kube" || lower == ".docker" {
		return true
	}
	return strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key")
}

func (p Policy) allowsWrite(relative string) bool {
	if p.Phase == PhaseSpecification {
		prefix := fmt.Sprintf("openspec/changes/task-%d/", p.TaskID)
		return strings.HasPrefix(relative, prefix)
	}
	for _, allowed := range p.WritablePaths {
		if strings.HasSuffix(allowed, "/") {
			if strings.HasPrefix(relative, allowed) {
				return true
			}
		} else if relative == allowed {
			return true
		}
	}
	return false
}

func (p Policy) AllowsWritePath(value string) bool {
	relative, err := validateRelative(value, false)
	return err == nil && !p.isReserved(relative) && p.allowsWrite(relative)
}

func ValidateChangedFiles(policy Policy, changed []string) error {
	for _, value := range changed {
		if !policy.AllowsWritePath(value) {
			return fmt.Errorf("%w: changed file is outside the approved write scope", ErrDenied)
		}
	}
	return nil
}

func (p Policy) isReserved(relative string) bool {
	clean := strings.TrimSuffix(relative, "/")
	for _, component := range strings.Split(clean, "/") {
		if component == ".git" || component == ".fabrica" || component == "node_modules" || component == "vendor" || component == ".cache" || isSensitiveName(component) {
			return true
		}
	}
	if p.Phase == PhaseSpecification {
		return false
	}
	taskSpecPath := fmt.Sprintf("openspec/changes/task-%d", p.TaskID)
	return clean == taskSpecPath || strings.HasPrefix(clean, taskSpecPath+"/")
}

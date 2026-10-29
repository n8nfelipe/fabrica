package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTaskWriteScopeValidatesManifestAndSymlinks(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "openspec", "changes", "task-31", "scope.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"version":1,"writablePaths":["backend/internal/example/"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := readTaskWriteScope(root, 31)
	if err != nil || len(paths) != 1 || paths[0] != "backend/internal/example/" {
		t.Fatalf("expected parsed write scope, paths=%v err=%v", paths, err)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "scope.json")
	if err := os.WriteFile(outside, []byte(`{"version":1,"writablePaths":["backend/"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := readTaskWriteScope(root, 31); err == nil {
		t.Fatal("expected symlinked scope manifest to be rejected")
	}
}

func TestReadTaskWriteScopeRejectsMissingAndInvalidManifest(t *testing.T) {
	root := t.TempDir()
	if _, err := readTaskWriteScope(root, 32); err == nil {
		t.Fatal("expected missing manifest to fail")
	}
	manifest := filepath.Join(root, "openspec", "changes", "task-32", "scope.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{"version":1,"writablePaths":["../outside"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTaskWriteScope(root, 32); err == nil {
		t.Fatal("expected invalid scope manifest to fail")
	}
	if err := os.WriteFile(manifest, []byte(`{"version":1,"writablePaths":["openspec/changes/task-32/"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTaskWriteScope(root, 32); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("expected task specification path to be rejected with a useful reason, got %v", err)
	}
}

func TestSpecificationScopeMustBePartOfGeneratedArtifactList(t *testing.T) {
	wanted := []byte(`["openspec/changes/task-33/scope.json"]`)
	if !specificationContainsScopeArtifact(wanted, 33) {
		t.Fatal("expected listed scope artifact to be recognized")
	}
	for _, input := range [][]byte{nil, []byte(`not-json`), []byte(`["openspec/changes/task-34/scope.json"]`)} {
		if specificationContainsScopeArtifact(input, 33) {
			t.Errorf("unexpected scope artifact acceptance: %s", input)
		}
	}
}

package opencode

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCaptureSnapshotAndChangedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := CaptureSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "ignored"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := CaptureSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ChangedFiles(before, after), []string{"existing.txt", "new.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangedFiles() = %v, want %v", got, want)
	}
}

func TestIsWithinConfinesPathsToRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "nested", "file.go")
	if !IsWithin(root, inside) {
		t.Fatalf("path inside root was rejected: %s", inside)
	}
	if IsWithin(root, filepath.Join(root+"-other", "file.go")) {
		t.Fatal("sibling path should not be inside root")
	}
}

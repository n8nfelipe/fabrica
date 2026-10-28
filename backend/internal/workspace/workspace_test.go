package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateProjectName(t *testing.T) {
	for _, name := range []string{"../outside", "..", ".", "nested/name", "space name", "-leading"} {
		if err := ValidateProjectName(name); err == nil {
			t.Errorf("ValidateProjectName(%q) accepted an invalid name", name)
		}
	}
	if err := ValidateProjectName("my-project_2.0"); err != nil {
		t.Fatalf("ValidateProjectName rejected a valid name: %v", err)
	}
}

func TestCreateProjectDoesNotOverwriteExistingPath(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "existing")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(project, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateProject(root, "existing"); err == nil {
		t.Fatal("CreateProject accepted a workspace collision")
	}
	contents, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("existing project file was removed: %v", err)
	}
	if string(contents) != "keep" {
		t.Fatalf("existing project file changed to %q", contents)
	}
}

func TestCreateProjectCreatesChildOfWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	project, err := CreateProject(root, "new-project")
	if err != nil {
		t.Fatal(err)
	}
	if project != filepath.Join(root, "new-project") {
		t.Fatalf("CreateProject returned %q", project)
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		t.Fatalf("project workspace was not created: %v", err)
	}
}

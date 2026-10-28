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

func TestRootResolvesConfiguredAndTildePaths(t *testing.T) {
	root, err := Root("  ./workspace-root  ")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(root) || filepath.Base(root) != "workspace-root" {
		t.Fatalf("configured root=%q", root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, configured := range []string{"~", "~/fabrica-workspace"} {
		got, err := Root(configured)
		if err != nil {
			t.Fatal(err)
		}
		want := home
		if configured != "~" {
			want = filepath.Join(home, "fabrica-workspace")
		}
		if got != want {
			t.Errorf("Root(%q)=%q want %q", configured, got, want)
		}
	}
}

func TestRootDefaultsToHomeWorkspace(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Root("  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, "workspace") {
		t.Fatalf("default root=%q", got)
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

func TestCreateProjectResolvesSymlinkedWorkspaceAndRejectsInvalidName(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	project, err := CreateProject(link, "safe-project")
	if err != nil {
		t.Fatal(err)
	}
	if project != filepath.Join(target, "safe-project") {
		t.Fatalf("project path=%q", project)
	}
	if _, err := CreateProject(link, "../outside"); err == nil {
		t.Fatal("invalid project name should be rejected")
	}
}

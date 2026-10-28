package environments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDescriptorReadsVersionedProjectDescriptor(t *testing.T) {
	root, compose := descriptorFixture(t)
	descriptorDir := filepath.Join(root, ".fabrica")
	if err := os.Mkdir(descriptorDir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(Descriptor{Version: 1, ComposeFile: "compose.yaml", QAService: "web", ContainerPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(descriptorDir, "project.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, composePath, err := LoadDescriptor(root)
	if err != nil || got.QAService != "web" || composePath != compose {
		t.Fatalf("LoadDescriptor() = %#v, %q, %v", got, composePath, err)
	}
}

func TestLoadDescriptorRejectsExternalDescriptorSymlink(t *testing.T) {
	root, _ := descriptorFixture(t)
	externalDir := t.TempDir()
	externalDescriptor := filepath.Join(externalDir, "project.json")
	if err := os.WriteFile(externalDescriptor, []byte(`{"version":1,"composeFile":"compose.yaml","qaService":"web","containerPort":3000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalDir, filepath.Join(root, ".fabrica")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := LoadDescriptor(root); err == nil {
		t.Fatal("expected external descriptor symlink to be rejected")
	}
}

func descriptorFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	compose := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, compose
}

func TestValidateDescriptorAcceptsValidComposeAndOptionalHealthCheck(t *testing.T) {
	root, compose := descriptorFixture(t)
	descriptor := Descriptor{Version: 1, ComposeFile: "compose.yaml", QAService: "web-app", ContainerPort: 4173,
		HealthCheck: &HealthCheck{Path: "/healthz"}}
	got, err := ValidateDescriptor(root, descriptor)
	if err != nil || got != compose {
		t.Fatalf("ValidateDescriptor() = %q, %v", got, err)
	}
	descriptor.HealthCheck = nil
	if _, err := ValidateDescriptor(root, descriptor); err != nil {
		t.Fatalf("optional health check rejected: %v", err)
	}
}

func TestValidateDescriptorRejectsMissingRequiredFields(t *testing.T) {
	root, _ := descriptorFixture(t)
	for _, descriptor := range []Descriptor{
		{Version: 1, QAService: "web", ContainerPort: 3000},
		{Version: 1, ComposeFile: "compose.yaml", ContainerPort: 3000},
		{Version: 1, ComposeFile: "compose.yaml", QAService: "web"},
	} {
		if _, err := ValidateDescriptor(root, descriptor); err == nil {
			t.Fatalf("expected invalid descriptor to be rejected: %#v", descriptor)
		}
	}
}

func TestValidateDescriptorRejectsTraversalAbsoluteAndExternalSymlink(t *testing.T) {
	root, _ := descriptorFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(outside, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.yaml", outside} {
		if _, err := ValidateDescriptor(root, Descriptor{Version: 1, ComposeFile: path, QAService: "web", ContainerPort: 3000}); err == nil {
			t.Fatalf("expected Compose path %q to be rejected", path)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "external.yaml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ValidateDescriptor(root, Descriptor{Version: 1, ComposeFile: "external.yaml", QAService: "web", ContainerPort: 3000}); err == nil {
		t.Fatal("expected external Compose symlink to be rejected")
	}
}

func TestValidateDescriptorRejectsInvalidHealthCheckPath(t *testing.T) {
	root, _ := descriptorFixture(t)
	for _, path := range []string{"healthz", "//example.test/health", "/../secret", "https://example.test/health"} {
		if _, err := ValidateDescriptor(root, Descriptor{Version: 1, ComposeFile: "compose.yaml", QAService: "web", ContainerPort: 3000, HealthCheck: &HealthCheck{Path: path}}); err == nil {
			t.Fatalf("expected invalid health path %q to be rejected", path)
		}
	}
}

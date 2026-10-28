package environments

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestComposeLifecycleIsolatesProjects(t *testing.T) {
	if os.Getenv("FABRICA_DOCKER_INTEGRATION") != "1" {
		t.Skip("set FABRICA_DOCKER_INTEGRATION=1 to run the local Docker lifecycle check")
	}
	manager := ComposeManager{}
	project1, root1 := createNginxProject(t)
	project2, root2 := createNginxProject(t)
	cleanup := func(id int64, root string) {
		name := "fabrica-" + strconv.FormatInt(id, 10)
		_ = exec.Command("docker", "compose", "--project-directory", root, "-f", filepath.Join(root, "compose.yaml"), "-p", name, "down", "--remove-orphans").Run()
	}
	t.Cleanup(func() { cleanup(project1, root1); cleanup(project2, root2) })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	first, err := manager.StartWithPort(ctx, project1, root1, freeHostPort(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.StartWithPort(ctx, project2, root2, freeHostPort(t))
	if err != nil {
		t.Fatal(err)
	}
	if !hasRunningContainer(first) || !hasRunningContainer(second) {
		t.Fatalf("both isolated projects should run: first=%#v second=%#v", first, second)
	}
	if err := manager.Stop(ctx, project1, root1); err != nil {
		t.Fatal(err)
	}
	first, err = manager.Inspect(ctx, project1, root1)
	if err != nil {
		t.Fatal(err)
	}
	second, err = manager.Inspect(ctx, project2, root2)
	if err != nil {
		t.Fatal(err)
	}
	if hasRunningContainer(first) || !hasRunningContainer(second) {
		t.Fatalf("stopping one project affected another: first=%#v second=%#v", first, second)
	}
	first, err = manager.Restart(ctx, project1, root1)
	if err != nil || !hasRunningContainer(first) {
		t.Fatalf("restart did not restore first project: %#v, %v", first, err)
	}
}

func freeHostPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func createNginxProject(t *testing.T) (int64, string) {
	t.Helper()
	root := t.TempDir()
	compose := "services:\n  web:\n    image: nginx:1.29-alpine\n"
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".fabrica"), 0o700); err != nil {
		t.Fatal(err)
	}
	descriptor, err := json.Marshal(Descriptor{Version: 1, ComposeFile: "compose.yaml", QAService: "web", ContainerPort: 80})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DescriptorPath), descriptor, 0o600); err != nil {
		t.Fatal(err)
	}
	return time.Now().UnixNano(), root
}

func hasRunningContainer(environment Environment) bool {
	for _, container := range environment.Containers {
		if container.State == "running" {
			return true
		}
	}
	return false
}

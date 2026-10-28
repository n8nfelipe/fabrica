package environments

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func composeFixture(t *testing.T, services string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	descriptorDir := filepath.Join(root, ".fabrica")
	if err := os.Mkdir(descriptorDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(descriptorDir, "project.json"), []byte(`{"version":1,"composeFile":"compose.yaml","qaService":"web","containerPort":80}`), 0o600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "compose-commands.log")
	fakeDocker := filepath.Join(root, "fake-docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$COMPOSE_TEST_LOG\"\ncase \"$*\" in\n  *'config --services'*) printf '%s\\n' '" + services + "' ;;\n  *'logs --no-color --tail 200'*) printf '%s\\n' 'web started' 'database started' ;;\n  *'ps --format json'*) printf '%s\\n' '{\"ID\":\"id\",\"Name\":\"web-1\",\"Project\":\"fabrica-42\",\"Service\":\"web\",\"State\":\"running\",\"Health\":\"healthy\",\"ExitCode\":0}' ;;\nesac\n"
	if err := os.WriteFile(fakeDocker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSE_TEST_LOG", logPath)
	return root, fakeDocker, logPath
}

func TestComposeManagerScopesLifecycleToProjectAndInspectsServices(t *testing.T) {
	root, binary, logPath := composeFixture(t, "web db")
	manager := ComposeManager{DockerPath: binary}
	state, err := manager.Start(context.Background(), 42, root)
	if err != nil {
		t.Fatal(err)
	}
	if state.ProjectName != "fabrica-42" || len(state.Containers) != 1 || state.Containers[0].Service != "web" || state.Containers[0].Health != "healthy" {
		t.Fatalf("unexpected Compose state: %#v", state)
	}
	if err := manager.Stop(context.Background(), 42, root); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Restart(context.Background(), 42, root); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(commands)
	for _, command := range []string{"-p fabrica-42", "up --detach", "ps --format json", "stop", "restart"} {
		if !strings.Contains(log, command) {
			t.Fatalf("Compose command log is missing %q", command)
		}
	}
}

func TestComposeManagerRejectsMissingQAServiceBeforeStarting(t *testing.T) {
	root, binary, _ := composeFixture(t, "db cache")
	_, err := (ComposeManager{DockerPath: binary}).Start(context.Background(), 42, root)
	if err == nil || !strings.Contains(err.Error(), "QA service") {
		t.Fatalf("expected missing QA service error, got %v", err)
	}
}

func TestComposeManagerLogsAllServicesForQASetupFailures(t *testing.T) {
	root, binary, logPath := composeFixture(t, "web db")
	logs, err := (ComposeManager{DockerPath: binary}).Logs(context.Background(), 42, root, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs, "web started") || !strings.Contains(logs, "database started") {
		t.Fatalf("Compose logs did not include all services: %q", logs)
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(commands), "logs --no-color --tail 200\n") {
		t.Fatalf("logs command unexpectedly targets only one service: %s", commands)
	}
}

func TestComposeManagerExplainsWhenNoContainerLogsAreAvailable(t *testing.T) {
	root, binary, _ := composeFixture(t, "web db")
	script := "#!/bin/sh\ncase \"$*\" in\n  *'config --services'*) printf '%s\\n' 'web db' ;;\n  *'logs --no-color --tail 200'*) exit 0 ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	logs, err := (ComposeManager{DockerPath: binary}).Logs(context.Background(), 42, root, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs, "failed before creating containers") {
		t.Fatalf("empty log response is not actionable: %q", logs)
	}
}

func TestComposeManagerGeneratesLoopbackRuntimePortOverride(t *testing.T) {
	root, binary, logPath := composeFixture(t, "web db")
	if _, err := (ComposeManager{DockerPath: binary}).StartWithPort(context.Background(), 42, root, 41888); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".fabrica", "runtime-compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "127.0.0.1:41888:80") {
		t.Fatalf("runtime override does not publish the allocated loopback port: %s", data)
	}
	if _, err := (ComposeManager{DockerPath: binary}).Inspect(context.Background(), 42, root); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(commands), "runtime-compose.yaml") {
		t.Fatal("lifecycle inspection did not reuse the managed runtime override")
	}
}

func TestComposeManagerDoesNotOverwriteUnmanagedRuntimeOverride(t *testing.T) {
	root, binary, _ := composeFixture(t, "web db")
	override := filepath.Join(root, ".fabrica", "runtime-compose.yaml")
	const userFile = "services:\n  web: {}\n"
	if err := os.WriteFile(override, []byte(userFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (ComposeManager{DockerPath: binary}).StartWithPort(context.Background(), 42, root, 41888); err == nil {
		t.Fatal("expected unmanaged runtime override to be rejected")
	}
	data, err := os.ReadFile(override)
	if err != nil || string(data) != userFile {
		t.Fatal("unmanaged runtime override was changed")
	}
}

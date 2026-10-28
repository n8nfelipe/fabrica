package environments

import (
	"context"
	"errors"
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

func TestLogsByProjectLabelHandlesMissingAndInvalidContainerIDs(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-docker")
	for _, tc := range []struct {
		name, ids string
		want      string
	}{{"no containers", "", "No container logs are available"}, {"invalid container id", "not-an-id", "No container logs are available"}, {"valid container id", "0123456789abcdef", "Container 0123456789abcdef:\nlogs from container"}} {
		t.Run(tc.name, func(t *testing.T) {
			script := "#!/bin/sh\ncase \"$1\" in\n ps) printf '%s\\n' '" + tc.ids + "' ;;\n logs) printf '%s\\n' 'logs from container' ;;\nesac\n"
			if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			got, err := (ComposeManager{DockerPath: binary}).logsByProjectLabel(context.Background(), 42, root, 100, errors.New("missing descriptor"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("logs=%q want substring %q", got, tc.want)
			}
		})
	}
}

func TestComposeRestartRecoversFailedRestartAndEmptyInspection(t *testing.T) {
	for _, mode := range []string{"restart-error", "empty-inspection"} {
		t.Run(mode, func(t *testing.T) {
			root, binary, _ := composeFixture(t, "web")
			logPath := filepath.Join(root, "restart.log")
			countPath := filepath.Join(root, "inspect-count")
			container := `{"ID":"id","Name":"web-1","Project":"fabrica-42","Service":"web","State":"running","Health":"healthy","ExitCode":0}`
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\nif [ \"$8\" = config ] && [ \"$9\" = --services ]; then echo web; exit 0; fi\n"
			if mode == "restart-error" {
				script += "if [ \"$8\" = restart ]; then echo 'restart unavailable' >&2; exit 1; fi\n"
			}
			if mode == "empty-inspection" {
				script += "if [ \"$8\" = ps ]; then count=0; [ -f '" + countPath + "' ] && count=$(cat '" + countPath + "'); if [ \"$count\" -eq 0 ]; then echo 1 > '" + countPath + "'; else echo '" + container + "'; fi; fi\n"
			} else {
				script += "if [ \"$8\" = ps ]; then echo '" + container + "'; fi\n"
			}
			if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			state, err := (ComposeManager{DockerPath: binary}).Restart(context.Background(), 42, root)
			if err != nil || len(state.Containers) != 1 {
				t.Fatalf("restart state=%#v err=%v", state, err)
			}
			commands, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(commands), "restart") || !strings.Contains(string(commands), "up --detach") {
				t.Fatalf("restart fallback commands=%s", commands)
			}
		})
	}
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

func TestRunDockerReturnsBoundedFailureAndRejectsOversizedOutput(t *testing.T) {
	t.Run("bounded stderr", func(t *testing.T) {
		root := t.TempDir()
		binary := filepath.Join(root, "fake-docker")
		if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'daemon unavailable' >&2\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := (ComposeManager{DockerPath: binary}).runDocker(context.Background(), root, "ps")
		if err == nil || !strings.Contains(err.Error(), "daemon unavailable") {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("oversized stdout", func(t *testing.T) {
		root := t.TempDir()
		binary := filepath.Join(root, "fake-docker")
		if err := os.WriteFile(binary, []byte("#!/bin/sh\nhead -c 3000000 /dev/zero\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		output, err := (ComposeManager{DockerPath: binary}).runDocker(context.Background(), root, "ps")
		if err == nil || !strings.Contains(err.Error(), "capture limit") {
			t.Fatalf("output size=%d error=%v", len(output), err)
		}
	})
}

func TestComposeLogsRejectsInvalidProjectAndTail(t *testing.T) {
	manager := ComposeManager{}
	if _, err := manager.Logs(context.Background(), 0, t.TempDir(), 20); err == nil {
		t.Fatal("expected invalid project ID to be rejected")
	}
	if _, err := manager.Logs(context.Background(), 1, t.TempDir(), 1001); err == nil {
		t.Fatal("expected oversized tail to be rejected")
	}
}

func TestParseContainersRejectsMalformedAndOversizedRecords(t *testing.T) {
	for name, output := range map[string]string{
		"invalid json":   "{broken}",
		"oversized line": strings.Repeat("x", composeOutputLimit+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseContainers(output); err == nil {
				t.Fatal("expected malformed container output to be rejected")
			}
		})
	}
	containers, err := parseContainers("\n{\"ID\":\"one\",\"Service\":\"web\"}\n\n{\"ID\":\"two\",\"Service\":\"db\"}\n")
	if err != nil || len(containers) != 2 {
		t.Fatalf("containers=%#v err=%v", containers, err)
	}
}

func TestWriteManagedOverrideUpdatesOnlyManagedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime-compose.yaml")
	if err := os.WriteFile(path, []byte(runtimeOverrideMarker+"\nold: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated := []byte(runtimeOverrideMarker + "\nnew: true\n")
	if err := writeManagedOverride(path, updated); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != string(updated) {
		t.Fatalf("managed override contents=%q err=%v", contents, err)
	}
	target := filepath.Join(dir, "target.yaml")
	if err := os.WriteFile(target, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked-compose.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedOverride(link, updated); err == nil {
		t.Fatal("symlink override should be rejected")
	}
}

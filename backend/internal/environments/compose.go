package environments

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const composeOutputLimit = 2 << 20

type Container struct {
	ID       string `json:"ID"`
	Name     string `json:"Name"`
	Project  string `json:"Project"`
	Service  string `json:"Service"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

type Environment struct {
	ProjectName string      `json:"projectName"`
	ComposeFile string      `json:"composeFile"`
	Containers  []Container `json:"containers"`
}

type ComposeManager struct {
	DockerPath string
	Timeout    time.Duration
}

func (m ComposeManager) Start(ctx context.Context, projectID int64, workspace string) (Environment, error) {
	args, descriptor, composePath, projectName, err := m.prepare(ctx, projectID, workspace, 0)
	if err != nil {
		return Environment{}, err
	}
	if _, err := m.run(ctx, workspace, append(args, "up", "--detach")...); err != nil {
		return Environment{}, err
	}
	return m.inspectPrepared(ctx, workspace, descriptor, composePath, projectName, args)
}

func (m ComposeManager) StartWithPort(ctx context.Context, projectID int64, workspace string, hostPort int) (Environment, error) {
	args, descriptor, composePath, projectName, err := m.prepare(ctx, projectID, workspace, hostPort)
	if err != nil {
		return Environment{}, err
	}
	if _, err := m.run(ctx, workspace, append(args, "up", "--detach")...); err != nil {
		return Environment{}, err
	}
	return m.inspectPrepared(ctx, workspace, descriptor, composePath, projectName, args)
}

func (m ComposeManager) Inspect(ctx context.Context, projectID int64, workspace string) (Environment, error) {
	args, descriptor, composePath, projectName, err := m.prepare(ctx, projectID, workspace, 0)
	if err != nil {
		return Environment{}, err
	}
	return m.inspectPrepared(ctx, workspace, descriptor, composePath, projectName, args)
}

func (m ComposeManager) Stop(ctx context.Context, projectID int64, workspace string) error {
	args, _, _, _, err := m.prepare(ctx, projectID, workspace, 0)
	if err != nil {
		return err
	}
	_, err = m.run(ctx, workspace, append(args, "stop")...)
	return err
}

func (m ComposeManager) Restart(ctx context.Context, projectID int64, workspace string) (Environment, error) {
	args, descriptor, composePath, projectName, err := m.prepare(ctx, projectID, workspace, 0)
	if err != nil {
		return Environment{}, err
	}
	if _, err := m.run(ctx, workspace, append(args, "restart")...); err != nil {
		if _, startErr := m.run(ctx, workspace, append(args, "up", "--detach")...); startErr != nil {
			return Environment{}, err
		}
	}
	state, err := m.inspectPrepared(ctx, workspace, descriptor, composePath, projectName, args)
	if err != nil || len(state.Containers) > 0 {
		return state, err
	}
	if _, err := m.run(ctx, workspace, append(args, "up", "--detach")...); err != nil {
		return Environment{}, err
	}
	return m.inspectPrepared(ctx, workspace, descriptor, composePath, projectName, args)
}

func (m ComposeManager) Logs(ctx context.Context, projectID int64, workspace string, tail int) (string, error) {
	if tail < 1 || tail > 1000 {
		return "", errors.New("log tail must be between 1 and 1000")
	}
	args, descriptor, _, _, err := m.prepare(ctx, projectID, workspace, 0)
	if err != nil {
		return "", err
	}
	return m.run(ctx, workspace, append(args, "logs", "--no-color", "--tail", strconv.Itoa(tail), descriptor.QAService)...)
}

func (m ComposeManager) prepare(ctx context.Context, projectID int64, workspace string, hostPort int) ([]string, Descriptor, string, string, error) {
	if projectID <= 0 {
		return nil, Descriptor{}, "", "", errors.New("project ID must be positive")
	}
	descriptor, composePath, err := LoadDescriptor(workspace)
	if err != nil {
		return nil, Descriptor{}, "", "", err
	}
	root, err := canonicalDirectory(workspace)
	if err != nil {
		return nil, Descriptor{}, "", "", err
	}
	projectName := "fabrica-" + strconv.FormatInt(projectID, 10)
	args := []string{"compose", "--project-directory", root, "-f", composePath, "-p", projectName}
	overridePath, err := ensureRuntimeOverride(root, descriptor, hostPort)
	if err != nil {
		return nil, Descriptor{}, "", "", err
	}
	if overridePath != "" {
		args = append(args, "-f", overridePath)
	}
	if _, err := m.run(ctx, root, append(args, "config", "--quiet")...); err != nil {
		return nil, Descriptor{}, "", "", fmt.Errorf("validate Compose project: %w", err)
	}
	servicesOutput, err := m.run(ctx, root, append(args, "config", "--services")...)
	if err != nil {
		return nil, Descriptor{}, "", "", fmt.Errorf("list Compose services: %w", err)
	}
	found := false
	for _, name := range strings.Fields(servicesOutput) {
		if name == descriptor.QAService {
			found = true
			break
		}
	}
	if !found {
		return nil, Descriptor{}, "", "", fmt.Errorf("QA service %q is not defined in Compose", descriptor.QAService)
	}
	return args, descriptor, composePath, projectName, nil
}

const runtimeOverrideMarker = "# Generated by Fábrica. Do not edit.\n"

func ensureRuntimeOverride(root string, descriptor Descriptor, hostPort int) (string, error) {
	path := filepath.Join(root, ".fabrica", "runtime-compose.yaml")
	if hostPort < 0 || hostPort > 65535 {
		return "", errors.New("QA host port must be between 1 and 65535")
	}
	if hostPort > 0 {
		mapping := fmt.Sprintf("127.0.0.1:%d:%d", hostPort, descriptor.ContainerPort)
		data, err := json.MarshalIndent(map[string]any{"services": map[string]any{descriptor.QAService: map[string]any{"ports": []string{mapping}}}}, "", "  ")
		if err != nil {
			return "", err
		}
		contents := append([]byte(runtimeOverrideMarker), data...)
		contents = append(contents, '\n')
		if err := writeManagedOverride(path, contents); err != nil {
			return "", err
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("runtime Compose override must be a regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(contents, []byte(runtimeOverrideMarker)) {
		return "", errors.New("runtime Compose override exists but is not managed by Fábrica")
	}
	return path, nil
}

func writeManagedOverride(path string, contents []byte) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("runtime Compose override must be a regular file")
		}
		previous, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(previous, []byte(runtimeOverrideMarker)) {
			return errors.New("runtime Compose override exists but is not managed by Fábrica")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".runtime-compose-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (m ComposeManager) inspectPrepared(ctx context.Context, workspace string, descriptor Descriptor, composePath, projectName string, args []string) (Environment, error) {
	output, err := m.run(ctx, workspace, append(args, "ps", "--format", "json")...)
	if err != nil {
		return Environment{}, err
	}
	containers, err := parseContainers(output)
	if err != nil {
		return Environment{}, err
	}
	return Environment{ProjectName: projectName, ComposeFile: composePath, Containers: containers}, nil
}

func (m ComposeManager) run(parent context.Context, directory string, args ...string) (string, error) {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	binary := m.DockerPath
	if binary == "" {
		binary = "docker"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = directory
	var stdout, stderr limitedOutput
	stdout.limit, stderr.limit = composeOutputLimit, 64<<10
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return stdout.String(), ctx.Err()
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		if len(message) > 1000 {
			message = message[:1000]
		}
		return stdout.String(), fmt.Errorf("docker compose %s failed: %s", args[len(args)-1], message)
	}
	if stdout.truncated || stderr.truncated {
		return stdout.String(), errors.New("docker Compose output exceeded the capture limit")
	}
	return stdout.String(), nil
}

type limitedOutput struct {
	buffer    bytes.Buffer
	limit     int64
	truncated bool
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - int64(b.buffer.Len())
	if remaining > 0 {
		if int64(len(p)) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buffer.Write(p)
	}
	if len(p) < original {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedOutput) String() string { return b.buffer.String() }

func parseContainers(output string) ([]Container, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return []Container{}, nil
	}
	if strings.HasPrefix(output, "[") {
		var containers []Container
		if err := json.Unmarshal([]byte(output), &containers); err != nil {
			return nil, fmt.Errorf("decode Compose container status: %w", err)
		}
		return containers, nil
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), composeOutputLimit)
	containers := make([]Container, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item Container
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("decode Compose container status: %w", err)
		}
		containers = append(containers, item)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return containers, nil
}

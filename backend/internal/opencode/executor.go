package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

const DefaultOutputLimit int64 = 4 << 20
const DefaultModel = "opencode/deepseek-v4.1-flash-free"
const ChatGPTPlanModel = "openai/gpt-5.6-luna"

type Event struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp,omitempty"`
	SessionID string          `json:"sessionID,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
}

type Request struct {
	WorkspaceRoot string
	ProjectDir    string
	Prompt        string
	OutputLimit   int64
	OnStart       func(pid int, processToken string) error
}

type Result struct {
	Events          []Event
	Stdout          string
	Stderr          string
	ExitCode        int
	StdoutTruncated bool
	StderrTruncated bool
}

type Executor struct {
	Binary string
	Model  string
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int64
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
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

// Run invokes OpenCode without a shell, with the prompt on stdin and JSON events on stdout.
func (e Executor) Run(ctx context.Context, request Request) (Result, error) {
	var result Result
	if strings.TrimSpace(request.Prompt) == "" {
		return result, errors.New("OpenCode prompt cannot be empty")
	}
	projectDir, err := resolveProjectDir(request.WorkspaceRoot, request.ProjectDir)
	if err != nil {
		return result, err
	}
	limit := request.OutputLimit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	binary := e.Binary
	if binary == "" {
		binary = "opencode"
	}
	model := e.Model
	if model == "" {
		model = DefaultModel
	}
	cmd := exec.CommandContext(ctx, binary, "run", "--model", model, "--format", "json")
	cmd.Dir = projectDir
	configureProcess(cmd)
	cmd.Stdin = strings.NewReader(request.Prompt)
	stdout := &boundedBuffer{limit: limit}
	stderr := &boundedBuffer{limit: limit}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	var runErr error
	if runErr = cmd.Start(); runErr == nil {
		if request.OnStart != nil {
			if startErr := request.OnStart(cmd.Process.Pid, processIdentity(cmd.Process.Pid)); startErr != nil {
				_ = TerminateProcessGroup(cmd.Process.Pid, processIdentity(cmd.Process.Pid))
				runErr = fmt.Errorf("persist OpenCode process identity: %w", startErr)
			}
		}
		waitErr := cmd.Wait()
		if runErr == nil {
			runErr = waitErr
		}
	}
	result.Stdout, result.Stderr = stdout.buffer.String(), stderr.buffer.String()
	result.StdoutTruncated, result.StderrTruncated = stdout.truncated, stderr.truncated
	result.ExitCode = 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	result.Events, err = parseEvents(result.Stdout)
	if err != nil {
		return result, fmt.Errorf("parse OpenCode JSON events: %w", err)
	}
	for _, event := range result.Events {
		if event.Type != "error" {
			continue
		}
		var detail struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if json.Unmarshal(event.Error, &detail) == nil && detail.Message != "" {
			return result, fmt.Errorf("OpenCode reported %s: %s", detail.Type, detail.Message)
		}
		return result, errors.New("OpenCode reported an error")
	}
	if runErr != nil {
		return result, fmt.Errorf("OpenCode exited with status %d: %w", result.ExitCode, runErr)
	}
	return result, nil
}

func resolveProjectDir(root, project string) (string, error) {
	if root == "" || project == "" {
		return "", errors.New("workspace root and project directory are required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	project, err = filepath.Abs(project)
	if err != nil {
		return "", fmt.Errorf("resolve project directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", fmt.Errorf("resolve project directory symlinks: %w", err)
	}
	relative, err := filepath.Rel(root, project)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("project directory must be a child of the workspace root")
	}
	return project, nil
}

func parseEvents(output string) ([]Event, error) {
	var events []Event
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return events, fmt.Errorf("invalid event line: %w", err)
		}
		if event.Type == "" {
			return events, errors.New("event is missing type")
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return events, err
	}
	return events, nil
}

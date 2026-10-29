package securitygateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxRequestBytes = 3 << 20
	maxFileBytes    = 2 << 20
	maxListEntries  = 1000
	maxSearchBytes  = 1 << 20
	maxToolCalls    = 300
	maxRepeatCalls  = 8
)

var ErrLoopLimit = errors.New("security gateway stopped a repetitive agent loop")

type Gateway struct {
	policy   Policy
	calls    int
	repeated map[string]int
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func New(policy Policy) *Gateway { return &Gateway{policy: policy, repeated: make(map[string]int)} }

func (g *Gateway) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxRequestBytes)
	writer := bufio.NewWriter(output)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		response := g.handle(line)
		if response == nil {
			continue
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("encode gateway response: %w", err)
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (g *Gateway) handle(line []byte) *rpcResponse {
	var request rpcRequest
	if err := json.Unmarshal(line, &request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		return &rpcResponse{JSONRPC: "2.0", ID: nil, Error: rpcError{Code: -32700, Message: "invalid JSON-RPC request"}}
	}
	hasID := len(request.ID) > 0 && string(request.ID) != "null"
	response := func(result any) *rpcResponse {
		if !hasID {
			return nil
		}
		var id any
		_ = json.Unmarshal(request.ID, &id)
		return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
	}
	failure := func(code int, message string) *rpcResponse {
		if !hasID {
			return nil
		}
		var id any
		_ = json.Unmarshal(request.ID, &id)
		return &rpcResponse{JSONRPC: "2.0", ID: id, Error: rpcError{Code: code, Message: message}}
	}

	switch request.Method {
	case "initialize":
		return response(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "fabrica-security-gateway", "version": "1.0.0"},
		})
	case "notifications/initialized", "notifications/cancelled":
		return nil
	case "ping":
		return response(map[string]any{})
	case "tools/list":
		return response(map[string]any{"tools": toolDefinitions()})
	case "tools/call":
		if !hasID {
			return nil
		}
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
			return failure(-32602, "invalid tool call")
		}
		result, err := g.call(params.Name, params.Arguments)
		if err != nil {
			return response(map[string]any{"content": []any{map[string]string{"type": "text", "text": err.Error()}}, "isError": true})
		}
		return response(map[string]any{"content": []any{map[string]string{"type": "text", "text": result}}})
	default:
		return failure(-32601, "method not found")
	}
}

func toolDefinitions() []map[string]any {
	return []map[string]any{
		{"name": "list_files", "description": "List regular files under a workspace-relative directory. Symlinks and sensitive paths are omitted.", "inputSchema": objectSchema(map[string]any{"path": map[string]any{"type": "string", "description": "Workspace-relative directory; use an empty string for the project root."}}, nil)},
		{"name": "search_files", "description": "Search text in non-sensitive regular files inside the workspace.", "inputSchema": objectSchema(map[string]any{"query": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}}, []string{"query"})},
		{"name": "read_file", "description": "Read one non-sensitive regular file inside the workspace.", "inputSchema": objectSchema(map[string]any{"path": map[string]any{"type": "string"}}, []string{"path"})},
		{"name": "write_file", "description": "Atomically create or replace a file when the active phase policy permits its path.", "inputSchema": objectSchema(map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, []string{"path", "content"})},
		{"name": "delete_file", "description": "Delete one regular file when the active phase policy permits its path.", "inputSchema": objectSchema(map[string]any{"path": map[string]any{"type": "string"}}, []string{"path"})},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func (g *Gateway) call(name string, raw json.RawMessage) (string, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", errors.New("tool arguments must be an object")
	}
	allowedArguments := map[string]map[string]bool{
		"list_files":   {"path": true},
		"search_files": {"path": true, "query": true},
		"read_file":    {"path": true},
		"write_file":   {"path": true, "content": true},
		"delete_file":  {"path": true},
	}
	for key := range args {
		if !allowedArguments[name][key] {
			return "", errors.New("tool argument is not supported")
		}
	}
	canonicalArgs, err := json.Marshal(args)
	if err != nil {
		return "", errors.New("tool arguments are invalid")
	}
	fingerprint := name + "\x00" + string(canonicalArgs)
	g.calls++
	if g.calls > maxToolCalls {
		g.recordViolation(name, "call_budget_exceeded")
		return "", ErrLoopLimit
	}
	g.repeated[fingerprint]++
	if g.repeated[fingerprint] > maxRepeatCalls {
		g.recordViolation(name, "repeated_call_loop")
		return "", ErrLoopLimit
	}
	var result string
	err = nil
	switch name {
	case "list_files":
		var relative string
		err = decodeString(args, "path", &relative, false)
		if err == nil {
			result, err = g.listFiles(relative)
		}
	case "search_files":
		var query, relative string
		err = decodeString(args, "query", &query, true)
		if err == nil {
			err = decodeString(args, "path", &relative, false)
		}
		if err == nil {
			result, err = g.searchFiles(query, relative)
		}
	case "read_file":
		var relative string
		err = decodeString(args, "path", &relative, true)
		if err == nil {
			result, err = g.readFile(relative)
		}
	case "write_file":
		var relative, content string
		err = decodeString(args, "path", &relative, true)
		if err == nil {
			err = decodeString(args, "content", &content, false)
		}
		if err == nil {
			err = g.writeFile(relative, content)
			if err == nil {
				result = "File written."
			}
		}
	case "delete_file":
		var relative string
		err = decodeString(args, "path", &relative, true)
		if err == nil {
			err = g.deleteFile(relative)
			if err == nil {
				result = "File deleted."
			}
		}
	default:
		err = fmt.Errorf("%w: tool is not allowed", ErrDenied)
	}
	if errors.Is(err, ErrDenied) {
		g.recordViolation(name, "scope_violation")
	}
	return result, err
}

func decodeString(args map[string]json.RawMessage, key string, target *string, required bool) error {
	raw, exists := args[key]
	if !exists {
		if required {
			return fmt.Errorf("%s is required", key)
		}
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%s must be a string", key)
	}
	return nil
}

func (g *Gateway) listFiles(value string) (string, error) {
	start := ""
	if value != "" {
		clean, err := validateRelative(value, false)
		if err != nil || g.policy.isReserved(clean) {
			return "", ErrDenied
		}
		start = clean
	}
	base, err := g.resolve(start, false, true)
	if err != nil {
		return "", err
	}
	var files []string
	err = filepath.WalkDir(base, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if current != base && isSensitiveName(entry.Name()) || entry.Name() == ".git" || entry.Name() == ".fabrica" {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || isSensitiveName(entry.Name()) {
			return nil
		}
		relative, relErr := filepath.Rel(g.policy.Root, current)
		if relErr != nil {
			return relErr
		}
		files = append(files, filepath.ToSlash(relative))
		if len(files) > maxListEntries {
			return fmt.Errorf("workspace listing exceeds the limit")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(files)
	return string(encoded), err
}

func (g *Gateway) searchFiles(query, value string) (string, error) {
	if strings.TrimSpace(query) == "" || len(query) > 1024 {
		return "", errors.New("search query must contain between 1 and 1024 bytes")
	}
	listing, err := g.listFiles(value)
	if err != nil {
		return "", err
	}
	var files []string
	if err := json.Unmarshal([]byte(listing), &files); err != nil {
		return "", err
	}
	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches := make([]match, 0)
	truncated := false
	for _, relative := range files {
		file, err := g.resolve(relative, false, false)
		if err != nil {
			continue
		}
		info, err := os.Stat(file)
		if err != nil || info.Size() > maxFileBytes {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil || !bytes.Contains(data, []byte(query)) {
			continue
		}
		for lineNumber, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, query) {
				if len(line) > 1024 {
					line = line[:1024]
				}
				matches = append(matches, match{Path: relative, Line: lineNumber + 1, Text: strings.TrimSpace(line)})
				if len(matches) >= 256 {
					truncated = true
					break
				}
			}
		}
		if truncated {
			break
		}
	}
	encoded, err := json.Marshal(struct {
		Matches   []match `json:"matches"`
		Truncated bool    `json:"truncated"`
	}{matches, truncated})
	if len(encoded) > maxSearchBytes {
		return "", errors.New("search result exceeds the response limit")
	}
	return string(encoded), err
}

func (g *Gateway) readFile(value string) (string, error) {
	relative, err := validateRelative(value, false)
	if err != nil || g.policy.isReserved(relative) {
		return "", ErrDenied
	}
	file, err := g.resolve(relative, false, false)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("file is unavailable")
	}
	if info.Size() > maxFileBytes {
		return "", errors.New("file exceeds the read limit")
	}
	data, err := os.ReadFile(file)
	return string(data), err
}

func (g *Gateway) writeFile(value, content string) error {
	relative, err := validateRelative(value, false)
	if err != nil || g.policy.isReserved(relative) || !g.policy.allowsWrite(relative) {
		return ErrDenied
	}
	if len(content) > maxFileBytes {
		return errors.New("file exceeds the write limit")
	}
	parent := filepath.Dir(filepath.Join(g.policy.Root, filepath.FromSlash(relative)))
	if err := g.ensureDirectories(filepath.Dir(relative)); err != nil {
		return err
	}
	target := filepath.Join(g.policy.Root, filepath.FromSlash(relative))
	if _, err := g.resolve(relative, true, false); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return ErrDenied
		}
		mode = info.Mode().Perm()
	}
	temporary, err := os.CreateTemp(parent, ".fabrica-agent-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, target)
}

func (g *Gateway) deleteFile(value string) error {
	relative, err := validateRelative(value, false)
	if err != nil || g.policy.isReserved(relative) || !g.policy.allowsWrite(relative) {
		return ErrDenied
	}
	file, err := g.resolve(relative, false, false)
	if err != nil {
		return err
	}
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrDenied
	}
	return os.Remove(file)
}

func (g *Gateway) resolve(relative string, allowMissing, directory bool) (string, error) {
	if relative == "" {
		if !directory {
			return "", ErrDenied
		}
		return g.policy.Root, nil
	}
	clean, err := validateRelative(relative, false)
	if err != nil || g.policy.isReserved(clean) {
		return "", ErrDenied
	}
	current := g.policy.Root
	parts := strings.Split(clean, "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) && allowMissing && i == len(parts)-1 {
			return current, nil
		}
		if statErr != nil {
			return "", errors.New("workspace path is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrDenied
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", ErrDenied
		}
	}
	info, err := os.Lstat(current)
	if err != nil {
		return "", errors.New("workspace path is unavailable")
	}
	if directory && !info.IsDir() {
		return "", errors.New("workspace directory is unavailable")
	}
	if !directory && !info.Mode().IsRegular() {
		return "", ErrDenied
	}
	return current, nil
}

func (g *Gateway) ensureDirectories(relative string) error {
	if relative == "." || relative == "" {
		return nil
	}
	clean, err := validateRelative(relative, false)
	if err != nil {
		return ErrDenied
	}
	current := g.policy.Root
	for _, component := range strings.Split(clean, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrDenied
		}
	}
	return nil
}

func (g *Gateway) recordViolation(tool, category string) {
	if g.policy.AuditFile == "" {
		return
	}
	file, err := os.OpenFile(g.policy.AuditFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	knownTool := "unknown"
	switch tool {
	case "list_files", "search_files", "read_file", "write_file", "delete_file":
		knownTool = tool
	}
	_ = json.NewEncoder(file).Encode(map[string]string{"tool": knownTool, "category": category})
}

package projects

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fabrica/backend/internal/workflow"
)

func TestSpecificationArtifactHelpers(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "openspec", "changes", "task-27", "proposal.md")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("proposal"), 0o600); err != nil {
		t.Fatal(err)
	}
	content, err := readSpecificationArtifacts(root, []string{"openspec/changes/task-27/proposal.md"})
	if err != nil || !strings.Contains(content, "proposal") {
		t.Fatalf("read valid artifact: %q %v", content, err)
	}
	for _, paths := range [][]string{{}, {"../outside"}, {"missing.md"}} {
		if _, err := readSpecificationArtifacts(root, paths); err == nil {
			t.Errorf("readSpecificationArtifacts(%v) should fail", paths)
		}
	}
	if _, err := collectSpecificationArtifacts(root, 27); err != nil {
		t.Fatal(err)
	}
	if _, err := collectSpecificationArtifacts(root, 28); err == nil {
		t.Fatal("missing change directory should fail")
	}
	paths := []string{"openspec/changes/task-27/proposal.md", "openspec/changes/task-27/design.md", "openspec/changes/task-27/tasks.md", "openspec/changes/task-27/specs/core/spec.md"}
	if !usableSpecification(paths, 27) {
		t.Fatal("complete specification should be usable")
	}
	if usableSpecification(paths[1:], 27) {
		t.Fatal("specification without its task proposal should not be usable")
	}
}

func TestProjectRequestAndWorkflowHelpers(t *testing.T) {
	for _, tc := range []struct {
		title, description string
		valid              bool
	}{{"title", "", true}, {"", "", false}, {strings.Repeat("x", 201), "", false}, {"title", strings.Repeat("x", 10001), false}} {
		if err := validateTask(tc.title, tc.description); (err == nil) != tc.valid {
			t.Errorf("validateTask(%d,%d) err=%v", len(tc.title), len(tc.description), err)
		}
	}
	for action, want := range map[workflow.Action]string{workflow.ActionApproveSpecification: "approval", workflow.ActionRejectQA: "approval", workflow.ActionImplementationError: "failure", workflow.ActionChecksFailed: "failure", workflow.ActionMarkReady: "transition"} {
		if got := eventType(action); got != want {
			t.Errorf("eventType(%q)=%q", action, got)
		}
	}
	for _, tc := range []struct {
		path string
		want int64
		ok   bool
	}{{"5", 5, true}, {"0", 0, false}, {"bad", 0, false}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("id", tc.path)
		w := httptest.NewRecorder()
		got, ok := parseID(w, r, "id")
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseID(%q)=(%d,%t)", tc.path, got, ok)
		}
	}
}

func TestDecodeJSONRejectsMalformedAndMultipleValues(t *testing.T) {
	for _, body := range []string{"{", `{"a":1} {"b":2}`, strings.Repeat("x", 33*1024)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		w := httptest.NewRecorder()
		var value map[string]any
		if err := decodeJSON(w, r, &value); err == nil {
			t.Errorf("decodeJSON accepted body with %d bytes", len(body))
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"ok":true}`))
	w := httptest.NewRecorder()
	var value map[string]bool
	if err := decodeJSON(w, r, &value); err != nil || !value["ok"] {
		t.Fatalf("valid JSON decode: %#v %v", value, err)
	}
}

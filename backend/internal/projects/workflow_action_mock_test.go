package projects

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fabrica/backend/internal/githubcli"
	"fabrica/backend/internal/opencode"
	"fabrica/backend/internal/workflow"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestMarkReadyActionRunsMockedSpecificationGeneration(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectRoot := filepath.Join(workspaceRoot, "demo")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	finished := filepath.Join(t.TempDir(), "agent-finished")
	script := "#!/bin/sh\nmkdir -p openspec/changes/task-43/specs/core\nprintf '%s\\n' proposal > openspec/changes/task-43/proposal.md\nprintf '%s\\n' design > openspec/changes/task-43/design.md\nprintf '%s\\n' tasks > openspec/changes/task-43/tasks.md\nprintf '%s\\n' spec > openspec/changes/task-43/specs/core/spec.md\nprintf '%s\\n' '{\"version\":1,\"writablePaths\":[\"main.go\"]}' > openspec/changes/task-43/scope.json\necho complete > '" + finished + "'\nprintf '%s\\n' '{\"type\":\"text\"}'\n"
	binary := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, workspaceRoot: workspaceRoot, opencode: opencode.Executor{Binary: binary}}
	token := "mark-ready-token"
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
	mock.ExpectBegin()
	selectedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(43)).WillReturnRows(sqlmock.NewRows(selectedCols).AddRow(43, 4, "Add a capability", "description", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", "", 7, now, 0, projectRoot))
	updatedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(43), workflow.StateSpecifying, workflow.StatusPending, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnRows(sqlmock.NewRows(updatedCols).AddRow(43, 4, "Add a capability", "description", "Especificando", "pending", "not_started", "not_started", "not_created", "unknown", 7, now))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(43), int64(7), "PO", "transition", string(workflow.ActionMarkReady), string(workflow.StateOpen), string(workflow.StateReady), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(43), int64(7), "system", "transition", string(workflow.ActionStartSpecification), string(workflow.StateReady), string(workflow.StateSpecifying), "", "").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt\),0\)\+1 FROM agent_runs WHERE task_id=\$1 AND phase=\$2`).WithArgs(int64(43), "specification").WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(1))
	mock.ExpectQuery(`INSERT INTO agent_runs\(task_id,phase,attempt,status\)`).WithArgs(int64(43), "specification", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(58))
	mock.ExpectCommit()
	initialCols := []string{"id", "project_id", "title", "description", "state", "requirements", "workspace_path", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, p.requirements`).WithArgs(int64(43)).WillReturnRows(sqlmock.NewRows(initialCols).AddRow(43, 4, "Add a capability", "description", "Especificando", "Requirements", projectRoot, "pending", "not_started", "not_started", "not_created", "unknown", 7, now))
	mock.ExpectExec(`UPDATE agent_runs SET process_pid=\$2, process_start_token=\$3`).WithArgs(int64(58), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(round\), 0\) \+ 1 FROM task_specifications WHERE task_id = \$1`).WithArgs(int64(43)).WillReturnRows(sqlmock.NewRows([]string{"round"}).AddRow(1))
	paths := []byte(`["openspec/changes/task-43/design.md","openspec/changes/task-43/proposal.md","openspec/changes/task-43/scope.json","openspec/changes/task-43/specs/core/spec.md","openspec/changes/task-43/tasks.md"]`)
	mock.ExpectExec(`INSERT INTO task_specifications \(task_id, round, status, artifact_paths, summary, run_id\)`).WithArgs(int64(43), 1, "generated", paths, "OpenSpec artifacts generated", int64(58)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(58), "succeeded", "OpenSpec artifacts generated", sqlmock.AnyArg(), sqlmock.AnyArg(), paths).WillReturnResult(sqlmock.NewResult(0, 1))
	lockedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(43)).WillReturnRows(sqlmock.NewRows(lockedCols).AddRow(43, 4, "Add a capability", "description", "Especificando", "pending", "not_started", "not_started", "not_created", "unknown", "", 7, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(43), workflow.StateWaitingSpecApproval, workflow.StatusPending, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(43), int64(7), "system", "transition", string(workflow.ActionSpecificationReady), string(workflow.StateSpecifying), string(workflow.StateWaitingSpecApproval), "", "").WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/43/actions", strings.NewReader(`{"action":"mark_ready"}`))
	r.SetPathValue("id", "43")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), string(workflow.StateSpecifying)) {
		t.Fatalf("mark ready=%d %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(finished); err == nil {
			if err := mock.ExpectationsWereMet(); err == nil {
				return
			}
		}
		if time.Now().After(deadline) {
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMergeTaskActionRequiresFreshPassingChecksAndCompletesWithMockDB(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	script := "#!/bin/sh\ncase \"$1 $2\" in\n 'pr checks') echo '[{\"name\":\"CI\",\"state\":\"SUCCESS\",\"link\":\"https://github.com/acme/demo/actions\",\"workflow\":\"CI\"}]' ;;\n 'pr merge') exit 0 ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	token := "merge-action-token"
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
	mock.ExpectBegin()
	selectedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(70)).WillReturnRows(sqlmock.NewRows(selectedCols).AddRow(70, 8, "Merge feature", "", "Pronta para merge", "approved", "succeeded", "approved", "open", "passed", "", 7, now, 42, root))
	updatedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(70), workflow.StateComplete, workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusMerged, workflow.StatusPassed, "").WillReturnRows(sqlmock.NewRows(updatedCols).AddRow(70, 8, "Merge feature", "", "Concluída", "approved", "succeeded", "approved", "merged", "passed", 7, now))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(70), int64(7), "PO", "transition", string(workflow.ActionMerge), string(workflow.StateReadyForMerge), string(workflow.StateComplete), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/70/actions", strings.NewReader(`{"action":"merge"}`))
	r.SetPathValue("id", "70")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), string(workflow.StateComplete)) {
		t.Fatalf("merge task=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReturnSpecificationActionRequiresCommentAndUpdatesLatestReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	token := "return-spec-token"
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
	mock.ExpectBegin()
	selectedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(71)).WillReturnRows(sqlmock.NewRows(selectedCols).AddRow(71, 8, "Feature", "", "Aguardando aprovação das especificações", "pending", "not_started", "not_started", "not_created", "unknown", "", 7, now, 0, "/tmp/project"))
	updatedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(71), workflow.StateSpecChanges, workflow.StatusChangesRequested, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnRows(sqlmock.NewRows(updatedCols).AddRow(71, 8, "Feature", "", "Ajustes nas especificações", "changes_requested", "not_started", "not_started", "not_created", "unknown", 7, now))
	mock.ExpectExec(`UPDATE task_specifications SET status=\$2, write_paths=\$3`).WithArgs(int64(71), "changes_requested", []byte("[]")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(71), int64(7), "PO", "approval", string(workflow.ActionReturnSpecification), "Aguardando aprovação das especificações", string(workflow.StateSpecChanges), "Revise a seção de API", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(71), int64(7), "PO", "comment", string(workflow.ActionReturnSpecification), "Aguardando aprovação das especificações", string(workflow.StateSpecChanges), "Revise a seção de API", "").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/71/actions", strings.NewReader(`{"action":"return_specification","comment":"Revise a seção de API"}`))
	r.SetPathValue("id", "71")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), string(workflow.StateSpecChanges)) {
		t.Fatalf("return specification=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMergeTaskActionBlocksWhenFreshRequiredCheckFails(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho '[{\"name\":\"CI\",\"state\":\"FAILURE\"}]'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	token := "failed-check-token"
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(9, "qa.user", "QA", "QA"))
	mock.ExpectBegin()
	selectedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(72)).WillReturnRows(sqlmock.NewRows(selectedCols).AddRow(72, 8, "Feature", "", "Pronta para merge", "approved", "succeeded", "approved", "open", "passed", "", 9, now, 42, root))
	mock.ExpectExec(`UPDATE tasks SET state=\$2,checks_status=\$3`).WithArgs(int64(72), workflow.StatePROpen, workflow.StatusChecksFailed).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(72), int64(9), "system", "transition", string(workflow.ActionChecksFailed), string(workflow.StateReadyForMerge), string(workflow.StatePROpen), "", "Required GitHub checks are not passing").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/72/actions", strings.NewReader(`{"action":"merge"}`))
	r.SetPathValue("id", "72")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "all required GitHub checks must pass") {
		t.Fatalf("blocked merge=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionValidationAndQAGatesWithMocks(t *testing.T) {
	newHandler := func(t *testing.T) (*Handler, sqlmock.Sqlmock) {
		t.Helper()
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return &Handler{db: db}, mock
	}
	t.Run("malformed body", func(t *testing.T) {
		h, _ := newHandler(t)
		r := httptest.NewRequest("POST", "/api/tasks/1/actions", strings.NewReader("{"))
		r.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		h.applyTaskAction(w, r)
		if w.Code != 400 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	t.Run("unauthenticated", func(t *testing.T) {
		h, mock := newHandler(t)
		token := "missing-action-session"
		tokenHash := sha256.Sum256([]byte(token))
		mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}))
		r := httptest.NewRequest("POST", "/api/tasks/1/actions", strings.NewReader(`{"action":"mark_ready"}`))
		r.SetPathValue("id", "1")
		r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
		w := httptest.NewRecorder()
		h.applyTaskAction(w, r)
		if w.Code != 401 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		h, mock := newHandler(t)
		token := "missing-action-task"
		tokenHash := sha256.Sum256([]byte(token))
		mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}))
		r := httptest.NewRequest("POST", "/api/tasks/1/actions", strings.NewReader(`{"action":"mark_ready"}`))
		r.SetPathValue("id", "1")
		r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
		w := httptest.NewRecorder()
		h.applyTaskAction(w, r)
		if w.Code != 404 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("QA start requires healthy environment", func(t *testing.T) {
		h, mock := newHandler(t)
		token := "qa-action-gate"
		tokenHash := sha256.Sum256([]byte(token))
		now := time.Now()
		mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "qa.user", "QA", "QA"))
		mock.ExpectBegin()
		taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
		mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(2, 4, "QA", "", string(workflow.StateInDevelopment), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "", 7, now, 0, "/tmp/project"))
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM project_qa_environments`).WithArgs(int64(4), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(2), int64(7), "QA", "attempt", string(workflow.ActionStartQA), string(workflow.StateInDevelopment), string(workflow.StateInDevelopment), "", "QA environment is not healthy").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		r := httptest.NewRequest("POST", "/api/tasks/2/actions", strings.NewReader(`{"action":"qa_started"}`))
		r.SetPathValue("id", "2")
		r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
		w := httptest.NewRecorder()
		h.applyTaskAction(w, r)
		if w.Code != 409 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestApplyTaskActionRecordsForbiddenRoleAttemptWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	token := "forbidden-action-token"
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(8, "qa.user", "QA", "QA"))
	mock.ExpectBegin()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(81)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(81, 5, "Restricted", "", string(workflow.StateOpen), workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "", 8, now, 0, "/tmp/project"))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(81), int64(8), "QA", "attempt", string(workflow.ActionMarkReady), string(workflow.StateOpen), string(workflow.StateOpen), "", workflow.ErrForbidden.Error()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/81/actions", strings.NewReader(`{"action":"mark_ready"}`))
	r.SetPathValue("id", "81")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 403 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionSpecificationApprovalAndCorrectionGatesWithMocks(t *testing.T) {
	t.Run("approved specification is not generated", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		h := &Handler{db: db}
		token := "approve-spec-missing-artifact"
		hash := sha256.Sum256([]byte(token))
		now := time.Now()
		projectRoot := t.TempDir()
		scopePath := filepath.Join(projectRoot, "openspec", "changes", "task-82", "scope.json")
		if err := os.MkdirAll(filepath.Dir(scopePath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(scopePath, []byte(`{"version":1,"writablePaths":["main.go"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
		mock.ExpectBegin()
		cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
		mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(82)).WillReturnRows(sqlmock.NewRows(cols).AddRow(82, 5, "Spec approval", "", string(workflow.StateWaitingSpecApproval), workflow.StatusPending, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "", 7, now, 0, projectRoot))
		mock.ExpectQuery(`SELECT artifact_paths FROM task_specifications WHERE task_id=\$1 AND status='generated'`).WithArgs(int64(82)).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths"}).AddRow([]byte(`["openspec/changes/task-82/scope.json"]`)))
		updatedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
		mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(82), workflow.StateInDevelopment, workflow.StatusApproved, workflow.StatusRunning, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnRows(sqlmock.NewRows(updatedCols).AddRow(82, 5, "Spec approval", "", string(workflow.StateInDevelopment), workflow.StatusApproved, workflow.StatusRunning, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, 7, now))
		mock.ExpectExec(`UPDATE task_specifications SET status=\$2, write_paths=\$3`).WithArgs(int64(82), "approved", []byte(`["main.go"]`)).WillReturnResult(sqlmock.NewResult(0, 0))
		r := httptest.NewRequest("POST", "/api/tasks/82/actions", strings.NewReader(`{"action":"approve_specification"}`))
		r.SetPathValue("id", "82")
		r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
		w := httptest.NewRecorder()
		h.applyTaskAction(w, r)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "no generated specification") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("correction lacks QA rejection feedback", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		h := &Handler{db: db}
		token := "correction-no-feedback"
		hash := sha256.Sum256([]byte(token))
		now := time.Now()
		mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(9, "dev.user", "Developer", "Desenvolvedor"))
		mock.ExpectBegin()
		cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
		mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(83)).WillReturnRows(sqlmock.NewRows(cols).AddRow(83, 5, "Correction", "", string(workflow.StateCorrectionNeeded), workflow.StatusApproved, workflow.StatusFailed, workflow.StatusRejected, workflow.StatusNotCreated, workflow.StatusUnknown, "", 9, now, 0, "/tmp/project"))
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM task_events WHERE task_id=\$1 AND action=\$2 AND event_type='approval'\)`).WithArgs(int64(83), string(workflow.ActionRejectQA)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()
		r := httptest.NewRequest("POST", "/api/tasks/83/actions", strings.NewReader(`{"action":"start_correction"}`))
		r.SetPathValue("id", "83")
		r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
		w := httptest.NewRecorder()
		h.applyTaskAction(w, r)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "QA rejection feedback is required") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestApplyTaskActionMergeAndDatabaseGatesWithMocks(t *testing.T) {
	for _, tc := range []struct {
		name, role, action string
		prNumber           any
		beginErr, taskErr  bool
		want               int
	}{
		{name: "transaction cannot start", role: "PO", action: string(workflow.ActionMarkReady), beginErr: true, want: 500},
		{name: "task lookup fails", role: "PO", action: string(workflow.ActionMarkReady), taskErr: true, want: 500},
		{name: "role cannot merge", role: "Scrum Master", action: string(workflow.ActionMerge), prNumber: 9, want: 403},
		{name: "PR is not registered", role: "Admin", action: string(workflow.ActionMerge), prNumber: 0, want: 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			h := &Handler{db: db}
			token := "merge-gate-" + tc.name
			hash := sha256.Sum256([]byte(token))
			now := time.Now()
			mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "actor", "Actor", tc.role))
			if tc.beginErr {
				mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
			} else {
				mock.ExpectBegin()
				if tc.taskErr {
					mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(84)).WillReturnError(errors.New("read failed"))
				} else {
					cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
					pr := tc.prNumber
					if pr == nil {
						pr = 0
					}
					mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(84)).WillReturnRows(sqlmock.NewRows(cols).AddRow(84, 5, "Merge gate", "", string(workflow.StateReadyForMerge), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusOpen, workflow.StatusPassed, "", 7, now, pr, "/tmp/project"))
				}
			}
			r := httptest.NewRequest("POST", "/api/tasks/84/actions", strings.NewReader(fmt.Sprintf(`{"action":%q}`, tc.action)))
			r.SetPathValue("id", "84")
			r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
			w := httptest.NewRecorder()
			h.applyTaskAction(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplyTaskActionRecordsGitHubMergeFailureWithMockDB(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	script := "#!/bin/sh\ncase \"$1 $2\" in\n 'pr checks') echo '[{\"name\":\"CI\",\"state\":\"SUCCESS\"}]' ;;\n 'pr merge') echo 'merge rejected' >&2; exit 1 ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	token := "github-merge-failure"
	hash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
	mock.ExpectBegin()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(85)).WillReturnRows(sqlmock.NewRows(cols).AddRow(85, 5, "Merge failure", "", string(workflow.StateReadyForMerge), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusOpen, workflow.StatusPassed, "", 7, now, 45, root))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(85), int64(7), "system", "failure", string(workflow.ActionMerge), string(workflow.StateReadyForMerge), string(workflow.StateReadyForMerge), "", "GitHub did not complete the pull request merge").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/85/actions", strings.NewReader(`{"action":"merge"}`))
	r.SetPathValue("id", "85")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionMergesOpenPRAfterFreshChecksPass(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	script := "#!/bin/sh\ncase \"$1 $2\" in\n 'pr checks') echo '[{\"name\":\"CI\",\"state\":\"SUCCESS\"}]' ;;\n 'pr merge') exit 0 ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	token := "merge-open-pr"
	hash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "po.user", "PO", "PO"))
	mock.ExpectBegin()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(107)).WillReturnRows(sqlmock.NewRows(cols).AddRow(107, 5, "Merge from PR open", "", string(workflow.StatePROpen), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusOpen, workflow.StatusPending, "", 7, now, 47, root))
	mock.ExpectExec(`UPDATE tasks SET state=\$2,checks_status=\$3`).WithArgs(int64(107), workflow.StateReadyForMerge, workflow.StatusPassed).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(107), int64(7), "system", "transition", string(workflow.ActionChecksPassed), string(workflow.StatePROpen), string(workflow.StateReadyForMerge), "", "Required GitHub checks passed").WillReturnResult(sqlmock.NewResult(1, 1))
	updated := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(107), workflow.StateComplete, workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusMerged, workflow.StatusPassed, "").WillReturnRows(sqlmock.NewRows(updated).AddRow(107, 5, "Merge from PR open", "", string(workflow.StateComplete), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusMerged, workflow.StatusPassed, 7, now))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(107), int64(7), "PO", "transition", string(workflow.ActionMerge), string(workflow.StateReadyForMerge), string(workflow.StateComplete), "", "").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/107/actions", strings.NewReader(`{"action":"merge"}`))
	r.SetPathValue("id", "107")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), string(workflow.StateComplete)) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionReturnsBadGatewayWhenChecksCannotBeRead(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'network unavailable' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	token := "checks-read-failure"
	hash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "admin", "Admin", "Admin"))
	mock.ExpectBegin()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(108)).WillReturnRows(sqlmock.NewRows(cols).AddRow(108, 5, "Checks unavailable", "", string(workflow.StateReadyForMerge), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusOpen, workflow.StatusPassed, "", 7, now, 48, root))
	r := httptest.NewRequest("POST", "/api/tasks/108/actions", strings.NewReader(`{"action":"merge"}`))
	r.SetPathValue("id", "108")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

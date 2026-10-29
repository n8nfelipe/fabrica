package projects

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fabrica/backend/internal/environments"
	"fabrica/backend/internal/githubcli"
	"fabrica/backend/internal/opencode"
	"fabrica/backend/internal/workflow"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestProjectReadAndListHandlersWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	projectColumns := []string{"id", "name", "description", "requirements", "workspace_path", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at\s+FROM projects ORDER`).
		WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(1, "demo", "desc", "req", "/tmp/demo", 7, now))
	w := httptest.NewRecorder()
	h.listProjects(w, httptest.NewRequest("GET", "/api/projects", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"demo"`) {
		t.Fatalf("listProjects = %d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at\s+FROM projects WHERE id`).
		WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(1, "demo", "desc", "req", "/tmp/demo", 7, now))
	r := httptest.NewRequest("GET", "/api/projects/1", nil)
	r.SetPathValue("id", "1")
	w = httptest.NewRecorder()
	h.getProject(w, r)
	if w.Code != 200 {
		t.Fatalf("getProject = %d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListHandlersReturnDatabaseErrorsWithMocks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	cases := []struct {
		name    string
		route   string
		path    map[string]string
		prepare func()
		call    func(http.ResponseWriter, *http.Request)
	}{
		{name: "projects", route: "/api/projects", call: h.listProjects, prepare: func() {
			mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at`).WillReturnError(errors.New("database unavailable"))
		}},
		{name: "tasks project lookup", route: "/api/projects/3/tasks", path: map[string]string{"projectID": "3"}, call: h.listTasks, prepare: func() {
			mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE id = \$1\)`).WithArgs(int64(3)).WillReturnError(errors.New("database unavailable"))
		}},
		{name: "task history lookup", route: "/api/tasks/3/events", path: map[string]string{"id": "3"}, call: h.listTaskEvents, prepare: func() {
			mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM tasks WHERE id = \$1\)`).WithArgs(int64(3)).WillReturnError(errors.New("database unavailable"))
		}},
		{name: "specifications", route: "/api/tasks/3/specifications", path: map[string]string{"id": "3"}, call: h.listTaskSpecifications, prepare: func() {
			mock.ExpectQuery(`SELECT id, task_id, round, status, artifact_paths, summary, created_at FROM task_specifications`).WithArgs(int64(3)).WillReturnError(errors.New("database unavailable"))
		}},
		{name: "implementations", route: "/api/tasks/3/implementations", path: map[string]string{"id": "3"}, call: h.listTaskImplementations, prepare: func() {
			mock.ExpectQuery(`SELECT id, task_id, attempt, status, changed_files, summary, created_at FROM task_implementations`).WithArgs(int64(3)).WillReturnError(errors.New("database unavailable"))
		}},
		{name: "agent runs", route: "/api/tasks/3/agent-runs", path: map[string]string{"id": "3"}, call: h.listAgentRuns, prepare: func() {
			mock.ExpectQuery(`SELECT id, task_id, phase, attempt, status, summary, result_refs, started_at, finished_at FROM agent_runs`).WithArgs(int64(3)).WillReturnError(errors.New("database unavailable"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.prepare()
			r := httptest.NewRequest("GET", tc.route, nil)
			for key, value := range tc.path {
				r.SetPathValue(key, value)
			}
			w := httptest.NewRecorder()
			tc.call(w, r)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListTasksHandlesProjectAndTaskQueryFailuresWithMocks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE id = \$1\)`).WithArgs(int64(109)).WillReturnError(errors.New("lookup failed"))
	r := httptest.NewRequest("GET", "/api/projects/109/tasks", nil)
	r.SetPathValue("projectID", "109")
	w := httptest.NewRecorder()
	h.listTasks(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("project lookup status=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE id = \$1\)`).WithArgs(int64(110)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status, implementation_status`).WithArgs(int64(110)).WillReturnError(errors.New("list failed"))
	r = httptest.NewRequest("GET", "/api/projects/110/tasks", nil)
	r.SetPathValue("projectID", "110")
	w = httptest.NewRecorder()
	h.listTasks(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("task list status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListHandlersRejectMalformedDatabaseRowsWithMocks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	t.Run("project scan", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("not-an-id"))
		w := httptest.NewRecorder()
		h.listProjects(w, httptest.NewRequest("GET", "/api/projects", nil))
		if w.Code != 500 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	t.Run("task event scan", func(t *testing.T) {
		mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM tasks WHERE id = \$1\)`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(`SELECT e.id, e.task_id, e.actor_id`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("not-an-id"))
		r := httptest.NewRequest("GET", "/api/tasks/3/events", nil)
		r.SetPathValue("id", "3")
		w := httptest.NewRecorder()
		h.listTaskEvents(w, r)
		if w.Code != 500 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	t.Run("specification JSON", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, task_id, round, status, artifact_paths, summary, created_at FROM task_specifications`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "round", "status", "artifact_paths", "summary", "created_at"}).AddRow(1, 3, 1, "generated", []byte("{"), "bad", time.Now()))
		r := httptest.NewRequest("GET", "/api/tasks/3/specifications", nil)
		r.SetPathValue("id", "3")
		w := httptest.NewRecorder()
		h.listTaskSpecifications(w, r)
		if w.Code != 500 || !strings.Contains(w.Body.String(), "stored specification paths are invalid") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	t.Run("implementation JSON", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, task_id, attempt, status, changed_files, summary, created_at FROM task_implementations`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "attempt", "status", "changed_files", "summary", "created_at"}).AddRow(1, 3, 1, "succeeded", []byte("{"), "bad", time.Now()))
		r := httptest.NewRequest("GET", "/api/tasks/3/implementations", nil)
		r.SetPathValue("id", "3")
		w := httptest.NewRecorder()
		h.listTaskImplementations(w, r)
		if w.Code != 500 || !strings.Contains(w.Body.String(), "stored implementation paths are invalid") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	t.Run("agent run JSON", func(t *testing.T) {
		mock.ExpectQuery(`SELECT id, task_id, phase, attempt, status, summary, result_refs, started_at, finished_at FROM agent_runs`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "status", "summary", "result_refs", "started_at", "finished_at"}).AddRow(1, 3, "implementation", 1, "succeeded", "bad", []byte("{"), time.Now(), nil))
		r := httptest.NewRequest("GET", "/api/tasks/3/agent-runs", nil)
		r.SetPathValue("id", "3")
		w := httptest.NewRecorder()
		h.listAgentRuns(w, r)
		if w.Code != 500 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectAndTaskHandlerValidationAndMissingRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	cases := []struct {
		name      string
		pathValue string
		handler   func(http.ResponseWriter, *http.Request)
		want      int
	}{
		{name: "invalid project id", pathValue: "bad", handler: h.getProject, want: 400},
		{name: "missing project", pathValue: "99", handler: func(w http.ResponseWriter, r *http.Request) {
			mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at`).WithArgs(int64(99)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "requirements", "workspace_path", "created_by", "created_at"}))
			h.getProject(w, r)
		}, want: 404},
		{name: "invalid task id", pathValue: "0", handler: h.getTask, want: 400},
		{name: "invalid patch", pathValue: "1", handler: func(w http.ResponseWriter, r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{}`))
			h.updateTask(w, r)
		}, want: 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("PATCH", "/", nil)
			r.SetPathValue("id", tc.pathValue)
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetProjectAndTaskMapDatabaseErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at\s+FROM projects WHERE id`).WithArgs(int64(111)).WillReturnError(errors.New("read failed"))
	r := httptest.NewRequest("GET", "/api/projects/111", nil)
	r.SetPathValue("id", "111")
	w := httptest.NewRecorder()
	h.getProject(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("project read status=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, created_by, created_at\s+FROM tasks WHERE id`).WithArgs(int64(112)).WillReturnError(errors.New("read failed"))
	r = httptest.NewRequest("GET", "/api/tasks/112", nil)
	r.SetPathValue("id", "112")
	w = httptest.NewRecorder()
	h.getTask(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("task read status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskListCreateAndUpdateWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE id = \$1\)`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status, implementation_status,\s+qa_status, pr_status, checks_status, created_by, created_at\s+FROM tasks WHERE project_id`).WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}).AddRow(8, 3, "Task", "Description", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 5, now))
	r := httptest.NewRequest("GET", "/api/projects/3/tasks", nil)
	r.SetPathValue("projectID", "3")
	w := httptest.NewRecorder()
	h.listTasks(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Task") {
		t.Fatalf("listTasks=%d %s", w.Code, w.Body.String())
	}

	token := "create-task-token"
	r = httptest.NewRequest("POST", "/api/projects/3/tasks", strings.NewReader(`{"title":"Created","description":"Details"}`))
	r.SetPathValue("projectID", "3")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	tokenHash := sha256.Sum256([]byte(token))
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(5, "owner", "Owner", "Admin"))
	mock.ExpectQuery(`INSERT INTO tasks \(project_id, title, description, created_by\)`).WithArgs(int64(3), "Created", "Details", int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}).AddRow(9, 3, "Created", "Details", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 5, now))
	w = httptest.NewRecorder()
	h.createTask(w, r)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "Created") {
		t.Fatalf("createTask=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectAndTaskUpdatesWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	projectColumns := []string{"id", "name", "description", "requirements", "workspace_path", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at\s+FROM projects WHERE id`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(2, "p", "old", "old req", "/tmp/p", 5, now))
	mock.ExpectQuery(`UPDATE projects SET description = \$2, requirements = \$3`).WithArgs(int64(2), "new", "old req").WillReturnRows(sqlmock.NewRows(projectColumns).AddRow(2, "p", "new", "old req", "/tmp/p", 5, now))
	r := httptest.NewRequest("PATCH", "/api/projects/2", strings.NewReader(`{"description":" new "}`))
	r.SetPathValue("id", "2")
	w := httptest.NewRecorder()
	h.updateProject(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"description":"new"`) {
		t.Fatalf("updateProject=%d %s", w.Code, w.Body.String())
	}
	taskColumns := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, created_by, created_at\s+FROM tasks WHERE id`).WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows(taskColumns).AddRow(8, 2, "old", "desc", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 5, now))
	mock.ExpectQuery(`UPDATE tasks SET title = \$2, description = \$3`).WithArgs(int64(8), "new title", "desc").WillReturnRows(sqlmock.NewRows(taskColumns).AddRow(8, 2, "new title", "desc", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 5, now))
	r = httptest.NewRequest("PATCH", "/api/tasks/8", strings.NewReader(`{"title":" new title "}`))
	r.SetPathValue("id", "8")
	w = httptest.NewRecorder()
	h.updateTask(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "new title") {
		t.Fatalf("updateTask=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectAndTaskPatchErrorBranchesWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	projectCols := []string{"id", "name", "description", "requirements", "workspace_path", "created_by", "created_at"}
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	projectRead := `SELECT id, name, description, requirements, workspace_path, created_by, created_at\s+FROM projects WHERE id`
	taskRead := `SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, created_by, created_at\s+FROM tasks WHERE id`
	callProject := func(id, body string, want int) {
		r := httptest.NewRequest("PATCH", "/api/projects/"+id, strings.NewReader(body))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.updateProject(w, r)
		if w.Code != want {
			t.Fatalf("project patch id=%s status=%d want=%d body=%s", id, w.Code, want, w.Body.String())
		}
	}
	callTask := func(id, body string, want int) {
		r := httptest.NewRequest("PATCH", "/api/tasks/"+id, strings.NewReader(body))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.updateTask(w, r)
		if w.Code != want {
			t.Fatalf("task patch id=%s status=%d want=%d body=%s", id, w.Code, want, w.Body.String())
		}
	}
	callProject("1", `{}`, 400)
	mock.ExpectQuery(projectRead).WithArgs(int64(2)).WillReturnError(errors.New("read failed"))
	callProject("2", `{"description":"x"}`, 500)
	mock.ExpectQuery(projectRead).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows(projectCols).AddRow(3, "demo", "old", "req", "/tmp/demo", 1, now))
	mock.ExpectQuery(`UPDATE projects SET description = \$2`).WithArgs(int64(3), "new", "req").WillReturnRows(sqlmock.NewRows(projectCols))
	callProject("3", `{"description":"new"}`, 404)
	mock.ExpectQuery(projectRead).WithArgs(int64(4)).WillReturnRows(sqlmock.NewRows(projectCols).AddRow(4, "demo", "old", "req", "/tmp/demo", 1, now))
	mock.ExpectQuery(`UPDATE projects SET description = \$2`).WithArgs(int64(4), "new", "req").WillReturnError(errors.New("write failed"))
	callProject("4", `{"description":"new"}`, 500)
	callTask("5", `{}`, 400)
	mock.ExpectQuery(taskRead).WithArgs(int64(6)).WillReturnRows(sqlmock.NewRows(taskCols))
	callTask("6", `{"title":"new"}`, 404)
	mock.ExpectQuery(taskRead).WithArgs(int64(7)).WillReturnError(errors.New("read failed"))
	callTask("7", `{"title":"new"}`, 500)
	mock.ExpectQuery(taskRead).WithArgs(int64(8)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(8, 1, "old", "desc", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 1, now))
	mock.ExpectQuery(`UPDATE tasks SET title = \$2`).WithArgs(int64(8), "new", "desc").WillReturnRows(sqlmock.NewRows(taskCols))
	callTask("8", `{"title":"new"}`, 404)
	mock.ExpectQuery(taskRead).WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(9, 1, "old", "desc", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 1, now))
	mock.ExpectQuery(`UPDATE tasks SET title = \$2`).WithArgs(int64(9), "new", "desc").WillReturnError(errors.New("write failed"))
	callTask("9", `{"title":"new"}`, 500)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionRecordsInvalidTransitionWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	token := "action-token"
	hash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(4, "po.user", "Product Owner", "PO"))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(6)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}).AddRow(6, 2, "Ready", "", "Pronta", "approved", "not_started", "not_started", "not_created", "unknown", "", 4, now, 0, "/tmp/project"))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(6), int64(4), "PO", "attempt", "mark_ready", "Pronta", "Pronta", "", "action is not valid for the current task state").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/6/actions", strings.NewReader(`{"action":"mark_ready"}`))
	r.SetPathValue("id", "6")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "workflow gate") {
		t.Fatalf("applyTaskAction=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionBlocksTaskAndRecordsCommentWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	token := "block-task-token"
	hash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(4, "admin", "Administrator", "Admin"))
	mock.ExpectBegin()
	currentCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(6)).WillReturnRows(sqlmock.NewRows(currentCols).AddRow(6, 2, "Task", "", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", "", 4, now, 0, "/tmp/project"))
	updateCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(6), "Bloqueada", "not_started", "not_started", "not_started", "not_created", "unknown", "Aberta").WillReturnRows(sqlmock.NewRows(updateCols).AddRow(6, 2, "Task", "", "Bloqueada", "not_started", "not_started", "not_started", "not_created", "unknown", 4, now))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(6), int64(4), "Admin", "transition", "block", "Aberta", "Bloqueada", "blocked by dependency", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(6), int64(4), "Admin", "comment", "block", "Aberta", "Bloqueada", "blocked by dependency", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/6/actions", strings.NewReader(`{"action":"block","comment":"blocked by dependency"}`))
	r.SetPathValue("id", "6")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Bloqueada") {
		t.Fatalf("applyTaskAction=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionApprovesQAWhenEnvironmentIsHealthy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	token := "qa-approve-token"
	hash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "qa.user", "QA User", "QA"))
	mock.ExpectBegin()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(15)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(15, 3, "QA task", "", "Em QA", "approved", "succeeded", "pending", "not_created", "unknown", "", 7, now, 0, "/tmp/project"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM project_qa_environments`).WithArgs(int64(3), int64(15)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	updatedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(15), workflow.StateWaitingPR, workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnRows(sqlmock.NewRows(updatedCols).AddRow(15, 3, "QA task", "", "Aguardando PR", "approved", "succeeded", "approved", "not_created", "unknown", 7, now))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(15), int64(7), "QA", "approval", string(workflow.ActionApproveQA), "Em QA", string(workflow.StateWaitingPR), "QA aprovado", "smoke test passed").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(15), int64(7), "QA", "comment", string(workflow.ActionApproveQA), "Em QA", string(workflow.StateWaitingPR), "QA aprovado", "").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/15/actions", strings.NewReader(`{"action":"approve_qa","comment":"QA aprovado","evidence":"smoke test passed"}`))
	r.SetPathValue("id", "15")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.applyTaskAction(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), string(workflow.StateWaitingPR)) {
		t.Fatalf("approve QA=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyTaskActionCoversQAReviewAndUnblockTransitions(t *testing.T) {
	for _, scenario := range []struct {
		name, role, token, action, comment, evidence, state, blocked string
		updated                                                      workflow.State
		qaStatus                                                     string
		wantCode                                                     int
	}{
		{name: "start QA", role: "QA", token: "qa-start-token", action: string(workflow.ActionStartQA), state: string(workflow.StateWaitingQA), updated: workflow.StateInQA, qaStatus: workflow.StatusPending, wantCode: 200},
		{name: "reject QA with evidence", role: "QA", token: "qa-reject-token", action: "reject_qa", comment: "needs correction", evidence: "error screenshot", state: string(workflow.StateInQA), updated: workflow.StateCorrectionNeeded, qaStatus: workflow.StatusRejected, wantCode: 200},
		{name: "unblock task", role: "Admin", token: "unblock-token", action: "unblock", state: string(workflow.StateBlocked), blocked: string(workflow.StateReady), updated: workflow.StateReady, qaStatus: workflow.StatusNotStarted, wantCode: 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			h := &Handler{db: db}
			hash := sha256.Sum256([]byte(scenario.token))
			now := time.Now()
			mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(7, "actor", "Actor", scenario.role))
			mock.ExpectBegin()
			taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at", "pr_number", "workspace_path"}
			prStatus, checks := workflow.StatusNotCreated, workflow.StatusUnknown
			mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state`).WithArgs(int64(38)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(38, 4, "Task", "", scenario.state, "approved", "succeeded", map[string]string{"start QA": workflow.StatusPending, "reject QA with evidence": workflow.StatusPending, "unblock task": workflow.StatusNotStarted}[scenario.name], prStatus, checks, scenario.blocked, 7, now, 0, "/tmp/project"))
			if scenario.action == string(workflow.ActionStartQA) {
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM project_qa_environments`).WithArgs(int64(4), int64(38)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			}
			updatedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
			mock.ExpectQuery(`UPDATE tasks SET state = \$2, specification_status = \$3`).WithArgs(int64(38), scenario.updated, workflow.StatusApproved, workflow.StatusSucceeded, scenario.qaStatus, prStatus, checks, "").WillReturnRows(sqlmock.NewRows(updatedCols).AddRow(38, 4, "Task", "", scenario.updated, "approved", "succeeded", scenario.qaStatus, prStatus, checks, 7, now))
			eventTypeValue := "transition"
			if scenario.action == "reject_qa" {
				eventTypeValue = "approval"
			}
			mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(38), int64(7), scenario.role, eventTypeValue, scenario.action, scenario.state, string(scenario.updated), scenario.comment, scenario.evidence).WillReturnResult(sqlmock.NewResult(1, 1))
			if scenario.comment != "" {
				mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(38), int64(7), scenario.role, "comment", scenario.action, scenario.state, string(scenario.updated), scenario.comment, "").WillReturnResult(sqlmock.NewResult(2, 1))
			}
			mock.ExpectCommit()
			body := fmt.Sprintf(`{"action":%q,"comment":%q,"evidence":%q}`, scenario.action, scenario.comment, scenario.evidence)
			r := httptest.NewRequest("POST", "/api/tasks/38/actions", strings.NewReader(body))
			r.SetPathValue("id", "38")
			r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: scenario.token})
			w := httptest.NewRecorder()
			h.applyTaskAction(w, r)
			if w.Code != scenario.wantCode || !strings.Contains(w.Body.String(), string(scenario.updated)) {
				t.Fatalf("transition=%d body=%s", w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentRunAndEventPersistenceHelpersWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt\),0\)\+1 FROM agent_runs WHERE task_id=\$1 AND phase=\$2`).WithArgs(int64(6), "specification").WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(2))
	mock.ExpectQuery(`INSERT INTO agent_runs\(task_id,phase,attempt,status\)`).WithArgs(int64(6), "specification", 2).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(33))
	id, attempt, err := createAgentRun(ctx, tx, 6, "specification")
	if err != nil || id != 33 || attempt != 2 {
		t.Fatalf("createAgentRun=(%d,%d,%v)", id, attempt, err)
	}
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(6), int64(4), "Admin", "comment", "comment", "Aberta", "Aberta", "note", "").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := appendTaskEvent(ctx, tx, 6, 4, "Admin", "comment", "comment", "Aberta", "Aberta", " note ", ""); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(33), "succeeded", "complete", sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`["artifact"]`)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := completeAgentRun(ctx, tx, 33, "succeeded", "complete", opencode.Result{Stdout: "done"}, []string{"artifact"}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(33), "failed", "retry", sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`null`)).WillReturnResult(sqlmock.NewResult(0, 1))
	(&Handler{db: db}).updateAgentRun(ctx, 33, "failed", "retry", opencode.Result{}, nil)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistCreatedPullRequestAndSystemEventWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id,project_id,title,description,state,specification_status,implementation_status,qa_status,pr_status,checks_status,blocked_from_state,created_by,created_at FROM tasks WHERE id=\$1 FOR UPDATE`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(5, 2, "Feature", "desc", "Aguardando PR", "approved", "succeeded", "approved", "not_created", "unknown", "", 4, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2,pr_status=\$3,pr_number=\$4`).WithArgs(int64(5), workflow.StatePROpen, workflow.StatusOpen, 42, "https://github.com/acme/demo/pull/42", "fabrica/task-5").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(5), int64(4), "system", "transition", string(workflow.ActionCreatePR), string(workflow.StateWaitingPR), string(workflow.StatePROpen), "", "https://github.com/acme/demo/pull/42").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := (&Handler{db: db}).persistCreatedPullRequest(ctx, 5, 4, githubcli.PullRequest{Number: 42, URL: "https://github.com/acme/demo/pull/42"}, "fabrica/task-5"); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(5), int64(4), "system", "failure", "qa_environment_start_failed", "Aguardando QA", "Aguardando QA", "", "startup failed").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	if err := (&Handler{db: db}).recordSystemTaskEvent(ctx, 5, 4, "failure", "qa_environment_start_failed", string(workflow.StateWaitingQA), string(workflow.StateWaitingQA), "startup failed"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitSystemTaskActionAppliesAndRecordsTransition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows(cols).AddRow(12, 3, "Spec", "", "Especificando", "pending", "not_started", "not_started", "not_created", "unknown", "", 8, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(12), workflow.StateWaitingSpecApproval, workflow.StatusPending, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(12), int64(8), "system", "transition", string(workflow.ActionSpecificationReady), "Especificando", string(workflow.StateWaitingSpecApproval), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := (&Handler{db: db}).commitSystemTaskAction(context.Background(), 12, 8, workflow.ActionSpecificationReady, "", "proposal.md"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStopQAEnvironmentAndReadLogsWithMocks(t *testing.T) {
	root, binary := projectComposeFixture(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, compose: environments.ComposeManager{DockerPath: binary}, ports: environments.NewPortAllocator(db)}
	mock.ExpectQuery(`SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(20)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_path"}).AddRow(6, root))
	mock.ExpectExec(`UPDATE qa_port_leases SET status='released'`).WithArgs(int64(6)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE project_qa_environments SET status='stopped'`).WithArgs(int64(6)).WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/api/tasks/20/environment/stop", nil)
	r.SetPathValue("id", "20")
	w := httptest.NewRecorder()
	h.stopQAEnvironment(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "stopped") {
		t.Fatalf("stop QA=%d %s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(20)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_path"}).AddRow(6, root))
	r = httptest.NewRequest("GET", "/api/tasks/20/environment/logs", nil)
	r.SetPathValue("id", "20")
	w = httptest.NewRecorder()
	h.qaEnvironmentLogs(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "logs from service") {
		t.Fatalf("QA logs=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStopQAEnvironmentMapsMissingTaskAndDatabaseErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	query := `SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`
	for _, tc := range []struct {
		id     string
		result func()
	}{
		{id: "95", result: func() {
			mock.ExpectQuery(query).WithArgs(int64(95)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_path"}))
		}},
		{id: "96", result: func() {
			mock.ExpectQuery(query).WithArgs(int64(96)).WillReturnError(errors.New("database unavailable"))
		}},
	} {
		tc.result()
		r := httptest.NewRequest("POST", "/api/tasks/"+tc.id+"/environment/stop", nil)
		r.SetPathValue("id", tc.id)
		w := httptest.NewRecorder()
		h.stopQAEnvironment(w, r)
		want := 404
		if tc.id == "96" {
			want = 500
		}
		if w.Code != want {
			t.Fatalf("id=%s status=%d want=%d body=%s", tc.id, w.Code, want, w.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQAEnvironmentLogsMapsMissingTaskAndDatabaseErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	query := `SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`
	mock.ExpectQuery(query).WithArgs(int64(104)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_path"}))
	r := httptest.NewRequest("GET", "/api/tasks/104/environment/logs", nil)
	r.SetPathValue("id", "104")
	w := httptest.NewRecorder()
	h.qaEnvironmentLogs(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing task status=%d", w.Code)
	}
	mock.ExpectQuery(query).WithArgs(int64(105)).WillReturnError(errors.New("database unavailable"))
	r = httptest.NewRequest("GET", "/api/tasks/105/environment/logs", nil)
	r.SetPathValue("id", "105")
	w = httptest.NewRecorder()
	h.qaEnvironmentLogs(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("database error status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStopQAEnvironmentReportsComposeCommandFailure(t *testing.T) {
	root, binary := projectComposeFixture(t)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *' stop') echo 'daemon unavailable' >&2; exit 1 ;;\nesac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(97)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "workspace_path"}).AddRow(41, root))
	h := &Handler{db: db, compose: environments.ComposeManager{DockerPath: binary}}
	r := httptest.NewRequest("POST", "/api/tasks/97/environment/stop", nil)
	r.SetPathValue("id", "97")
	w := httptest.NewRecorder()
	h.stopQAEnvironment(w, r)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func projectComposeFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".fabrica"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, environments.DescriptorPath), []byte(`{"version":1,"composeFile":"compose.yaml","qaService":"web","containerPort":80}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "fake-docker")
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'logs --no-color --tail 200'*) echo 'logs from service' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return root, binary
}

func TestRefreshTaskChecksUpdatesWorkflowWithMockDB(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho '[{\"name\":\"CI\",\"state\":\"SUCCESS\",\"link\":\"https://github.com/acme/demo/actions\",\"workflow\":\"CI\"}]'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	now := time.Now()
	mock.ExpectQuery(`SELECT p.workspace_path,t.pr_number,t.created_by FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(22)).WillReturnRows(sqlmock.NewRows([]string{"workspace_path", "pr_number", "created_by"}).AddRow(root, 42, 9))
	mock.ExpectBegin()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(22)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(22, 5, "Feature", "", "PR aberto", "approved", "succeeded", "approved", "open", "pending", "", 9, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(22), workflow.StateReadyForMerge, workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusOpen, workflow.StatusPassed, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(22), int64(9), "system", "transition", string(workflow.ActionChecksPassed), "PR aberto", string(workflow.StateReadyForMerge), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/22/pull-request/checks", nil)
	r.SetPathValue("id", "22")
	w := httptest.NewRecorder()
	h.refreshTaskChecks(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "passed") {
		t.Fatalf("refresh checks=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateImplementationPersistsFailureWhenApprovedSpecIsMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, t.specification_status`).WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(17, 3, "Implement", "", "Em desenvolvimento", "approved", "running", "not_started", "not_created", "unknown", 8, now, "/tmp/project"))
	mock.ExpectQuery(`SELECT artifact_paths FROM task_specifications WHERE task_id=\$1 AND status='approved'`).WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt\),0\)\+1 FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(2))
	summary := "approved specification artifacts are unavailable"
	mock.ExpectExec(`INSERT INTO task_implementations`).WithArgs(int64(17), 2, "failed", []byte(`null`), summary, int64(44)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(44), "failed", summary, sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`null`)).WillReturnResult(sqlmock.NewResult(0, 1))
	lockedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(17)).WillReturnRows(sqlmock.NewRows(lockedCols).AddRow(17, 3, "Implement", "", "Em desenvolvimento", "approved", "running", "not_started", "not_created", "unknown", "", 8, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(17), workflow.StateInDevelopment, workflow.StatusApproved, workflow.StatusFailed, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(17), int64(8), "system", "transition", string(workflow.ActionImplementationError), string(workflow.StateInDevelopment), string(workflow.StateInDevelopment), summary, "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	h.generateImplementation(17, 44)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProjectUsesAuthenticatedUserAndWorkspace(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	h := &Handler{db: db, workspaceRoot: root}
	name := "project-under-test"
	token := "project-owner-token"
	tokenHash := sha256.Sum256([]byte(token))
	now := time.Now()
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE name = \$1\)`).WithArgs(name).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(17, "po.user", "Product Owner", "PO"))
	mock.ExpectQuery(`INSERT INTO projects \(name, description, requirements, workspace_path, created_by\)`).WithArgs(name, "description", "requirements", filepath.Join(root, name), int64(17)).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "requirements", "workspace_path", "created_by", "created_at"}).AddRow(25, name, "description", "requirements", filepath.Join(root, name), 17, now))
	r := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":" project-under-test ","description":" description ","requirements":" requirements "}`))
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w := httptest.NewRecorder()
	h.createProject(w, r)
	if w.Code != 201 || !strings.Contains(w.Body.String(), name) {
		t.Fatalf("createProject=%d %s", w.Code, w.Body.String())
	}
	if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.IsDir() {
		t.Fatalf("workspace was not created: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProjectRejectsInvalidDuplicateAndUnauthenticatedRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	h := &Handler{db: db, workspaceRoot: root}
	r := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":"../escape"}`))
	w := httptest.NewRecorder()
	h.createProject(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid name status=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE name = \$1\)`).WithArgs("exists").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	r = httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":"exists"}`))
	w = httptest.NewRecorder()
	h.createProject(w, r)
	if w.Code != 409 {
		t.Fatalf("duplicate name status=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE name = \$1\)`).WithArgs("unauthenticated").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	r = httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":"unauthenticated"}`))
	w = httptest.NewRecorder()
	h.createProject(w, r)
	if w.Code != 401 {
		t.Fatalf("unauthenticated status=%d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "unauthenticated")); !os.IsNotExist(err) {
		t.Fatalf("unauthenticated workspace should be removed, stat err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateProjectCleansWorkspaceAfterAuthenticationAndDatabaseFailures(t *testing.T) {
	t.Run("authentication missing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		root := t.TempDir()
		h := &Handler{db: db, workspaceRoot: root}
		mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE name = \$1\)`).WithArgs("orphan-project").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		r := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":"orphan-project"}`))
		w := httptest.NewRecorder()
		h.createProject(w, r)
		if w.Code != 401 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if _, err := os.Stat(filepath.Join(root, "orphan-project")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unauthenticated request left workspace behind: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("project insert failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		root := t.TempDir()
		h := &Handler{db: db, workspaceRoot: root}
		name := "failed-insert-project"
		token := "project-insert-failure"
		hash := sha256.Sum256([]byte(token))
		mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE name = \$1\)`).WithArgs(name).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(17, "po.user", "PO", "PO"))
		mock.ExpectQuery(`INSERT INTO projects \(name, description, requirements, workspace_path, created_by\)`).WillReturnError(errors.New("insert failed"))
		r := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"name":"failed-insert-project"}`))
		r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
		w := httptest.NewRecorder()
		h.createProject(w, r)
		if w.Code != 409 {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed insert left workspace behind: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestGetTaskReturnsNotFoundWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(100)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}))
	r := httptest.NewRequest("GET", "/api/tasks/100", nil)
	r.SetPathValue("id", "100")
	w := httptest.NewRecorder()
	h.getTask(w, r)
	if w.Code != 404 {
		t.Fatalf("getTask status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTaskRejectsInvalidUnauthorizedAndMissingProjectRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	r := httptest.NewRequest("POST", "/api/projects/bad/tasks", strings.NewReader(`{"title":"Task"}`))
	r.SetPathValue("projectID", "bad")
	w := httptest.NewRecorder()
	h.createTask(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid project id=%d", w.Code)
	}
	r = httptest.NewRequest("POST", "/api/projects/2/tasks", strings.NewReader(`{"title":""}`))
	r.SetPathValue("projectID", "2")
	w = httptest.NewRecorder()
	h.createTask(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid task body=%d", w.Code)
	}
	r = httptest.NewRequest("POST", "/api/projects/2/tasks", strings.NewReader(`{"title":"Valid task"}`))
	r.SetPathValue("projectID", "2")
	w = httptest.NewRecorder()
	h.createTask(w, r)
	if w.Code != 401 {
		t.Fatalf("anonymous task creation=%d", w.Code)
	}
	token := "task-create-token"
	tokenHash := sha256.Sum256([]byte(token))
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(tokenHash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(4, "owner", "Owner", "PO"))
	mock.ExpectQuery(`INSERT INTO tasks \(project_id, title, description, created_by\)`).WithArgs(int64(2), "Valid task", "", int64(4)).WillReturnError(errors.New("foreign key violation"))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE id = \$1\)`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	r = httptest.NewRequest("POST", "/api/projects/2/tasks", strings.NewReader(`{"title":"Valid task"}`))
	r.SetPathValue("projectID", "2")
	r.AddCookie(&http.Cookie{Name: "fabrica_session", Value: token})
	w = httptest.NewRecorder()
	h.createTask(w, r)
	if w.Code != 404 {
		t.Fatalf("missing project task status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectAndTaskHandlersRejectMissingFieldsAndOverlongValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	r := httptest.NewRequest("PATCH", "/api/projects/44", strings.NewReader(`{}`))
	r.SetPathValue("id", "44")
	w := httptest.NewRecorder()
	h.updateProject(w, r)
	if w.Code != 400 {
		t.Fatalf("empty project patch=%d", w.Code)
	}
	projectCols := []string{"id", "name", "description", "requirements", "workspace_path", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, name, description, requirements, workspace_path, created_by, created_at\s+FROM projects WHERE id`).WithArgs(int64(44)).WillReturnRows(sqlmock.NewRows(projectCols).AddRow(44, "demo", "", "", "/tmp/demo", 3, now))
	r = httptest.NewRequest("PATCH", "/api/projects/44", strings.NewReader(`{"description":"`+strings.Repeat("x", 10001)+`"}`))
	r.SetPathValue("id", "44")
	w = httptest.NewRecorder()
	h.updateProject(w, r)
	if w.Code != 400 {
		t.Fatalf("overlong project patch=%d", w.Code)
	}
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, created_by, created_at\s+FROM tasks WHERE id`).WithArgs(int64(44)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(44, 3, "old", "", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 3, now))
	r = httptest.NewRequest("PATCH", "/api/tasks/44", strings.NewReader(`{"title":"   "}`))
	r.SetPathValue("id", "44")
	w = httptest.NewRecorder()
	h.updateTask(w, r)
	if w.Code != 400 {
		t.Fatalf("blank task title=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListTasksReturnsNotFoundForMissingProjectAndGetTaskReturnsRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM projects WHERE id = \$1\)`).WithArgs(int64(90)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	r := httptest.NewRequest("GET", "/api/projects/90/tasks", nil)
	r.SetPathValue("projectID", "90")
	w := httptest.NewRecorder()
	h.listTasks(w, r)
	if w.Code != 404 {
		t.Fatalf("missing project task list=%d", w.Code)
	}
	now := time.Now()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, created_by, created_at\s+FROM tasks WHERE id`).WithArgs(int64(90)).WillReturnRows(sqlmock.NewRows(cols).AddRow(90, 8, "Found", "description", "Aberta", "not_started", "not_started", "not_started", "not_created", "unknown", 3, now))
	r = httptest.NewRequest("GET", "/api/tasks/90", nil)
	r.SetPathValue("id", "90")
	w = httptest.NewRecorder()
	h.getTask(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Found") {
		t.Fatalf("get task=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQAEnvironmentSmallHelpersWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	ctx := context.Background()
	mock.ExpectExec(`INSERT INTO project_qa_environments`).WithArgs(int64(2), int64(3), "starting", "", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := h.saveQAEnvironment(ctx, 2, 3, "starting", "", nil); err != nil {
		t.Fatal(err)
	}
	port := 41000
	mock.ExpectExec(`INSERT INTO project_qa_environments`).WithArgs(int64(2), int64(3), "healthy", "http://127.0.0.1:41000", port).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := h.saveQAEnvironment(ctx, 2, 3, "healthy", "http://127.0.0.1:41000", &port); err != nil {
		t.Fatal(err)
	}
	if got := timePointer(time.Unix(1, 0).UTC()); got == nil || !got.Equal(time.Unix(1, 0).UTC()) {
		t.Fatalf("timePointer=%v", got)
	}
	r := httptest.NewRequest("POST", "/api/tasks/bad/environment/restart", nil)
	r.SetPathValue("id", "bad")
	w := httptest.NewRecorder()
	h.restartQAEnvironment(w, r)
	if w.Code != 400 {
		t.Fatalf("restart invalid task status=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(2, string(workflow.StateWaitingQA), "/tmp/project"))
	if _, err := h.startQAEnvironmentForTask(ctx, 3, "pause"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported operation error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRestartQAEnvironmentUsesExistingPortLeaseAndHealthCheck(t *testing.T) {
	root, binary := projectComposeFixture(t)
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'ps --format json'*) echo '{\"ID\":\"id\",\"Name\":\"web-1\",\"Project\":\"fabrica-2\",\"Service\":\"web\",\"State\":\"running\",\"Health\":\"healthy\",\"ExitCode\":0}' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, compose: environments.ComposeManager{DockerPath: binary}, ports: environments.NewPortAllocator(db)}
	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(33)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(2, string(workflow.StateInQA), root))
	mock.ExpectExec(`INSERT INTO project_qa_environments`).WithArgs(int64(2), int64(33), "starting", "", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(846029117)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}).AddRow(11, 2, "127.0.0.1", port, "active"))
	mock.ExpectCommit()
	mock.ExpectExec(`INSERT INTO project_qa_environments`).WithArgs(int64(2), int64(33), "healthy", server.URL+"/", port).WillReturnResult(sqlmock.NewResult(0, 1))
	environment, err := h.startQAEnvironmentForTask(context.Background(), 33, "restart")
	if err != nil || environment.Status != "healthy" || environment.Port != port || environment.URL != server.URL+"/" {
		t.Fatalf("restart environment=%#v err=%v", environment, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRestartQAEnvironmentStopsAndReleasesPortWhenHealthCheckFails(t *testing.T) {
	root, binary := projectComposeFixture(t)
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'ps --format json'*) echo '{\"ID\":\"id\",\"Name\":\"web-1\",\"Project\":\"fabrica-2\",\"Service\":\"web\",\"State\":\"running\",\"Health\":\"healthy\",\"ExitCode\":0}' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const projectID, taskID, port = int64(2), int64(33), 41033
	h := &Handler{
		db:      db,
		compose: environments.ComposeManager{DockerPath: binary},
		ports:   environments.NewPortAllocator(db),
		qaHealthCheck: func(context.Context, int, string) (string, error) {
			return "", errors.New("health endpoint unavailable")
		},
	}
	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(projectID, string(workflow.StateInQA), root))
	query := `INSERT INTO project_qa_environments\(project_id,task_id,status,url,port,health_checked_at\)`
	mock.ExpectExec(query).WithArgs(projectID, taskID, "starting", "", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(846029117)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(projectID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}).AddRow(11, projectID, "127.0.0.1", port, "active"))
	mock.ExpectCommit()
	mock.ExpectExec(`UPDATE qa_port_leases SET status='released', released_at=now\(\) WHERE project_id=\$1 AND status='active'`).WithArgs(projectID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(query).WithArgs(projectID, taskID, "unhealthy", "", port).WillReturnResult(sqlmock.NewResult(0, 1))

	environment, err := h.startQAEnvironmentForTask(context.Background(), taskID, "restart")
	if err == nil || !strings.Contains(err.Error(), "health endpoint unavailable") || environment.Status != "unhealthy" || environment.Port != port {
		t.Fatalf("restart environment=%#v err=%v", environment, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRestartQAEnvironmentReleasesPortWhenHealthyStateCannotBeSaved(t *testing.T) {
	root, binary := projectComposeFixture(t)
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'ps --format json'*) echo '{\"ID\":\"id\",\"Name\":\"web-1\",\"Project\":\"fabrica-2\",\"Service\":\"web\",\"State\":\"running\",\"Health\":\"healthy\",\"ExitCode\":0}' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const projectID, taskID, port = int64(2), int64(34), 41034
	h := &Handler{
		db:      db,
		compose: environments.ComposeManager{DockerPath: binary},
		ports:   environments.NewPortAllocator(db),
		qaHealthCheck: func(context.Context, int, string) (string, error) {
			return "http://127.0.0.1:41034/", nil
		},
	}
	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(taskID).
		WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(projectID, string(workflow.StateInQA), root))
	query := `INSERT INTO project_qa_environments\(project_id,task_id,status,url,port,health_checked_at\)`
	mock.ExpectExec(query).WithArgs(projectID, taskID, "starting", "", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(846029117)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(projectID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}).AddRow(12, projectID, "127.0.0.1", port, "active"))
	mock.ExpectCommit()
	mock.ExpectExec(query).WithArgs(projectID, taskID, "healthy", "http://127.0.0.1:41034/", port).WillReturnError(errors.New("database unavailable"))
	mock.ExpectExec(`UPDATE qa_port_leases SET status='released', released_at=now\(\) WHERE project_id=\$1 AND status='active'`).WithArgs(projectID).WillReturnResult(sqlmock.NewResult(0, 1))

	if _, err := h.startQAEnvironmentForTask(context.Background(), taskID, "restart"); err == nil || !strings.Contains(err.Error(), "could not persist QA environment") {
		t.Fatalf("restart error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTaskPullRequestUsesFakeGitHubAndMockDB(t *testing.T) {
	projectRoot := t.TempDir()
	bare := filepath.Join(t.TempDir(), "origin.git")
	cmd := exec.Command("git", "init", "--bare", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init bare origin: %v: %s", err, out)
	}
	runGitTest(t, projectRoot, "init", "-b", "main")
	runGitTest(t, projectRoot, "config", "user.name", "Factory test")
	runGitTest(t, projectRoot, "config", "user.email", "factory-test@example.invalid")
	if err := os.WriteFile(filepath.Join(projectRoot, "base.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, projectRoot, "add", "base.txt")
	runGitTest(t, projectRoot, "commit", "-m", "base")
	runGitTest(t, projectRoot, "remote", "add", "origin", bare)
	if err := os.WriteFile(filepath.Join(projectRoot, "implementation.txt"), []byte("implementation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(t.TempDir(), "fake-gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\ncase \"$1 $2\" in\n 'auth status') exit 0 ;;\n 'pr create') echo https://github.com/acme/demo/pull/31 ;;\nesac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: gh}}
	columns := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "pr_number", "pr_url", "pr_branch", "workspace_path"}
	now := time.Now()
	mock.ExpectQuery(`SELECT t.id,t.project_id,t.title,t.description,t.state`).WithArgs(int64(31)).WillReturnRows(sqlmock.NewRows(columns).AddRow(31, 2, "Add endpoint", "Description", "Aguardando PR", "approved", "succeeded", "approved", "not_created", "unknown", 7, now, nil, nil, nil, projectRoot))
	mock.ExpectQuery(`SELECT changed_files FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(31)).WillReturnRows(sqlmock.NewRows([]string{"changed_files"}).AddRow([]byte(`["implementation.txt"]`)))
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id,project_id,title,description,state,specification_status,implementation_status,qa_status,pr_status,checks_status,blocked_from_state,created_by,created_at FROM tasks WHERE id=\$1 FOR UPDATE`).WithArgs(int64(31)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(31, 2, "Add endpoint", "Description", "Aguardando PR", "approved", "succeeded", "approved", "not_created", "unknown", "", 7, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2,pr_status=\$3,pr_number=\$4`).WithArgs(int64(31), workflow.StatePROpen, workflow.StatusOpen, 31, "https://github.com/acme/demo/pull/31", "fabrica/task-31-add-endpoint").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(31), int64(7), "system", "transition", string(workflow.ActionCreatePR), string(workflow.StateWaitingPR), string(workflow.StatePROpen), "", "https://github.com/acme/demo/pull/31").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/31/pull-request", nil)
	r.SetPathValue("id", "31")
	w := httptest.NewRecorder()
	h.createTaskPullRequest(w, r)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "pull/31") {
		t.Fatalf("create PR=%d %s", w.Code, w.Body.String())
	}
	if got := runGitTest(t, projectRoot, "status", "--porcelain"); got != "" {
		t.Fatalf("working tree should be clean after PR commit: %s", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTaskPullRequestReturnsExistingPRAndRequiresSuccessfulImplementation(t *testing.T) {
	columns := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "pr_number", "pr_url", "pr_branch", "workspace_path"}
	now := time.Now()
	{
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		h := &Handler{db: db}
		mock.ExpectQuery(`SELECT t.id,t.project_id,t.title,t.description,t.state`).WithArgs(int64(31)).WillReturnRows(sqlmock.NewRows(columns).AddRow(31, 2, "Feature", "", string(workflow.StateWaitingPR), "approved", "succeeded", "approved", "open", "pending", 7, now, int64(31), "https://github.com/acme/demo/pull/31", "fabrica/task-31", "/tmp/demo"))
		r := httptest.NewRequest("POST", "/api/tasks/31/pull-request", nil)
		r.SetPathValue("id", "31")
		w := httptest.NewRecorder()
		h.createTaskPullRequest(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "pull/31") {
			t.Fatalf("existing PR=%d %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
	{
		root := t.TempDir()
		binary := filepath.Join(root, "fake-gh")
		if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
		mock.ExpectQuery(`SELECT t.id,t.project_id,t.title,t.description,t.state`).WithArgs(int64(32)).WillReturnRows(sqlmock.NewRows(columns).AddRow(32, 2, "Feature", "", "Aguardando PR", "approved", "succeeded", "approved", "not_created", "unknown", 7, now, nil, nil, nil, root))
		mock.ExpectQuery(`SELECT changed_files FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(32)).WillReturnRows(sqlmock.NewRows([]string{"changed_files"}))
		r := httptest.NewRequest("POST", "/api/tasks/32/pull-request", nil)
		r.SetPathValue("id", "32")
		w := httptest.NewRecorder()
		h.createTaskPullRequest(w, r)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "successful implementation files") {
			t.Fatalf("missing implementation=%d %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateTaskPullRequestRecordsUnauthenticatedGitHubFailure(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho 'private auth diagnostic' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	now := time.Now()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "pr_number", "pr_url", "pr_branch", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id,t.project_id,t.title,t.description,t.state`).WithArgs(int64(73)).WillReturnRows(sqlmock.NewRows(cols).AddRow(73, 8, "Feature", "description", string(workflow.StateWaitingPR), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusNotCreated, workflow.StatusUnknown, 4, now, nil, nil, nil, root))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(73), int64(4), "system", "failure", "create_pull_request", string(workflow.StateWaitingPR), string(workflow.StateWaitingPR), "", "GitHub CLI authentication failed").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r := httptest.NewRequest("POST", "/api/tasks/73/pull-request", nil)
	r.SetPathValue("id", "73")
	w := httptest.NewRecorder()
	h.createTaskPullRequest(w, r)
	if w.Code != 503 || strings.Contains(w.Body.String(), "private auth diagnostic") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTaskPullRequestRequiresSuccessfulImplementationArtifacts(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	now := time.Now()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "pr_number", "pr_url", "pr_branch", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id,t.project_id,t.title,t.description,t.state`).WithArgs(int64(98)).WillReturnRows(sqlmock.NewRows(cols).AddRow(98, 8, "Feature", "", string(workflow.StateWaitingPR), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusNotCreated, workflow.StatusUnknown, 4, now, nil, nil, nil, root))
	mock.ExpectQuery(`SELECT changed_files FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(98)).WillReturnRows(sqlmock.NewRows([]string{"changed_files"}))
	r := httptest.NewRequest("POST", "/api/tasks/98/pull-request", nil)
	r.SetPathValue("id", "98")
	w := httptest.NewRecorder()
	h.createTaskPullRequest(w, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "successful implementation files are unavailable") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTaskPullRequestRejectsMissingTaskAndInvalidFileReferences(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	now := time.Now()
	query := `SELECT t.id,t.project_id,t.title,t.description,t.state,t.specification_status,t.implementation_status,t.qa_status,t.pr_status,t.checks_status,t.created_by,t.created_at,t.pr_number,t.pr_url,t.pr_branch,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`
	mock.ExpectQuery(query).WithArgs(int64(102)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	r := httptest.NewRequest("POST", "/api/tasks/102/pull-request", nil)
	r.SetPathValue("id", "102")
	w := httptest.NewRecorder()
	h.createTaskPullRequest(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing task status=%d", w.Code)
	}
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "pr_number", "pr_url", "pr_branch", "workspace_path"}
	mock.ExpectQuery(query).WithArgs(int64(103)).WillReturnRows(sqlmock.NewRows(cols).AddRow(103, 8, "Feature", "", string(workflow.StateWaitingPR), workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, workflow.StatusNotCreated, workflow.StatusUnknown, 4, now, nil, nil, nil, root))
	mock.ExpectQuery(`SELECT changed_files FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(103)).WillReturnRows(sqlmock.NewRows([]string{"changed_files"}).AddRow([]byte("{")))
	r = httptest.NewRequest("POST", "/api/tasks/103/pull-request", nil)
	r.SetPathValue("id", "103")
	w = httptest.NewRecorder()
	h.createTaskPullRequest(w, r)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "implementation file references are invalid") {
		t.Fatalf("invalid files status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterRoutesAndGitHubStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mux := http.NewServeMux()
	RegisterRoutes(mux, db, t.TempDir())
	_ = mock
	for _, route := range []string{"GET /api/projects", "POST /api/projects", "POST /api/tasks/{id}/actions", "GET /api/tasks/{id}/environment", "POST /api/tasks/{id}/pull-request"} {
		if _, pattern := mux.Handler(httptest.NewRequest(strings.Fields(route)[0], strings.ReplaceAll(strings.Fields(route)[1], "{id}", "1"), nil)); pattern == "" {
			t.Errorf("route %q was not registered", route)
		}
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := &Handler{workspaceRoot: root, github: githubcli.Client{Binary: binary}}
	w := httptest.NewRecorder()
	h.githubStatus(w, httptest.NewRequest("GET", "/api/github/status", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"authenticated":true`) {
		t.Fatalf("githubStatus=%d %s", w.Code, w.Body.String())
	}
}

func TestPullRequestAndQAReadHandlersCoverMissingAndErrorStates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT COALESCE\(pr_number,0\),COALESCE\(pr_url,''\),COALESCE\(pr_branch,''\) FROM tasks WHERE id=\$1`).WithArgs(int64(90)).WillReturnRows(sqlmock.NewRows([]string{"number", "url", "branch"}))
	r := httptest.NewRequest("GET", "/api/tasks/90/pull-request", nil)
	r.SetPathValue("id", "90")
	w := httptest.NewRecorder()
	h.getTaskPullRequest(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing PR status=%d body=%s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,e.status,COALESCE`).WithArgs(int64(90)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "task_id", "status", "url", "port", "health_checked_at"}))
	r = httptest.NewRequest("GET", "/api/tasks/90/environment", nil)
	r.SetPathValue("id", "90")
	w = httptest.NewRecorder()
	h.getQAEnvironment(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"stopped"`) {
		t.Fatalf("missing QA env status=%d body=%s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,e.status,COALESCE`).WithArgs(int64(91)).WillReturnError(errors.New("database unavailable"))
	r = httptest.NewRequest("GET", "/api/tasks/91/environment", nil)
	r.SetPathValue("id", "91")
	w = httptest.NewRecorder()
	h.getQAEnvironment(w, r)
	if w.Code != 500 {
		t.Fatalf("QA env DB error status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetTaskPullRequestMapsDatabaseError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT COALESCE\(pr_number,0\),COALESCE\(pr_url,''\),COALESCE\(pr_branch,''\) FROM tasks WHERE id=\$1`).WithArgs(int64(106)).WillReturnError(errors.New("database unavailable"))
	r := httptest.NewRequest("GET", "/api/tasks/106/pull-request", nil)
	r.SetPathValue("id", "106")
	w := httptest.NewRecorder()
	h.getTaskPullRequest(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubStatusReportsAuthenticatedAndUnauthenticatedCLI(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	h := &Handler{workspaceRoot: root, github: githubcli.Client{Binary: binary}}
	for _, tc := range []struct {
		name, script  string
		authenticated bool
	}{
		{name: "unauthenticated", script: "#!/bin/sh\nexit 1\n", authenticated: false},
		{name: "authenticated", script: "#!/bin/sh\nexit 0\n", authenticated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(binary, []byte(tc.script), 0o700); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.githubStatus(w, httptest.NewRequest("GET", "/api/github/status", nil))
			want := `"authenticated":false`
			if tc.authenticated {
				want = `"authenticated":true`
			}
			if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRefreshTaskChecksHandlesMissingAndUnavailablePullRequests(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := &Handler{db: db, github: githubcli.Client{Binary: binary}}
	query := `SELECT p.workspace_path,t.pr_number,t.created_by FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`
	mock.ExpectQuery(query).WithArgs(int64(92)).WillReturnRows(sqlmock.NewRows([]string{"path", "number", "actor"}))
	r := httptest.NewRequest("POST", "/api/tasks/92/pull-request/checks", nil)
	r.SetPathValue("id", "92")
	w := httptest.NewRecorder()
	h.refreshTaskChecks(w, r)
	if w.Code != 404 {
		t.Fatalf("missing task status=%d", w.Code)
	}
	mock.ExpectQuery(query).WithArgs(int64(93)).WillReturnRows(sqlmock.NewRows([]string{"path", "number", "actor"}).AddRow(root, 0, 4))
	r = httptest.NewRequest("POST", "/api/tasks/93/pull-request/checks", nil)
	r.SetPathValue("id", "93")
	w = httptest.NewRecorder()
	h.refreshTaskChecks(w, r)
	if w.Code != 409 {
		t.Fatalf("missing PR status=%d", w.Code)
	}
	mock.ExpectQuery(query).WithArgs(int64(94)).WillReturnRows(sqlmock.NewRows([]string{"path", "number", "actor"}).AddRow(root, 44, 4))
	r = httptest.NewRequest("POST", "/api/tasks/94/pull-request/checks", nil)
	r.SetPathValue("id", "94")
	w = httptest.NewRecorder()
	h.refreshTaskChecks(w, r)
	if w.Code != 502 {
		t.Fatalf("gh checks failure status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskHistoryAndArtifactHandlersWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM tasks WHERE id = \$1\)`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT e.id, e.task_id, e.actor_id, COALESCE`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "actor_id", "actor_name", "actor_role", "event_type", "action", "from_state", "to_state", "comment", "reason", "occurred_at"}).AddRow(1, 5, 2, "Owner", "PO", "transition", "mark_ready", "Aberta", "Pronta", "ready", "", now))
	r := httptest.NewRequest("GET", "/api/tasks/5/events", nil)
	r.SetPathValue("id", "5")
	w := httptest.NewRecorder()
	h.listTaskEvents(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Owner") {
		t.Fatalf("listTaskEvents=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT id, task_id, round, status, artifact_paths, summary, created_at FROM task_specifications`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "round", "status", "artifact_paths", "summary", "created_at"}).AddRow(2, 5, 1, "generated", []byte(`["proposal.md"]`), "summary", now))
	r = httptest.NewRequest("GET", "/api/tasks/5/specifications", nil)
	r.SetPathValue("id", "5")
	w = httptest.NewRecorder()
	h.listTaskSpecifications(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "proposal.md") {
		t.Fatalf("listTaskSpecifications=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT id, task_id, attempt, status, changed_files, summary, created_at FROM task_implementations`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "attempt", "status", "changed_files", "summary", "created_at"}).AddRow(3, 5, 1, "succeeded", []byte(`["main.go"]`), "done", now))
	r = httptest.NewRequest("GET", "/api/tasks/5/implementations", nil)
	r.SetPathValue("id", "5")
	w = httptest.NewRecorder()
	h.listTaskImplementations(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "main.go") {
		t.Fatalf("listTaskImplementations=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT id, task_id, phase, attempt, status, summary, result_refs, started_at, finished_at FROM agent_runs`).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "status", "summary", "result_refs", "started_at", "finished_at"}).AddRow(4, 5, "specification", 1, "succeeded", "done", []byte(`["log"]`), now, now))
	r = httptest.NewRequest("GET", "/api/tasks/5/agent-runs", nil)
	r.SetPathValue("id", "5")
	w = httptest.NewRecorder()
	h.listAgentRuns(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "log") {
		t.Fatalf("listAgentRuns=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequestAndEnvironmentReadHandlersWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT COALESCE\(pr_number,0\),COALESCE\(pr_url,''\),COALESCE\(pr_branch,''\) FROM tasks WHERE id=\$1`).WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"number", "url", "branch"}).AddRow(42, "https://github.com/acme/demo/pull/42", "fabrica/task-11"))
	r := httptest.NewRequest("GET", "/api/tasks/11/pull-request", nil)
	r.SetPathValue("id", "11")
	w := httptest.NewRecorder()
	h.getTaskPullRequest(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "pull/42") {
		t.Fatalf("getTaskPullRequest=%d %s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,e.status`).WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "task_id", "status", "url", "port", "health_checked_at"}).AddRow(3, 11, "healthy", "http://127.0.0.1:32100", 32100, nil))
	r = httptest.NewRequest("GET", "/api/tasks/11/environment", nil)
	r.SetPathValue("id", "11")
	w = httptest.NewRecorder()
	h.getQAEnvironment(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "healthy") {
		t.Fatalf("getQAEnvironment=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSpecificationArtifactServesOnlyListedFiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "spec.md"), []byte("approved spec"), 0o600); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT s.artifact_paths, p.workspace_path`).WithArgs(int64(5), 1).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "workspace_path"}).AddRow([]byte(`["docs/spec.md"]`), root))
	r := httptest.NewRequest("GET", "/api/tasks/5/specifications/1/artifact?path=docs/spec.md", nil)
	r.SetPathValue("id", "5")
	r.SetPathValue("round", "1")
	w := httptest.NewRecorder()
	h.getTaskSpecificationArtifact(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "approved spec") {
		t.Fatalf("artifact=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT s.artifact_paths, p.workspace_path`).WithArgs(int64(5), 1).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "workspace_path"}).AddRow([]byte(`["docs/spec.md"]`), root))
	r = httptest.NewRequest("GET", "/api/tasks/5/specifications/1/artifact?path=docs/other.md", nil)
	r.SetPathValue("id", "5")
	r.SetPathValue("round", "1")
	w = httptest.NewRecorder()
	h.getTaskSpecificationArtifact(w, r)
	if w.Code != 404 {
		t.Fatalf("unlisted artifact status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetSpecificationArtifactRejectsCorruptEscapingAndOversizedFiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "docs", "link.md")); err != nil {
		t.Fatal(err)
	}
	largePath := filepath.Join(root, "large.md")
	if err := os.WriteFile(largePath, []byte(strings.Repeat("x", (256<<10)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/artifact?path="+path, nil)
		r.SetPathValue("id", "5")
		r.SetPathValue("round", "1")
		w := httptest.NewRecorder()
		h.getTaskSpecificationArtifact(w, r)
		return w
	}
	mock.ExpectQuery(`SELECT s.artifact_paths, p.workspace_path`).WithArgs(int64(5), 1).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "workspace_path"}).AddRow([]byte(`not-json`), root))
	if w := call("docs/link.md"); w.Code != 500 {
		t.Fatalf("invalid stored paths status=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT s.artifact_paths, p.workspace_path`).WithArgs(int64(5), 1).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "workspace_path"}).AddRow([]byte(`["docs/link.md"]`), root))
	if w := call("docs/link.md"); w.Code != 403 {
		t.Fatalf("escaping symlink status=%d body=%s", w.Code, w.Body.String())
	}
	mock.ExpectQuery(`SELECT s.artifact_paths, p.workspace_path`).WithArgs(int64(5), 1).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "workspace_path"}).AddRow([]byte(`["large.md"]`), root))
	if w := call("large.md"); w.Code != 413 {
		t.Fatalf("large artifact status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectHandlersHandleMissingRecordsWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	query := func(pattern string, args ...driver.Value) {
		mock.ExpectQuery(pattern).WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"unused"}))
	}
	call := func(method string, handler func(http.ResponseWriter, *http.Request), want int) {
		r := httptest.NewRequest("GET", "/api/tasks/1", nil)
		r.SetPathValue("id", "1")
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != want {
			t.Errorf("%s status=%d want=%d: %s", method, w.Code, want, w.Body.String())
		}
	}
	query(`SELECT COALESCE\(pr_number,0\),COALESCE\(pr_url,''\),COALESCE\(pr_branch,''\) FROM tasks WHERE id=\$1`, int64(1))
	call("getTaskPullRequest", h.getTaskPullRequest, 404)
	query(`SELECT p.workspace_path,t.pr_number,t.created_by FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`, int64(1))
	call("refreshTaskChecks", h.refreshTaskChecks, 404)
	query(`SELECT e.project_id,e.task_id,e.status,COALESCE\(e.url,''\),COALESCE\(e.port,0\),e.health_checked_at FROM project_qa_environments e WHERE e.task_id=\$1`, int64(1))
	call("getQAEnvironment", h.getQAEnvironment, 200)
	query(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`, int64(1))
	call("startQAEnvironment", h.startQAEnvironment, 404)
	query(`SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`, int64(1))
	call("stopQAEnvironment", h.stopQAEnvironment, 404)
	query(`SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`, int64(1))
	call("qaEnvironmentLogs", h.qaEnvironmentLogs, 404)
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM tasks WHERE id = \$1\)`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	call("listTaskEvents", h.listTaskEvents, 404)
	mock.ExpectQuery(`SELECT id, task_id, round, status, artifact_paths, summary, created_at FROM task_specifications`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "round", "status", "artifact_paths", "summary", "created_at"}))
	call("listTaskSpecifications", h.listTaskSpecifications, 200)
	mock.ExpectQuery(`SELECT id, task_id, attempt, status, changed_files, summary, created_at FROM task_implementations`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "attempt", "status", "changed_files", "summary", "created_at"}))
	call("listTaskImplementations", h.listTaskImplementations, 200)
	mock.ExpectQuery(`SELECT id, task_id, phase, attempt, status, summary, result_refs, started_at, finished_at FROM agent_runs`).WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "status", "summary", "result_refs", "started_at", "finished_at"}))
	call("listAgentRuns", h.listAgentRuns, 200)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskPullRequestAndQAStartGatesWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	prQuery := `SELECT t.id,t.project_id,t.title,t.description,t.state,t.specification_status,t.implementation_status,t.qa_status,t.pr_status,t.checks_status,t.created_by,t.created_at,t.pr_number,t.pr_url,t.pr_branch,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`
	mock.ExpectQuery(prQuery).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "pr_number", "pr_url", "pr_branch", "workspace_path"}).AddRow(7, 3, "Not ready", "", "Aberta", "approved", "succeeded", "pending", "not_created", "unknown", 2, now, nil, nil, nil, "/tmp/project"))
	r := httptest.NewRequest("POST", "/api/tasks/7/pull-request", nil)
	r.SetPathValue("id", "7")
	w := httptest.NewRecorder()
	h.createTaskPullRequest(w, r)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "QA approval is required") {
		t.Fatalf("PR gate=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(3, "Aberta", "/tmp/project"))
	_, err = h.startQAEnvironmentForTask(context.Background(), 7, "start")
	if !errors.Is(err, errQAEnvironmentUnavailable) {
		t.Fatalf("QA start gate error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStartQAEnvironmentMapsLookupAndGateFailuresWithMocks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	query := `SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`
	cases := []struct {
		name, id string
		prepare  func()
		want     int
	}{
		{name: "invalid id", id: "bad", want: 400},
		{name: "missing task", id: "71", prepare: func() {
			mock.ExpectQuery(query).WithArgs(int64(71)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}))
		}, want: 404},
		{name: "database error", id: "72", prepare: func() {
			mock.ExpectQuery(query).WithArgs(int64(72)).WillReturnError(errors.New("database unavailable"))
		}, want: 500},
		{name: "task not ready for QA", id: "73", prepare: func() {
			mock.ExpectQuery(query).WithArgs(int64(73)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(4, string(workflow.StateOpen), "/tmp/project"))
		}, want: 409},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepare != nil {
				tc.prepare()
			}
			r := httptest.NewRequest("POST", "/api/tasks/"+tc.id+"/environment/start", nil)
			r.SetPathValue("id", tc.id)
			w := httptest.NewRecorder()
			h.startQAEnvironment(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStartQAEnvironmentMarksUnavailableAllocatorAsUnhealthy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=\$1`).WithArgs(int64(101)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}).AddRow(4, string(workflow.StateWaitingQA), "/tmp/project"))
	query := `INSERT INTO project_qa_environments\(project_id,task_id,status,url,port,health_checked_at\)`
	mock.ExpectExec(query).WithArgs(int64(4), int64(101), "starting", "", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(query).WithArgs(int64(4), int64(101), "unhealthy", "", nil).WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/api/tasks/101/environment/start", nil)
	r.SetPathValue("id", "101")
	w := httptest.NewRecorder()
	h.startQAEnvironment(w, r)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "port allocator requires a database") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

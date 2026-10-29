package projects

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"fabrica/backend/internal/environments"
	"fabrica/backend/internal/githubcli"
	"fabrica/backend/internal/workflow"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestReconciliationQueriesAndEmptySnapshotsWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	mock.ExpectQuery(`SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs WHERE status='running' ORDER BY id`).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "process_pid", "process_start_token"}))
	if err := ReconcileAgentRuns(ctx, db); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,p.workspace_path,COALESCE`).WillReturnRows(sqlmock.NewRows([]string{"project_id", "task_id", "workspace", "url", "port"}))
	if err := ReconcileQAEnvironments(ctx, db); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT t.id,t.created_by,t.pr_number,p.workspace_path,t.state FROM tasks`).WillReturnRows(sqlmock.NewRows([]string{"id", "created_by", "pr_number", "workspace", "state"}))
	h := &Handler{db: db, workspaceRoot: root, github: githubcli.Client{Binary: binary}}
	if err := h.ReconcileGitHubPullRequests(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQAServiceRunningRequiresDeclaredRunningService(t *testing.T) {
	root, binary := projectComposeFixture(t)
	_ = binary
	if !qaServiceRunning(environments.Environment{Containers: []environments.Container{{Service: "web", State: "RUNNING"}}}, root) {
		t.Fatal("running QA service should be recognized case-insensitively")
	}
	if qaServiceRunning(environments.Environment{Containers: []environments.Container{{Service: "db", State: "running"}}}, root) {
		t.Fatal("unrelated service should not satisfy QA health")
	}
	if qaServiceRunning(environments.Environment{}, t.TempDir()) {
		t.Fatal("missing project descriptor should fail")
	}
}

func TestReconcileInterruptedSpecificationRunWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(`SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs WHERE status='running' ORDER BY id`).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "process_pid", "process_start_token"}).AddRow(52, 27, "specification", 1, 0, ""))
	mock.ExpectBegin()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(27)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(27, 4, "Generate spec", "", "Especificando", "pending", "not_started", "not_started", "not_created", "unknown", "", 8, now))
	mock.ExpectExec(`UPDATE agent_runs SET status='failed', summary='Execution interrupted`).WithArgs(int64(52)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(round\),0\)\+1 FROM task_specifications WHERE task_id=\$1`).WithArgs(int64(27)).WillReturnRows(sqlmock.NewRows([]string{"round"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO task_specifications\(task_id,round,status,artifact_paths,summary,run_id\)`).WithArgs(int64(27), 2, int64(52)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE tasks SET state=\$2,specification_status=\$3`).WithArgs(int64(27), workflow.StateReady, workflow.StatusSpecFailed).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(27), int64(8), "system", "failure", string(workflow.ActionSpecificationFailed), string(workflow.StateSpecifying), string(workflow.StateReady), "", "Specification generation interrupted by backend restart").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := ReconcileAgentRuns(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileInterruptedImplementationRunWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(`SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs WHERE status='running' ORDER BY id`).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "process_pid", "process_start_token"}).AddRow(53, 28, "implementation", 1, 0, ""))
	mock.ExpectBegin()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(28)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(28, 4, "Implement feature", "", string(workflow.StateInDevelopment), workflow.StatusApproved, workflow.StatusRunning, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "", 8, now))
	mock.ExpectExec(`UPDATE agent_runs SET status='failed', summary='Execution interrupted`).WithArgs(int64(53)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt\),0\)\+1 FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(28)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(2))
	mock.ExpectExec(`INSERT INTO task_implementations\(task_id,attempt,status,changed_files,summary,run_id\)`).WithArgs(int64(28), 2, []byte("[]"), int64(53)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE tasks SET implementation_status=\$2`).WithArgs(int64(28), workflow.StatusFailed).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(28), int64(8), "system", "failure", string(workflow.ActionImplementationError), string(workflow.StateInDevelopment), string(workflow.StateInDevelopment), "", "Implementation interrupted by backend restart").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := ReconcileAgentRuns(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileAgentRunsHandlesReadFailuresAndInactiveTaskStates(t *testing.T) {
	t.Run("query failure", func(t *testing.T) {
		db, m, e := sqlmock.New()
		if e != nil {
			t.Fatal(e)
		}
		defer db.Close()
		m.ExpectQuery(`SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs`).WillReturnError(errors.New("database unavailable"))
		if err := ReconcileAgentRuns(context.Background(), db); err == nil {
			t.Fatal("expected query failure")
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("scan failure", func(t *testing.T) {
		db, m, e := sqlmock.New()
		if e != nil {
			t.Fatal(e)
		}
		defer db.Close()
		m.ExpectQuery(`SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs`).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "process_pid", "process_start_token"}).AddRow("bad", 1, "specification", 1, 0, ""))
		if err := ReconcileAgentRuns(context.Background(), db); err == nil {
			t.Fatal("expected scan failure")
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("run no longer matches active task phase", func(t *testing.T) {
		db, m, e := sqlmock.New()
		if e != nil {
			t.Fatal(e)
		}
		defer db.Close()
		now := time.Now()
		m.ExpectQuery(`SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs`).WillReturnRows(sqlmock.NewRows([]string{"id", "task_id", "phase", "attempt", "process_pid", "process_start_token"}).AddRow(54, 29, "specification", 1, 0, ""))
		m.ExpectBegin()
		cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
		m.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(29)).WillReturnRows(sqlmock.NewRows(cols).AddRow(29, 4, "Already moved", "", string(workflow.StateOpen), workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "", 8, now))
		m.ExpectExec(`UPDATE agent_runs SET status='failed', summary='Execution interrupted`).WithArgs(int64(54)).WillReturnResult(sqlmock.NewResult(0, 1))
		m.ExpectCommit()
		if err := ReconcileAgentRuns(context.Background(), db); err != nil {
			t.Fatal(err)
		}
		if err := m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestReconcileMergedGitHubPRUpdatesTaskThroughMockDB(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	script := "#!/bin/sh\ncase \"$1 $2 $3\" in\n 'auth status ') exit 0 ;;\n 'pr view 42') echo '{\"state\":\"MERGED\",\"url\":\"https://github.com/acme/demo/pull/42\",\"number\":42}' ;;\n 'pr checks 42') echo '[{\"name\":\"CI\",\"state\":\"SUCCESS\",\"link\":\"https://github.com/acme/demo/actions\",\"workflow\":\"CI\"}]' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(`SELECT t.id,t.created_by,t.pr_number,p.workspace_path,t.state FROM tasks t`).WillReturnRows(sqlmock.NewRows([]string{"task_id", "actor_id", "number", "workspace", "state"}).AddRow(21, 7, 42, root, string(workflow.StatePROpen)))
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	for _, state := range []workflow.State{workflow.StatePROpen, workflow.StateReadyForMerge} {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(21)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(21, 4, "Feature", "", state, "approved", "succeeded", "approved", "open", map[workflow.State]string{workflow.StatePROpen: workflow.StatusPending, workflow.StateReadyForMerge: workflow.StatusPassed}[state], "", 7, now))
		updatedState, checks, prStatus := workflow.StateReadyForMerge, workflow.StatusPassed, workflow.StatusOpen
		if state == workflow.StateReadyForMerge {
			updatedState, prStatus = workflow.StateComplete, workflow.StatusMerged
		}
		mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(21), updatedState, workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusApproved, prStatus, checks, "").WillReturnResult(sqlmock.NewResult(0, 1))
		action := workflow.ActionChecksPassed
		eventFrom := string(workflow.StatePROpen)
		if state == workflow.StateReadyForMerge {
			action = workflow.ActionMerge
			eventFrom = string(workflow.StateReadyForMerge)
		}
		mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(21), int64(7), "system", "transition", string(action), eventFrom, string(updatedState), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
	}
	h := &Handler{db: db, workspaceRoot: root, github: githubcli.Client{Binary: binary}}
	if err := h.ReconcileGitHubPullRequests(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubReconciliationWrapperTreatsUnauthenticatedCLIAsNoop(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "fake-gh")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, workspaceRoot: root, github: githubcli.Client{Binary: binary}}
	if err := h.ReconcileGitHubPullRequests(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubReconciliationFunctionSkipsWhenCLIIsUnauthenticated(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := ReconcileGitHubPullRequests(context.Background(), db, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileQAEnvironmentRestoresHealthyComposeProjectWithMocks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".fabrica"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, environments.DescriptorPath), []byte(`{"version":1,"composeFile":"compose.yaml","qaService":"web","containerPort":80,"healthCheck":{"path":"/healthz"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	docker := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'ps --format json'*) echo '{\"ID\":\"id\",\"Name\":\"web-1\",\"Project\":\"fabrica-41\",\"Service\":\"web\",\"State\":\"running\",\"Health\":\"healthy\",\"ExitCode\":0}' ;;\nesac\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,p.workspace_path,COALESCE`).WillReturnRows(sqlmock.NewRows([]string{"project_id", "task_id", "workspace", "url", "port"}).AddRow(41, 62, root, "http://127.0.0.1", port))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM qa_port_leases WHERE project_id=\$1 AND port=\$2`).WithArgs(int64(41), port).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE project_qa_environments SET status='healthy',url=\$2`).WithArgs(int64(41), "http://127.0.0.1:"+strconv.Itoa(port)+"/healthz").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := ReconcileQAEnvironments(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileQAEnvironmentMarksMissingPortLeaseUnhealthy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,p.workspace_path,COALESCE`).WillReturnRows(sqlmock.NewRows([]string{"project_id", "task_id", "workspace", "url", "port"}).AddRow(41, 62, root, "http://127.0.0.1", 41041))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM qa_port_leases WHERE project_id=\$1 AND port=\$2 AND status='active'\)`).WithArgs(int64(41), 41041).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`UPDATE project_qa_environments SET status='unhealthy',url=NULL`).WithArgs(int64(41)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := ReconcileQAEnvironments(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileQAEnvironmentReleasesLeaseForStoppedComposeService(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".fabrica"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, environments.DescriptorPath), []byte(`{"version":1,"composeFile":"compose.yaml","qaService":"web","containerPort":80}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	docker := filepath.Join(binDir, "docker")
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'ps --format json'*) echo '{\"ID\":\"dead\",\"Name\":\"web-1\",\"Project\":\"fabrica-41\",\"Service\":\"web\",\"State\":\"exited\",\"ExitCode\":1}' ;;\nesac\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT e.project_id,e.task_id,p.workspace_path,COALESCE`).WillReturnRows(sqlmock.NewRows([]string{"project_id", "task_id", "workspace", "url", "port"}).AddRow(41, 62, root, "http://127.0.0.1", 41041))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM qa_port_leases WHERE project_id=\$1 AND port=\$2 AND status='active'\)`).WithArgs(int64(41), 41041).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE project_qa_environments SET status='stopped',url=NULL`).WithArgs(int64(41)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE qa_port_leases SET status='released'`).WithArgs(int64(41)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := ReconcileQAEnvironments(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

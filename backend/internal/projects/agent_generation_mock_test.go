package projects

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fabrica/backend/internal/opencode"
	"fabrica/backend/internal/workflow"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestGenerateSpecificationPersistsArtifactsAndOpensPOReview(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectRoot := filepath.Join(workspaceRoot, "demo")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nmkdir -p openspec/changes/task-27/specs/core\nprintf '%s\\n' '# Proposal' > openspec/changes/task-27/proposal.md\nprintf '%s\\n' '# Design' > openspec/changes/task-27/design.md\nprintf '%s\\n' '- [x] task' > openspec/changes/task-27/tasks.md\nprintf '%s\\n' '# Core' > openspec/changes/task-27/specs/core/spec.md\nprintf '%s\\n' '{\"version\":1,\"writablePaths\":[\"main.go\"]}' > openspec/changes/task-27/scope.json\nprintf '%s\\n' '{\"type\":\"text\"}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, workspaceRoot: workspaceRoot, opencode: opencode.Executor{Binary: binary}}
	now := time.Now()
	initialCols := []string{"id", "project_id", "title", "description", "state", "requirements", "workspace_path", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, p.requirements`).WithArgs(int64(27)).WillReturnRows(sqlmock.NewRows(initialCols).AddRow(27, 4, "Add workflow", "Details", "Especificando", "Business requirements", projectRoot, "pending", "not_started", "not_started", "not_created", "unknown", 8, now))
	mock.ExpectExec(`UPDATE agent_runs SET process_pid=\$2, process_start_token=\$3`).WithArgs(int64(52), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(round\), 0\) \+ 1 FROM task_specifications WHERE task_id = \$1`).WithArgs(int64(27)).WillReturnRows(sqlmock.NewRows([]string{"round"}).AddRow(1))
	pathsJSON := []byte(`["openspec/changes/task-27/design.md","openspec/changes/task-27/proposal.md","openspec/changes/task-27/scope.json","openspec/changes/task-27/specs/core/spec.md","openspec/changes/task-27/tasks.md"]`)
	mock.ExpectExec(`INSERT INTO task_specifications \(task_id, round, status, artifact_paths, summary, run_id\)`).WithArgs(int64(27), 1, "generated", pathsJSON, "OpenSpec artifacts generated", int64(52)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(52), "succeeded", "OpenSpec artifacts generated", sqlmock.AnyArg(), sqlmock.AnyArg(), pathsJSON).WillReturnResult(sqlmock.NewResult(0, 1))
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(27)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(27, 4, "Add workflow", "Details", "Especificando", "pending", "not_started", "not_started", "not_created", "unknown", "", 8, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(27), workflow.StateWaitingSpecApproval, workflow.StatusPending, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(27), int64(8), "system", "transition", string(workflow.ActionSpecificationReady), string(workflow.StateSpecifying), string(workflow.StateWaitingSpecApproval), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	h.generateSpecification(27, 52)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateImplementationCancelsWhenTaskAlreadyMovedOn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	cols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, t.specification_status`).WithArgs(int64(30)).WillReturnRows(sqlmock.NewRows(cols).AddRow(30, 4, "Already advanced", "", "Aguardando QA", workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusPending, workflow.StatusNotCreated, workflow.StatusUnknown, 8, now, "/tmp/project"))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(55), "cancelled", "Implementation run was no longer active", sqlmock.AnyArg(), sqlmock.AnyArg(), []byte("null")).WillReturnResult(sqlmock.NewResult(0, 1))
	h.generateImplementation(30, 55)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateSpecificationRecordsFailureWhenArtifactsAreMissing(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectRoot := filepath.Join(workspaceRoot, "demo")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"text\"}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, workspaceRoot: workspaceRoot, opencode: opencode.Executor{Binary: binary}}
	now := time.Now()
	initialCols := []string{"id", "project_id", "title", "description", "state", "requirements", "workspace_path", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, p.requirements`).WithArgs(int64(28)).WillReturnRows(sqlmock.NewRows(initialCols).AddRow(28, 4, "Missing spec", "Details", "Especificando", "Requirements", projectRoot, "pending", "not_started", "not_started", "not_created", "unknown", 8, now))
	mock.ExpectExec(`UPDATE agent_runs SET process_pid=\$2, process_start_token=\$3`).WithArgs(int64(53), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(round\), 0\) \+ 1 FROM task_specifications WHERE task_id = \$1`).WithArgs(int64(28)).WillReturnRows(sqlmock.NewRows([]string{"round"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO task_specifications \(task_id, round, status, artifact_paths, summary, run_id\)`).WithArgs(int64(28), 1, "failed", []byte(`null`), "Specification generation failed", int64(53)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(53), "failed", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`null`)).WillReturnResult(sqlmock.NewResult(0, 1))
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(28)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(28, 4, "Missing spec", "Details", "Especificando", "pending", "not_started", "not_started", "not_created", "unknown", "", 8, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(28), workflow.StateReady, workflow.StatusSpecFailed, workflow.StatusNotStarted, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(28), int64(8), "system", "failure", string(workflow.ActionSpecificationFailed), string(workflow.StateSpecifying), string(workflow.StateReady), sqlmock.AnyArg(), "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	h.generateSpecification(28, 53)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateImplementationPersistsChangesAndRequestsQA(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectRoot := filepath.Join(workspaceRoot, "demo")
	spec := filepath.Join(projectRoot, "openspec", "changes", "task-29", "proposal.md")
	if err := os.MkdirAll(filepath.Dir(spec), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("Approved task scope"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "fake-opencode")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' 'package main' > main.go\nprintf '%s\\n' '{\"type\":\"text\"}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, workspaceRoot: workspaceRoot, opencode: opencode.Executor{Binary: binary}}
	now := time.Now()
	initialCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, t.specification_status`).WithArgs(int64(29)).WillReturnRows(sqlmock.NewRows(initialCols).AddRow(29, 4, "Implement feature", "Details", "Em desenvolvimento", "approved", "running", "not_started", "not_created", "unknown", 8, now, projectRoot))
	mock.ExpectQuery(`SELECT artifact_paths, write_paths FROM task_specifications WHERE task_id=\$1 AND status='approved'`).WithArgs(int64(29)).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "write_paths"}).AddRow([]byte(`["openspec/changes/task-29/proposal.md"]`), []byte(`["main.go"]`)))
	mock.ExpectQuery(`SELECT comment,reason FROM task_events WHERE task_id=\$1`).WithArgs(int64(29), string(workflow.ActionRejectQA)).WillReturnRows(sqlmock.NewRows([]string{"comment", "reason"}))
	mock.ExpectExec(`UPDATE agent_runs SET process_pid=\$2, process_start_token=\$3`).WithArgs(int64(54), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt\),0\)\+1 FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(29)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO task_implementations`).WithArgs(int64(29), 1, "succeeded", []byte(`["main.go"]`), "Implementation completed with changed files", int64(54)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(54), "succeeded", "Implementation completed with changed files", sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`["main.go"]`)).WillReturnResult(sqlmock.NewResult(0, 1))
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(29)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(29, 4, "Implement feature", "Details", "Em desenvolvimento", "approved", "running", "not_started", "not_created", "unknown", "", 8, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(29), workflow.StateWaitingQA, workflow.StatusApproved, workflow.StatusSucceeded, workflow.StatusPending, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(29), int64(8), "system", "transition", string(workflow.ActionImplementationDone), string(workflow.StateInDevelopment), string(workflow.StateWaitingQA), "", "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`INSERT INTO project_qa_environments\(project_id,task_id,status,url,port\)`).WithArgs(int64(4), int64(29)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p`).WithArgs(int64(29)).WillReturnRows(sqlmock.NewRows([]string{"project_id", "state", "workspace_path"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(29), int64(8), "system", "failure", "qa_environment_start_failed", string(workflow.StateWaitingQA), string(workflow.StateWaitingQA), "", "sql: no rows in result set").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	h.generateImplementation(29, 54)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateImplementationBlocksOutOfScopeFilesystemChanges(t *testing.T) {
	workspaceRoot := t.TempDir()
	projectRoot := filepath.Join(workspaceRoot, "demo")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(projectRoot, "openspec", "changes", "task-35", "proposal.md")
	if err := os.MkdirAll(filepath.Dir(spec), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("Approved task scope"), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "fake-opencode")
	script := "#!/bin/sh\nprintf '%s\\n' 'package main' > main.go\nprintf '%s\\n' 'unapproved change' > unrelated.txt\nprintf '%s\\n' '{\"type\":\"text\"}'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db, workspaceRoot: workspaceRoot, opencode: opencode.Executor{Binary: binary}}
	now := time.Now()
	taskCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "created_by", "created_at", "workspace_path"}
	mock.ExpectQuery(`SELECT t.id, t.project_id, t.title, t.description, t.state, t.specification_status`).WithArgs(int64(35)).WillReturnRows(sqlmock.NewRows(taskCols).AddRow(35, 4, "Implement scoped task", "Details", string(workflow.StateInDevelopment), workflow.StatusApproved, workflow.StatusRunning, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, 8, now, projectRoot))
	mock.ExpectQuery(`SELECT artifact_paths, write_paths FROM task_specifications WHERE task_id=\$1 AND status='approved'`).WithArgs(int64(35)).WillReturnRows(sqlmock.NewRows([]string{"artifact_paths", "write_paths"}).AddRow([]byte(`["openspec/changes/task-35/proposal.md"]`), []byte(`["main.go"]`)))
	mock.ExpectQuery(`SELECT comment,reason FROM task_events WHERE task_id=\$1`).WithArgs(int64(35), string(workflow.ActionRejectQA)).WillReturnRows(sqlmock.NewRows([]string{"comment", "reason"}))
	mock.ExpectExec(`UPDATE agent_runs SET process_pid=\$2, process_start_token=\$3`).WithArgs(int64(60), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempt\),0\)\+1 FROM task_implementations WHERE task_id=\$1`).WithArgs(int64(35)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(1))
	summary := "OpenCode implementation failed: security gateway denied operation: changed file is outside the approved write scope"
	mock.ExpectExec(`INSERT INTO task_implementations`).WithArgs(int64(35), 1, "failed", []byte(`["main.go","unrelated.txt"]`), summary, int64(60)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE agent_runs SET status=\$2, summary=\$3`).WithArgs(int64(60), "failed", summary, sqlmock.AnyArg(), sqlmock.AnyArg(), []byte(`["main.go","unrelated.txt"]`)).WillReturnResult(sqlmock.NewResult(0, 1))
	lockedCols := []string{"id", "project_id", "title", "description", "state", "specification_status", "implementation_status", "qa_status", "pr_status", "checks_status", "blocked_from_state", "created_by", "created_at"}
	mock.ExpectQuery(`SELECT id, project_id, title, description, state, specification_status`).WithArgs(int64(35)).WillReturnRows(sqlmock.NewRows(lockedCols).AddRow(35, 4, "Implement scoped task", "Details", string(workflow.StateInDevelopment), workflow.StatusApproved, workflow.StatusRunning, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "", 8, now))
	mock.ExpectExec(`UPDATE tasks SET state=\$2, specification_status=\$3`).WithArgs(int64(35), workflow.StateInDevelopment, workflow.StatusApproved, workflow.StatusFailed, workflow.StatusNotStarted, workflow.StatusNotCreated, workflow.StatusUnknown, "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO task_events`).WithArgs(int64(35), int64(8), "system", "transition", string(workflow.ActionImplementationError), string(workflow.StateInDevelopment), string(workflow.StateInDevelopment), summary, "").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	h.generateImplementation(35, 60)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

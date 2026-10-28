package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fabrica/backend/internal/auth"
	"fabrica/backend/internal/environments"
	"fabrica/backend/internal/githubcli"
	"fabrica/backend/internal/opencode"
	"fabrica/backend/internal/workflow"
	"fabrica/backend/internal/workspace"
)

type Handler struct {
	db            *sql.DB
	workspaceRoot string
	opencode      opencode.Executor
	compose       environments.ComposeManager
	ports         *environments.PortAllocator
	github        githubcli.Client
}

type Project struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Requirements  string    `json:"requirements"`
	WorkspacePath string    `json:"workspacePath"`
	CreatedBy     int64     `json:"createdBy"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Task struct {
	ID                   int64     `json:"id"`
	ProjectID            int64     `json:"projectId"`
	Title                string    `json:"title"`
	Description          string    `json:"description"`
	State                string    `json:"state"`
	SpecificationStatus  string    `json:"specificationStatus"`
	ImplementationStatus string    `json:"implementationStatus"`
	QAStatus             string    `json:"qaStatus"`
	PRStatus             string    `json:"prStatus"`
	ChecksStatus         string    `json:"checksStatus"`
	CreatedBy            int64     `json:"createdBy"`
	CreatedAt            time.Time `json:"createdAt"`
}

type TaskEvent struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"taskId"`
	ActorID    int64     `json:"actorId"`
	ActorName  string    `json:"actorName"`
	ActorRole  string    `json:"actorRole"`
	Type       string    `json:"type"`
	Action     string    `json:"action"`
	FromState  string    `json:"fromState"`
	ToState    string    `json:"toState"`
	Comment    string    `json:"comment,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

type TaskSpecification struct {
	ID            int64     `json:"id"`
	TaskID        int64     `json:"taskId"`
	Round         int       `json:"round"`
	Status        string    `json:"status"`
	ArtifactPaths []string  `json:"artifactPaths"`
	Summary       string    `json:"summary"`
	CreatedAt     time.Time `json:"createdAt"`
}

type TaskImplementation struct {
	ID           int64     `json:"id"`
	TaskID       int64     `json:"taskId"`
	Attempt      int       `json:"attempt"`
	Status       string    `json:"status"`
	ChangedFiles []string  `json:"changedFiles"`
	Summary      string    `json:"summary"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AgentRun struct {
	ID         int64      `json:"id"`
	TaskID     int64      `json:"taskId"`
	Phase      string     `json:"phase"`
	Attempt    int        `json:"attempt"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary"`
	ResultRefs []string   `json:"resultRefs"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type createProjectRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Requirements string `json:"requirements"`
}

type updateProjectRequest struct {
	Description  *string `json:"description"`
	Requirements *string `json:"requirements"`
}

type createTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type updateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

func RegisterRoutes(mux *http.ServeMux, db *sql.DB, workspaceRoot string) {
	RegisterRoutesWithExecutor(mux, db, workspaceRoot, opencode.Executor{})
}

func RegisterRoutesWithExecutor(mux *http.ServeMux, db *sql.DB, workspaceRoot string, executor opencode.Executor) {
	h := &Handler{db: db, workspaceRoot: workspaceRoot, opencode: executor}
	h.ports = environments.NewPortAllocator(db)
	mux.HandleFunc("GET /api/projects", auth.RequireAction(db, auth.ActionViewProgress, h.listProjects))
	mux.HandleFunc("POST /api/projects", auth.RequireAction(db, auth.ActionOrganizeProjects, h.createProject))
	mux.HandleFunc("GET /api/projects/{id}", auth.RequireAction(db, auth.ActionViewProgress, h.getProject))
	mux.HandleFunc("PATCH /api/projects/{id}", auth.RequireAction(db, auth.ActionOrganizeProjects, h.updateProject))
	mux.HandleFunc("GET /api/projects/{projectID}/tasks", auth.RequireAction(db, auth.ActionViewProgress, h.listTasks))
	mux.HandleFunc("POST /api/projects/{projectID}/tasks", auth.RequireAction(db, auth.ActionOrganizeProjects, h.createTask))
	mux.HandleFunc("GET /api/tasks/{id}", auth.RequireAction(db, auth.ActionViewProgress, h.getTask))
	mux.HandleFunc("PATCH /api/tasks/{id}", auth.RequireAction(db, auth.ActionOrganizeProjects, h.updateTask))
	mux.HandleFunc("POST /api/tasks/{id}/actions", h.applyTaskAction)
	mux.HandleFunc("GET /api/tasks/{id}/events", auth.RequireAction(db, auth.ActionViewProgress, h.listTaskEvents))
	mux.HandleFunc("GET /api/tasks/{id}/specifications", auth.RequireAction(db, auth.ActionViewProgress, h.listTaskSpecifications))
	mux.HandleFunc("GET /api/tasks/{id}/specifications/{round}/artifact", auth.RequireAction(db, auth.ActionViewProgress, h.getTaskSpecificationArtifact))
	mux.HandleFunc("GET /api/tasks/{id}/implementations", auth.RequireAction(db, auth.ActionViewProgress, h.listTaskImplementations))
	mux.HandleFunc("GET /api/tasks/{id}/agent-runs", auth.RequireAction(db, auth.ActionViewProgress, h.listAgentRuns))
	mux.HandleFunc("GET /api/tasks/{id}/environment", auth.RequireAction(db, auth.ActionViewProgress, h.getQAEnvironment))
	mux.HandleFunc("POST /api/tasks/{id}/environment/start", auth.RequireAction(db, auth.ActionPerformQA, h.startQAEnvironment))
	mux.HandleFunc("POST /api/tasks/{id}/environment/stop", auth.RequireAction(db, auth.ActionPerformQA, h.stopQAEnvironment))
	mux.HandleFunc("POST /api/tasks/{id}/environment/restart", auth.RequireAction(db, auth.ActionPerformQA, h.restartQAEnvironment))
	mux.HandleFunc("GET /api/tasks/{id}/environment/logs", auth.RequireAction(db, auth.ActionViewProgress, h.qaEnvironmentLogs))
	mux.HandleFunc("GET /api/github/status", auth.RequireAction(db, auth.ActionMerge, h.githubStatus))
	mux.HandleFunc("POST /api/tasks/{id}/pull-request", auth.RequireAction(db, auth.ActionMerge, h.createTaskPullRequest))
	mux.HandleFunc("GET /api/tasks/{id}/pull-request", auth.RequireAction(db, auth.ActionViewProgress, h.getTaskPullRequest))
	mux.HandleFunc("POST /api/tasks/{id}/pull-request/checks", auth.RequireAction(db, auth.ActionMerge, h.refreshTaskChecks))
}

type taskPullRequest struct {
	Number int               `json:"number"`
	URL    string            `json:"url"`
	Branch string            `json:"branch"`
	Checks []githubcli.Check `json:"checks,omitempty"`
}

func (h *Handler) createTaskPullRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var task Task
	var projectPath string
	var prNumber sql.NullInt64
	var prURL, prBranch sql.NullString
	err := h.db.QueryRowContext(ctx, `SELECT t.id,t.project_id,t.title,t.description,t.state,t.specification_status,t.implementation_status,t.qa_status,t.pr_status,t.checks_status,t.created_by,t.created_at,t.pr_number,t.pr_url,t.pr_branch,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=$1`, id).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State, &task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus, &task.PRStatus, &task.ChecksStatus, &task.CreatedBy, &task.CreatedAt, &prNumber, &prURL, &prBranch, &projectPath)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "could not read task", 500)
		return
	}
	if task.State != string(workflow.StateWaitingPR) || task.QAStatus != workflow.StatusApproved {
		http.Error(w, "QA approval is required before creating a pull request", 409)
		return
	}
	if prNumber.Valid && prURL.Valid {
		writeJSON(w, 200, taskPullRequest{Number: int(prNumber.Int64), URL: prURL.String, Branch: prBranch.String})
		return
	}
	if err := h.github.CheckAuth(ctx, projectPath); err != nil {
		_ = h.recordSystemTaskEvent(ctx, id, task.CreatedBy, "failure", "create_pull_request", task.State, task.State, "GitHub CLI authentication failed")
		http.Error(w, "GitHub CLI is not authenticated; run gh auth login", 503)
		return
	}
	var rawFiles []byte
	err = h.db.QueryRowContext(ctx, `SELECT changed_files FROM task_implementations WHERE task_id=$1 AND status='succeeded' ORDER BY attempt DESC LIMIT 1`, id).Scan(&rawFiles)
	if err != nil {
		http.Error(w, "successful implementation files are unavailable", 409)
		return
	}
	var files []string
	if json.Unmarshal(rawFiles, &files) != nil {
		http.Error(w, "implementation file references are invalid", 500)
		return
	}
	branch, err := prepareTaskBranch(ctx, projectPath, id, task.Title, files)
	if err != nil {
		_ = h.recordSystemTaskEvent(ctx, id, task.CreatedBy, "failure", "create_pull_request", task.State, task.State, "Could not prepare task branch")
		http.Error(w, err.Error(), 409)
		return
	}
	if err := pushTaskBranch(ctx, projectPath, branch); err != nil {
		_ = h.recordSystemTaskEvent(ctx, id, task.CreatedBy, "failure", "create_pull_request", task.State, task.State, "Could not push task branch")
		http.Error(w, err.Error(), 502)
		return
	}
	pull, err := h.github.CreatePullRequest(ctx, projectPath, task.Title, fmt.Sprintf("Tarefa #%d: %s\n\n%s\n\nAprovada por QA na Fábrica.", id, task.Title, task.Description), branch)
	if err != nil {
		_ = h.recordSystemTaskEvent(ctx, id, task.CreatedBy, "failure", "create_pull_request", task.State, task.State, "GitHub CLI could not create the pull request")
		http.Error(w, "could not create pull request with gh", 502)
		return
	}
	if err := h.persistCreatedPullRequest(ctx, id, task.CreatedBy, pull, branch); err != nil {
		http.Error(w, "pull request exists but task state could not be saved", 500)
		return
	}
	writeJSON(w, 201, taskPullRequest{Number: pull.Number, URL: pull.URL, Branch: branch})
}

func (h *Handler) persistCreatedPullRequest(ctx context.Context, taskID, actorID int64, pull githubcli.PullRequest, branch string) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task Task
	var blocked string
	if err := tx.QueryRowContext(ctx, `SELECT id,project_id,title,description,state,specification_status,implementation_status,qa_status,pr_status,checks_status,blocked_from_state,created_by,created_at FROM tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State, &task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus, &task.PRStatus, &task.ChecksStatus, &blocked, &task.CreatedBy, &task.CreatedAt); err != nil {
		return err
	}
	updated, err := workflow.Apply(workflow.Task{State: workflow.State(task.State), SpecificationStatus: task.SpecificationStatus, ImplementationStatus: task.ImplementationStatus, QAStatus: task.QAStatus, PRStatus: task.PRStatus, ChecksStatus: task.ChecksStatus, BlockedFromState: blocked}, workflow.ActionCreatePR, "system", "", pull.URL)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE tasks SET state=$2,pr_status=$3,pr_number=$4,pr_url=$5,pr_branch=$6,updated_at=now() WHERE id=$1`, taskID, updated.State, updated.PRStatus, pull.Number, pull.URL, branch)
	if err != nil {
		return err
	}
	if err := appendTaskEvent(ctx, tx, taskID, actorID, "system", "transition", string(workflow.ActionCreatePR), task.State, string(updated.State), "", pull.URL); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) getTaskPullRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var pull taskPullRequest
	err := h.db.QueryRowContext(r.Context(), `SELECT COALESCE(pr_number,0),COALESCE(pr_url,''),COALESCE(pr_branch,'') FROM tasks WHERE id=$1`, id).Scan(&pull.Number, &pull.URL, &pull.Branch)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "could not read pull request", 500)
		return
	}
	writeJSON(w, 200, pull)
}

func (h *Handler) refreshTaskChecks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var projectPath string
	var number int
	var actorID int64
	err := h.db.QueryRowContext(r.Context(), `SELECT p.workspace_path,t.pr_number,t.created_by FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=$1`, id).Scan(&projectPath, &number, &actorID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", 404)
		return
	}
	if err != nil || number < 1 {
		http.Error(w, "pull request is not available", 409)
		return
	}
	checks, err := h.github.Checks(r.Context(), projectPath, number)
	if err != nil {
		http.Error(w, "could not read required pull request checks", 502)
		return
	}
	status := checkStatus(checks)
	action := workflow.ActionChecksPending
	if status == workflow.StatusPassed {
		action = workflow.ActionChecksPassed
	} else if status == workflow.StatusChecksFailed {
		action = workflow.ActionChecksFailed
	}
	if err := h.commitSystemTaskAction(r.Context(), id, actorID, action, "", ""); err != nil {
		http.Error(w, "could not save pull request check state", 409)
		return
	}
	writeJSON(w, 200, map[string]any{"status": status, "checks": checks})
}

func checkStatus(checks []githubcli.Check) string {
	if len(checks) == 0 {
		return workflow.StatusPending
	}
	pending := false
	for _, check := range checks {
		switch strings.ToUpper(check.State) {
		case "SUCCESS":
		case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STALE":
			return workflow.StatusChecksFailed
		default:
			pending = true
		}
	}
	if pending {
		return workflow.StatusPending
	}
	return workflow.StatusPassed
}

func (h *Handler) commitSystemTaskAction(ctx context.Context, taskID, actorID int64, action workflow.Action, comment, evidence string) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := h.applySystemTaskAction(ctx, tx, taskID, actorID, action, comment, evidence); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) recordSystemTaskEvent(ctx context.Context, taskID, actorID int64, eventKind, action, fromState, toState, reason string) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := appendTaskEvent(ctx, tx, taskID, actorID, "system", eventKind, action, fromState, toState, "", reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) githubStatus(w http.ResponseWriter, r *http.Request) {
	err := h.github.CheckAuth(r.Context(), h.workspaceRoot)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "message": "Execute gh auth login no terminal do executor."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

type qaEnvironment struct {
	ProjectID int64      `json:"projectId"`
	TaskID    int64      `json:"taskId"`
	Status    string     `json:"status"`
	URL       string     `json:"url,omitempty"`
	Port      int        `json:"port,omitempty"`
	CheckedAt *time.Time `json:"healthCheckedAt,omitempty"`
}

var errQAEnvironmentUnavailable = errors.New("QA environment is available only after implementation succeeds")

func (h *Handler) getQAEnvironment(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var env qaEnvironment
	err := h.db.QueryRowContext(r.Context(), `SELECT e.project_id,e.task_id,e.status,COALESCE(e.url,''),COALESCE(e.port,0),e.health_checked_at FROM project_qa_environments e WHERE e.task_id=$1`, taskID).Scan(&env.ProjectID, &env.TaskID, &env.Status, &env.URL, &env.Port, &env.CheckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, qaEnvironment{TaskID: taskID, Status: "stopped"})
		return
	}
	if err != nil {
		http.Error(w, "could not read QA environment", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (h *Handler) startQAEnvironment(w http.ResponseWriter, r *http.Request) {
	h.runQAEnvironment(w, r, "start")
}
func (h *Handler) restartQAEnvironment(w http.ResponseWriter, r *http.Request) {
	h.runQAEnvironment(w, r, "restart")
}

func (h *Handler) runQAEnvironment(w http.ResponseWriter, r *http.Request, operation string) {
	taskID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	result, err := h.startQAEnvironmentForTask(ctx, taskID, operation)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, errQAEnvironmentUnavailable) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		status := http.StatusBadGateway
		if strings.HasPrefix(err.Error(), "could not read task:") {
			status = http.StatusInternalServerError
		}
		http.Error(w, "could not start QA environment: "+err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) startQAEnvironmentForTask(ctx context.Context, taskID int64, operation string) (qaEnvironment, error) {
	var projectID int64
	var state, workspacePath string
	err := h.db.QueryRowContext(ctx, `SELECT t.project_id,t.state,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=$1`, taskID).Scan(&projectID, &state, &workspacePath)
	if errors.Is(err, sql.ErrNoRows) {
		return qaEnvironment{}, sql.ErrNoRows
	}
	if err != nil {
		return qaEnvironment{}, fmt.Errorf("could not read task: %w", err)
	}
	if state != string(workflow.StateWaitingQA) && state != string(workflow.StateInQA) {
		return qaEnvironment{}, errQAEnvironmentUnavailable
	}
	if operation != "start" && operation != "restart" {
		return qaEnvironment{}, errors.New("unsupported QA environment operation")
	}
	if err := h.saveQAEnvironment(ctx, projectID, taskID, "starting", "", nil); err != nil {
		return qaEnvironment{}, fmt.Errorf("could not record QA environment startup: %w", err)
	}
	var lease environments.PortLease
	if operation == "start" {
		_, lease, err = h.ports.StartWithRetry(ctx, h.compose, projectID, workspacePath)
	} else {
		lease, err = h.ports.Reserve(ctx, projectID)
		if err == nil {
			_, err = h.compose.StartWithPort(ctx, projectID, workspacePath, lease.Port)
		}
	}
	if err != nil {
		_ = h.saveQAEnvironment(context.Background(), projectID, taskID, "unhealthy", "", nil)
		return qaEnvironment{ProjectID: projectID, TaskID: taskID, Status: "unhealthy"}, fmt.Errorf("start Compose project: %w", err)
	}
	url, healthErr := waitForQAHealth(ctx, lease.Port, workspacePath)
	if healthErr != nil {
		_ = h.compose.Stop(context.Background(), projectID, workspacePath)
		_ = h.ports.Release(context.Background(), projectID)
		_ = h.saveQAEnvironment(context.Background(), projectID, taskID, "unhealthy", "", &lease.Port)
		return qaEnvironment{ProjectID: projectID, TaskID: taskID, Status: "unhealthy", Port: lease.Port}, fmt.Errorf("QA service did not become healthy: %w", healthErr)
	}
	if err := h.saveQAEnvironment(ctx, projectID, taskID, "healthy", url, &lease.Port); err != nil {
		_ = h.compose.Stop(context.Background(), projectID, workspacePath)
		_ = h.ports.Release(context.Background(), projectID)
		return qaEnvironment{}, fmt.Errorf("could not persist QA environment: %w", err)
	}
	return qaEnvironment{ProjectID: projectID, TaskID: taskID, Status: "healthy", URL: url, Port: lease.Port, CheckedAt: timePointer(time.Now().UTC())}, nil
}

func (h *Handler) saveQAEnvironment(ctx context.Context, projectID, taskID int64, status, url string, port *int) error {
	var portValue any
	if port != nil {
		portValue = *port
	}
	_, err := h.db.ExecContext(ctx, `INSERT INTO project_qa_environments(project_id,task_id,status,url,port,health_checked_at) VALUES($1,$2,$3,NULLIF($4,''),$5,CASE WHEN $3='healthy' THEN now() ELSE NULL END) ON CONFLICT(project_id) DO UPDATE SET task_id=EXCLUDED.task_id,status=EXCLUDED.status,url=EXCLUDED.url,port=EXCLUDED.port,health_checked_at=EXCLUDED.health_checked_at,updated_at=now()`, projectID, taskID, status, url, portValue)
	return err
}

func waitForQAHealth(ctx context.Context, port int, workspacePath string) (string, error) {
	descriptor, _, err := environments.LoadDescriptor(workspacePath)
	if err != nil {
		return "", err
	}
	path := "/"
	if descriptor.HealthCheck != nil {
		path = descriptor.HealthCheck.Path
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	client := http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			response, requestErr := client.Do(req)
			if requestErr == nil {
				response.Body.Close()
				if response.StatusCode >= 200 && response.StatusCode < 400 {
					return url, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func timePointer(value time.Time) *time.Time { return &value }

func (h *Handler) stopQAEnvironment(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var projectID int64
	var workspacePath string
	err := h.db.QueryRowContext(r.Context(), `SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=$1`, taskID).Scan(&projectID, &workspacePath)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read task", 500)
		return
	}
	if err = h.compose.Stop(r.Context(), projectID, workspacePath); err != nil {
		http.Error(w, "could not stop QA environment", http.StatusBadGateway)
		return
	}
	if err = h.ports.Release(r.Context(), projectID); err != nil {
		http.Error(w, "could not release QA port", 500)
		return
	}
	_, err = h.db.ExecContext(r.Context(), `UPDATE project_qa_environments SET status='stopped',url=NULL,updated_at=now() WHERE project_id=$1`, projectID)
	if err != nil {
		http.Error(w, "could not save QA environment", 500)
		return
	}
	writeJSON(w, http.StatusOK, qaEnvironment{ProjectID: projectID, TaskID: taskID, Status: "stopped"})
}

func (h *Handler) qaEnvironmentLogs(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var projectID int64
	var workspacePath string
	err := h.db.QueryRowContext(r.Context(), `SELECT t.project_id,p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=$1`, taskID).Scan(&projectID, &workspacePath)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "could not read task", 500)
		return
	}
	logs, err := h.compose.Logs(r.Context(), projectID, workspacePath, 200)
	if err != nil {
		http.Error(w, "could not collect Compose service logs: "+opencode.Redact(err.Error()), 502)
		return
	}
	writeJSON(w, 200, map[string]string{"logs": opencode.Redact(logs)})
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, name, description, requirements, workspace_path, created_by, created_at
		FROM projects ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		http.Error(w, "could not list projects", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	projects := make([]Project, 0)
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.Name, &project.Description, &project.Requirements, &project.WorkspacePath, &project.CreatedBy, &project.CreatedAt); err != nil {
			http.Error(w, "could not read projects", http.StatusInternalServerError)
			return
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read projects", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var input createProjectRequest
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Requirements = strings.TrimSpace(input.Requirements)
	if err := workspace.ValidateProjectName(input.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(input.Description) > 10000 || len(input.Requirements) > 20000 {
		http.Error(w, "project description or requirements exceed the allowed size", http.StatusBadRequest)
		return
	}
	var exists bool
	if err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM projects WHERE name = $1)`, input.Name).Scan(&exists); err != nil {
		http.Error(w, "could not check project name", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "a project with this name already exists", http.StatusConflict)
		return
	}
	projectPath, err := workspace.CreateProject(h.workspaceRoot, input.Name)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, "a project workspace with this name already exists", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actor, err := auth.CurrentUser(h.db, r)
	if err != nil {
		_ = os.Remove(projectPath)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var project Project
	err = h.db.QueryRowContext(r.Context(), `
		INSERT INTO projects (name, description, requirements, workspace_path, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, requirements, workspace_path, created_by, created_at
	`, input.Name, input.Description, input.Requirements, projectPath, actor.ID).Scan(
		&project.ID, &project.Name, &project.Description, &project.Requirements,
		&project.WorkspacePath, &project.CreatedBy, &project.CreatedAt,
	)
	if err != nil {
		_ = os.Remove(projectPath)
		http.Error(w, "could not create project", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	project, err := h.readProject(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read project", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var input updateProjectRequest
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Description == nil && input.Requirements == nil {
		http.Error(w, "description or requirements must be provided", http.StatusBadRequest)
		return
	}
	project, err := h.readProject(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read project", http.StatusInternalServerError)
		return
	}
	description, requirements := project.Description, project.Requirements
	if input.Description != nil {
		description = strings.TrimSpace(*input.Description)
	}
	if input.Requirements != nil {
		requirements = strings.TrimSpace(*input.Requirements)
	}
	if len(description) > 10000 || len(requirements) > 20000 {
		http.Error(w, "project description or requirements exceed the allowed size", http.StatusBadRequest)
		return
	}
	err = h.db.QueryRowContext(r.Context(), `
		UPDATE projects SET description = $2, requirements = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, name, description, requirements, workspace_path, created_by, created_at
	`, id, description, requirements).Scan(
		&project.ID, &project.Name, &project.Description, &project.Requirements,
		&project.WorkspacePath, &project.CreatedBy, &project.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not update project", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseID(w, r, "projectID")
	if !ok {
		return
	}
	var exists bool
	if err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&exists); err != nil {
		http.Error(w, "could not check project", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, project_id, title, description, state, specification_status, implementation_status,
		       qa_status, pr_status, checks_status, created_by, created_at
		FROM tasks WHERE project_id = $1 ORDER BY created_at, id
	`, projectID)
	if err != nil {
		http.Error(w, "could not list tasks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State,
			&task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus, &task.PRStatus,
			&task.ChecksStatus, &task.CreatedBy, &task.CreatedAt); err != nil {
			http.Error(w, "could not read tasks", http.StatusInternalServerError)
			return
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read tasks", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseID(w, r, "projectID")
	if !ok {
		return
	}
	var input createTaskRequest
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if err := validateTask(input.Title, input.Description); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	actor, err := auth.CurrentUser(h.db, r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	var task Task
	err = h.db.QueryRowContext(r.Context(), `
		INSERT INTO tasks (project_id, title, description, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, project_id, title, description, state, specification_status, implementation_status,
		          qa_status, pr_status, checks_status, created_by, created_at
	`, projectID, input.Title, input.Description, actor.ID).Scan(
		&task.ID, &task.ProjectID, &task.Title, &task.Description,
		&task.State, &task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus,
		&task.PRStatus, &task.ChecksStatus, &task.CreatedBy, &task.CreatedAt,
	)
	if err != nil {
		var projectExists bool
		if checkErr := h.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&projectExists); checkErr == nil && !projectExists {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "could not create task", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	task, err := h.readTask(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read task", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var input updateTaskRequest
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Title == nil && input.Description == nil {
		http.Error(w, "title or description must be provided", http.StatusBadRequest)
		return
	}
	task, err := h.readTask(r, id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read task", http.StatusInternalServerError)
		return
	}
	title, description := task.Title, task.Description
	if input.Title != nil {
		title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		description = strings.TrimSpace(*input.Description)
	}
	if err := validateTask(title, description); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = h.db.QueryRowContext(r.Context(), `
		UPDATE tasks SET title = $2, description = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, project_id, title, description, state, specification_status, implementation_status,
		          qa_status, pr_status, checks_status, created_by, created_at
	`, id, title, description).Scan(
		&task.ID, &task.ProjectID, &task.Title, &task.Description,
		&task.State, &task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus,
		&task.PRStatus, &task.ChecksStatus, &task.CreatedBy, &task.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not update task", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) applyTaskAction(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Action   workflow.Action `json:"action"`
		Comment  string          `json:"comment"`
		Evidence string          `json:"evidence"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	actor, err := auth.CurrentUser(h.db, r)
	if err != nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "could not start task transition", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var current Task
	var blockedFromState string
	var prNumber int
	var projectPath string
	err = tx.QueryRowContext(r.Context(), `
		SELECT t.id, t.project_id, t.title, t.description, t.state, t.specification_status, t.implementation_status,
		       t.qa_status, t.pr_status, t.checks_status, t.blocked_from_state, t.created_by, t.created_at,
	       COALESCE(t.pr_number,0), p.workspace_path
		FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id = $1 FOR UPDATE OF t
	`, id).Scan(&current.ID, &current.ProjectID, &current.Title, &current.Description, &current.State,
		&current.SpecificationStatus, &current.ImplementationStatus, &current.QAStatus, &current.PRStatus,
		&current.ChecksStatus, &blockedFromState, &current.CreatedBy, &current.CreatedAt, &prNumber, &projectPath)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read task", http.StatusInternalServerError)
		return
	}
	fromState := current.State
	if input.Action == workflow.ActionMerge {
		if !auth.Can(actor.Role, auth.ActionMerge) {
			http.Error(w, "your role cannot merge pull requests", http.StatusForbidden)
			return
		}
		if prNumber < 1 {
			http.Error(w, "pull request is not registered", http.StatusConflict)
			return
		}
		checks, checkErr := h.github.Checks(r.Context(), projectPath, prNumber)
		if checkErr != nil {
			http.Error(w, "could not verify required GitHub checks", http.StatusBadGateway)
			return
		}
		freshStatus := checkStatus(checks)
		if freshStatus != workflow.StatusPassed {
			action := workflow.ActionChecksPending
			if freshStatus == workflow.StatusChecksFailed {
				action = workflow.ActionChecksFailed
			}
			checkTask, applyErr := workflow.Apply(workflow.Task{State: workflow.State(current.State), SpecificationStatus: current.SpecificationStatus, ImplementationStatus: current.ImplementationStatus, QAStatus: current.QAStatus, PRStatus: current.PRStatus, ChecksStatus: current.ChecksStatus, BlockedFromState: blockedFromState}, action, "system", "", "")
			if applyErr == nil {
				_, _ = tx.ExecContext(r.Context(), `UPDATE tasks SET state=$2,checks_status=$3,updated_at=now() WHERE id=$1`, id, checkTask.State, checkTask.ChecksStatus)
				_ = appendTaskEvent(r.Context(), tx, id, actor.ID, "system", "transition", string(action), current.State, string(checkTask.State), "", "Required GitHub checks are not passing")
				_ = tx.Commit()
			}
			http.Error(w, "all required GitHub checks must pass before merge", http.StatusConflict)
			return
		}
		if current.State == string(workflow.StatePROpen) {
			checkTask, applyErr := workflow.Apply(workflow.Task{State: workflow.State(current.State), SpecificationStatus: current.SpecificationStatus, ImplementationStatus: current.ImplementationStatus, QAStatus: current.QAStatus, PRStatus: current.PRStatus, ChecksStatus: current.ChecksStatus, BlockedFromState: blockedFromState}, workflow.ActionChecksPassed, "system", "", "")
			if applyErr != nil {
				http.Error(w, "required check state could not be updated", http.StatusConflict)
				return
			}
			current.State = string(checkTask.State)
			current.ChecksStatus = checkTask.ChecksStatus
			if _, err := tx.ExecContext(r.Context(), `UPDATE tasks SET state=$2,checks_status=$3,updated_at=now() WHERE id=$1`, id, current.State, current.ChecksStatus); err != nil {
				http.Error(w, "could not save GitHub checks", 500)
				return
			}
			if err := appendTaskEvent(r.Context(), tx, id, actor.ID, "system", "transition", string(workflow.ActionChecksPassed), fromState, current.State, "", "Required GitHub checks passed"); err != nil {
				http.Error(w, "could not record GitHub checks", 500)
				return
			}
			fromState = current.State
		}
	}
	if input.Action == workflow.ActionStartQA || input.Action == workflow.ActionApproveQA {
		var healthy bool
		if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM project_qa_environments WHERE project_id=$1 AND task_id=$2 AND status='healthy' AND url IS NOT NULL)`, current.ProjectID, id).Scan(&healthy); err != nil {
			http.Error(w, "could not verify QA environment", http.StatusInternalServerError)
			return
		}
		if !healthy {
			if eventErr := appendTaskEvent(r.Context(), tx, id, actor.ID, actor.Role, "attempt", string(input.Action), fromState, fromState, "", "QA environment is not healthy"); eventErr != nil {
				http.Error(w, "could not record workflow attempt", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "could not record workflow attempt", http.StatusInternalServerError)
				return
			}
			http.Error(w, "QA environment must be healthy before review starts or is approved", http.StatusConflict)
			return
		}
	}
	updated, err := workflow.Apply(workflow.Task{
		State: workflow.State(current.State), SpecificationStatus: current.SpecificationStatus,
		ImplementationStatus: current.ImplementationStatus, QAStatus: current.QAStatus,
		PRStatus: current.PRStatus, ChecksStatus: current.ChecksStatus, BlockedFromState: blockedFromState,
	}, input.Action, actor.Role, input.Comment, input.Evidence)
	if err != nil {
		if eventErr := appendTaskEvent(r.Context(), tx, id, actor.ID, actor.Role, "attempt", string(input.Action), fromState, fromState, "", err.Error()); eventErr != nil {
			http.Error(w, "could not record workflow attempt", http.StatusInternalServerError)
			return
		}
		if commitErr := tx.Commit(); commitErr != nil {
			http.Error(w, "could not record workflow attempt", http.StatusInternalServerError)
			return
		}
		if errors.Is(err, workflow.ErrForbidden) {
			http.Error(w, "your role cannot perform this workflow action", http.StatusForbidden)
			return
		}
		if errors.Is(err, workflow.ErrTransition) {
			http.Error(w, "workflow gate is not satisfied for this action", http.StatusConflict)
			return
		}
		http.Error(w, "unknown workflow action", http.StatusBadRequest)
		return
	}
	if input.Action == workflow.ActionStartCorrection {
		var hasQADecision bool
		if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM task_events WHERE task_id=$1 AND action=$2 AND event_type='approval')`, id, string(workflow.ActionRejectQA)).Scan(&hasQADecision); err != nil {
			http.Error(w, "could not verify QA correction feedback", http.StatusInternalServerError)
			return
		}
		if !hasQADecision {
			http.Error(w, "QA rejection feedback is required before starting a correction", http.StatusConflict)
			return
		}
	}
	if input.Action == workflow.ActionMerge {
		if err := h.github.MergePullRequest(r.Context(), projectPath, prNumber); err != nil {
			_ = appendTaskEvent(r.Context(), tx, id, actor.ID, "system", "failure", string(workflow.ActionMerge), current.State, current.State, "", "GitHub did not complete the pull request merge")
			_ = tx.Commit()
			http.Error(w, "GitHub did not complete the pull request merge", http.StatusBadGateway)
			return
		}
	}
	markReadyAction := input.Action == workflow.ActionMarkReady
	startSpecification := input.Action == workflow.ActionStartSpecification
	startImplementation := input.Action == workflow.ActionStartImplementation || input.Action == workflow.ActionStartCorrection
	approveSpecification := input.Action == workflow.ActionApproveSpecification
	var agentRunID int64
	readyState := updated.State
	if markReadyAction {
		updated, err = workflow.Apply(updated, workflow.ActionStartSpecification, "system", "", "")
		if err != nil {
			http.Error(w, "could not start specification generation", http.StatusInternalServerError)
			return
		}
		startSpecification = true
	}
	if approveSpecification {
		updated, err = workflow.Apply(updated, workflow.ActionStartImplementation, "system", "", "")
		if err != nil {
			http.Error(w, "could not start approved implementation", http.StatusInternalServerError)
			return
		}
		startImplementation = true
	}
	err = tx.QueryRowContext(r.Context(), `
		UPDATE tasks SET state = $2, specification_status = $3, implementation_status = $4,
		       qa_status = $5, pr_status = $6, checks_status = $7, blocked_from_state = $8, updated_at = now()
		WHERE id = $1
		RETURNING id, project_id, title, description, state, specification_status, implementation_status,
		          qa_status, pr_status, checks_status, created_by, created_at
	`, id, string(updated.State), updated.SpecificationStatus, updated.ImplementationStatus,
		updated.QAStatus, updated.PRStatus, updated.ChecksStatus, updated.BlockedFromState).Scan(
		&current.ID, &current.ProjectID, &current.Title, &current.Description, &current.State,
		&current.SpecificationStatus, &current.ImplementationStatus, &current.QAStatus, &current.PRStatus,
		&current.ChecksStatus, &current.CreatedBy, &current.CreatedAt,
	)
	if err != nil {
		http.Error(w, "could not save task transition", http.StatusInternalServerError)
		return
	}
	if input.Action == workflow.ActionApproveSpecification || input.Action == workflow.ActionReturnSpecification {
		status := "approved"
		if input.Action == workflow.ActionReturnSpecification {
			status = "changes_requested"
		}
		result, statusErr := tx.ExecContext(r.Context(), `UPDATE task_specifications SET status=$2 WHERE id=(SELECT id FROM task_specifications WHERE task_id=$1 ORDER BY round DESC LIMIT 1) AND status='generated'`, id, status)
		if statusErr != nil {
			http.Error(w, "could not save specification decision", http.StatusInternalServerError)
			return
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			http.Error(w, "no generated specification is available for review", http.StatusConflict)
			return
		}
	}
	toState := current.State
	if markReadyAction {
		toState = string(readyState)
	}
	if approveSpecification {
		if err := appendTaskEvent(r.Context(), tx, id, actor.ID, "system", "transition", string(workflow.ActionStartImplementation), string(workflow.StateInDevelopment), string(workflow.StateInDevelopment), "", ""); err != nil {
			http.Error(w, "could not record implementation start", http.StatusInternalServerError)
			return
		}
	}
	eventEvidence := ""
	if input.Action == workflow.ActionApproveQA || input.Action == workflow.ActionRejectQA {
		eventEvidence = input.Evidence
	}
	if err := appendTaskEvent(r.Context(), tx, id, actor.ID, actor.Role, eventType(input.Action), string(input.Action), fromState, toState, input.Comment, eventEvidence); err != nil {
		http.Error(w, "could not record task transition", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(input.Comment) != "" {
		commentToState := current.State
		if markReadyAction {
			commentToState = string(readyState)
		}
		if err := appendTaskEvent(r.Context(), tx, id, actor.ID, actor.Role, "comment", string(input.Action), fromState, commentToState, input.Comment, ""); err != nil {
			http.Error(w, "could not record task comment", http.StatusInternalServerError)
			return
		}
	}
	if startSpecification {
		if err := appendTaskEvent(r.Context(), tx, id, actor.ID, "system", "transition", string(workflow.ActionStartSpecification), string(workflow.StateReady), string(updated.State), "", ""); err != nil {
			http.Error(w, "could not record specification start", http.StatusInternalServerError)
			return
		}
	}
	if startSpecification || startImplementation {
		phase := "specification"
		if startImplementation {
			phase = "implementation"
		}
		agentRunID, _, err = createAgentRun(r.Context(), tx, id, phase)
		if err != nil {
			http.Error(w, "could not register agent execution", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "could not commit task transition", http.StatusInternalServerError)
		return
	}
	if startSpecification {
		go h.generateSpecification(id, agentRunID)
	}
	if startImplementation {
		go h.generateImplementation(id, agentRunID)
	}
	writeJSON(w, http.StatusOK, current)
}

func (h *Handler) generateSpecification(taskID, runID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var task Task
	var requirements, projectPath string
	var createdBy int64
	if err := h.db.QueryRowContext(ctx, `
		SELECT t.id, t.project_id, t.title, t.description, t.state, p.requirements, p.workspace_path,
	       t.specification_status, t.implementation_status, t.qa_status, t.pr_status, t.checks_status, t.created_by, t.created_at
		FROM tasks t JOIN projects p ON p.id = t.project_id WHERE t.id = $1
	`, taskID).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State, &requirements,
		&projectPath, &task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus,
		&task.PRStatus, &task.ChecksStatus, &createdBy, &task.CreatedAt); err != nil {
		return
	}
	if task.State != string(workflow.StateSpecifying) {
		h.updateAgentRun(context.Background(), runID, "cancelled", "Specification run was no longer active", opencode.Result{}, nil)
		return
	}
	prompt := fmt.Sprintf("Gere somente os artefatos de especificação OpenSpec para a tarefa abaixo. Não implemente código, não altere arquivos de aplicação e não inicie QA. Grave os artefatos em openspec/changes/task-%d/ com proposal.md, design.md, tasks.md e pelo menos uma especificação em specs/<capacidade>/spec.md. Inclua cenários de aceitação verificáveis e mantenha o escopo restrito à tarefa.\n\nProjeto: %s\nRequisitos do projeto: %s\nTarefa: %s\nDescrição: %s", task.ID, task.Title, requirements, task.Title, task.Description)
	result, runErr := h.opencode.Run(ctx, opencode.Request{WorkspaceRoot: h.workspaceRoot, ProjectDir: projectPath, Prompt: prompt, OnStart: func(pid int, token string) error {
		_, err := h.db.ExecContext(ctx, `UPDATE agent_runs SET process_pid=$2, process_start_token=$3 WHERE id=$1 AND status='running'`, runID, pid, token)
		return err
	}})
	artifactPaths, artifactErr := collectSpecificationArtifacts(projectPath, task.ID)
	if runErr == nil && (artifactErr != nil || !usableSpecification(artifactPaths, task.ID)) {
		runErr = fmt.Errorf("OpenCode completed without a usable OpenSpec artifact set: %v", artifactErr)
	}
	status := "generated"
	evidence := strings.Join(artifactPaths, ", ")
	if runErr != nil {
		status = "failed"
	}
	pathsJSON, _ := json.Marshal(artifactPaths)
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var round int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(round), 0) + 1 FROM task_specifications WHERE task_id = $1`, taskID).Scan(&round); err != nil {
		return
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_specifications (task_id, round, status, artifact_paths, summary, run_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		taskID, round, status, pathsJSON, map[bool]string{true: "OpenSpec artifacts generated", false: "Specification generation failed"}[runErr == nil], runID); err != nil {
		return
	}
	runStatus, runSummary := "succeeded", "OpenSpec artifacts generated"
	if runErr != nil {
		runStatus = "failed"
		runSummary = "Specification generation failed: " + opencode.Redact(runErr.Error())
	}
	if err := completeAgentRun(ctx, tx, runID, runStatus, runSummary, result, artifactPaths); err != nil {
		return
	}
	action := workflow.ActionSpecificationReady
	comment := ""
	if runErr != nil {
		action = workflow.ActionSpecificationFailed
		comment = "OpenCode specification generation failed: " + opencode.Redact(runErr.Error())
	}
	if err := h.applySystemTaskAction(ctx, tx, taskID, createdBy, action, comment, evidence); err != nil {
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}
}

func (h *Handler) applySystemTaskAction(ctx context.Context, tx *sql.Tx, taskID, actorID int64, action workflow.Action, comment, evidence string) error {
	var task Task
	var blockedFromState string
	if err := tx.QueryRowContext(ctx, `SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, blocked_from_state, created_by, created_at FROM tasks WHERE id = $1 FOR UPDATE`, taskID).Scan(
		&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State, &task.SpecificationStatus,
		&task.ImplementationStatus, &task.QAStatus, &task.PRStatus, &task.ChecksStatus, &blockedFromState, &task.CreatedBy, &task.CreatedAt); err != nil {
		return err
	}
	from := task.State
	updated, err := workflow.Apply(workflow.Task{State: workflow.State(task.State), SpecificationStatus: task.SpecificationStatus,
		ImplementationStatus: task.ImplementationStatus, QAStatus: task.QAStatus, PRStatus: task.PRStatus,
		ChecksStatus: task.ChecksStatus, BlockedFromState: blockedFromState}, action, "system", comment, evidence)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET state=$2, specification_status=$3, implementation_status=$4, qa_status=$5, pr_status=$6, checks_status=$7, blocked_from_state=$8, updated_at=now() WHERE id=$1`,
		taskID, updated.State, updated.SpecificationStatus, updated.ImplementationStatus, updated.QAStatus, updated.PRStatus, updated.ChecksStatus, updated.BlockedFromState); err != nil {
		return err
	}
	return appendTaskEvent(ctx, tx, taskID, actorID, "system", map[bool]string{true: "failure", false: "transition"}[action == workflow.ActionSpecificationFailed], string(action), from, string(updated.State), comment, "")
}

func (h *Handler) generateImplementation(taskID, runID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var task Task
	var projectPath string
	if err := h.db.QueryRowContext(ctx, `SELECT t.id, t.project_id, t.title, t.description, t.state, t.specification_status, t.implementation_status, t.qa_status, t.pr_status, t.checks_status, t.created_by, t.created_at, p.workspace_path FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.id=$1`, taskID).Scan(
		&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State, &task.SpecificationStatus, &task.ImplementationStatus,
		&task.QAStatus, &task.PRStatus, &task.ChecksStatus, &task.CreatedBy, &task.CreatedAt, &projectPath); err != nil || task.State != string(workflow.StateInDevelopment) || task.ImplementationStatus != workflow.StatusRunning {
		h.updateAgentRun(context.Background(), runID, "cancelled", "Implementation run was no longer active", opencode.Result{}, nil)
		return
	}
	var rawPaths []byte
	if err := h.db.QueryRowContext(ctx, `SELECT artifact_paths FROM task_specifications WHERE task_id=$1 AND status='approved' ORDER BY round DESC LIMIT 1`, taskID).Scan(&rawPaths); err != nil {
		h.persistImplementationResult(ctx, task, runID, nil, "failed", "approved specification artifacts are unavailable", opencode.Result{})
		return
	}
	var paths []string
	if err := json.Unmarshal(rawPaths, &paths); err != nil {
		h.persistImplementationResult(ctx, task, runID, nil, "failed", "approved specification artifact references are invalid", opencode.Result{})
		return
	}
	artifactContent, err := readSpecificationArtifacts(projectPath, paths)
	if err != nil {
		h.persistImplementationResult(ctx, task, runID, nil, "failed", "approved specification artifacts could not be read", opencode.Result{})
		return
	}
	qaFeedback := ""
	var qaComment, qaEvidence string
	feedbackErr := h.db.QueryRowContext(ctx, `SELECT comment,reason FROM task_events WHERE task_id=$1 AND action=$2 AND event_type='approval' ORDER BY id DESC LIMIT 1`, taskID, string(workflow.ActionRejectQA)).Scan(&qaComment, &qaEvidence)
	if feedbackErr != nil && !errors.Is(feedbackErr, sql.ErrNoRows) {
		h.persistImplementationResult(ctx, task, runID, nil, "failed", "QA correction feedback could not be read", opencode.Result{})
		return
	}
	if feedbackErr == nil {
		qaFeedback = fmt.Sprintf("\n\nQA correction recommendations from the latest rejected review:\nComment: %s\nEvidence: %s\nAddress these recommendations while staying within the approved specification scope.", qaComment, qaEvidence)
	}
	before, err := opencode.CaptureSnapshot(projectPath)
	if err != nil {
		h.persistImplementationResult(ctx, task, runID, nil, "failed", "project workspace could not be inspected before implementation", opencode.Result{})
		return
	}
	prompt := fmt.Sprintf("Implemente a tarefa aprovada a seguir no projeto. Siga os artefatos OpenSpec aprovados e não altere escopo fora deles. Preserve alterações pré-existentes, não modifique artefatos de especificação e não tente executar QA, criar PR ou fazer merge. Para bancos em Docker Compose, conecte a aplicação ao nome do serviço e à porta interna; não publique a porta padrão do banco no host. Só publique o banco no host quando os requisitos pedirem acesso local, usando porta configurável vinculada somente a 127.0.0.1. O serviço de QA deve expor a porta interna indicada pelo descritor, sem fixar porta de host.\n\nTarefa: %s\nDescrição: %s\n\nArtefatos aprovados:\n%s%s", task.Title, task.Description, artifactContent, qaFeedback)
	result, runErr := h.opencode.Run(ctx, opencode.Request{WorkspaceRoot: h.workspaceRoot, ProjectDir: projectPath, Prompt: prompt, OnStart: func(pid int, token string) error {
		_, err := h.db.ExecContext(ctx, `UPDATE agent_runs SET process_pid=$2, process_start_token=$3 WHERE id=$1 AND status='running'`, runID, pid, token)
		return err
	}})
	after, snapshotErr := opencode.CaptureSnapshot(projectPath)
	changed := []string(nil)
	if snapshotErr == nil {
		changed = opencode.ChangedFiles(before, after)
	}
	if snapshotErr != nil && runErr == nil {
		runErr = snapshotErr
	}
	status := "succeeded"
	if runErr != nil || len(changed) == 0 {
		status = "failed"
	}
	if status == "succeeded" {
		h.persistImplementationResult(ctx, task, runID, changed, status, "Implementation completed with changed files", result)
		return
	}
	message := "OpenCode produced no verifiable file changes"
	if runErr != nil {
		message = "OpenCode implementation failed: " + opencode.Redact(runErr.Error())
	}
	h.persistImplementationResult(ctx, task, runID, changed, status, message, result)
}

func (h *Handler) persistImplementationResult(ctx context.Context, task Task, runID int64, changed []string, status, summary string, result opencode.Result) {
	pathsJSON, _ := json.Marshal(changed)
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()
	var attempt int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt),0)+1 FROM task_implementations WHERE task_id=$1`, task.ID).Scan(&attempt); err != nil {
		return
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_implementations(task_id,attempt,status,changed_files,summary,run_id) VALUES($1,$2,$3,$4,$5,$6)`, task.ID, attempt, status, pathsJSON, summary, runID); err != nil {
		return
	}
	runStatus := status
	if err := completeAgentRun(ctx, tx, runID, runStatus, summary, result, changed); err != nil {
		return
	}
	action, comment, evidence := workflow.ActionImplementationDone, "", strings.Join(changed, ", ")
	if status != "succeeded" {
		action, comment, evidence = workflow.ActionImplementationError, summary, ""
	}
	if err := h.applySystemTaskAction(ctx, tx, task.ID, task.CreatedBy, action, comment, evidence); err != nil {
		return
	}
	if status == "succeeded" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_qa_environments(project_id,task_id,status,url,port) VALUES($1,$2,'starting',NULL,NULL) ON CONFLICT(project_id) DO UPDATE SET task_id=EXCLUDED.task_id,status='starting',url=NULL,port=NULL,health_checked_at=NULL,updated_at=now()`, task.ProjectID, task.ID); err != nil {
			return
		}
	}
	if err := tx.Commit(); err != nil {
		return
	}
	if status != "succeeded" {
		return
	}
	startCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	_, environmentErr := h.startQAEnvironmentForTask(startCtx, task.ID, "start")
	cancel()
	if environmentErr != nil {
		eventCtx, eventCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = h.recordSystemTaskEvent(eventCtx, task.ID, task.CreatedBy, "failure", "qa_environment_start_failed", string(workflow.StateWaitingQA), string(workflow.StateWaitingQA), opencode.Redact(environmentErr.Error()))
		eventCancel()
	}
}

func readSpecificationArtifacts(projectPath string, paths []string) (string, error) {
	projectRoot, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		return "", err
	}
	var content strings.Builder
	for _, relative := range paths {
		clean := filepath.Clean(filepath.FromSlash(relative))
		if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("invalid artifact path")
		}
		artifact, err := filepath.EvalSymlinks(filepath.Join(projectRoot, clean))
		if err != nil || !opencode.IsWithin(projectRoot, artifact) {
			return "", fmt.Errorf("artifact path is unavailable")
		}
		info, err := os.Stat(artifact)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
			return "", fmt.Errorf("artifact cannot be loaded")
		}
		data, err := os.ReadFile(artifact)
		if err != nil {
			return "", err
		}
		content.WriteString("\n--- ")
		content.WriteString(filepath.ToSlash(clean))
		content.WriteString(" ---\n")
		content.Write(data)
		if content.Len() > 1<<20 {
			return "", fmt.Errorf("approved specification exceeds the prompt limit")
		}
	}
	if content.Len() == 0 {
		return "", fmt.Errorf("no approved specification artifacts")
	}
	return content.String(), nil
}

func collectSpecificationArtifacts(projectPath string, taskID int64) ([]string, error) {
	base := filepath.Join(projectPath, "openspec", "changes", fmt.Sprintf("task-%d", taskID))
	var paths []string
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(projectPath, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	return paths, err
}

func usableSpecification(paths []string, taskID int64) bool {
	found := map[string]bool{}
	for _, path := range paths {
		if path == fmt.Sprintf("openspec/changes/task-%d/proposal.md", taskID) {
			found["proposal"] = true
		}
		if strings.HasSuffix(path, "/design.md") {
			found["design"] = true
		}
		if strings.HasSuffix(path, "/tasks.md") {
			found["tasks"] = true
		}
		if strings.Contains(path, "/specs/") && strings.HasSuffix(path, "/spec.md") {
			found["spec"] = true
		}
	}
	return found["proposal"] && found["design"] && found["tasks"] && found["spec"]
}

func (h *Handler) listTaskEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	var exists bool
	if err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM tasks WHERE id = $1)`, id).Scan(&exists); err != nil {
		http.Error(w, "could not check task", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT e.id, e.task_id, e.actor_id, COALESCE(u.display_name, 'Conta removida'), e.actor_role,
		       e.event_type, e.action, e.from_state, e.to_state, e.comment, e.reason, e.occurred_at
		FROM task_events e LEFT JOIN users u ON u.id = e.actor_id
		WHERE e.task_id = $1 ORDER BY e.id
	`, id)
	if err != nil {
		http.Error(w, "could not read task history", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	events := make([]TaskEvent, 0)
	for rows.Next() {
		var event TaskEvent
		if err := rows.Scan(&event.ID, &event.TaskID, &event.ActorID, &event.ActorName, &event.ActorRole, &event.Type, &event.Action,
			&event.FromState, &event.ToState, &event.Comment, &event.Reason, &event.OccurredAt); err != nil {
			http.Error(w, "could not read task history", http.StatusInternalServerError)
			return
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read task history", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) listTaskSpecifications(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, task_id, round, status, artifact_paths, summary, created_at FROM task_specifications WHERE task_id=$1 ORDER BY round`, id)
	if err != nil {
		http.Error(w, "could not read task specifications", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := make([]TaskSpecification, 0)
	for rows.Next() {
		var item TaskSpecification
		var paths []byte
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Round, &item.Status, &paths, &item.Summary, &item.CreatedAt); err != nil {
			http.Error(w, "could not read task specifications", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(paths, &item.ArtifactPaths); err != nil {
			http.Error(w, "stored specification paths are invalid", http.StatusInternalServerError)
			return
		}
		if item.ArtifactPaths == nil {
			item.ArtifactPaths = []string{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read task specifications", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) getTaskSpecificationArtifact(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	round, err := strconv.Atoi(r.PathValue("round"))
	if err != nil || round < 1 {
		http.Error(w, "invalid specification round", http.StatusBadRequest)
		return
	}
	relative := filepath.Clean(filepath.FromSlash(r.URL.Query().Get("path")))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		http.Error(w, "invalid artifact path", http.StatusBadRequest)
		return
	}
	var rawPaths []byte
	var projectPath string
	err = h.db.QueryRowContext(r.Context(), `SELECT s.artifact_paths, p.workspace_path FROM task_specifications s JOIN tasks t ON t.id=s.task_id JOIN projects p ON p.id=t.project_id WHERE s.task_id=$1 AND s.round=$2`, taskID, round).Scan(&rawPaths, &projectPath)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "specification not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read specification", http.StatusInternalServerError)
		return
	}
	var paths []string
	if err := json.Unmarshal(rawPaths, &paths); err != nil {
		http.Error(w, "stored artifact paths are invalid", http.StatusInternalServerError)
		return
	}
	requested := filepath.ToSlash(relative)
	allowed := false
	for _, path := range paths {
		if path == requested {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "artifact is not associated with this specification", http.StatusNotFound)
		return
	}
	projectRoot, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		http.Error(w, "project workspace is unavailable", http.StatusNotFound)
		return
	}
	artifactPath, err := filepath.EvalSymlinks(filepath.Join(projectRoot, relative))
	if err != nil {
		http.Error(w, "artifact is unavailable", http.StatusNotFound)
		return
	}
	inside, err := filepath.Rel(projectRoot, artifactPath)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		http.Error(w, "artifact path is outside the project", http.StatusForbidden)
		return
	}
	info, err := os.Stat(artifactPath)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "artifact is unavailable", http.StatusNotFound)
		return
	}
	if info.Size() > 256<<10 {
		http.Error(w, "artifact exceeds the preview size limit", http.StatusRequestEntityTooLarge)
		return
	}
	content, err := os.ReadFile(artifactPath)
	if err != nil {
		http.Error(w, "could not read artifact", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": requested, "content": string(content)})
}

func (h *Handler) listTaskImplementations(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, task_id, attempt, status, changed_files, summary, created_at FROM task_implementations WHERE task_id=$1 ORDER BY attempt`, id)
	if err != nil {
		http.Error(w, "could not read implementation results", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := make([]TaskImplementation, 0)
	for rows.Next() {
		var item TaskImplementation
		var changed []byte
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Attempt, &item.Status, &changed, &item.Summary, &item.CreatedAt); err != nil {
			http.Error(w, "could not read implementation results", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(changed, &item.ChangedFiles); err != nil {
			http.Error(w, "stored implementation paths are invalid", http.StatusInternalServerError)
			return
		}
		if item.ChangedFiles == nil {
			item.ChangedFiles = []string{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read implementation results", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func appendTaskEvent(ctx context.Context, tx *sql.Tx, taskID, actorID int64, actorRole, eventType, action, fromState, toState, comment, reason string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO task_events (task_id, actor_id, actor_role, event_type, action, from_state, to_state, comment, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, taskID, actorID, actorRole, eventType, action, fromState, toState, strings.TrimSpace(comment), reason)
	return err
}

func createAgentRun(ctx context.Context, tx *sql.Tx, taskID int64, phase string) (int64, int, error) {
	var attempt int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt),0)+1 FROM agent_runs WHERE task_id=$1 AND phase=$2`, taskID, phase).Scan(&attempt); err != nil {
		return 0, 0, err
	}
	var runID int64
	err := tx.QueryRowContext(ctx, `INSERT INTO agent_runs(task_id,phase,attempt,status) VALUES($1,$2,$3,'running') RETURNING id`, taskID, phase, attempt).Scan(&runID)
	return runID, attempt, err
}

func completeAgentRun(ctx context.Context, tx *sql.Tx, runID int64, status, summary string, result opencode.Result, refs []string) error {
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET status=$2, summary=$3, stdout_redacted=$4, stderr_redacted=$5, result_refs=$6, process_pid=0, process_start_token='', finished_at=now() WHERE id=$1 AND status='running'`,
		runID, status, opencode.Redact(summary), opencode.Redact(result.Stdout), opencode.Redact(result.Stderr), refsJSON)
	return err
}

func (h *Handler) updateAgentRun(ctx context.Context, runID int64, status, summary string, result opencode.Result, refs []string) {
	refsJSON, _ := json.Marshal(refs)
	_, _ = h.db.ExecContext(ctx, `UPDATE agent_runs SET status=$2, summary=$3, stdout_redacted=$4, stderr_redacted=$5, result_refs=$6, process_pid=0, process_start_token='', finished_at=now() WHERE id=$1 AND status='running'`,
		runID, status, opencode.Redact(summary), opencode.Redact(result.Stdout), opencode.Redact(result.Stderr), refsJSON)
}

func (h *Handler) listAgentRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, task_id, phase, attempt, status, summary, result_refs, started_at, finished_at FROM agent_runs WHERE task_id=$1 ORDER BY id`, id)
	if err != nil {
		http.Error(w, "could not read agent runs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	runs := make([]AgentRun, 0)
	for rows.Next() {
		var run AgentRun
		var rawRefs []byte
		var finished sql.NullTime
		if err := rows.Scan(&run.ID, &run.TaskID, &run.Phase, &run.Attempt, &run.Status, &run.Summary, &rawRefs, &run.StartedAt, &finished); err != nil {
			http.Error(w, "could not read agent runs", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(rawRefs, &run.ResultRefs); err != nil {
			http.Error(w, "stored run references are invalid", http.StatusInternalServerError)
			return
		}
		if finished.Valid {
			run.FinishedAt = &finished.Time
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read agent runs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func eventType(action workflow.Action) string {
	switch action {
	case workflow.ActionApproveSpecification, workflow.ActionReturnSpecification,
		workflow.ActionApproveQA, workflow.ActionRejectQA:
		return "approval"
	case workflow.ActionImplementationError, workflow.ActionChecksFailed:
		return "failure"
	default:
		return "transition"
	}
}

func (h *Handler) readProject(r *http.Request, id int64) (Project, error) {
	var project Project
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, name, description, requirements, workspace_path, created_by, created_at
		FROM projects WHERE id = $1
	`, id).Scan(&project.ID, &project.Name, &project.Description, &project.Requirements, &project.WorkspacePath, &project.CreatedBy, &project.CreatedAt)
	return project, err
}

func (h *Handler) readTask(r *http.Request, id int64) (Task, error) {
	var task Task
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, created_by, created_at
		FROM tasks WHERE id = $1
	`, id).Scan(&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State,
		&task.SpecificationStatus, &task.ImplementationStatus, &task.QAStatus, &task.PRStatus,
		&task.ChecksStatus, &task.CreatedBy, &task.CreatedAt)
	return task, err
}

func parseID(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, fmt.Sprintf("invalid %s", key), http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func validateTask(title, description string) error {
	if title == "" || len(title) > 200 {
		return fmt.Errorf("task title must contain between 1 and 200 characters")
	}
	if len(description) > 10000 {
		return fmt.Errorf("task description exceeds the allowed size")
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

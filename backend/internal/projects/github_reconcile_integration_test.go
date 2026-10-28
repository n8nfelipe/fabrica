package projects

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fabrica/backend/internal/database"
	"fabrica/backend/internal/githubcli"
)

func TestGitHubReconciliationCompletesMergedPullRequest(t *testing.T) {
	databaseURL := os.Getenv("FABRICA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set FABRICA_TEST_DATABASE_URL to run GitHub reconciliation integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	userID, projectID, taskID := createReconcileFixture(t, ctx, db)
	root := t.TempDir()
	if _, err := db.ExecContext(ctx, `UPDATE projects SET workspace_path=$2 WHERE id=$1`, projectID, root); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "fake-gh")
	script := `#!/bin/sh
case "$1 $2" in
  'auth status') exit 0 ;;
  'pr view') printf '%s\n' '{"state":"MERGED","url":"https://github.com/acme/demo/pull/42","number":42}' ;;
  'pr checks') printf '%s\n' '[{"name":"CI","state":"SUCCESS","link":"https://github.com/acme/demo/actions","workflow":"CI"}]' ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE tasks SET state='PR aberto',qa_status='approved',pr_status='open',checks_status='pending',pr_number=42 WHERE id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	h := &Handler{db: db, workspaceRoot: root, github: githubcli.Client{Binary: binary}}
	if err := h.github.CheckAuth(ctx, root); err != nil {
		t.Fatalf("fake gh auth failed: %v", err)
	}
	if state, err := h.github.ViewPullRequest(ctx, root, 42); err != nil || state.State != "MERGED" {
		t.Fatalf("fake PR state = %#v, %v", state, err)
	}
	if checks, err := h.github.Checks(ctx, root, 42); err != nil || checkStatus(checks) != "passed" {
		t.Fatalf("fake checks = %#v, %v", checks, err)
	}
	if err := h.ReconcileGitHubPullRequests(ctx); err != nil {
		t.Fatal(err)
	}
	var state, prStatus, checks string
	if err := db.QueryRowContext(ctx, `SELECT state,pr_status,checks_status FROM tasks WHERE id=$1`, taskID).Scan(&state, &prStatus, &checks); err != nil {
		t.Fatal(err)
	}
	if state != "Concluída" || prStatus != "merged" || checks != "passed" {
		t.Fatalf("reconciled task state=%s PR=%s checks=%s", state, prStatus, checks)
	}
	_ = userID
	_ = projectID
}

func createReconcileFixture(t *testing.T, ctx context.Context, db *sql.DB) (int64, int64, int64) {
	t.Helper()
	suffix := time.Now().UnixNano()
	var userID, projectID, taskID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO users(username,display_name,password_hash,role) VALUES($1,'Reconcile test','unused','Admin') RETURNING id`, fmt.Sprintf("reconcile-%d", suffix)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO projects(name,description,requirements,workspace_path,created_by) VALUES($1,'','',$2,$3) RETURNING id`, fmt.Sprintf("reconcile-%d", suffix), fmt.Sprintf("/tmp/reconcile-%d", suffix), userID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO tasks(project_id,title,description,created_by) VALUES($1,'Reconcile PR','',$2) RETURNING id`, projectID, userID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	return userID, projectID, taskID
}

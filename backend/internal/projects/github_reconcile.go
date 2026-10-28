package projects

import (
	"context"
	"database/sql"
	"strings"

	"fabrica/backend/internal/workflow"
)

type pullRequestRecord struct {
	taskID, actorID  int64
	number           int
	workspace, state string
}

func ReconcileGitHubPullRequests(ctx context.Context, db *sql.DB, workspaceRoot string) error {
	h := &Handler{db: db, workspaceRoot: workspaceRoot}
	return h.ReconcileGitHubPullRequests(ctx)
}

// ReconcileGitHubPullRequests refreshes persisted PR/check state from gh after restart.
func (h *Handler) ReconcileGitHubPullRequests(ctx context.Context) error {
	if err := h.github.CheckAuth(ctx, h.workspaceRoot); err != nil {
		return nil
	}
	rows, err := h.db.QueryContext(ctx, `SELECT t.id,t.created_by,t.pr_number,p.workspace_path,t.state FROM tasks t JOIN projects p ON p.id=t.project_id WHERE t.pr_number IS NOT NULL AND t.pr_status='open'`)
	if err != nil {
		return err
	}
	records := make([]pullRequestRecord, 0)
	for rows.Next() {
		var item pullRequestRecord
		if err := rows.Scan(&item.taskID, &item.actorID, &item.number, &item.workspace, &item.state); err != nil {
			rows.Close()
			return err
		}
		records = append(records, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range records {
		state, err := h.github.ViewPullRequest(ctx, item.workspace, item.number)
		if err != nil {
			continue
		}
		checks, err := h.github.Checks(ctx, item.workspace, item.number)
		if err != nil {
			continue
		}
		status := checkStatus(checks)
		action := workflow.ActionChecksPending
		if status == workflow.StatusPassed {
			action = workflow.ActionChecksPassed
		} else if status == workflow.StatusChecksFailed {
			action = workflow.ActionChecksFailed
		}
		if err := h.commitSystemTaskAction(ctx, item.taskID, item.actorID, action, "", ""); err != nil {
			continue
		}
		if strings.EqualFold(state.State, "MERGED") && status == workflow.StatusPassed {
			_ = h.commitSystemTaskAction(ctx, item.taskID, item.actorID, workflow.ActionMerge, "", "reconciled from GitHub")
		}
	}
	return nil
}

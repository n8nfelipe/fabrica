package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"fabrica/backend/internal/opencode"
	"fabrica/backend/internal/workflow"
)

type staleRun struct {
	ID           int64
	TaskID       int64
	Phase        string
	Attempt      int
	PID          int
	ProcessToken string
}

// ReconcileAgentRuns marks work interrupted by an API restart as retryable and
// terminates its process group when the persisted process identity still matches.
func ReconcileAgentRuns(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT id, task_id, phase, attempt, process_pid, process_start_token FROM agent_runs WHERE status='running' ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list interrupted agent runs: %w", err)
	}
	var stale []staleRun
	for rows.Next() {
		var run staleRun
		if err := rows.Scan(&run.ID, &run.TaskID, &run.Phase, &run.Attempt, &run.PID, &run.ProcessToken); err != nil {
			rows.Close()
			return fmt.Errorf("read interrupted agent run: %w", err)
		}
		stale = append(stale, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read interrupted agent runs: %w", err)
	}
	rows.Close()
	for _, run := range stale {
		if run.PID > 0 {
			_ = opencode.TerminateProcessGroup(run.PID, run.ProcessToken)
		}
		if err := reconcileRun(ctx, db, run); err != nil {
			return fmt.Errorf("reconcile agent run %d: %w", run.ID, err)
		}
	}
	return nil
}

func reconcileRun(ctx context.Context, db *sql.DB, run staleRun) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task Task
	var blockedFrom string
	err = tx.QueryRowContext(ctx, `SELECT id, project_id, title, description, state, specification_status, implementation_status, qa_status, pr_status, checks_status, blocked_from_state, created_by, created_at FROM tasks WHERE id=$1 FOR UPDATE`, run.TaskID).Scan(
		&task.ID, &task.ProjectID, &task.Title, &task.Description, &task.State, &task.SpecificationStatus,
		&task.ImplementationStatus, &task.QAStatus, &task.PRStatus, &task.ChecksStatus, &blockedFrom, &task.CreatedBy, &task.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_runs SET status='failed', summary='Execution interrupted by backend restart', process_pid=0, process_start_token='', stderr_redacted='', finished_at=now() WHERE id=$1 AND status='running'`, run.ID)
	if err != nil {
		return err
	}
	if run.Phase == "specification" && task.State == string(workflow.StateSpecifying) {
		updated, applyErr := workflow.Apply(workflow.Task{State: workflow.State(task.State), SpecificationStatus: task.SpecificationStatus,
			ImplementationStatus: task.ImplementationStatus, QAStatus: task.QAStatus, PRStatus: task.PRStatus,
			ChecksStatus: task.ChecksStatus, BlockedFromState: blockedFrom}, workflow.ActionSpecificationFailed, "system", "Specification generation interrupted by backend restart", "")
		if applyErr != nil {
			return applyErr
		}
		var round int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(round),0)+1 FROM task_specifications WHERE task_id=$1`, task.ID).Scan(&round); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_specifications(task_id,round,status,artifact_paths,summary,run_id) VALUES($1,$2,'failed','[]'::jsonb,'Specification generation interrupted', $3)`, task.ID, round, run.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET state=$2,specification_status=$3,updated_at=now() WHERE id=$1`, task.ID, updated.State, updated.SpecificationStatus); err != nil {
			return err
		}
		if err := appendTaskEvent(ctx, tx, task.ID, task.CreatedBy, "system", "failure", string(workflow.ActionSpecificationFailed), task.State, string(updated.State), "", "Specification generation interrupted by backend restart"); err != nil {
			return err
		}
	} else if run.Phase == "implementation" && task.State == string(workflow.StateInDevelopment) && task.ImplementationStatus == workflow.StatusRunning {
		updated, applyErr := workflow.Apply(workflow.Task{State: workflow.State(task.State), SpecificationStatus: task.SpecificationStatus,
			ImplementationStatus: task.ImplementationStatus, QAStatus: task.QAStatus, PRStatus: task.PRStatus,
			ChecksStatus: task.ChecksStatus, BlockedFromState: blockedFrom}, workflow.ActionImplementationError, "system", "Implementation interrupted by backend restart", "")
		if applyErr != nil {
			return applyErr
		}
		var attempt int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt),0)+1 FROM task_implementations WHERE task_id=$1`, task.ID).Scan(&attempt); err != nil {
			return err
		}
		emptyPaths, _ := json.Marshal([]string{})
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_implementations(task_id,attempt,status,changed_files,summary,run_id) VALUES($1,$2,'failed',$3,'Implementation interrupted by backend restart',$4)`, task.ID, attempt, emptyPaths, run.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET implementation_status=$2,updated_at=now() WHERE id=$1`, task.ID, updated.ImplementationStatus); err != nil {
			return err
		}
		if err := appendTaskEvent(ctx, tx, task.ID, task.CreatedBy, "system", "failure", string(workflow.ActionImplementationError), task.State, string(updated.State), "", "Implementation interrupted by backend restart"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

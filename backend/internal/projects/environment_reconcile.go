package projects

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"fabrica/backend/internal/environments"
)

type qaEnvironmentRecord struct {
	projectID, taskID int64
	workspace, url    string
	port              int
}

// ReconcileQAEnvironments restores persisted environment state from Docker after an API restart.
func ReconcileQAEnvironments(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT e.project_id,e.task_id,p.workspace_path,COALESCE(e.url,''),COALESCE(e.port,0) FROM project_qa_environments e JOIN projects p ON p.id=e.project_id WHERE e.status IN ('healthy','starting','unhealthy')`)
	if err != nil {
		return err
	}
	records := make([]qaEnvironmentRecord, 0)
	for rows.Next() {
		var item qaEnvironmentRecord
		if err := rows.Scan(&item.projectID, &item.taskID, &item.workspace, &item.url, &item.port); err != nil {
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
	manager := environments.ComposeManager{}
	allocator := environments.NewPortAllocator(db)
	for _, item := range records {
		var leaseActive bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qa_port_leases WHERE project_id=$1 AND port=$2 AND status='active')`, item.projectID, item.port).Scan(&leaseActive); err != nil {
			return err
		}
		if !leaseActive {
			stopCtx, stopCancel := context.WithTimeout(ctx, 20*time.Second)
			_ = manager.Stop(stopCtx, item.projectID, item.workspace)
			stopCancel()
			_, _ = db.ExecContext(ctx, `UPDATE project_qa_environments SET status='unhealthy',url=NULL,updated_at=now() WHERE project_id=$1`, item.projectID)
			continue
		}
		inspectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		environment, inspectErr := manager.Inspect(inspectCtx, item.projectID, item.workspace)
		cancel()
		if inspectErr != nil || !qaServiceRunning(environment, item.workspace) {
			_, _ = db.ExecContext(ctx, `UPDATE project_qa_environments SET status='stopped',url=NULL,updated_at=now() WHERE project_id=$1`, item.projectID)
			_ = allocator.Release(ctx, item.projectID)
			continue
		}
		healthCtx, healthCancel := context.WithTimeout(ctx, 8*time.Second)
		url, healthErr := waitForQAHealth(healthCtx, item.port, item.workspace)
		healthCancel()
		if healthErr != nil {
			stopCtx, stopCancel := context.WithTimeout(ctx, 20*time.Second)
			_ = manager.Stop(stopCtx, item.projectID, item.workspace)
			stopCancel()
			_, _ = db.ExecContext(ctx, `UPDATE project_qa_environments SET status='unhealthy',url=NULL,updated_at=now() WHERE project_id=$1`, item.projectID)
			_ = allocator.Release(ctx, item.projectID)
			continue
		}
		_, err = db.ExecContext(ctx, `UPDATE project_qa_environments SET status='healthy',url=$2,health_checked_at=now(),updated_at=now() WHERE project_id=$1`, item.projectID, url)
		if err != nil {
			return fmt.Errorf("restore QA environment for project %d: %w", item.projectID, err)
		}
	}
	return nil
}

func qaServiceRunning(environment environments.Environment, workspace string) bool {
	descriptor, _, err := environments.LoadDescriptor(workspace)
	if err != nil {
		return false
	}
	for _, container := range environment.Containers {
		if container.Service == descriptor.QAService && strings.EqualFold(container.State, "running") {
			return true
		}
	}
	return false
}

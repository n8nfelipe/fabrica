package environments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	DefaultPortStart = 41000
	DefaultPortEnd   = 41999
	portPoolLockID   = int64(846029117)
)

var ErrPortPoolExhausted = errors.New("no free QA host ports are available")

type PortLease struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
	Created   bool   `json:"-"`
}

type PortAllocator struct {
	db            *sql.DB
	start         int
	end           int
	host          string
	portAvailable func(string, int) bool
}

func NewPortAllocator(db *sql.DB) *PortAllocator {
	return &PortAllocator{db: db, start: DefaultPortStart, end: DefaultPortEnd, host: "127.0.0.1", portAvailable: portAvailable}
}

func (a *PortAllocator) Reserve(ctx context.Context, projectID int64) (PortLease, error) {
	var lease PortLease
	if a == nil || a.db == nil {
		return lease, errors.New("port allocator requires a database")
	}
	if projectID <= 0 {
		return lease, errors.New("project ID must be positive")
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return lease, err
	}
	defer tx.Rollback()
	var lockResult any
	if err := tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock($1)`, portPoolLockID).Scan(&lockResult); err != nil {
		return lease, err
	}
	err = tx.QueryRowContext(ctx, `SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=$1 AND status='active' FOR UPDATE`, projectID).Scan(
		&lease.ID, &lease.ProjectID, &lease.Host, &lease.Port, &lease.Status)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return PortLease{}, err
		}
		return lease, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PortLease{}, err
	}
	for port := a.start; port <= a.end; port++ {
		var allocated bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM qa_port_leases WHERE host=$1 AND port=$2 AND status='active')`, a.host, port).Scan(&allocated); err != nil {
			return PortLease{}, err
		}
		if allocated || !a.portAvailable(a.host, port) {
			continue
		}
		err := tx.QueryRowContext(ctx, `INSERT INTO qa_port_leases(project_id,host,port,status) VALUES($1,$2,$3,'active') RETURNING id`, projectID, a.host, port).Scan(&lease.ID)
		if err != nil {
			return PortLease{}, err
		}
		lease.ProjectID, lease.Host, lease.Port, lease.Status, lease.Created = projectID, a.host, port, "active", true
		if err := tx.Commit(); err != nil {
			return PortLease{}, err
		}
		return lease, nil
	}
	return PortLease{}, ErrPortPoolExhausted
}

func (a *PortAllocator) Release(ctx context.Context, projectID int64) error {
	if a == nil || a.db == nil {
		return errors.New("port allocator requires a database")
	}
	_, err := a.db.ExecContext(ctx, `UPDATE qa_port_leases SET status='released', released_at=now() WHERE project_id=$1 AND status='active'`, projectID)
	return err
}

func (a *PortAllocator) StartWithRetry(ctx context.Context, manager ComposeManager, projectID int64, workspace string) (Environment, PortLease, error) {
	if a == nil || a.db == nil {
		return Environment{}, PortLease{}, errors.New("port allocator requires a database")
	}
	var lastErr error
	maxAttempts := a.end - a.start + 1
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lease, err := a.Reserve(ctx, projectID)
		if err != nil {
			return Environment{}, PortLease{}, err
		}
		environment, startErr := manager.StartWithPort(ctx, projectID, workspace, lease.Port)
		if startErr == nil {
			return environment, lease, nil
		}
		lastErr = startErr
		if IsPortBindConflict(startErr) {
			if releaseErr := a.Release(ctx, projectID); releaseErr != nil {
				return Environment{}, PortLease{}, errors.Join(startErr, releaseErr)
			}
			continue
		}
		if lease.Created {
			_ = a.Release(ctx, projectID)
		}
		return Environment{}, PortLease{}, startErr
	}
	if lastErr != nil {
		return Environment{}, PortLease{}, fmt.Errorf("%w after %d attempts: %v", ErrPortPoolExhausted, maxAttempts, lastErr)
	}
	return Environment{}, PortLease{}, ErrPortPoolExhausted
}

func IsPortBindConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"port is already allocated", "address already in use", "failed to bind host port", "bind: address already in use"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func portAvailable(host string, port int) bool {
	listener, err := net.Listen("tcp4", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

package environments

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPortAllocatorRejectsMissingDBAndInvalidProject(t *testing.T) {
	if _, err := (*PortAllocator)(nil).Reserve(context.Background(), 1); err == nil {
		t.Fatal("nil allocator should fail")
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	if _, err := allocator.Reserve(context.Background(), 0); err == nil {
		t.Fatal("invalid project should fail")
	}
	if err := (*PortAllocator)(nil).Release(context.Background(), 1); err == nil {
		t.Fatal("nil allocator release should fail")
	}
	mock.ExpectExec(`UPDATE qa_port_leases`).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := allocator.Release(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReserveReturnsExistingLeaseAndReleaseExecutes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(portPoolLockID)).WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(int64(18)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}).AddRow(2, 18, "127.0.0.1", 41001, "active"))
	mock.ExpectCommit()
	lease, err := allocator.Reserve(context.Background(), 18)
	if err != nil || lease.ID != 2 || lease.Port != 41001 || lease.Created {
		t.Fatalf("Reserve=%#v, %v", lease, err)
	}
	mock.ExpectExec(`UPDATE qa_port_leases SET status='released', released_at=now\(\) WHERE project_id=\$1 AND status='active'`).WithArgs(int64(18)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := allocator.Release(context.Background(), 18); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReserveCreatesPortLeaseWhenNoneExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	allocator.start = 0
	allocator.end = 0
	allocator.portAvailable = func(string, int) bool { return true }
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(portPoolLockID)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(int64(25)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM qa_port_leases WHERE host=\$1 AND port=\$2 AND status='active'\)`).WithArgs("127.0.0.1", 0).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO qa_port_leases\(project_id,host,port,status\)`).WithArgs(int64(25), "127.0.0.1", 0).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(13))
	mock.ExpectCommit()
	lease, err := allocator.Reserve(context.Background(), 25)
	if err != nil || lease.ID != 13 || lease.Port != 0 || !lease.Created {
		t.Fatalf("new lease=%#v err=%v", lease, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPortAllocatorPropagatesDatabaseErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	mock.ExpectBegin().WillReturnError(errors.New("busy"))
	if _, err := allocator.Reserve(context.Background(), 4); err == nil {
		t.Fatal("reserve should return begin error")
	}
	mock.ExpectExec(`UPDATE qa_port_leases`).WithArgs(int64(4)).WillReturnError(errors.New("write failed"))
	if err := allocator.Release(context.Background(), 4); err == nil {
		t.Fatal("release should return update error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithRetryRejectsUnavailableAllocator(t *testing.T) {
	if _, _, err := (*PortAllocator)(nil).StartWithRetry(context.Background(), ComposeManager{}, 1, "/tmp"); err == nil {
		t.Fatal("nil allocator should fail")
	}
	if _, _, err := NewPortAllocator(nil).StartWithRetry(context.Background(), ComposeManager{}, 1, "/tmp"); err == nil {
		t.Fatal("allocator without DB should fail")
	}
}

func TestStartWithRetryReusesLeaseAndStartsCompose(t *testing.T) {
	root, binary, _ := composeFixture(t, "web db")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(portPoolLockID)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}).AddRow(10, 42, "127.0.0.1", 41042, "active"))
	mock.ExpectCommit()
	environment, lease, err := allocator.StartWithRetry(context.Background(), ComposeManager{DockerPath: binary}, 42, root)
	if err != nil || lease.Port != 41042 || environment.ProjectName != "fabrica-42" {
		t.Fatalf("StartWithRetry environment=%#v lease=%#v err=%v", environment, lease, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithRetryReturnsComposeErrorForExistingLease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(portPoolLockID)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
	mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}).AddRow(10, 42, "127.0.0.1", 41042, "active"))
	mock.ExpectCommit()
	_, _, err = allocator.StartWithRetry(context.Background(), ComposeManager{}, 42, t.TempDir())
	if err == nil {
		t.Fatal("missing descriptor should fail Compose startup")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithRetryRetriesDockerPortBindCollision(t *testing.T) {
	root, _, _ := composeFixture(t, "web")
	binary := filepath.Join(root, "retry-docker")
	counter := filepath.Join(root, "start-count")
	script := "#!/bin/sh\ncase \"$*\" in\n *'config --services'*) echo web ;;\n *'up --detach'*)\n  count=0; [ -f '" + counter + "' ] && count=$(cat '" + counter + "');\n  if [ \"$count\" -eq 0 ]; then echo 'port is already allocated' >&2; echo 1 > '" + counter + "'; exit 1; fi ;;\n *'ps --format json'*) echo '{\"ID\":\"id\",\"Name\":\"web-1\",\"Project\":\"fabrica-42\",\"Service\":\"web\",\"State\":\"running\",\"Health\":\"healthy\",\"ExitCode\":0}' ;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	allocator := NewPortAllocator(db)
	allocator.start = 41000
	allocator.end = 41001
	allocator.portAvailable = func(string, int) bool { return true }
	reserve := func(id int64, port int) {
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(int64(portPoolLockID)).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))
		mock.ExpectQuery(`SELECT id, project_id, host, port, status FROM qa_port_leases WHERE project_id=\$1 AND status='active' FOR UPDATE`).WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "host", "port", "status"}))
		for candidate := 41000; candidate <= port; candidate++ {
			allocated := id == 2 && candidate == 41000
			mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM qa_port_leases WHERE host=\$1 AND port=\$2 AND status='active'\)`).WithArgs("127.0.0.1", candidate).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(allocated))
		}
		mock.ExpectQuery(`INSERT INTO qa_port_leases\(project_id,host,port,status\)`).WithArgs(int64(42), "127.0.0.1", port).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
		mock.ExpectCommit()
	}
	reserve(1, 41000)
	mock.ExpectExec(`UPDATE qa_port_leases SET status='released'`).WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	reserve(2, 41001)
	environment, lease, err := allocator.StartWithRetry(context.Background(), ComposeManager{DockerPath: binary}, 42, root)
	if err != nil || lease.ID != 2 || lease.Port != 41001 || environment.ProjectName != "fabrica-42" {
		t.Fatalf("retry result environment=%#v lease=%#v err=%v", environment, lease, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPortAvailabilityDetectsOccupiedAndFreeAddresses(t *testing.T) {
	if !portAvailable("127.0.0.1", 0) {
		t.Fatal("ephemeral port should be available")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if portAvailable("127.0.0.1", port) {
		_ = listener.Close()
		t.Fatal("bound port should be unavailable")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if !portAvailable("127.0.0.1", port) {
		t.Fatal("released port should be available")
	}
}

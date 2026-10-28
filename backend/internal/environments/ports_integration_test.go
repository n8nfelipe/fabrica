package environments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"fabrica/backend/internal/database"
)

func TestPortAllocatorPersistsConcurrentLeasesAndSkipsOccupiedPort(t *testing.T) {
	databaseURL := os.Getenv("FABRICA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set FABRICA_TEST_DATABASE_URL to a dedicated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	userID, projectIDs := createPortTestProjects(t, ctx, db, 4)
	defer func() {
		for _, id := range projectIDs {
			_, _ = db.Exec(`DELETE FROM projects WHERE id=$1`, id)
		}
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, userID)
	}()
	base, listeners := findFreePortRange(t, 6)
	for _, listener := range listeners[1:] {
		_ = listener.Close()
	}
	listeners = listeners[:1]
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	allocator := NewPortAllocator(db)
	allocator.start, allocator.end = base, base+5
	lease, err := allocator.Reserve(ctx, projectIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if lease.Port == base {
		t.Fatal("allocator chose a port already held by an external listener")
	}
	var concurrentLeases [2]PortLease
	var wait sync.WaitGroup
	errorsByIndex := make([]error, 2)
	for index := range concurrentLeases {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			concurrentLeases[index], errorsByIndex[index] = allocator.Reserve(ctx, projectIDs[index+1])
		}(index)
	}
	wait.Wait()
	for _, err := range errorsByIndex {
		if err != nil {
			t.Fatal(err)
		}
	}
	if lease.Port == concurrentLeases[0].Port || lease.Port == concurrentLeases[1].Port || concurrentLeases[0].Port == concurrentLeases[1].Port {
		t.Fatalf("concurrent projects received conflicting leases: %d, %d, %d", lease.Port, concurrentLeases[0].Port, concurrentLeases[1].Port)
	}
	var active int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM qa_port_leases WHERE status='active' AND project_id = ANY($1)`, projectIDs[:3]).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 3 {
		t.Fatalf("expected three persisted leases, got %d", active)
	}
	if err := allocator.Release(ctx, projectIDs[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM qa_port_leases WHERE project_id=$1 AND status='released'`, projectIDs[0]).Scan(&active); err != nil || active != 1 {
		t.Fatalf("lease release not persisted: count=%d err=%v", active, err)
	}
}

func TestPortAllocatorRetriesDockerBindCollision(t *testing.T) {
	databaseURL := os.Getenv("FABRICA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set FABRICA_TEST_DATABASE_URL to a dedicated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	userID, projectIDs := createPortTestProjects(t, ctx, db, 1)
	defer func() {
		_, _ = db.Exec(`DELETE FROM projects WHERE id=$1`, projectIDs[0])
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, userID)
	}()
	base, listeners := findFreePortRange(t, 2)
	for _, listener := range listeners {
		_ = listener.Close()
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".fabrica"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DescriptorPath), []byte(`{"version":1,"composeFile":"compose.yaml","qaService":"web","containerPort":80}`), 0o600); err != nil {
		t.Fatal(err)
	}
	readyFile, releaseFile := filepath.Join(root, "first-up-ready"), filepath.Join(root, "allow-first-up-failure")
	secondUpFile, commandLog := filepath.Join(root, "second-up"), filepath.Join(root, "commands.log")
	for key, path := range map[string]string{"PORT_READY_FILE": readyFile, "PORT_RELEASE_FILE": releaseFile, "PORT_SECOND_UP_FILE": secondUpFile, "PORT_COMMAND_LOG": commandLog} {
		t.Setenv(key, path)
	}
	fakeDocker := filepath.Join(root, "fake-docker")
	script := `#!/bin/sh
echo "$*" >> "$PORT_COMMAND_LOG"
case "$*" in
  *'config --services'*) echo web ;;
  *'up --detach'*)
    attempts=$(grep -c 'up --detach' "$PORT_COMMAND_LOG")
    if [ "$attempts" = "1" ]; then
      touch "$PORT_READY_FILE"
      while [ ! -f "$PORT_RELEASE_FILE" ]; do sleep 0.01; done
      echo 'port is already allocated' >&2
      exit 1
    fi
    touch "$PORT_SECOND_UP_FILE"
    ;;
  *'ps --format json'*) echo '[]' ;;
esac
`
	if err := os.WriteFile(fakeDocker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	allocator := NewPortAllocator(db)
	allocator.start, allocator.end = base, base+1
	listenerDone := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(readyFile); err == nil {
				listener, listenErr := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(base)))
				if listenErr != nil {
					listenerDone <- listenErr
					return
				}
				_ = os.WriteFile(releaseFile, []byte("continue"), 0o600)
				for time.Now().Before(deadline) {
					if _, err := os.Stat(secondUpFile); err == nil {
						listener.Close()
						listenerDone <- nil
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				listener.Close()
				listenerDone <- errors.New("second Compose start did not occur")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		listenerDone <- errors.New("first Compose start did not occur")
	}()
	manager := ComposeManager{DockerPath: fakeDocker}
	_, lease, err := allocator.StartWithRetry(ctx, manager, projectIDs[0], root)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-listenerDone; err != nil {
		t.Fatal(err)
	}
	if lease.Port != base+1 {
		t.Fatalf("bind collision retry chose port %d, want %d", lease.Port, base+1)
	}
	var activeCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM qa_port_leases WHERE project_id=$1 AND status='active'`, projectIDs[0]).Scan(&activeCount); err != nil || activeCount != 1 {
		t.Fatalf("expected one active retry lease, count=%d err=%v", activeCount, err)
	}
}

func createPortTestProjects(t *testing.T, ctx context.Context, db *sql.DB, count int) (int64, []int64) {
	t.Helper()
	suffix := time.Now().UnixNano()
	var userID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO users(username,display_name,password_hash,role) VALUES($1,'Port test','unused','Admin') RETURNING id`, fmt.Sprintf("port-test-%d", suffix)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, count)
	for index := 0; index < count; index++ {
		var id int64
		name := fmt.Sprintf("port-test-%d-%d", suffix, index)
		path := fmt.Sprintf("/tmp/fabrica-port-test-%d-%d", suffix, index)
		if err := db.QueryRowContext(ctx, `INSERT INTO projects(name,workspace_path,created_by) VALUES($1,$2,$3) RETURNING id`, name, path, userID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return userID, ids
}

func findFreePortRange(t *testing.T, count int) (int, []net.Listener) {
	t.Helper()
	for base := 43000; base < 60000-count; base += count {
		listeners := make([]net.Listener, 0, count)
		free := true
		for port := base; port < base+count; port++ {
			listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				free = false
				break
			}
			listeners = append(listeners, listener)
		}
		if free {
			return base, listeners
		}
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	t.Fatal("could not find a free contiguous test port range")
	return 0, nil
}

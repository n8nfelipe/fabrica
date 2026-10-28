package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"fabrica/backend/internal/database"
	"fabrica/backend/internal/environments"
)

func TestQAEnvironmentReconciliationKeepsHealthyComposeStack(t *testing.T) {
	if os.Getenv("FABRICA_DOCKER_INTEGRATION") != "1" {
		t.Skip("set FABRICA_DOCKER_INTEGRATION=1 to run Docker reconciliation")
	}
	databaseURL := os.Getenv("FABRICA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set FABRICA_TEST_DATABASE_URL to run QA reconciliation integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, projectID, taskID := createReconcileFixture(t, ctx, db)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".fabrica"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx:1.29-alpine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	descriptor, _ := json.Marshal(environments.Descriptor{Version: 1, ComposeFile: "compose.yaml", QAService: "web", ContainerPort: 80})
	if err := os.WriteFile(filepath.Join(root, environments.DescriptorPath), descriptor, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE projects SET workspace_path=$2 WHERE id=$1`, projectID, root); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	manager := environments.ComposeManager{}
	allocator := environments.NewPortAllocator(db)
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		_ = manager.Stop(cleanupCtx, projectID, root)
		_ = allocator.Release(cleanupCtx, projectID)
		_ = exec.CommandContext(cleanupCtx, "docker", "compose", "--project-directory", root, "-f", filepath.Join(root, "compose.yaml"), "-f", filepath.Join(root, ".fabrica", "runtime-compose.yaml"), "-p", "fabrica-"+strconv.FormatInt(projectID, 10), "down", "--remove-orphans").Run()
	})
	if _, err := manager.StartWithPort(ctx, projectID, root, port); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if _, err := db.ExecContext(ctx, `INSERT INTO qa_port_leases(project_id,host,port,status) VALUES($1,'127.0.0.1',$2,'active')`, projectID, port); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO project_qa_environments(project_id,task_id,status,url,port) VALUES($1,$2,'healthy',$3,$4)`, projectID, taskID, url, port); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileQAEnvironments(ctx, db); err != nil {
		t.Fatal(err)
	}
	var status, restoredURL string
	if err := db.QueryRowContext(ctx, `SELECT status,url FROM project_qa_environments WHERE project_id=$1`, projectID).Scan(&status, &restoredURL); err != nil {
		t.Fatal(err)
	}
	if status != "healthy" || restoredURL != url {
		t.Fatalf("environment status=%s URL=%q", status, restoredURL)
	}
}

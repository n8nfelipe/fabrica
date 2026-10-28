package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"fabrica/backend/internal/auth"
	"fabrica/backend/internal/database"
	"fabrica/backend/internal/opencode"
	"fabrica/backend/internal/projects"
	"fabrica/backend/internal/workspace"
)

type healthResponse struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	Database string `json:"database"`
}

func main() {
	workspaceRoot, err := workspace.Root(os.Getenv("FABRICA_WORKSPACE"))
	if err != nil {
		log.Fatalf("workspace configuration failed: %v", err)
	}

	address := os.Getenv("FABRICA_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}

	databaseContext, cancelDatabaseContext := context.WithTimeout(context.Background(), 30*time.Second)
	db, err := database.Open(databaseContext, os.Getenv("DATABASE_URL"))
	cancelDatabaseContext()
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()
	if err := projects.ReconcileAgentRuns(context.Background(), db); err != nil {
		log.Fatalf("agent run reconciliation failed: %v", err)
	}
	reconciliationContext, cancelReconciliation := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := projects.ReconcileQAEnvironments(reconciliationContext, db); err != nil {
		log.Printf("QA environment reconciliation incomplete: %v", err)
	}
	if err := projects.ReconcileGitHubPullRequests(reconciliationContext, db, workspaceRoot); err != nil {
		log.Printf("GitHub pull request reconciliation incomplete: %v", err)
	}
	cancelReconciliation()

	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db)
	opencodeModel := strings.TrimSpace(os.Getenv("OPENCODE_MODEL"))
	switch strings.ToLower(opencodeModel) {
	case "codex", "chatgpt":
		opencodeModel = opencode.ChatGPTPlanModel
	case "", "zen":
		opencodeModel = opencode.DefaultModel
	}
	projects.RegisterRoutesWithExecutor(mux, db, workspaceRoot, opencode.Executor{Binary: os.Getenv("OPENCODE_BIN"), Model: opencodeModel})
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		response := healthResponse{Status: "ok", Name: "fabrica-api", Database: "connected"}
		if err := db.PingContext(r.Context()); err != nil {
			response.Status = "degraded"
			response.Database = "unavailable"
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("write health response: %v", err)
		}
	})

	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Fábrica API listening on http://%s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

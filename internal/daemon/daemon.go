package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Ducstii/Baton/internal/config"
)

const Version = "0.1.0"

// Daemon is the Baton REST API server.
type Daemon struct {
	config     *config.Config
	configPath string
	mux        *http.ServeMux
	runs       *RunStore
	projects   *ProjectStore
	sseBroker  *SSEBroker
	server     *http.Server
	startTime  time.Time
}

// New creates a new Daemon with routes registered.
func New(cfg *config.Config) *Daemon {
	d := &Daemon{
		config:     cfg,
		configPath: config.ConfigPath(),
		mux:        http.NewServeMux(),
		runs:       NewRunStore(),
		projects:   NewProjectStore(),
		sseBroker:  NewSSEBroker(),
		startTime:  time.Now(),
	}
	d.registerRoutes()
	return d
}

func (d *Daemon) registerRoutes() {
	d.mux.HandleFunc("GET /health", d.handleHealth)

	d.mux.HandleFunc("GET /runs", d.auth(d.handleListRuns))
	d.mux.HandleFunc("POST /runs", d.auth(d.handleCreateRun))
	d.mux.HandleFunc("GET /runs/{id}", d.auth(d.handleGetRun))
	d.mux.HandleFunc("GET /runs/{id}/events", d.auth(d.handleRunEvents))
	d.mux.HandleFunc("POST /runs/{id}/checkpoint/confirm", d.auth(d.handleCheckpointConfirm))
	d.mux.HandleFunc("POST /runs/{id}/checkpoint/reject", d.auth(d.handleCheckpointReject))
	d.mux.HandleFunc("POST /runs/{id}/chat", d.auth(d.handleRunChat))

	d.mux.HandleFunc("GET /projects", d.auth(d.handleListProjects))
	d.mux.HandleFunc("POST /projects", d.auth(d.handleOpenProject))

	d.mux.HandleFunc("GET /providers", d.auth(d.handleListProviders))
	d.mux.HandleFunc("POST /providers/configure", d.auth(d.handleConfigureProvider))
}

// Start starts the HTTP server on the configured port. Blocks until Stop is called.
func (d *Daemon) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", d.config.DaemonPort)
	d.server = &http.Server{
		Addr:    addr,
		Handler: d.mux,
	}
	return d.server.ListenAndServe()
}

// Stop gracefully shuts down the HTTP server with a 5-second timeout.
func (d *Daemon) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.server.Shutdown(ctx)
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        Version,
		"uptime_seconds": int(time.Since(d.startTime).Seconds()),
		"active_runs":    d.runs.Count(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal encoding error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

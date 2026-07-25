package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RunStatus values.
const (
	StatusPending   = "pending"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Run represents a single fix/feature run.
type Run struct {
	ID          string    `json:"id"`
	ProjectPath string    `json:"project_path"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateRunRequest is the JSON body for POST /runs.
type CreateRunRequest struct {
	ProjectPath string `json:"project_path"`
}

// RunStore is an in-memory, concurrency-safe store for runs.
type RunStore struct {
	mu   sync.RWMutex
	runs map[string]*Run
}

// NewRunStore creates a new run store.
func NewRunStore() *RunStore {
	return &RunStore{
		runs: make(map[string]*Run),
	}
}

// Add stores a run. Returns an error if the ID already exists.
func (s *RunStore) Add(run *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.ID]; ok {
		return fmt.Errorf("run %q already exists", run.ID)
	}
	s.runs[run.ID] = run
	return nil
}

// Get retrieves a run by ID.
func (s *RunStore) Get(id string) (*Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	return run, ok
}

// List returns all runs.
func (s *RunStore) List() []*Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Run, 0, len(s.runs))
	for _, run := range s.runs {
		result = append(result, run)
	}
	return result
}

// Count returns the total number of runs.
func (s *RunStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.runs)
}

// Set stores a run by ID, overwriting any existing entry.
func (s *RunStore) Set(id string, run *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id] = run
}

// generateID returns a short random hex string suitable for run IDs.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (d *Daemon) handleListRuns(w http.ResponseWriter, r *http.Request) {
	runs := d.runs.List()
	writeJSON(w, http.StatusOK, runs)
}

func (d *Daemon) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req CreateRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.ProjectPath == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "project_path is required")
		return
	}

	id, err := generateID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to generate run ID")
		return
	}
	run := &Run{
		ID:          id,
		ProjectPath: req.ProjectPath,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := d.runs.Add(run); err != nil {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

func (d *Daemon) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := d.runs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (d *Daemon) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if _, ok := d.runs.Get(runID); !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream_error", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := d.sseBroker.Subscribe(runID)
	defer d.sseBroker.Unsubscribe(runID, ch)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, "data: {\"type\":\"heartbeat\",\"data\":{}}\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

type chatRequest struct {
	Message string `json:"message"`
}

func (d *Daemon) handleRunChat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := d.runs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "message is required")
		return
	}

	// Get or create a brain session for this run.
	d.brainSessionsMu.Lock()
	brain, exists := d.brainSessions[id]
	if !exists {
		var err error
		brain, err = NewBrainSession(d.ocClient, d.agentRuntime, d.registry, d.conversationStore, d.runs, id, run.ProjectPath, "deepseek", "deepseek-v4-pro")
		if err != nil {
			d.brainSessionsMu.Unlock()
			writeError(w, http.StatusInternalServerError, "brain_error", fmt.Sprintf("create brain session: %v", err))
			return
		}
		d.brainSessions[id] = brain
	}
	d.brainSessionsMu.Unlock()

	response, err := brain.SendMessage(r.Context(), req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "brain_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"response": response,
	})
}

func (d *Daemon) handleRunHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := d.runs.Get(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	messages, err := d.conversationStore.LoadConversation(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error())
		return
	}
	if messages == nil {
		messages = []ChatMessage{}
	}

	writeJSON(w, http.StatusOK, messages)
}

// agentResponse is the JSON payload for agent instances exposed to the TUI.
type agentResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
	Error        string    `json:"error,omitempty"`
}

func (d *Daemon) handleRunAgents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := d.runs.Get(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	instances := d.agentRuntime.ListAgents()
	agents := make([]agentResponse, 0, len(instances))
	for _, inst := range instances {
		a := agentResponse{
			ID:           inst.ID,
			Name:         inst.Name,
			Status:       string(inst.Status),
			WorktreePath: inst.WorktreePath,
			StartedAt:    inst.StartedAt,
			Error:        inst.Error,
		}
		if !inst.CompletedAt.IsZero() {
			a.CompletedAt = inst.CompletedAt
		}
		agents = append(agents, a)
	}
	writeJSON(w, http.StatusOK, agents)
}

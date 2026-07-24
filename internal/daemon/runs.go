package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Ducstii/Baton/internal/agents"
	"github.com/Ducstii/Baton/internal/taskgraph"
	"github.com/Ducstii/Baton/internal/worktree"
)

// RunStatus values.
const (
	StatusDiagnosing         = "diagnosing"
	StatusAwaitingCheckpoint = "awaiting_checkpoint"
	StatusInProgress         = "in_progress"
	StatusCompleted          = "completed"
	StatusFailed             = "failed"
)

// WorkUnitStats tracks work unit progress for a run.
type WorkUnitStats struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	InProgress int `json:"in_progress"`
	Blocked    int `json:"blocked"`
	Failed     int `json:"failed"`
}

// SessionInfo describes a session within a run.
type SessionInfo struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	Model   string `json:"model"`
}

// Run represents a single fix/feature run.
type Run struct {
	ID               string            `json:"id"`
	ProjectPath      string            `json:"project_path"`
	Input            RunInput          `json:"input"`
	TargetAgentCount int               `json:"target_agent_count"`
	ModelMapping     map[string]string `json:"model_mapping"`
	Status           string            `json:"status"`
	WorkUnits        WorkUnitStats     `json:"work_units"`
	Sessions         []SessionInfo     `json:"sessions"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// CreateRunRequest is the JSON body for POST /runs.
type CreateRunRequest struct {
	ProjectPath      string            `json:"project_path"`
	Input            RunInput          `json:"input"`
	TargetAgentCount int               `json:"target_agent_count"`
	ModelMapping     map[string]string `json:"model_mapping"`
}

// RunInput describes the input for a diagnostic run.
type RunInput struct {
	Type    string `json:"type"`
	Source  string `json:"source"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
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
		ID:               id,
		ProjectPath:      req.ProjectPath,
		Input:            req.Input,
		TargetAgentCount: req.TargetAgentCount,
		ModelMapping:     req.ModelMapping,
		Status:           StatusDiagnosing,
		WorkUnits:        WorkUnitStats{},
		Sessions:         []SessionInfo{},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := d.runs.Add(run); err != nil {
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	}

	d.sseBroker.Publish(run.ID, SSEEvent{
		Type: "run.created",
		Data: run,
	})

	// Launch diagnostic phase in background.
	go d.runDiagnosticPhase(id, req)

	writeJSON(w, http.StatusCreated, run)
}

// runDiagnosticPhase runs the diagnostic agent, builds the task graph from the
// resulting work units, and transitions the run to awaiting_checkpoint.
func (d *Daemon) runDiagnosticPhase(runID string, req CreateRunRequest) {
	run, ok := d.runs.Get(runID)
	if !ok {
		return
	}

	providerID, modelID := d.getModelForRun(run, "diagnostic")

	origDir, _ := os.Getwd()
	_ = os.Chdir(run.ProjectPath)
	result, err := agents.RunDiagnostic(d.ocClient, req.Input.Content, req.Input.Type, providerID, modelID)
	_ = os.Chdir(origDir)
	if err != nil {
		log.Printf("run %s: diagnostic failed: %v", runID, err)
		run.Status = StatusFailed
		run.UpdatedAt = time.Now()
		d.runs.Set(runID, run)
		d.publishRunStatus(runID, StatusDiagnosing, StatusFailed)
		return
	}

	// Build task graph from work units.
	graph := taskgraph.NewGraph()
	for _, wu := range result.WorkUnits {
		node := &taskgraph.Node{
			ID:          wu.ID,
			Description: wu.Description,
			Issues:      wu.Issues,
			Status:      taskgraph.StatusPending,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := graph.AddNode(node); err != nil {
			log.Printf("run %s: add node %s: %v", runID, wu.ID, err)
			continue
		}
	}

	// Build issue ID -> work unit ID map.
	issueToWU := make(map[string]string, len(result.WorkUnits)*2)
	for _, wu := range result.WorkUnits {
		for _, issueID := range wu.Issues {
			issueToWU[issueID] = wu.ID
		}
	}

	// Build issue ID -> issue map.
	issueMap := make(map[string]agents.DiagnosticIssue, len(result.Issues))
	for _, issue := range result.Issues {
		issueMap[issue.ID] = issue
	}

	// Add dependency edges between work units from issue dependency_edges.
	for _, wu := range result.WorkUnits {
		for _, issueID := range wu.Issues {
			if di, ok := issueMap[issueID]; ok {
				for _, depID := range di.DependencyEdges {
					if depWU, ok := issueToWU[depID]; ok && depWU != wu.ID {
						if err := graph.AddEdge(depWU, wu.ID); err != nil {
							log.Printf("run %s: add edge %s->%s: %v", runID, depWU, wu.ID, err)
						}
					}
				}
			}
		}
	}

	// Store the graph.
	d.taskGraphMu.Lock()
	d.taskGraphs[runID] = graph
	d.taskGraphMu.Unlock()

	// Update run status and work unit stats.
	run.Status = StatusAwaitingCheckpoint
	run.UpdatedAt = time.Now()
	run.WorkUnits = WorkUnitStats{
		Total:   len(result.WorkUnits),
		Done:    0,
		Blocked: 0,
		Failed:  0,
	}
	d.runs.Set(runID, run)

	// Publish diagnostic complete event.
	d.sseBroker.Publish(runID, SSEEvent{
		Type: "diagnostic.complete",
		Data: map[string]any{
			"work_units": result.WorkUnits,
			"issues":     result.Issues,
		},
	})
	d.publishRunStatus(runID, StatusDiagnosing, StatusAwaitingCheckpoint)
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

func (d *Daemon) handleCheckpointConfirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := d.runs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	if run.Status != StatusAwaitingCheckpoint {
		writeError(w, http.StatusBadRequest, "invalid_state",
			"run is not awaiting checkpoint confirmation")
		return
	}
	run.Status = StatusInProgress
	run.UpdatedAt = time.Now()
	d.runs.Set(id, run)
	d.publishRunStatus(id, StatusAwaitingCheckpoint, StatusInProgress)

	// Count ready nodes for the response.
	d.taskGraphMu.RLock()
	graph := d.taskGraphs[id]
	d.taskGraphMu.RUnlock()

	dispatchedCount := 0
	if graph != nil {
		dispatchedCount = len(graph.ReadyNodes())
	}

	// Launch fixer phase in background.
	go d.runFixerPhase(id)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           run.Status,
		"dispatched_count": dispatchedCount,
	})
}

func (d *Daemon) handleCheckpointReject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := d.runs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	if run.Status != StatusAwaitingCheckpoint {
		writeError(w, http.StatusBadRequest, "invalid_state",
			"run is not awaiting checkpoint confirmation")
		return
	}
	run.Status = StatusDiagnosing
	run.UpdatedAt = time.Now()
	d.runs.Set(id, run)
	d.publishRunStatus(id, StatusAwaitingCheckpoint, StatusDiagnosing)

	// Discard the task graph so re-diagnosis can rebuild it.
	d.taskGraphMu.Lock()
	delete(d.taskGraphs, id)
	d.taskGraphMu.Unlock()

	writeJSON(w, http.StatusOK, run)
}

func (d *Daemon) handleRunChat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := d.runs.Get(id); !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Chat is not yet implemented. This is a placeholder response.",
	})
}

// runFixerPhase iteratively dispatches ready work units to fixer agents,
// waiting for each wave to complete before checking for newly unblocked nodes.
func (d *Daemon) runFixerPhase(runID string) {
	d.taskGraphMu.RLock()
	graph, ok := d.taskGraphs[runID]
	d.taskGraphMu.RUnlock()
	if !ok {
		log.Printf("run %s: no task graph found", runID)
		return
	}

	run, ok := d.runs.Get(runID)
	if !ok {
		return
	}

	providerID, modelID := d.getModelForRun(run, "fixer")

	for {
		readyNodes := graph.ReadyNodes()
		if len(readyNodes) == 0 {
			stats := graph.Stats()
			if stats[taskgraph.StatusInProgress] == 0 {
				break // nothing running, nothing pending — all done
			}
			// Some nodes still in progress; wait before rechecking.
			time.Sleep(2 * time.Second)
			continue
		}

		var wg sync.WaitGroup
		for _, node := range readyNodes {
			wg.Add(1)
			n := node
			if err := graph.SetStatus(n.ID, taskgraph.StatusInProgress); err != nil {
				log.Printf("run %s: set status %s: %v", runID, n.ID, err)
				wg.Done()
				continue
			}
			d.publishTaskUpdate(runID, n.ID, taskgraph.StatusPending, taskgraph.StatusInProgress)

			go func() {
				defer wg.Done()
				d.runOneFixer(runID, graph, n, providerID, modelID)
			}()
		}
		wg.Wait()
	}

	d.finalizeRun(runID, graph)
}

// runOneFixer creates a git worktree, dispatches the fixer agent, and updates
// the graph node status based on the result.
func (d *Daemon) runOneFixer(runID string, graph *taskgraph.Graph, node *taskgraph.Node, providerID, modelID string) {
	run, ok := d.runs.Get(runID)
	if !ok {
		_ = graph.SetStatus(node.ID, taskgraph.StatusFailed)
		d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusFailed)
		return
	}

	// Changing into the project directory is required for git worktree commands
	// to run from a valid repository root.
	origDir, err := os.Getwd()
	if err != nil {
		log.Printf("run %s: getwd: %v", runID, err)
		_ = graph.SetStatus(node.ID, taskgraph.StatusFailed)
		d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusFailed)
		return
	}
	if err := os.Chdir(run.ProjectPath); err != nil {
		log.Printf("run %s: chdir %s: %v", runID, run.ProjectPath, err)
		_ = graph.SetStatus(node.ID, taskgraph.StatusFailed)
		d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusFailed)
		return
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Create an isolated git worktree for this fixer.
	wt, err := worktree.Create(d.wtBasePath, runID, node.ID)
	if err != nil {
		log.Printf("run %s: worktree create for %s: %v", runID, node.ID, err)
		_ = graph.SetStatus(node.ID, taskgraph.StatusFailed)
		d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusFailed)
		return
	}

	wu := agents.WorkUnitGroup{
		ID:          node.ID,
		Description: node.Description,
		Issues:      node.Issues,
	}

	result, err := agents.RunFixer(d.ocClient, wt.Path(), wu, providerID, modelID)
	if err != nil {
		log.Printf("run %s: fixer for %s: %v", runID, node.ID, err)
		_ = graph.SetStatus(node.ID, taskgraph.StatusFailed)
		d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusFailed)
		return
	}
	if !result.Success {
		log.Printf("run %s: fixer for %s reported failure: %s", runID, node.ID, result.Summary)
		_ = graph.SetStatus(node.ID, taskgraph.StatusFailed)
		d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusFailed)
		return
	}

	// Success.
	_ = graph.SetStatus(node.ID, taskgraph.StatusCompleted)
	d.publishTaskUpdate(runID, node.ID, taskgraph.StatusInProgress, taskgraph.StatusCompleted)

	// Best-effort cleanup of the worktree on success.
	if err := wt.Remove(); err != nil {
		log.Printf("run %s: warning: worktree remove %s: %v", runID, node.ID, err)
	}
}

// finalizeRun sets the run status to completed or failed based on graph stats
// and publishes the final SSE event.
func (d *Daemon) finalizeRun(runID string, graph *taskgraph.Graph) {
	stats := graph.Stats()
	nodes := graph.AllNodes()

	run, ok := d.runs.Get(runID)
	if !ok {
		return
	}

	oldStatus := run.Status

	if stats[taskgraph.StatusFailed] > 0 {
		run.Status = StatusFailed
	} else {
		run.Status = StatusCompleted
	}
	run.UpdatedAt = time.Now()
	run.WorkUnits = WorkUnitStats{
		Total:   len(nodes),
		Done:    stats[taskgraph.StatusCompleted],
		Blocked: stats[taskgraph.StatusBlocked],
		Failed:  stats[taskgraph.StatusFailed],
	}
	d.runs.Set(runID, run)

	d.taskGraphMu.Lock()
	delete(d.taskGraphs, runID)
	d.taskGraphMu.Unlock()

	d.publishRunStatus(runID, oldStatus, run.Status)
}

// getModelForRun returns the provider and model IDs for the given phase.
// Priority: run.ModelMapping, config.Models, then hardcoded defaults.
func (d *Daemon) getModelForRun(run *Run, phase string) (providerID, modelID string) {
	if run.ModelMapping != nil {
		if m, ok := run.ModelMapping[phase]; ok {
			return "deepseek", m
		}
	}
	if d.config.Models != nil {
		if m, ok := d.config.Models[phase]; ok {
			return "deepseek", m
		}
	}
	switch phase {
	case "diagnostic":
		return "deepseek", "sonnet"
	case "fixer":
		return "deepseek", "sonnet"
	default:
		return "deepseek", "sonnet"
	}
}

// publishRunStatus sends a run.status SSE event for the given run.
func (d *Daemon) publishRunStatus(runID, oldStatus, newStatus string) {
	d.sseBroker.Publish(runID, SSEEvent{
		Type: "run.status",
		Data: map[string]any{
			"run_id":     runID,
			"old_status": oldStatus,
			"new_status": newStatus,
			"timestamp":  time.Now(),
		},
	})
}

// publishTaskUpdate sends a taskgraph.update SSE event for a node status change.
func (d *Daemon) publishTaskUpdate(runID, nodeID, oldStatus, newStatus string) {
	d.sseBroker.Publish(runID, SSEEvent{
		Type: "taskgraph.update",
		Data: map[string]any{
			"node_id":    nodeID,
			"old_status": oldStatus,
			"new_status": newStatus,
			"timestamp":  time.Now(),
		},
	})
}

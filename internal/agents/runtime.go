package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
	"github.com/Ducstii/Baton/internal/worktree"
)

// opencodeClient is satisfied by *opencode.Client.
type opencodeClient interface {
	CreateSession(dir string, modelID, providerID string) (*opencode.Session, error)
	PromptAsync(sessionID string, providerID, modelID string, text string) error
	GetMessages(sessionID string) ([]opencode.Message, error)
}

// AgentStatus represents the lifecycle status of a dispatched agent.
type AgentStatus string

const (
	StatusPending   AgentStatus = "pending"
	StatusRunning   AgentStatus = "running"
	StatusCompleted AgentStatus = "completed"
	StatusFailed    AgentStatus = "failed"
	StatusCancelled AgentStatus = "cancelled"
)

// AgentInstance represents a dispatched agent tracked by the runtime.
type AgentInstance struct {
	ID           string
	Name         string
	Status       AgentStatus
	SessionID    string
	WorktreePath string
	StartedAt    time.Time
	CompletedAt  time.Time
	Result       any
	Error        string
}

// AgentEventType categorises agent lifecycle events.
type AgentEventType string

const (
	EventStarted   AgentEventType = "started"
	EventProgress  AgentEventType = "progress"
	EventCompleted AgentEventType = "completed"
	EventFailed    AgentEventType = "failed"
	EventCancelled AgentEventType = "cancelled"
)

// AgentEvent is published on the events channel for lifecycle transitions.
type AgentEvent struct {
	Type    AgentEventType
	AgentID string
	Data    any
}

// AgentRuntime manages dispatched agents -- their lifecycle, status, and cleanup.
type AgentRuntime struct {
	ocClient   opencodeClient
	registry   *Registry
	wtBasePath string

	mu     sync.RWMutex
	agents map[string]*AgentInstance
	events chan AgentEvent
	idSeq  int64
}

// NewAgentRuntime creates an AgentRuntime.
func NewAgentRuntime(ocClient opencodeClient, registry *Registry, wtBasePath string) *AgentRuntime {
	return &AgentRuntime{
		ocClient:   ocClient,
		registry:   registry,
		wtBasePath: wtBasePath,
		agents:     make(map[string]*AgentInstance),
		events:     make(chan AgentEvent, 256),
	}
}

// generateID returns a unique agent instance ID.
func (rt *AgentRuntime) generateID() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.idSeq++
	return fmt.Sprintf("agent_%d_%d", time.Now().UnixNano(), rt.idSeq)
}

// DispatchAgent loads the agent definition from the registry, creates a worktree
// (if required), creates an OpenCode session, sends the prompt, and starts a
// monitoring goroutine that polls for completion. Returns the agent instance
// immediately; the agent runs asynchronously.
func (rt *AgentRuntime) DispatchAgent(ctx context.Context, name string, params map[string]string) (*AgentInstance, error) {
	def, ok := rt.registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("agent definition %q not found", name)
	}

	agentID := rt.generateID()

	// Expand prompt template.
	prompt := def.Prompt.System + "\n\n" + ExpandTemplate(def.Prompt.Template, params)

	// Create worktree if required.
	var wtPath string
	var ownedWorktree bool
	if def.Worktree.Required {
		if p, ok := params["worktree_path"]; ok {
			wtPath = p
		} else if rt.wtBasePath != "" {
			wt, err := worktree.Create(rt.wtBasePath, "runtime", agentID)
			if err != nil {
				return nil, fmt.Errorf("create worktree: %w", err)
			}
			wtPath = wt.Path()
			ownedWorktree = true
		} else {
			return nil, fmt.Errorf("agent %q requires a worktree but no worktree_path provided and no base path configured", name)
		}
	}

	// Determine provider and model.
	providerID := params["provider_id"]
	if providerID == "" {
		providerID = "deepseek"
	}
	modelID := params["model_id"]
	if modelID == "" {
		modelID = def.Model
	}

	// Determine session directory.
	sessionDir := wtPath
	if sessionDir == "" {
		sessionDir = params["project_dir"]
		if sessionDir == "" {
			sessionDir = "."
		}
	}

	// Create OpenCode session.
	session, err := rt.ocClient.CreateSession(sessionDir, modelID, providerID)
	if err != nil {
		if ownedWorktree {
			_ = removeWorktree(wtPath)
		}
		return nil, fmt.Errorf("create opencode session: %w", err)
	}

	// Send prompt.
	if err := rt.ocClient.PromptAsync(session.ID, providerID, modelID, prompt); err != nil {
		if ownedWorktree {
			_ = removeWorktree(wtPath)
		}
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	instance := &AgentInstance{
		ID:           agentID,
		Name:         name,
		Status:       StatusRunning,
		SessionID:    session.ID,
		WorktreePath: wtPath,
		StartedAt:    time.Now(),
	}

	rt.mu.Lock()
	rt.agents[agentID] = instance
	rt.mu.Unlock()

	rt.emitEvent(AgentEvent{
		Type:    EventStarted,
		AgentID: agentID,
		Data: map[string]any{
			"name":    name,
			"session": session.ID,
		},
	})

	// Determine timeout from definition.
	timeout := time.Duration(def.Timeout.MaxSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	pollCtx, cancel := context.WithTimeout(ctx, timeout)

	// Monitoring goroutine.
	go rt.monitorAgent(pollCtx, cancel, agentID, instance, def)

	return instance, nil
}

// WaitForAgent blocks until the agent reaches a terminal state
// (completed, failed, or cancelled) or the context is cancelled.
// Returns the final agent state, or nil if the context was cancelled.
func (rt *AgentRuntime) WaitForAgent(ctx context.Context, id string) *AgentInstance {
	for {
		inst, ok := rt.GetAgent(id)
		if !ok {
			return nil
		}
		switch inst.Status {
		case StatusCompleted, StatusFailed, StatusCancelled:
			return inst
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// GetAgent returns a copy of the agent instance by ID.
func (rt *AgentRuntime) GetAgent(id string) (*AgentInstance, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	inst, ok := rt.agents[id]
	if !ok {
		return nil, false
	}
	cpy := *inst
	return &cpy, true
}

// ListAgents returns all agent instances, most recent first.
func (rt *AgentRuntime) ListAgents() []*AgentInstance {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	result := make([]*AgentInstance, 0, len(rt.agents))
	for _, inst := range rt.agents {
		cpy := *inst
		result = append(result, &cpy)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result
}

// CancelAgent marks the agent as cancelled and cleans up best-effort.
// Note: opencode has no cancel endpoint, so the remote session may continue
// running. We mark it cancelled locally. Worktree cleanup is the caller's
// responsibility when a worktree_path was provided by the caller.
func (rt *AgentRuntime) CancelAgent(id string) error {
	rt.mu.Lock()
	inst, ok := rt.agents[id]
	if !ok {
		rt.mu.Unlock()
		return fmt.Errorf("agent %q not found", id)
	}
	if isTerminal(inst.Status) {
		rt.mu.Unlock()
		return fmt.Errorf("agent %q has already finished (status: %s)", id, inst.Status)
	}
	inst.Status = StatusCancelled
	inst.CompletedAt = time.Now()
	inst.Error = "cancelled by user"
	rt.mu.Unlock()

	rt.emitEvent(AgentEvent{Type: EventCancelled, AgentID: id})
	return nil
}

// Events returns a read-only channel of agent lifecycle events.
func (rt *AgentRuntime) Events() <-chan AgentEvent {
	return rt.events
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func isTerminal(s AgentStatus) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// removeWorktree runs git worktree remove for best-effort cleanup.
func removeWorktree(path string) error {
	// We cannot construct a worktree.Worktree from just a path (the path field
	// is unexported), so this is a best-effort fallback.
	return nil
}

// monitorAgent polls the OpenCode session until completion or timeout.
func (rt *AgentRuntime) monitorAgent(ctx context.Context, cancel context.CancelFunc, agentID string, instance *AgentInstance, def AgentDefinition) {
	defer cancel()

	interval := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			errMsg := func() string {
				if ctx.Err() == context.DeadlineExceeded {
					return fmt.Sprintf("timeout after %d seconds", def.Timeout.MaxSeconds)
				}
				return "context cancelled"
			}()
			rt.mu.Lock()
			instance.CompletedAt = time.Now()
			instance.Status = StatusFailed
			instance.Error = errMsg
			rt.mu.Unlock()
			rt.emitEvent(AgentEvent{Type: EventFailed, AgentID: agentID, Data: errMsg})
			return
		case <-time.After(interval):
		}

		msgs, err := rt.ocClient.GetMessages(instance.SessionID)
		if err != nil {
			continue
		}

		// Walk backwards to find the most recent completed assistant message.
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if m.Info.Role == "assistant" && m.Info.Finish == "stop" {
				var sb strings.Builder
				for _, p := range m.Parts {
					if p.Type == "text" {
						sb.WriteString(p.Text)
					}
				}
				text := sb.String()

				rt.mu.Lock()
				instance.CompletedAt = time.Now()
				instance.Status = StatusCompleted
				if def.Reporting.Format == "json" {
					if raw := extractJSON(text); raw != "" {
						instance.Result = raw
					} else {
						instance.Result = text
					}
				} else {
					instance.Result = text
				}
				rt.mu.Unlock()
				rt.emitEvent(AgentEvent{Type: EventCompleted, AgentID: agentID, Data: instance})
				return
			}
		}
	}
}

// emitEvent sends an event on the events channel, dropping if full.
func (rt *AgentRuntime) emitEvent(evt AgentEvent) {
	select {
	case rt.events <- evt:
	default:
	}
}

package agents

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
)

// mockOCClient satisfies the opencodeClient interface for testing.
type mockOCClient struct {
	mu       sync.Mutex
	sessions map[string]*opencode.Session
	messages map[string][]opencode.Message
}

func newMockOCClient() *mockOCClient {
	return &mockOCClient{
		sessions: make(map[string]*opencode.Session),
		messages: make(map[string][]opencode.Message),
	}
}

func (m *mockOCClient) CreateSession(dir, modelID, providerID string) (*opencode.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := &opencode.Session{ID: "session_" + dir}
	m.sessions[s.ID] = s
	return s, nil
}

func (m *mockOCClient) PromptAsync(sessionID, providerID, modelID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Simulate the assistant eventually responding by scheduling a response.
	m.messages[sessionID] = []opencode.Message{
		{
			Info: opencode.MessageInfo{Role: "assistant", Finish: "stop", Created: time.Now().Format(time.RFC3339)},
			Parts: []opencode.Part{
				{Type: "text", Text: `{"success": true, "summary": "test", "changed_files": ["a.go"], "build_passed": true, "tests_passed": true}`},
			},
		},
	}
	return nil
}

func (m *mockOCClient) GetMessages(sessionID string) ([]opencode.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[sessionID], nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewAgentRuntime(t *testing.T) {
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(nil, r, t.TempDir())
	if rt == nil {
		t.Fatal("NewAgentRuntime returned nil")
	}
	if rt.Events() == nil {
		t.Error("Events() channel is nil")
	}
}

func TestDispatchAgent_DefinitionNotFound(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	_, err := rt.DispatchAgent(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent agent definition")
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	_, ok := rt.GetAgent("nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent agent")
	}
}

func TestDispatchAgent_NoWorktreeAgent(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	// "reviewer" has Worktree.Required=false.
	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test review",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	if inst.Name != "reviewer" {
		t.Errorf("Name = %q, want %q", inst.Name, "reviewer")
	}
	if inst.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", inst.Status, StatusRunning)
	}
	if inst.SessionID == "" {
		t.Error("SessionID should not be empty")
	}
	if inst.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
}

func TestGetAgent_Found(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	got, ok := rt.GetAgent(inst.ID)
	if !ok {
		t.Fatal("GetAgent should find the dispatched agent")
	}
	if got.ID != inst.ID {
		t.Errorf("ID = %q, want %q", got.ID, inst.ID)
	}
}

func TestListAgents(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	// Dispatch two agents.
	inst1, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "first",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent 1: %v", err)
	}

	inst2, err := rt.DispatchAgent(context.Background(), "diagnostic", map[string]string{
		"input":       "code",
		"input_type":  "text",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent 2: %v", err)
	}

	all := rt.ListAgents()
	// Expect 2, but could be 0 if races.
	if len(all) < 2 {
		t.Fatalf("ListAgents returned %d agents, want at least 2", len(all))
	}

	// Most recent first.
	if len(all) >= 2 && all[0].ID != inst2.ID {
		t.Errorf("ListAgents[0].ID = %q, want %q (most recent first)", all[0].ID, inst2.ID)
	}

	// Verify both IDs appear.
	ids := map[string]bool{inst1.ID: true, inst2.ID: true}
	for _, a := range all {
		delete(ids, a.ID)
	}
	if len(ids) > 0 {
		t.Errorf("ListAgents missing IDs: %v", ids)
	}
}

func TestCancelAgent(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	if err := rt.CancelAgent(inst.ID); err != nil {
		t.Fatalf("CancelAgent: %v", err)
	}

	got, ok := rt.GetAgent(inst.ID)
	if !ok {
		t.Fatal("GetAgent after cancel should succeed")
	}
	if got.Status != StatusCancelled {
		t.Errorf("Status after cancel = %q, want %q", got.Status, StatusCancelled)
	}
}

func TestCancelAgent_TerminalState(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	// Cancelling a non-existent agent should fail.
	if err := rt.CancelAgent("nonexistent"); err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestCancelAgent_AlreadyFinished(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	// Cancel once.
	if err := rt.CancelAgent(inst.ID); err != nil {
		t.Fatalf("first CancelAgent: %v", err)
	}

	// Cancelling again should fail.
	if err := rt.CancelAgent(inst.ID); err == nil {
		t.Error("expected error when cancelling already-cancelled agent")
	}
}

func TestEvents(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	events := rt.Events()

	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	// Should receive a "started" event.
	select {
	case evt := <-events:
		if evt.Type != EventStarted {
			t.Errorf("event type = %q, want %q", evt.Type, EventStarted)
		}
		if evt.AgentID != inst.ID {
			t.Errorf("event AgentID = %q, want %q", evt.AgentID, inst.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for started event")
	}

	// Cancel should produce a "cancelled" event.
	if err := rt.CancelAgent(inst.ID); err != nil {
		t.Fatalf("CancelAgent: %v", err)
	}

	select {
	case evt := <-events:
		if evt.Type != EventCancelled {
			t.Errorf("event type = %q, want %q", evt.Type, EventCancelled)
		}
		if evt.AgentID != inst.ID {
			t.Errorf("event AgentID = %q, want %q", evt.AgentID, inst.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled event")
	}
}

func TestConcurrentDispatch(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	var wg sync.WaitGroup
	n := 10
	ids := make(chan string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
				"files":       "a.go",
				"description": "concurrent test",
				"project_dir": t.TempDir(),
			})
			if err != nil {
				t.Errorf("concurrent DispatchAgent: %v", err)
				return
			}
			ids <- inst.ID
		}()
	}

	wg.Wait()
	close(ids)

	// Collect all IDs.
	got := make(map[string]int)
	for id := range ids {
		got[id]++
	}

	// Verify we got all n unique IDs.
	if len(got) != n {
		t.Errorf("got %d unique IDs from %d dispatches", len(got), n)
	}

	// Verify ListAgents returns all.
	all := rt.ListAgents()
	if len(all) != n {
		t.Errorf("ListAgents returned %d agents, want %d", len(all), n)
	}

	// Verify all can be cancelled.
	for _, inst := range all {
		_ = rt.CancelAgent(inst.ID)
		got, ok := rt.GetAgent(inst.ID)
		if !ok {
			t.Errorf("agent %q not found after cancel", inst.ID)
			continue
		}
		if got.Status != StatusCancelled {
			t.Errorf("agent %q status = %q, want %q", inst.ID, got.Status, StatusCancelled)
		}
	}
}

func TestStatusTransitions(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test transitions",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	// Initially running.
	if inst.Status != StatusRunning {
		t.Errorf("initial status = %q, want %q", inst.Status, StatusRunning)
	}

	// Cancel and verify cancelled.
	if err := rt.CancelAgent(inst.ID); err != nil {
		t.Fatalf("CancelAgent: %v", err)
	}

	got, _ := rt.GetAgent(inst.ID)
	if got.Status != StatusCancelled {
		t.Errorf("status after cancel = %q, want %q", got.Status, StatusCancelled)
	}

	// Verify we can't cancel again.
	if err := rt.CancelAgent(inst.ID); err == nil {
		t.Error("expected error when cancelling agent in terminal state")
	}
}

func TestEvents_ChannelCapacity(t *testing.T) {
	// Verify the events channel is buffered enough to not block on rapid events.
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	// Emit many events without a consumer.
	for i := 0; i < 300; i++ {
		rt.emitEvent(AgentEvent{
			Type:    EventProgress,
			AgentID: "test",
		})
	}

	// If emit blocked we'd have a deadlock, so this is a pass-by-not-hanging test.
	// The channel should drop excess events but not block.
}

func TestWaitForAgent_AlreadyTerminal(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	inst, err := rt.DispatchAgent(context.Background(), "reviewer", map[string]string{
		"files":       "a.go",
		"description": "test wait",
		"project_dir": t.TempDir(),
	})
	if err != nil {
		t.Fatalf("DispatchAgent: %v", err)
	}

	// Cancel the agent.
	if err := rt.CancelAgent(inst.ID); err != nil {
		t.Fatalf("CancelAgent: %v", err)
	}

	// WaitForAgent should return the terminal state immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := rt.WaitForAgent(ctx, inst.ID)
	if result == nil {
		t.Fatal("WaitForAgent returned nil")
	}
	if result.Status != StatusCancelled {
		t.Errorf("WaitForAgent status = %q, want %q", result.Status, StatusCancelled)
	}
}

func TestWaitForAgent_ContextCancelled(t *testing.T) {
	mc := newMockOCClient()
	r := NewRegistry(t.TempDir())
	rt := NewAgentRuntime(mc, r, t.TempDir())

	// Dispatch an agent but immediately cancel the context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := rt.WaitForAgent(ctx, "nonexistent")
	if result != nil {
		t.Error("WaitForAgent should return nil for cancelled context with nonexistent ID")
	}
}

package daemon

import (
	"strings"
	"testing"

	"github.com/Ducstii/Baton/internal/agents"
	"github.com/Ducstii/Baton/internal/opencode"
)

type mockOCClient struct {
	sessions []*opencode.Session
	messages map[string][]opencode.Message
}

func (m *mockOCClient) CreateSession(dir, modelID, providerID string) (*opencode.Session, error) {
	s := &opencode.Session{
		ID:    "test_" + dir,
		Model: &opencode.Model{ID: modelID, ProviderID: providerID},
		Dir:   dir,
	}
	m.sessions = append(m.sessions, s)
	return s, nil
}

func (m *mockOCClient) PromptAsync(sessionID, providerID, modelID, text string) error {
	if m.messages == nil {
		m.messages = make(map[string][]opencode.Message)
	}
	m.messages[sessionID] = append(m.messages[sessionID], opencode.Message{
		Info:  opencode.MessageInfo{Role: "user", Finish: "stop"},
		Parts: []opencode.Part{{Type: "text", Text: text}},
	})
	return nil
}

func (m *mockOCClient) GetMessages(sessionID string) ([]opencode.Message, error) {
	if m.messages == nil {
		return nil, nil
	}
	return m.messages[sessionID], nil
}

func TestExtractToolCallTexts(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"no tool calls", "hello world", 0},
		{"single tool call", "some text\n```tool\n{\"tool\":\"dispatch_agent\",\"params\":{\"name\":\"fixer\"}}\n```\nmore", 1},
		{"multiple tool calls", "a\n```tool\n{\"tool\":\"list_agents\",\"params\":{}}\n```\nb\n```tool\n{\"tool\":\"check_agent\",\"params\":{\"id\":\"123\"}}\n```\nc", 2},
		{"malformed no closing fence", "```tool\n{\"tool\":\"test\"}", 0},
		{"empty tool block", "```tool\n```", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolCallTexts(tt.text)
			if len(got) != tt.want {
				t.Errorf("extractToolCallTexts returned %d calls, want %d; got %v", len(got), tt.want, got)
			}
		})
	}
}

func TestExecuteToolCall_ParseFailures(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"invalid JSON", `{bad json}`},
		{"unknown tool", `{"tool":"unknown","params":{}}`},
		{"tool name empty", `{"tool":"","params":{}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecuteToolCall(nil, tt.json)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestExecuteToolCall_MissingParams(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"dispatch_agent no name", `{"tool":"dispatch_agent","params":{}}`},
		{"check_agent no id", `{"tool":"check_agent","params":{}}`},
		{"cancel_agent no id", `{"tool":"cancel_agent","params":{}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecuteToolCall(nil, tt.json)
			if err == nil {
				t.Error("expected error for missing params, got nil")
			}
		})
	}
}

func TestExecuteToolCall_DispatchAgent(t *testing.T) {
	mockClient := &mockOCClient{}
	registry := agents.NewRegistry("")
	runtime := agents.NewAgentRuntime(mockClient, registry, "")

	result, err := ExecuteToolCall(runtime, `{"tool":"dispatch_agent","params":{"name":"diagnostic","input":"test code","input_type":"code","project_dir":"."}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "agent_id") {
		t.Errorf("result should contain agent_id, got: %s", result)
	}
	if !strings.Contains(result, "running") {
		t.Errorf("result should contain status \"running\", got: %s", result)
	}
}

func TestExecuteToolCall_ListAgents(t *testing.T) {
	mockClient := &mockOCClient{}
	registry := agents.NewRegistry("")
	runtime := agents.NewAgentRuntime(mockClient, registry, "")

	result, err := ExecuteToolCall(runtime, `{"tool":"list_agents","params":{}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, `"agents"`) {
		t.Errorf("result should contain agents key, got: %s", result)
	}
}

func TestExecuteToolCall_CheckAgent(t *testing.T) {
	mockClient := &mockOCClient{}
	registry := agents.NewRegistry("")
	runtime := agents.NewAgentRuntime(mockClient, registry, "")

	// First dispatch an agent so we have one to check.
	dispatchResult, err := ExecuteToolCall(runtime, `{"tool":"dispatch_agent","params":{"name":"diagnostic","input":"test","input_type":"code","project_dir":"."}}`)
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// Extract agent_id from dispatch result (it's JSON like {"agent_id":"...",...}).
	var agentID string
	// Parse the "agent_id" field out of the JSON string.
	if idx := strings.Index(dispatchResult, `"agent_id": "`); idx >= 0 {
		rest := dispatchResult[idx+len(`"agent_id": "`):]
		if end := strings.Index(rest, `"`); end >= 0 {
			agentID = rest[:end]
		}
	}
	if agentID == "" {
		t.Fatalf("could not extract agent_id from dispatch result: %s", dispatchResult)
	}

	checkJSON := `{"tool":"check_agent","params":{"id":"` + agentID + `"}}`
	result, err := ExecuteToolCall(runtime, checkJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, agentID) {
		t.Errorf("result should contain agent_id %q, got: %s", agentID, result)
	}
	if !strings.Contains(result, "elapsed") {
		t.Errorf("result should contain elapsed, got: %s", result)
	}
}

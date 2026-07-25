package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ducstii/Baton/internal/agents"
)

// ToolCall is a parsed tool invocation from the brain.
type ToolCall struct {
	Tool   string            `json:"tool"`
	Params map[string]string `json:"params"`
}

// ExecuteToolCall parses and executes a tool call JSON string from the brain.
func ExecuteToolCall(runtime *agents.AgentRuntime, runStore *RunStore, runID string, toolCall string) (string, error) {
	var tc ToolCall
	if err := json.Unmarshal([]byte(toolCall), &tc); err != nil {
		return "", fmt.Errorf("parse tool call: %w", err)
	}

	switch tc.Tool {
	case "dispatch_agent":
		result, err := executeDispatch(runtime, tc.Params)
		if err == nil && runStore != nil && runID != "" {
			updateRunStatus(runStore, runID)
		}
		return result, err
	case "check_agent":
		return executeCheck(runtime, tc.Params)
	case "cancel_agent":
		return executeCancel(runtime, tc.Params)
	case "list_agents":
		return executeList(runtime)
	default:
		return "", fmt.Errorf("unknown tool: %q", tc.Tool)
	}
}

// extractToolCallTexts extracts raw JSON strings from ```tool ... ``` blocks.
func extractToolCallTexts(text string) []string {
	var calls []string
	remaining := text
	for {
		start := strings.Index(remaining, "```tool")
		if start < 0 {
			break
		}
		after := remaining[start+len("```tool"):]
		end := strings.Index(after, "```")
		if end < 0 {
			break
		}
		raw := strings.TrimSpace(after[:end])
		if raw != "" {
			calls = append(calls, raw)
		}
		remaining = after[end+3:]
	}
	return calls
}

func executeDispatch(runtime *agents.AgentRuntime, params map[string]string) (string, error) {
	name := params["name"]
	if name == "" {
		return "", fmt.Errorf("dispatch_agent requires 'name' parameter")
	}

	inst, err := runtime.DispatchAgent(context.Background(), name, params)
	if err != nil {
		return "", fmt.Errorf("dispatch agent: %w", err)
	}

	return fmt.Sprintf(`{"agent_id": %q, "status": %q, "session_id": %q}`, inst.ID, inst.Status, inst.SessionID), nil
}

func executeCheck(runtime *agents.AgentRuntime, params map[string]string) (string, error) {
	id := params["id"]
	if id == "" {
		return "", fmt.Errorf("check_agent requires 'id' parameter")
	}

	inst, ok := runtime.GetAgent(id)
	if !ok {
		return "", fmt.Errorf("agent %q not found", id)
	}

	elapsed := time.Since(inst.StartedAt).String()
	var outputExcerpt string
	if inst.Result != nil {
		s, _ := inst.Result.(string)
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		outputExcerpt = s
	}

	return fmt.Sprintf(`{"agent_id": %q, "status": %q, "elapsed": %q, "output": %q, "error": %q}`,
		inst.ID, inst.Status, elapsed, outputExcerpt, inst.Error), nil
}

func executeCancel(runtime *agents.AgentRuntime, params map[string]string) (string, error) {
	id := params["id"]
	if id == "" {
		return "", fmt.Errorf("cancel_agent requires 'id' parameter")
	}

	if err := runtime.CancelAgent(id); err != nil {
		return "", fmt.Errorf("cancel agent: %w", err)
	}

	return fmt.Sprintf(`{"agent_id": %q, "status": "cancelled"}`, id), nil
}

// updateRunStatus transitions a run from "pending" to "active" when an agent
// is dispatched.
func updateRunStatus(store *RunStore, runID string) {
	r, ok := store.Get(runID)
	if !ok || r.Status != StatusPending {
		return
	}
	updated := *r
	updated.Status = StatusActive
	updated.UpdatedAt = time.Now()
	store.Set(runID, &updated)
}

func executeList(runtime *agents.AgentRuntime) (string, error) {
	agents := runtime.ListAgents()
	if len(agents) == 0 {
		return `{"agents": []}`, nil
	}

	var parts []string
	for _, a := range agents {
		parts = append(parts, fmt.Sprintf(`{"id": %q, "name": %q, "status": %q, "elapsed": %q}`,
			a.ID, a.Name, a.Status, time.Since(a.StartedAt).String()))
	}

	return fmt.Sprintf(`{"agents": [%s]}`, strings.Join(parts, ",")), nil
}

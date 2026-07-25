package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
)

// DiagnosticIssue is one issue found by the diagnostic agent.
type DiagnosticIssue struct {
	ID              string   `json:"id"`
	Description     string   `json:"description"`
	Confidence      string   `json:"confidence"`
	Group           string   `json:"group"`
	SuspectedFiles  []string `json:"suspected_files"`
	DependencyEdges []string `json:"dependency_edges"`
}

// WorkUnitGroup groups related issues into a single fixer task.
type WorkUnitGroup struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Issues      []string `json:"issues"`
}

// DiagnosticResult is the full output from a diagnostic run.
type DiagnosticResult struct {
	Issues    []DiagnosticIssue `json:"issues"`
	WorkUnits []WorkUnitGroup   `json:"work_units"`
}

// FixerResult is the output from a fixer session.
type FixerResult struct {
	Success      bool     `json:"success"`
	Summary      string   `json:"summary"`
	ChangedFiles []string `json:"changed_files"`
	BuildPassed  bool     `json:"build_passed"`
	TestsPassed  bool     `json:"tests_passed"`
}

// extractJSON strips markdown fences and leading/trailing non-JSON content
// from s, returning the first JSON object or array it finds.
func extractJSON(s string) string {
	// Strip markdown code fences.
	s = strings.TrimSpace(s)

	// Remove ```json ... ``` block.
	if idx := strings.Index(s, "```json"); idx >= 0 {
		end := strings.LastIndex(s, "```")
		if end > idx {
			s = s[idx+7 : end]
		}
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		end := strings.LastIndex(s, "```")
		if end > idx {
			s = s[idx+3 : end]
		}
	}

	s = strings.TrimSpace(s)

	// Find the outermost { or [.
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return ""
	}
	s = s[start:]

	// Find matching closing bracket -- crude but sufficient for one level.
	var depth int
	var inStr bool
	for i, r := range s {
		if inStr {
			if r == '\\' {
				continue // skip escaped char
			}
			if r == '"' {
				inStr = false
			}
			continue
		}
		switch r {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}

	return s
}

// ParseDiagnosticResult unmarshals raw JSON into a DiagnosticResult.
func ParseDiagnosticResult(data []byte) (*DiagnosticResult, error) {
	var res DiagnosticResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse diagnostic result: %w", err)
	}
	// Basic validation.
	if len(res.Issues) == 0 {
		return nil, fmt.Errorf("diagnostic result contains no issues")
	}
	for _, wu := range res.WorkUnits {
		if len(wu.Issues) == 0 {
			return nil, fmt.Errorf("work unit %q contains no issues", wu.ID)
		}
	}
	return &res, nil
}

// ParseFixerResult unmarshals raw JSON into a FixerResult.
func ParseFixerResult(data []byte) (*FixerResult, error) {
	var res FixerResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse fixer result: %w", err)
	}
	return &res, nil
}

// ---------------------------------------------------------------------------
// Backward-compatible wrappers backed by AgentRuntime
// ---------------------------------------------------------------------------

// RunDiagnostic dispatches a diagnostic agent and waits for the result.
// It creates a temporary AgentRuntime internally for backward compatibility
// with existing callers. New code should use AgentRuntime directly.
func RunDiagnostic(client *opencode.Client, registry *Registry, projectDir, input, inputType, providerID, modelID string) (*DiagnosticResult, error) {
	rt := NewAgentRuntime(client, registry, "")

	params := map[string]string{
		"input":       input,
		"input_type":  inputType,
		"provider_id": providerID,
		"model_id":    modelID,
		"project_dir": projectDir,
	}

	instance, err := rt.DispatchAgent(context.Background(), "diagnostic", params)
	if err != nil {
		return nil, fmt.Errorf("dispatch diagnostic: %w", err)
	}

	inst := rt.WaitForAgent(context.Background(), instance.ID)
	if inst == nil {
		return nil, fmt.Errorf("diagnostic agent did not complete")
	}
	if inst.Status != StatusCompleted {
		return nil, fmt.Errorf("diagnostic agent failed: %s", inst.Error)
	}

	raw, ok := inst.Result.(string)
	if !ok || raw == "" {
		return nil, fmt.Errorf("diagnostic agent returned no result data")
	}

	return ParseDiagnosticResult([]byte(raw))
}

// RunFixer dispatches a fixer agent and waits for the result.
// It creates a temporary AgentRuntime internally for backward compatibility
// with existing callers. New code should use AgentRuntime directly.
func RunFixer(client *opencode.Client, registry *Registry, worktreePath string, workUnit WorkUnitGroup, providerID, modelID string) (*FixerResult, error) {
	rt := NewAgentRuntime(client, registry, "")

	var issueLines []string
	for _, issueID := range workUnit.Issues {
		issueLines = append(issueLines, "- "+issueID)
	}

	params := map[string]string{
		"work_unit_id":  workUnit.ID,
		"description":   workUnit.Description,
		"issues":        strings.Join(issueLines, "\n"),
		"provider_id":   providerID,
		"model_id":      modelID,
		"worktree_path": worktreePath,
	}

	instance, err := rt.DispatchAgent(context.Background(), "fixer", params)
	if err != nil {
		return nil, fmt.Errorf("dispatch fixer: %w", err)
	}

	inst := rt.WaitForAgent(context.Background(), instance.ID)
	if inst == nil {
		return nil, fmt.Errorf("fixer agent did not complete")
	}
	if inst.Status != StatusCompleted {
		return nil, fmt.Errorf("fixer agent failed: %s", inst.Error)
	}

	raw, ok := inst.Result.(string)
	if !ok || raw == "" {
		return nil, fmt.Errorf("fixer agent returned no result data")
	}

	return ParseFixerResult([]byte(raw))
}

// ---------------------------------------------------------------------------
// Legacy helpers (kept for backward compatibility)
// ---------------------------------------------------------------------------

// pollComplete polls GetMessages until an assistant message with finish="stop"
// is found, or the timeout expires. DEPRECATED: AgentRuntime does its own
// polling. Kept for any existing external consumers.
func pollComplete(client *opencode.Client, sessionID string, timeout time.Duration) (string, error) {
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		msgs, err := client.GetMessages(sessionID)
		if err != nil {
			return "", fmt.Errorf("get messages: %w", err)
		}

		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if m.Info.Role == "assistant" && m.Info.Finish == "stop" {
				var sb strings.Builder
				for _, p := range m.Parts {
					if p.Type == "text" {
						sb.WriteString(p.Text)
					}
				}
				return sb.String(), nil
			}
		}

		time.Sleep(interval)
	}

	return "", fmt.Errorf("timeout after %v waiting for assistant response", timeout)
}

// parseDiagnosticResult is kept for backward compatibility within the package.
// New code should use ParseDiagnosticResult.
func parseDiagnosticResult(data []byte) (*DiagnosticResult, error) {
	return ParseDiagnosticResult(data)
}

// parseFixerResult is kept for backward compatibility within the package.
// New code should use ParseFixerResult.
func parseFixerResult(data []byte) (*FixerResult, error) {
	return ParseFixerResult(data)
}

// Package agents dispatches OpenCode sessions for code diagnosis and fixing.
package agents

import (
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

// pollComplete polls GetMessages until an assistant message with finish="stop"
// is found, or the timeout expires. It returns the concatenated text of the
// first such assistant message.
func pollComplete(client *opencode.Client, sessionID string, timeout time.Duration) (string, error) {
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		msgs, err := client.GetMessages(sessionID)
		if err != nil {
			return "", fmt.Errorf("get messages: %w", err)
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
				return sb.String(), nil
			}
		}

		time.Sleep(interval)
	}

	return "", fmt.Errorf("timeout after %v waiting for assistant response", timeout)
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

	// Find matching closing bracket — crude but sufficient for one level.
	// Walk runes to handle nesting.
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

// parseDiagnosticResult unmarshals raw JSON into a DiagnosticResult.
func parseDiagnosticResult(data []byte) (*DiagnosticResult, error) {
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

// RunDiagnostic creates an OpenCode session, sends a diagnostic prompt, waits
// for the model to finish, and returns the parsed structured result.
func RunDiagnostic(client *opencode.Client, input, inputType, providerID, modelID string) (*DiagnosticResult, error) {
	projectDir := "." // opencode serve resolves from its workspace root; caller should have CWD set.

	session, err := client.CreateSession(projectDir, modelID, providerID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	prompt := fmt.Sprintf(`You are a code diagnostician. Analyze the following input and produce a structured JSON issue list.

Input type: %s
Input: %s

For each issue, provide:
- id: unique short identifier
- description: what is wrong
- confidence: "high", "medium", or "low"
- group: semantic category (e.g., "null_reference", "logic_error", "performance")
- suspected_files: list of file paths likely containing the bug
- dependency_edges: list of other issue IDs this issue depends on (empty if none)

Group related issues into work units. A work unit is a set of related issues that should be fixed together.
Each work unit needs:
- id: "wu_01" style
- description: what this work unit addresses
- issues: list of issue IDs

Respond ONLY with valid JSON, no other text. The JSON must have this exact structure:
{"issues": [{"id": "...", "description": "...", "confidence": "...", "group": "...", "suspected_files": ["..."], "dependency_edges": ["..."]}], "work_units": [{"id": "...", "description": "...", "issues": ["..."]}]}`, inputType, input)

	if err := client.PromptAsync(session.ID, providerID, modelID, prompt); err != nil {
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	text, err := pollComplete(client, session.ID, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("wait for completion: %w", err)
	}

	raw := extractJSON(text)
	if raw == "" {
		return nil, fmt.Errorf("no JSON found in assistant response")
	}

	return parseDiagnosticResult([]byte(raw))
}

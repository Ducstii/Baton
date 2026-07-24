package agents

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
)

// FixerResult is the output from a fixer session.
type FixerResult struct {
	Success      bool     `json:"success"`
	Summary      string   `json:"summary"`
	ChangedFiles []string `json:"changed_files"`
	BuildPassed  bool     `json:"build_passed"`
	TestsPassed  bool     `json:"tests_passed"`
}

// parseFixerResult unmarshals raw JSON into a FixerResult.
func parseFixerResult(data []byte) (*FixerResult, error) {
	var res FixerResult
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("parse fixer result: %w", err)
	}
	return &res, nil
}

// RunFixer creates an OpenCode session in the given worktree, sends a prompt
// describing the issues to fix, waits for completion, and returns the result.
func RunFixer(client *opencode.Client, worktreePath string, workUnit WorkUnitGroup, providerID, modelID string) (*FixerResult, error) {
	session, err := client.CreateSession(worktreePath, modelID, providerID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Build issue detail lines.
	var issueLines []string
	for _, issueID := range workUnit.Issues {
		issueLines = append(issueLines, "- "+issueID)
	}

	prompt := fmt.Sprintf(`You are a code fixer working in a git worktree. Fix the following issues. You MUST use file editing tools (Write, Edit) to modify the files — do not just describe the changes. Make the actual edits.

Work Unit: %s
Description: %s
Issues:
%s

Instructions:
1. Read the relevant files first.
2. Use Edit or Write to make minimal changes. Touch only what is necessary.
3. After all changes, run the build command if available.
4. If tests exist, run them.

After completing ALL changes, respond with this JSON:
{"success": true, "summary": "what you changed", "changed_files": ["file1.go"], "build_passed": true, "tests_passed": true}

Set success to false if you could not complete the changes. Respond ONLY with the JSON.`, workUnit.ID, workUnit.Description, strings.Join(issueLines, "\n"))

	if err := client.PromptAsync(session.ID, providerID, modelID, prompt); err != nil {
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	text, err := pollComplete(client, session.ID, 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("wait for completion: %w", err)
	}

	raw := extractJSON(text)
	if raw == "" {
		return nil, fmt.Errorf("no JSON found in assistant response")
	}

	return parseFixerResult([]byte(raw))
}

package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Ducstii/Baton/internal/agents"
	"github.com/Ducstii/Baton/internal/opencode"
)

const maxBrainIterations = 10
const brainPollInterval = 500 * time.Millisecond
const brainSystemPromptTimeout = 30 * time.Second

// BrainSession manages the persistent orchestrator OpenCode session.
type BrainSession struct {
	sessionID    string
	ocClient     *opencode.Client
	agentRuntime *agents.AgentRuntime
	registry     *agents.Registry
	projectPath  string
	modelID      string
	providerID   string
	mu           sync.Mutex
}

// NewBrainSession creates or resumes a brain session.
func NewBrainSession(ocClient *opencode.Client, runtime *agents.AgentRuntime, registry *agents.Registry, projectPath, providerID, modelID string) (*BrainSession, error) {
	if providerID == "" {
		providerID = "deepseek"
	}
	if modelID == "" {
		modelID = "deepseek-v4-pro"
	}

	session, err := ocClient.CreateSession(projectPath, modelID, providerID)
	if err != nil {
		return nil, fmt.Errorf("create brain session: %w", err)
	}

	bs := &BrainSession{
		sessionID:    session.ID,
		ocClient:     ocClient,
		agentRuntime: runtime,
		registry:     registry,
		projectPath:  projectPath,
		modelID:      modelID,
		providerID:   providerID,
	}

	// Send the system prompt as the first message.
	if err := ocClient.PromptAsync(session.ID, providerID, modelID, bs.SystemPrompt()); err != nil {
		return nil, fmt.Errorf("send system prompt: %w", err)
	}

	// Wait for the brain to acknowledge the system prompt so that message
	// tracking in SendMessage starts from a clean state.
	bs.waitForFirstResponse()

	return bs, nil
}

// SendMessage sends a user message to the brain and handles the tool-call loop.
// If the brain responds with tool calls, they are executed and the results are
// fed back as follow-up prompts. Returns the final text response.
func (b *BrainSession) SendMessage(ctx context.Context, message string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get current message count to track new responses.
	msgs, err := b.ocClient.GetMessages(b.sessionID)
	if err != nil {
		return "", fmt.Errorf("get messages: %w", err)
	}
	startCount := len(msgs)

	for range maxBrainIterations {
		// Send message to brain.
		if err := b.ocClient.PromptAsync(b.sessionID, b.providerID, b.modelID, message); err != nil {
			return "", fmt.Errorf("send prompt: %w", err)
		}

		// Poll for brain's response.
		response, err := b.pollNewResponse(ctx, startCount)
		if err != nil {
			return "", err
		}

		// Check for tool calls.
		calls := extractToolCallTexts(response)
		if len(calls) == 0 {
			return response, nil
		}

		// Execute tool calls and build results.
		var results []string
		for _, raw := range calls {
			result, err := ExecuteToolCall(b.agentRuntime, raw)
			if err != nil {
				results = append(results, fmt.Sprintf("Tool result: {\"error\": %q}", err.Error()))
			} else {
				results = append(results, "Tool result: "+result)
			}
		}

		// Send tool results back to the brain.
		message = strings.Join(results, "\n")

		// Update start count for the next round.
		msgs, err = b.ocClient.GetMessages(b.sessionID)
		if err != nil {
			return "", fmt.Errorf("get messages: %w", err)
		}
		startCount = len(msgs)
	}

	return "", fmt.Errorf("brain exceeded max iterations (%d)", maxBrainIterations)
}

// SystemPrompt builds the brain's system prompt with registry and project context.
func (b *BrainSession) SystemPrompt() string {
	agentList := b.registry.List()

	var sb strings.Builder
	sb.WriteString("You are Baton's orchestrator. You coordinate subagents to accomplish tasks.\n\n")
	sb.WriteString("Available agents:\n")
	for _, a := range agentList {
		fmt.Fprintf(&sb, "- %s: %s\n", a.Name, a.Description)
	}

	fmt.Fprintf(&sb, "\nProject: %s\n\n", b.projectPath)
	sb.WriteString("You can dispatch agents, check their status, cancel them, and list all running agents.\n")
	sb.WriteString("When you need to use a tool, include a JSON block in your response:\n\n")
	sb.WriteString("```tool\n")
	sb.WriteString(`{"tool": "dispatch_agent", "params": {"name": "fixer", "description": "...", "files": "..."}}` + "\n")
	sb.WriteString("```\n\n")
	sb.WriteString("Available tools:\n")
	sb.WriteString("- dispatch_agent: launch an agent. params: name, plus template variables matching the agent's template\n")
	sb.WriteString("- check_agent: get status. params: id\n")
	sb.WriteString("- cancel_agent: kill an agent. params: id\n")
	sb.WriteString("- list_agents: list all agents. params: none\n\n")
	sb.WriteString("Be concise. Report progress clearly. Ask the user when you need clarification.")

	return sb.String()
}

// Close shuts down the brain session.
func (b *BrainSession) Close() error {
	return nil
}

// pollNewResponse polls GetMessages until a new assistant response with a finish
// reason appears at or after minMsgs.
func (b *BrainSession) pollNewResponse(ctx context.Context, minMsgs int) (string, error) {
	for {
		msgs, err := b.ocClient.GetMessages(b.sessionID)
		if err != nil {
			return "", fmt.Errorf("get messages: %w", err)
		}

		for i := len(msgs) - 1; i >= minMsgs; i-- {
			m := msgs[i]
			if m.Info.Role == "assistant" && m.Info.Finish != "" {
				var sb strings.Builder
				for _, p := range m.Parts {
					if p.Type == "text" {
						sb.WriteString(p.Text)
					}
				}
				return sb.String(), nil
			}
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(brainPollInterval):
		}
	}
}

// waitForFirstResponse polls until the brain has produced its first response
// (to the system prompt). Non-fatal on timeout.
func (b *BrainSession) waitForFirstResponse() {
	ctx, cancel := context.WithTimeout(context.Background(), brainSystemPromptTimeout)
	defer cancel()

	for {
		msgs, err := b.ocClient.GetMessages(b.sessionID)
		if err == nil && len(msgs) >= 2 {
			last := msgs[len(msgs)-1]
			if last.Info.Role == "assistant" && last.Info.Finish != "" {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(brainPollInterval):
		}
	}
}

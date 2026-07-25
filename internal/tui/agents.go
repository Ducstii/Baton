package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AgentInfo represents a dispatched agent displayed in the agent list.
type AgentInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// agentsLoadedMsg is delivered when agents are fetched from the daemon.
type agentsLoadedMsg []AgentInfo

// AgentList is the left-pane model displaying dispatched agents.
type AgentList struct {
	agents   []AgentInfo
	cursor   int
	expanded map[string]bool
}

// NewAgentList creates an AgentList.
func NewAgentList() AgentList {
	return AgentList{
		expanded: make(map[string]bool),
	}
}

// FetchAgents returns a tea.Cmd that fetches agents from GET /runs/{id}/agents.
func (a *AgentList) FetchAgents(daemonURL, token, runID string) tea.Cmd {
	return func() tea.Msg {
		req, err := http.NewRequest(http.MethodGet, daemonURL+"/runs/"+runID+"/agents", nil)
		if err != nil {
			return errMsg{err}
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return errMsg{fmt.Errorf("daemon: %s (HTTP %d)", resp.Status, resp.StatusCode)}
		}

		var agents []AgentInfo
		if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
			return errMsg{err}
		}
		return agentsLoadedMsg(agents)
	}
}

// agentStatusColor returns the lipgloss color for an agent status.
func agentStatusColor(status string) lipgloss.Color {
	switch status {
	case "running":
		return Green
	case "completed":
		return Dim
	case "failed":
		return Red
	case "cancelled":
		return Yellow
	default:
		return Dim
	}
}

// View renders the agent list at the given width and height.
func (a *AgentList) View(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(Accent).Render("Agents")

	if len(a.agents) == 0 {
		empty := lipgloss.NewStyle().Foreground(Dim).Padding(0, 1).Render("No agents yet")
		return lipgloss.JoinVertical(lipgloss.Top, title, "", empty)
	}

	var items []string
	for i, agent := range a.agents {
		items = append(items, a.renderAgent(agent, i, width))
	}

	content := lipgloss.JoinVertical(lipgloss.Top, items...)
	return lipgloss.JoinVertical(lipgloss.Top, title, "", content)
}

// renderAgent renders a single agent item in the list.
func (a *AgentList) renderAgent(agent AgentInfo, idx int, width int) string {
	dotColor := agentStatusColor(agent.Status)
	dot := lipgloss.NewStyle().Foreground(dotColor).Render("●")

	selected := idx == a.cursor
	sel := "  "
	if selected {
		sel = "▎ "
	}
	selStyle := lipgloss.NewStyle()
	if selected {
		selStyle = selStyle.Foreground(Accent)
	}

	// Row 1: selection, dot, name, status
	name := lipgloss.NewStyle().Bold(true).Foreground(Text).Render(agent.Name)
	statusStr := lipgloss.NewStyle().Foreground(dotColor).Render(agent.Status)

	row1 := selStyle.Render(sel) + " " + dot + "  " + name + "  " + statusStr

	// Row 2: time ago and worktree path
	timeStr := timeAgo(agent.StartedAt)
	pathStr := ""
	if agent.WorktreePath != "" {
		pathStr = "  " + shortPath(agent.WorktreePath)
	}
	row2 := selStyle.Render(sel) + "    " + lipgloss.NewStyle().Foreground(Dim).Render(timeStr+pathStr)

	card := lipgloss.JoinVertical(lipgloss.Left, row1, row2)

	// Expanded details
	if a.expanded[agent.ID] {
		if agent.Error != "" {
			errStyle := lipgloss.NewStyle().Foreground(Red).Padding(0, 2)
			card = lipgloss.JoinVertical(lipgloss.Left, card,
				errStyle.Render("Error: "+agent.Error))
		}
		timeStr := fmt.Sprintf("Started: %s", agent.StartedAt.Format("15:04:05"))
		if !agent.CompletedAt.IsZero() {
			timeStr += fmt.Sprintf(" | Completed: %s", agent.CompletedAt.Format("15:04:05"))
		}
		timeLine := lipgloss.NewStyle().Foreground(Dim).Padding(0, 2).Render(timeStr)
		card = lipgloss.JoinVertical(lipgloss.Left, card, timeLine)
	}

	return card
}

// ---------------------------------------------------------------------------
// Shared utility functions
// ---------------------------------------------------------------------------

// timeAgo returns a human-readable relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		return t.Format("Jan 2")
	}
}

// shortPath shortens an absolute path for display.
func shortPath(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/Users/") {
		parts := strings.SplitN(p, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	if strings.HasPrefix(p, "/home/") {
		parts := strings.SplitN(p, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	if len(p) > 55 {
		return "..." + p[len(p)-52:]
	}
	return p
}

// shortID shortens a hex ID for display.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

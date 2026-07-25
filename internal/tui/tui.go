package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Pane identifies the active pane.
type Pane int

const (
	PaneLeft Pane = iota
	PaneRight
)

// RunSummary is a lightweight representation of a run for the TUI.
type RunSummary struct {
	ID          string    `json:"id"`
	ProjectPath string    `json:"project_path"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Message types for async operations.
type runsLoadedMsg []RunSummary
type errMsg struct{ error }
type tickMsg struct{}
type agentTickMsg struct{}

// Model is the main Bubbletea model for the Baton TUI with a two-pane layout.
type Model struct {
	client    *http.Client
	daemonURL string
	token     string

	width, height int

	// Panes
	activePane Pane
	agentList  AgentList
	chatView   ChatView

	// Run state
	runID   string
	runs    []RunSummary
	loading bool
	err     error
}

// New creates a new Model.
func New(daemonURL, token string) *Model {
	url := daemonURL
	if url == "" {
		url = "http://127.0.0.1:8080"
	}
	url = strings.TrimRight(url, "/")
	return &Model{
		client:    &http.Client{Timeout: 30 * time.Second},
		daemonURL: url,
		token:     token,
		agentList: NewAgentList(),
		chatView:  NewChatView(),
		loading:   true,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.fetchRuns(), m.pollTicker(), m.pollAgentTicker())
}

func (m *Model) pollTicker() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *Model) pollAgentTicker() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return agentTickMsg{}
	})
}

// hasRun checks if a run ID is in the current runs list.
func (m *Model) hasRun(id string) bool {
	for _, r := range m.runs {
		if r.ID == id {
			return true
		}
	}
	return false
}

// selectRun sets the current run and loads its agents and chat history.
func (m *Model) selectRun() tea.Cmd {
	if m.runID == "" {
		return nil
	}
	return tea.Batch(
		m.agentList.FetchAgents(m.daemonURL, m.token, m.runID),
		m.chatView.LoadHistory(m.daemonURL, m.token, m.runID),
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.chatView.input.Width = paneWidth(m.width, false) - 6
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case runsLoadedMsg:
		m.runs = []RunSummary(msg)
		m.loading = false
		m.err = nil
		// Auto-select the latest run if none selected or current run is gone.
		if len(m.runs) > 0 {
			if m.runID == "" || !m.hasRun(m.runID) {
				m.runID = m.runs[0].ID
				return m, m.selectRun()
			}
		} else {
			m.runID = ""
		}
		return m, nil

	case agentsLoadedMsg:
		m.agentList.agents = []AgentInfo(msg)
		return m, nil

	case historyLoadedMsg:
		m.chatView.messages = []ChatMessage(msg)
		m.chatView.loading = false
		m.chatView.scrollPos = len(m.chatView.messages)
		return m, nil

	case chatSentMsg:
		m.chatView.loading = false
		// Reload history to get the brain's response.
		if m.runID != "" {
			return m, m.chatView.LoadHistory(m.daemonURL, m.token, m.runID)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchRuns(), m.pollTicker())

	case agentTickMsg:
		if m.runID != "" {
			return m, tea.Batch(
				m.agentList.FetchAgents(m.daemonURL, m.token, m.runID),
				m.pollAgentTicker(),
			)
		}
		return m, m.pollAgentTicker()

	case errMsg:
		m.loading = false
		m.err = msg.error
		return m, nil

	default:
		return m, nil
	}
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab":
		return m.switchPane()

	case "r":
		m.loading = true
		return m, m.fetchRuns()

	default:
		if m.activePane == PaneLeft {
			return m.handleAgentKey(msg)
		}
		return m.handleChatKey(msg)
	}
}

func (m *Model) switchPane() (tea.Model, tea.Cmd) {
	if m.activePane == PaneLeft {
		m.activePane = PaneRight
		return m, m.chatView.FocusInput()
	}
	m.activePane = PaneLeft
	m.chatView.BlurInput()
	return m, nil
}

func (m *Model) handleAgentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.agentList.cursor > 0 {
			m.agentList.cursor--
		}
	case "down", "j":
		if m.agentList.cursor < len(m.agentList.agents)-1 {
			m.agentList.cursor++
		}
	case "enter":
		if len(m.agentList.agents) > 0 {
			key := m.agentList.agents[m.agentList.cursor].ID
			if m.agentList.expanded[key] {
				delete(m.agentList.expanded, key)
			} else {
				m.agentList.expanded[key] = true
			}
		}
	}
	return m, nil
}

func (m *Model) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := m.chatView.input.Value()
		if val != "" && m.runID != "" {
			m.chatView.loading = true
			msgText := val
			m.chatView.input.SetValue("")
			return m, m.chatView.SendMessage(m.daemonURL, m.token, m.runID, msgText)
		}
		return m, nil
	case "esc":
		m.chatView.input.SetValue("")
		return m, nil
	default:
		var cmd tea.Cmd
		m.chatView.input, cmd = m.chatView.input.Update(msg)
		return m, cmd
	}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	header := m.headerView()
	help := m.helpView()

	headerHeight := lipgloss.Height(header)
	helpHeight := lipgloss.Height(help)

	paneHeight := m.height - headerHeight - helpHeight
	if paneHeight < 1 {
		paneHeight = 1
	}

	var content string
	if m.loading && len(m.runs) == 0 {
		content = lipgloss.PlaceHorizontal(m.width, lipgloss.Center,
			lipgloss.NewStyle().Foreground(Dim).Render("Loading runs..."))
	} else if m.err != nil && len(m.runs) == 0 {
		errStr := fmt.Sprintf("Error: %v", m.err)
		content = lipgloss.PlaceHorizontal(m.width, lipgloss.Center,
			lipgloss.NewStyle().Foreground(Red).Render(errStr))
	} else {
		content = m.paneView(paneHeight)
	}

	return lipgloss.JoinVertical(lipgloss.Top, header, content, help)
}

// headerView renders the top bar with title and current run ID.
func (m *Model) headerView() string {
	dot := lipgloss.NewStyle().Foreground(Accent).Render("●")
	title := lipgloss.NewStyle().Bold(true).Foreground(Text).Render("Baton")
	left := dot + " " + title

	if m.runID != "" {
		runStr := fmt.Sprintf("  run: %s  %s", shortID(m.runID),
			lipgloss.NewStyle().Foreground(Dim).Render(m.runStatus()))

		// Count active agents.
		activeCount := 0
		for _, a := range m.agentList.agents {
			if a.Status == "running" {
				activeCount++
			}
		}
		if activeCount > 0 {
			runStr += lipgloss.NewStyle().Foreground(Green).Render(fmt.Sprintf("  %d active", activeCount))
		}
		left += runStr
	}

	return HeaderStyle.Width(m.width).Render(left)
}

// runStatus returns a human-readable run status.
func (m *Model) runStatus() string {
	for _, r := range m.runs {
		if r.ID == m.runID {
			return r.Status
		}
	}
	return ""
}

// paneView renders the two-pane layout (side-by-side or stacked).
func (m *Model) paneView(paneHeight int) string {
	if m.width < 100 {
		// Stack vertically on narrow terminals.
		leftHeight := paneHeight / 2
		rightHeight := paneHeight - leftHeight

		leftView := m.agentList.View(m.width, leftHeight)
		rightView := m.chatView.View(m.width, rightHeight)

		return lipgloss.JoinVertical(lipgloss.Top, leftView, rightView)
	}

	// Side by side with a vertical border.
	borderWidth := 1
	leftWidth := paneWidth(m.width, true)
	rightWidth := m.width - leftWidth - borderWidth
	if leftWidth < 30 {
		leftWidth = 30
		rightWidth = m.width - leftWidth - borderWidth
	}

	leftView := m.agentList.View(leftWidth, paneHeight)
	rightView := m.chatView.View(rightWidth, paneHeight)

	// Build the vertical border to match the pane height.
	borderText := "\n"
	if paneHeight > 1 {
		borderText = strings.Repeat("│\n", paneHeight)
		borderText = strings.TrimRight(borderText, "\n")
	}
	border := PaneBorderStyle.Render(borderText)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftView, border, rightView)
}

// helpView renders the help bar at the bottom.
func (m *Model) helpView() string {
	if m.activePane == PaneLeft {
		return HelpBarStyle.Width(m.width).Render(
			"Tab:switch  ↑↓:nav  Enter:expand  r:refresh  Ctrl+C:quit")
	}
	return HelpBarStyle.Width(m.width).Render(
		"Tab:switch  Enter:send  Esc:clear  r:refresh  Ctrl+C:quit")
}

// paneWidth computes the left pane width.
func paneWidth(total int, isLeft bool) int {
	if isLeft {
		return total * 40 / 100
	}
	return total * 60 / 100
}

// ---------------------------------------------------------------------------
// HTTP commands
// ---------------------------------------------------------------------------

func (m *Model) fetchRuns() tea.Cmd {
	return func() tea.Msg {
		req, err := http.NewRequest(http.MethodGet, m.daemonURL+"/runs", nil)
		if err != nil {
			return errMsg{err}
		}
		m.setAuth(req)

		resp, err := m.client.Do(req)
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()

		if err := checkResponse(resp); err != nil {
			return errMsg{err}
		}

		var runs []RunSummary
		if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
			return errMsg{err}
		}
		return runsLoadedMsg(runs)
	}
}

func (m *Model) daemonGet(path string, result interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.daemonURL+path, nil)
	if err != nil {
		return fmt.Errorf("daemon get: %w", err)
	}
	m.setAuth(req)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("daemon get: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (m *Model) daemonPost(path string, body, result interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("daemon post: %w", err)
		}
		bodyReader = strings.NewReader(string(b))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.daemonURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("daemon post: %w", err)
	}
	m.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("daemon post: %w", err)
	}
	defer resp.Body.Close()

	if err := checkResponse(resp); err != nil {
		return err
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

func (m *Model) setAuth(req *http.Request) {
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon: %s (HTTP %d)", strings.TrimSpace(string(body)), resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

// Start starts the TUI program with the alternate screen buffer.
func Start(daemonURL, token string) error {
	p := tea.NewProgram(New(daemonURL, token), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

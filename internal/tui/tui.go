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

// View identifies the active view.
type View int

const (
	ViewSessions View = iota
	ViewRuns
)

// Message types for async operations.
type runsLoadedMsg []Run
type runCreatedMsg struct{}
type errMsg struct{ error }
type tickMsg struct{}

// Model is the main Bubbletea model for the Baton TUI.
type Model struct {
	client    *http.Client
	daemonURL string
	token     string

	width, height int

	activeView View
	activeTab  int

	sessions []Session
	runs     []Run
	groups   []SessionGroup
	cursor   int

	collapsed map[string]bool
	expanded  map[string]bool
	showMenu  bool
	menuTarget string
	loading   bool
	err       error

	inputMode bool
	inputText string
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
		collapsed: make(map[string]bool),
		expanded:  make(map[string]bool),
		loading:   true,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.fetchRuns(), m.pollTicker())
}

func (m *Model) pollTicker() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case runsLoadedMsg:
		m.runs = []Run(msg)
		m.sessions = extractSessions(m.runs)
		m.groups = groupSessions(m.sessions)
		m.loading = false
		m.err = nil
		m.clampCursor()
		return m, nil

	case runCreatedMsg:
		m.inputMode = false
		m.inputText = ""
		return m, m.fetchRuns()

	case tickMsg:
		return m, tea.Batch(m.fetchRuns(), m.pollTicker())

	case errMsg:
		m.loading = false
		m.err = msg.error
		return m, nil
	}
	return m, nil
}

// clampCursor ensures cursor is within valid range.
func (m *Model) clampCursor() {
	maxIdx := m.visibleSessionCount() - 1
	if m.activeView == ViewRuns {
		maxIdx = len(m.runs) - 1
	}
	if maxIdx < 0 {
		m.cursor = 0
		return
	}
	if m.cursor > maxIdx {
		m.cursor = maxIdx
	}
}

// visibleSessionCount returns the number of sessions in non-collapsed groups.
func (m *Model) visibleSessionCount() int {
	count := 0
	for _, g := range m.groups {
		if !m.collapsed["group:"+g.Status] {
			count += len(g.Sessions)
		}
	}
	return count
}

// sessionAtCursor returns the session at the current cursor position, accounting
// for collapsed groups that hide their sessions from the visible list.
func (m *Model) sessionAtCursor() (Session, bool) {
	visIdx := 0
	for _, g := range m.groups {
		if m.collapsed["group:"+g.Status] {
			continue
		}
		for _, s := range g.Sessions {
			if visIdx == m.cursor {
				return s, true
			}
			visIdx++
		}
	}
	return Session{}, false
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	if m.inputMode {
		return m.inputView()
	}

	header := m.headerView()
	tabs := m.tabsView()

	var content string
	if m.loading && m.visibleSessionCount() == 0 && len(m.runs) == 0 {
		content = lipgloss.PlaceHorizontal(m.width, lipgloss.Center,
			lipgloss.NewStyle().Foreground(Dim).Render("Loading runs..."),
		)
	} else if m.err != nil && m.visibleSessionCount() == 0 && len(m.runs) == 0 {
		errStr := fmt.Sprintf("Error: %v", m.err)
		content = lipgloss.PlaceHorizontal(m.width, lipgloss.Center,
			lipgloss.NewStyle().Foreground(Red).Render(errStr),
		)
	} else {
		switch m.activeView {
		case ViewSessions:
			content = m.sessionsView()
		case ViewRuns:
			content = m.runsView()
		}
	}

	help := m.helpView()

	// Pad to fill terminal height.
	headerH := lipgloss.Height(header)
	tabsH := lipgloss.Height(tabs)
	helpH := lipgloss.Height(help)
	contentH := lipgloss.Height(content)
	statusH := headerH + tabsH
	avail := m.height - statusH - 1
	if avail < 0 {
		avail = 0
	}
	pad := avail - contentH - helpH
	if pad < 0 {
		pad = 0
	}

	return lipgloss.JoinVertical(lipgloss.Top, header, tabs) + "\n" + content + strings.Repeat("\n", pad) + help
}

// inputView renders a simple text input for creating a new run.
func (m *Model) inputView() string {
	s := lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.NewStyle().Bold(true).Foreground(Text).Render("New Run"),
		"",
		lipgloss.NewStyle().Foreground(Dim).Render("Run name:"),
		m.inputText+"▌",
		"",
		lipgloss.NewStyle().Foreground(Dim).Render("Enter to create, Esc to cancel"),
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Top, lipgloss.Top, s)
}

// headerView renders the top bar with title and active count.
func (m *Model) headerView() string {
	activeCount := 0
	for _, s := range m.sessions {
		if s.Status == "working" {
			activeCount++
		}
	}
	countStr := fmt.Sprintf("%d active", activeCount)

	dot := lipgloss.NewStyle().Foreground(Accent).Render("●")
	title := lipgloss.NewStyle().Bold(true).Foreground(Text).Render("Baton")
	countStyle := lipgloss.NewStyle().Foreground(Dim).Render(countStr)

	left := dot + " " + title + "  " + countStyle
	right := lipgloss.NewStyle().Foreground(Dim).Render("[+]")

	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}

	return HeaderStyle.Width(m.width).Render(left + strings.Repeat(" ", pad) + right)
}

// tabsView renders the tab bar.
func (m *Model) tabsView() string {
	names := []string{"Sessions", "Runs"}
	var tabs []string
	for i, name := range names {
		if i == m.activeTab {
			tabs = append(tabs, TabActiveStyle.Render(name))
		} else {
			tabs = append(tabs, TabStyle.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// sessionsView renders the session list grouped by status.
func (m *Model) sessionsView() string {
	var b strings.Builder
	visIdx := 0

	for _, g := range m.groups {
		groupKey := "group:" + g.Status
		collapsed := m.collapsed[groupKey]
		icon := "▼"
		if collapsed {
			icon = "▶"
		}

		// Group header.
		headerText := fmt.Sprintf("  %s  %s (%d)", icon, g.Title, len(g.Sessions))
		b.WriteString(GroupHeaderStyle.Render(headerText))
		b.WriteString("\n")

		if collapsed {
			continue
		}

		for _, s := range g.Sessions {
			isSel := visIdx == m.cursor && m.activeView == ViewSessions
			b.WriteString(m.renderSessionCard(s, isSel))
			b.WriteString("\n")
			visIdx++
		}
	}

	if b.Len() == 0 && !m.loading {
		b.WriteString(lipgloss.NewStyle().Foreground(Dim).Padding(0, 2).Render("No sessions found."))
		b.WriteString("\n")
	}

	return b.String()
}

// renderSessionCard renders a single session card.
func (m *Model) renderSessionCard(s Session, selected bool) string {
	var dotColor lipgloss.Color
	switch s.Status {
	case "working":
		dotColor = Green
	case "waiting":
		dotColor = Yellow
	case "idle":
		dotColor = Dim
	case "stopped":
		dotColor = Red
	}

	dot := lipgloss.NewStyle().Foreground(dotColor).Render("●")
	name := lipgloss.NewStyle().Bold(true).Foreground(Text).Render(s.Name)
	dir := lipgloss.NewStyle().Foreground(Dim).Render(shortPath(s.Directory))
	ts := lipgloss.NewStyle().Foreground(Dim).Render(timeAgo(s.UpdatedAt))

	sel := " "
	if selected {
		sel = "▎"
	}
	selStyle := lipgloss.NewStyle().Foreground(Accent)

	// Row 1: selection indicator, dot, status badge, name, provider, model.
	row1Items := []string{
		selStyle.Render(sel),
		" ",
		dot,
		" ",
		StatusBadge(s.Status),
		" ",
		name,
	}
	if s.Provider != "" {
		row1Items = append(row1Items, "  ", RoleBadge(s.Provider))
	}
	if s.Model != "" {
		modelStyle := lipgloss.NewStyle().Foreground(Cyan).Padding(0, 1)
		row1Items = append(row1Items, modelStyle.Render(s.Model))
	}
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, row1Items...)

	// Row 2: directory and time.
	row2 := "    " + dir + "  " + ts

	cardContent := lipgloss.JoinVertical(lipgloss.Top, row1, row2)

	// Expanded detail.
	expandedKey := "session:" + s.ID
	if m.expanded[expandedKey] {
		detailStyle := lipgloss.NewStyle().Foreground(Dim).Padding(0, 1)
		details := "\n" + detailStyle.Render("Run: "+shortID(s.RunID))
		cardContent += details
	}

	cardWidth := m.width - 4
	if cardWidth < 40 {
		cardWidth = 40
	}

	if selected {
		return CardSelectedStyle.Width(cardWidth).Render(cardContent)
	}
	return CardStyle.Width(cardWidth).Render(cardContent)
}

// runsView renders the runs list.
func (m *Model) runsView() string {
	var b strings.Builder
	for i, r := range m.runs {
		selected := i == m.cursor && m.activeView == ViewRuns
		sel := "  "
		if selected {
			sel = "▎ "
		}
		idStr := shortID(r.ID)
		name := r.InputSource
		if name == "" {
			name = idStr
		}
		status := mapStatus(r.Status)

		line := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Foreground(Accent).Render(sel),
			StatusBadge(status),
			" ",
			lipgloss.NewStyle().Bold(true).Foreground(Text).Render(name),
			"  ",
			lipgloss.NewStyle().Foreground(Dim).Render(fmt.Sprintf("%d sessions", len(r.Sessions))),
			"  ",
			lipgloss.NewStyle().Foreground(Dim).Render(shortPath(r.ProjectPath)),
		)

		cardContent := line
		runKey := "run:" + r.ID
		if m.expanded[runKey] {
			var sub strings.Builder
			sub.WriteString("\n")
			for _, s := range r.Sessions {
				sub.WriteString("    ")
				sub.WriteString(lipgloss.NewStyle().Foreground(Cyan).Render(s.AgentID))
				sub.WriteString(" ")
				sub.WriteString(StatusBadge(mapStatus(s.Status)))
				sub.WriteString(" ")
				sub.WriteString(lipgloss.NewStyle().Foreground(Dim).Render(s.Model))
				sub.WriteString("\n")
			}
			cardContent += sub.String()
		}

		cardWidth := m.width - 4
		if cardWidth < 40 {
			cardWidth = 40
		}

		if selected {
			b.WriteString(CardSelectedStyle.Width(cardWidth).Render(cardContent))
		} else {
			b.WriteString(CardStyle.Width(cardWidth).Render(cardContent))
		}
		b.WriteString("\n")
	}

	if len(m.runs) == 0 && !m.loading {
		b.WriteString(lipgloss.NewStyle().Foreground(Dim).Padding(0, 2).Render("No runs found."))
		b.WriteString("\n")
	}

	return b.String()
}

// helpView renders the help bar.
func (m *Model) helpView() string {
	return HelpBarStyle.Width(m.width).Render(
		"↑↓ nav  Enter expand  Tab switch  g toggle group  r refresh  n new run  q quit",
	)
}

// shortID shortens a hex ID for display.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ---------------------------------------------------------------------------
// Key handling
// ---------------------------------------------------------------------------

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inputMode {
		return m.handleInputKey(msg)
	}
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		return m.switchTab()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		maxIdx := m.visibleSessionCount() - 1
		if m.activeView == ViewRuns {
			maxIdx = len(m.runs) - 1
		}
		if m.cursor < maxIdx {
			m.cursor++
		}
		return m, nil
	case "enter":
		return m.handleEnter()
	case "r":
		m.loading = true
		return m, m.fetchRuns()
	case "n":
		m.inputMode = true
		m.inputText = ""
		return m, nil
	case "g":
		return m.toggleGroup()
	case "m":
		if m.activeView == ViewSessions {
			if _, ok := m.sessionAtCursor(); ok {
				m.showMenu = !m.showMenu
				if m.showMenu {
					if s, ok := m.sessionAtCursor(); ok {
						m.menuTarget = s.ID
					}
				}
			}
		}
		return m, nil
	case "esc":
		m.showMenu = false
		return m, nil
	}
	return m, nil
}

func (m *Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.inputText != "" {
			m.loading = true
			return m, m.createRun(m.inputText)
		}
		m.inputMode = false
		return m, nil
	case "esc":
		m.inputMode = false
		m.inputText = ""
		return m, nil
	case "backspace":
		if len(m.inputText) > 0 {
			m.inputText = m.inputText[:len(m.inputText)-1]
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.inputText += msg.String()
		}
		return m, nil
	}
}

func (m *Model) switchTab() (tea.Model, tea.Cmd) {
	m.activeTab = (m.activeTab + 1) % 2
	if m.activeTab == 0 {
		m.activeView = ViewSessions
	} else {
		m.activeView = ViewRuns
	}
	m.cursor = 0
	return m, nil
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.activeView {
	case ViewSessions:
		if s, ok := m.sessionAtCursor(); ok {
			key := "session:" + s.ID
			if m.expanded[key] {
				delete(m.expanded, key)
			} else {
				m.expanded[key] = true
			}
		}
	case ViewRuns:
		if m.cursor >= 0 && m.cursor < len(m.runs) {
			key := "run:" + m.runs[m.cursor].ID
			if m.expanded[key] {
				delete(m.expanded, key)
			} else {
				m.expanded[key] = true
			}
		}
	}
	return m, nil
}

// toggleGroup toggles collapse of the group containing the current session.
func (m *Model) toggleGroup() (tea.Model, tea.Cmd) {
	if m.activeView != ViewSessions {
		return m, nil
	}
	s, ok := m.sessionAtCursor()
	if !ok {
		return m, nil
	}
	for _, g := range m.groups {
		for _, gs := range g.Sessions {
			if gs.ID == s.ID {
				key := "group:" + g.Status
				if m.collapsed[key] {
					delete(m.collapsed, key)
				} else {
					m.collapsed[key] = true
				}
				m.clampCursor()
				return m, nil
			}
		}
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// HTTP commands
// ---------------------------------------------------------------------------

func (m *Model) fetchRuns() tea.Cmd {
	return func() tea.Msg {
		var runs []Run
		if err := m.daemonGet("/runs", &runs); err != nil {
			return errMsg{err}
		}
		return runsLoadedMsg(runs)
	}
}

func (m *Model) createRun(name string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]string{
			"project_path": name,
			"input_type":   "manual",
			"input_source": name,
		}
		var r Run
		if err := m.daemonPost("/runs", body, &r); err != nil {
			return errMsg{err}
		}
		return runCreatedMsg{}
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

package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ducstii/Baton/internal/opencode"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Screen identifies which view is currently active.
type Screen int

const (
	ScreenProjectNav Screen = iota
	ScreenRunSwitcher
	ScreenSessionList
)

// ---------------------------------------------------------------------------
// Message types for async daemon operations
// ---------------------------------------------------------------------------

type projectsLoadedMsg []Project
type projectOpenedMsg Project
type runsLoadedMsg []Run
type runCreatedMsg Run
type sessionsLoadedMsg []Session

type errMsg struct{ error }

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Model is the top-level Bubbletea model.
type Model struct {
	daemonClient *opencode.Client
	daemonURL    string
	token        string
	httpClient   *http.Client

	screen      Screen
	projectNav  ProjectNavigator
	runSwitcher RunSwitcher
	sessionList SessionList

	width, height int
	ready         bool
}

// New creates a Model. If daemonURL is empty it defaults to
// http://127.0.0.1:8080.
func New(daemonURL, token string) *Model {
	url := daemonURL
	if url == "" {
		url = "http://127.0.0.1:8080"
	}
	url = strings.TrimRight(url, "/")

	pn := NewProjectNavigator()
	pn.loading = true

	return &Model{
		daemonClient: opencode.NewClient(daemonURL),
		daemonURL:    url,
		token:        token,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		screen:       ScreenProjectNav,
		projectNav:   pn,
	}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.fetchProjects()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case projectsLoadedMsg:
		m.projectNav.projects = []Project(msg)
		m.projectNav.loading = false
		return m, nil

	case projectOpenedMsg:
		p := Project(msg)
		rs := NewRunSwitcher(p)
		rs.loading = true
		m.runSwitcher = rs
		m.screen = ScreenRunSwitcher
		return m, m.fetchRuns(p.Name)

	case runsLoadedMsg:
		m.runSwitcher.runs = []Run(msg)
		m.runSwitcher.loading = false
		return m, nil

	case runCreatedMsg:
		m.runSwitcher.newRunMode = false
		m.runSwitcher.newRunInput = ""
		return m, m.fetchRuns(m.runSwitcher.project.Name)

	case sessionsLoadedMsg:
		m.sessionList.sessions = []Session(msg)
		m.sessionList.loading = false
		return m, nil

	case errMsg:
		switch m.screen {
		case ScreenProjectNav:
			m.projectNav.loading = false
			m.projectNav.err = msg.error
		case ScreenRunSwitcher:
			m.runSwitcher.loading = false
			m.runSwitcher.err = msg.error
		case ScreenSessionList:
			m.sessionList.loading = false
			m.sessionList.err = msg.error
		}
		return m, nil
	}
	return m, nil
}

// View implements tea.Model.
func (m *Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var screenName, content, help string

	switch m.screen {
	case ScreenProjectNav:
		screenName = "Project Navigator"
		content = m.projectNavView()
		help = "Tab:Switch  ↑↓:Navigate  Enter:Select/Open  Ctrl+C:Quit"
	case ScreenRunSwitcher:
		screenName = fmt.Sprintf("Runs — %s", m.runSwitcher.project.Name)
		content = m.runSwitcherView()
		help = "Tab:Switch  ↑↓:Navigate  Enter:Select  n:New Run  Esc:Back  Ctrl+C:Quit"
	case ScreenSessionList:
		id := m.sessionList.run.ID
		if len(id) > 8 {
			id = id[:8]
		}
		screenName = fmt.Sprintf("Sessions — %s", id)
		content = m.sessionListView()
		help = "Tab:Switch  ↑↓:Navigate  Esc:Back  Ctrl+C:Quit"
	}

	title := StyleTitle.Width(m.width).Render(screenName)
	helpBar := StyleHelp.Width(m.width).Render(help)

	return lipgloss.JoinVertical(lipgloss.Top, title, "", content, "", helpBar)
}

// ---------------------------------------------------------------------------
// Key dispatch
// ---------------------------------------------------------------------------

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		return m.cycleScreen()
	}

	switch m.screen {
	case ScreenProjectNav:
		return m.handleProjectNavKey(msg)
	case ScreenRunSwitcher:
		return m.handleRunSwitcherKey(msg)
	case ScreenSessionList:
		return m.handleSessionListKey(msg)
	}
	return m, nil
}

// cycleScreen advances through the three screens in order.
func (m *Model) cycleScreen() (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenProjectNav:
		m.screen = ScreenRunSwitcher
	case ScreenRunSwitcher:
		m.screen = ScreenSessionList
	case ScreenSessionList:
		m.screen = ScreenProjectNav
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Project Navigator key handling
// ---------------------------------------------------------------------------

func (m *Model) handleProjectNavKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "up":
		if m.projectNav.cursor > 0 {
			m.projectNav.cursor--
		}
	case "down":
		if m.projectNav.cursor < len(m.projectNav.projects)-1 {
			m.projectNav.cursor++
		}
	case "enter":
		if text := m.projectNav.textInput.Value(); text != "" {
			m.projectNav.loading = true
			return m, m.openProject(text)
		}
		if len(m.projectNav.projects) > 0 {
			p := m.projectNav.projects[m.projectNav.cursor]
			rs := NewRunSwitcher(p)
			rs.loading = true
			m.runSwitcher = rs
			m.screen = ScreenRunSwitcher
			return m, m.fetchRuns(p.Name)
		}
	default:
		m.projectNav.textInput, cmd = m.projectNav.textInput.Update(msg)
	}

	return m, cmd
}

// ---------------------------------------------------------------------------
// Run Switcher key handling
// ---------------------------------------------------------------------------

func (m *Model) handleRunSwitcherKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.runSwitcher.newRunMode {
		switch msg.String() {
		case "enter":
			if m.runSwitcher.newRunInput != "" {
				m.runSwitcher.loading = true
				return m, m.createRun(m.runSwitcher.project.Name, m.runSwitcher.newRunInput)
			}
			m.runSwitcher.newRunMode = false
			return m, nil
		case "esc":
			m.runSwitcher.newRunMode = false
			m.runSwitcher.newRunInput = ""
			return m, nil
		case "backspace":
			if len(m.runSwitcher.newRunInput) > 0 {
				m.runSwitcher.newRunInput = m.runSwitcher.newRunInput[:len(m.runSwitcher.newRunInput)-1]
			}
		default:
			if len(msg.String()) == 1 && msg.String()[0] >= 32 {
				m.runSwitcher.newRunInput += msg.String()
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "up":
		if m.runSwitcher.cursor > 0 {
			m.runSwitcher.cursor--
		}
	case "down":
		if m.runSwitcher.cursor < len(m.runSwitcher.runs)-1 {
			m.runSwitcher.cursor++
		}
	case "enter":
		if len(m.runSwitcher.runs) > 0 {
			r := m.runSwitcher.runs[m.runSwitcher.cursor]
			sl := NewSessionList(r)
			sl.loading = true
			m.sessionList = sl
			m.screen = ScreenSessionList
			return m, m.fetchSessions(r.ID)
		}
	case "n":
		m.runSwitcher.newRunMode = true
		m.runSwitcher.newRunInput = ""
	case "esc":
		m.screen = ScreenProjectNav
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Session List key handling
// ---------------------------------------------------------------------------

func (m *Model) handleSessionListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		if m.sessionList.cursor > 0 {
			m.sessionList.cursor--
		}
	case "down":
		if m.sessionList.cursor < len(m.sessionList.sessions)-1 {
			m.sessionList.cursor++
		}
	case "esc":
		m.screen = ScreenRunSwitcher
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Screen views
// ---------------------------------------------------------------------------

func (m *Model) projectNavView() string {
	var b strings.Builder

	b.WriteString("Path: ")
	b.WriteString(m.projectNav.textInput.View())
	b.WriteString("\n\n")

	if m.projectNav.loading {
		b.WriteString("Loading projects...\n")
	} else if m.projectNav.err != nil {
		b.WriteString(StyleError.Render(fmt.Sprintf("Error: %v\n", m.projectNav.err)))
	}

	if len(m.projectNav.projects) > 0 {
		b.WriteString("Projects:\n")
		for i, p := range m.projectNav.projects {
			line := fmt.Sprintf("%s (%s) %s", p.Name, p.BuildSystem, p.Path)
			if i == m.projectNav.cursor {
				b.WriteString(StyleSelected.Render("> " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteByte('\n')
		}
	} else if !m.projectNav.loading && m.projectNav.err == nil {
		b.WriteString(StyleDimmed.Render("No projects open. Type a path and press Enter.\n"))
	}

	return b.String()
}

func (m *Model) runSwitcherView() string {
	var b strings.Builder

	if m.runSwitcher.newRunMode {
		b.WriteString("New Run Name: ")
		b.WriteString(m.runSwitcher.newRunInput)
		b.WriteString("\n")
		b.WriteString(StyleDimmed.Render("Press Enter to create, Esc to cancel.\n"))
		return b.String()
	}

	if m.runSwitcher.loading {
		b.WriteString("Loading runs...\n")
	} else if m.runSwitcher.err != nil {
		b.WriteString(StyleError.Render(fmt.Sprintf("Error: %v\n", m.runSwitcher.err)))
	}

	if len(m.runSwitcher.runs) > 0 {
		for i, r := range m.runSwitcher.runs {
			idPrefix := r.ID
			if len(idPrefix) > 8 {
				idPrefix = idPrefix[:8]
			}
			badge := StatusBadge(r.Status)
			line := fmt.Sprintf("%s %s  %d work units", idPrefix, badge, r.WorkUnits)
			if i == m.runSwitcher.cursor {
				b.WriteString(StyleSelected.Render("> " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteByte('\n')
		}
	} else if !m.runSwitcher.loading && m.runSwitcher.err == nil {
		b.WriteString(StyleDimmed.Render("No runs yet. Press 'n' to create one.\n"))
	}

	return b.String()
}

func (m *Model) sessionListView() string {
	var b strings.Builder

	if m.sessionList.loading {
		b.WriteString("Loading sessions...\n")
	} else if m.sessionList.err != nil {
		b.WriteString(StyleError.Render(fmt.Sprintf("Error: %v\n", m.sessionList.err)))
	}

	if len(m.sessionList.sessions) > 0 {
		for i, sess := range m.sessionList.sessions {
			roleB := RoleBadge(sess.Role)
			statusB := StatusBadge(sess.Status)
			line := fmt.Sprintf("%s %s  %s", roleB, statusB, sess.Duration)
			if i == m.sessionList.cursor {
				b.WriteString(StyleSelected.Render("> " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteByte('\n')
		}
	} else if !m.sessionList.loading && m.sessionList.err == nil {
		b.WriteString(StyleDimmed.Render("No sessions yet.\n"))
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Commands (async HTTP calls)
// ---------------------------------------------------------------------------

func (m *Model) fetchProjects() tea.Cmd {
	return func() tea.Msg {
		var projects []Project
		if err := m.daemonGet("/projects", &projects); err != nil {
			return errMsg{err}
		}
		return projectsLoadedMsg(projects)
	}
}

func (m *Model) openProject(path string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]string{"path": path}
		var p Project
		if err := m.daemonPost("/projects", body, &p); err != nil {
			return errMsg{err}
		}
		return projectOpenedMsg(p)
	}
}

func (m *Model) fetchRuns(projectName string) tea.Cmd {
	return func() tea.Msg {
		var runs []Run
		if err := m.daemonGet("/projects/"+projectName+"/runs", &runs); err != nil {
			return errMsg{err}
		}
		return runsLoadedMsg(runs)
	}
}

func (m *Model) createRun(projectName, name string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]string{"project": projectName, "name": name}
		var r Run
		if err := m.daemonPost("/runs", body, &r); err != nil {
			return errMsg{err}
		}
		return runCreatedMsg(r)
	}
}

func (m *Model) fetchSessions(runID string) tea.Cmd {
	return func() tea.Msg {
		var sessions []Session
		if err := m.daemonGet("/runs/"+runID+"/sessions", &sessions); err != nil {
			return errMsg{err}
		}
		return sessionsLoadedMsg(sessions)
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (m *Model) daemonGet(path string, result interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.daemonURL+path, nil)
	if err != nil {
		return fmt.Errorf("daemon get: %w", err)
	}
	m.setAuth(req)

	resp, err := m.httpClient.Do(req)
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

	resp, err := m.httpClient.Do(req)
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

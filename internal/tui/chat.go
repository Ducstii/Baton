package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ChatMessage represents a single message in the brain chat.
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// historyLoadedMsg is delivered when chat history is fetched.
type historyLoadedMsg []ChatMessage

// chatSentMsg is delivered when a chat message has been sent.
type chatSentMsg struct{}

// ChatView is the right-pane model for brain chat.
type ChatView struct {
	messages  []ChatMessage
	input     textinput.Model
	scrollPos int
	loading   bool
}

// NewChatView creates a ChatView with a text input.
func NewChatView() ChatView {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Prompt = "> "
	ti.CharLimit = 1000
	ti.Focus()

	return ChatView{
		input: ti,
	}
}

// FocusInput focuses the text input.
func (c *ChatView) FocusInput() tea.Cmd {
	return c.input.Focus()
}

// BlurInput blurs the text input.
func (c *ChatView) BlurInput() {
	c.input.Blur()
}

// SendMessage returns a tea.Cmd that sends a message to the brain.
func (c *ChatView) SendMessage(daemonURL, token, runID, message string) tea.Cmd {
	return func() tea.Msg {
		body := map[string]string{"message": message}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return errMsg{err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			daemonURL+"/runs/"+runID+"/chat", &buf)
		if err != nil {
			return errMsg{err}
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return errMsg{fmt.Errorf("chat: %s (HTTP %d)", resp.Status, resp.StatusCode)}
		}

		return chatSentMsg{}
	}
}

// LoadHistory returns a tea.Cmd that fetches chat history from GET /runs/{id}/history.
func (c *ChatView) LoadHistory(daemonURL, token, runID string) tea.Cmd {
	return func() tea.Msg {
		req, err := http.NewRequest(http.MethodGet, daemonURL+"/runs/"+runID+"/history", nil)
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
			return errMsg{fmt.Errorf("history: %s (HTTP %d)", resp.Status, resp.StatusCode)}
		}

		var messages []ChatMessage
		if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
			return errMsg{err}
		}
		if messages == nil {
			messages = []ChatMessage{}
		}
		return historyLoadedMsg(messages)
	}
}

// View renders the chat view at the given width and height.
func (c *ChatView) View(width, height int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(Accent).Render("Brain Chat")

	inputHeight := 3
	titleHeight := 1
	msgAreaHeight := height - titleHeight - inputHeight - 1
	if msgAreaHeight < 0 {
		msgAreaHeight = 0
	}

	messages := c.renderMessages(width, msgAreaHeight)

	// Input area
	c.input.Width = width - 4
	inputView := ChatInputStyle.Width(width - 2).Render(c.input.View())

	return lipgloss.JoinVertical(lipgloss.Top, title, messages, "", inputView)
}

// renderMessages renders the message list, showing the most recent messages
// that fit in the available height.
func (c *ChatView) renderMessages(width, height int) string {
	if len(c.messages) == 0 {
		if c.loading {
			return lipgloss.NewStyle().Foreground(Dim).Padding(0, 1).
				Render("Loading...")
		}
		return lipgloss.NewStyle().Foreground(Dim).Padding(0, 1).
			Render("No messages yet. Type a message below to start chatting with the brain.")
	}

	// Render each message.
	rendered := make([]string, 0, len(c.messages))
	for _, msg := range c.messages {
		rendered = append(rendered, c.renderMessage(msg, width))
	}

	// Auto-scroll: show the last N messages that fit.
	if len(rendered) > height && height > 0 {
		rendered = rendered[len(rendered)-height:]
	}

	if len(rendered) == 0 {
		return ""
	}

	return lipgloss.JoinVertical(lipgloss.Top, rendered...)
}

// renderMessage renders a single chat message with appropriate styling.
func (c *ChatView) renderMessage(msg ChatMessage, width int) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ""
	}

	var prefix, style lipgloss.Style

	switch msg.Role {
	case "user":
		prefix = lipgloss.NewStyle().Foreground(Accent).Bold(true)
		style = lipgloss.NewStyle().Padding(0, 1)
		return prefix.Render("You:") + " " + style.Render(content)
	case "brain", "assistant":
		prefix = lipgloss.NewStyle().Foreground(Cyan).Bold(true)
		style = lipgloss.NewStyle().Padding(0, 1)
		return prefix.Render("Brain:") + " " + style.Render(content)
	default:
		// System / tool messages are dim.
		return lipgloss.NewStyle().Foreground(Dim).Padding(0, 1).Render(content)
	}
}

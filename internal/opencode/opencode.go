// Package opencode wraps the opencode serve REST API for programmatic session dispatch.
package opencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the default address for a local opencode serve instance.
const DefaultBaseURL = "http://127.0.0.1:4096"

// heartbeatInterval is the silence threshold before polling session status as a
// fallback heartbeat.
const heartbeatInterval = 15 * time.Second

// Model identifies a language model provider and model ID.
type Model struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
}

// TokenCounts tracks token usage for a session.
type TokenCounts struct {
	Input     int        `json:"input"`
	Output    int        `json:"output"`
	Reasoning int        `json:"reasoning"`
	Cache     CacheStats `json:"cache"`
}

// CacheStats tracks cache read/write counts.
type CacheStats struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

// Session represents an opencode conversation session.
type Session struct {
	ID     string      `json:"id"`
	Model  *Model      `json:"model,omitempty"`
	Dir    string      `json:"directory,omitempty"`
	Tokens TokenCounts `json:"tokens,omitempty"`
}

// MessageInfo contains metadata about a message.
type MessageInfo struct {
	Role    string `json:"role"`
	Created string `json:"created"`
	Finish  string `json:"finish,omitempty"`
}

// Part is a typed content fragment within a message.
type Part struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Message represents a single message in a session.
type Message struct {
	Info  MessageInfo `json:"info"`
	Parts []Part      `json:"parts"`
}

// HealthStatus represents the daemon health check response.
type HealthStatus struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// SessionState represents one session's status in the status map.
type SessionState struct {
	Type string `json:"type"` // "idle", "active", "completed", "error"
}

// Event is a server-sent event from the SSE stream.
type Event struct {
	Type string           `json:"type"`
	Data *json.RawMessage `json:"data,omitempty"`
}

// PermissionRequest represents a pending permission prompt.
type PermissionRequest struct {
	ID      string `json:"id,omitempty"`
	Session string `json:"session,omitempty"`
}

// HTTPError represents an unexpected HTTP response.
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("opencode: %s (HTTP %d)", e.Status, e.StatusCode)
}

// Client is an HTTP client for the opencode serve REST API.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient creates a new Client. If baseURL is empty, DefaultBaseURL is used.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// do sends an HTTP request, optionally marshaling body as JSON and decoding the response.
func (c *Client) do(ctx context.Context, method, path string, body, result interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("opencode: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("opencode: create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencode: %s %s: %w", method, path, err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(respBody)),
		}
	}

	if result != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return nil, fmt.Errorf("opencode: decode response: %w", err)
		}
	}

	return resp, nil
}

type createSessionRequest struct {
	Model    Model       `json:"model"`
	Location apiLocation `json:"location"`
}

type apiLocation struct {
	Directory string `json:"directory"`
}

// CreateSession creates a new session at the given directory.
func (c *Client) CreateSession(dir string, modelID, providerID string) (*Session, error) {
	body := createSessionRequest{
		Model: Model{
			ID:         modelID,
			ProviderID: providerID,
		},
		Location: apiLocation{Directory: dir},
	}
	// /api/session wraps responses in {data: ...}
	var wrapper struct {
		Data Session `json:"data"`
	}
	_, err := c.do(context.Background(), http.MethodPost, "/api/session", &body, &wrapper)
	if err != nil {
		return nil, err
	}
	return &wrapper.Data, nil
}

type promptAsyncRequest struct {
	Model promptModel `json:"model"`
	Parts []Part      `json:"parts"`
}

type promptModel struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// PromptAsync dispatches a prompt to the session and returns immediately.
func (c *Client) PromptAsync(sessionID string, providerID, modelID, text string) error {
	body := promptAsyncRequest{
		Model: promptModel{
			ProviderID: providerID,
			ModelID:    modelID,
		},
		Parts: []Part{{Type: "text", Text: text}},
	}
	resp, err := c.do(context.Background(), http.MethodPost, "/session/"+sessionID+"/prompt_async", &body, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// GetMessages retrieves all messages for a session.
func (c *Client) GetMessages(sessionID string) ([]Message, error) {
	var msgs []Message
	_, err := c.do(context.Background(), http.MethodGet, "/session/"+sessionID+"/message", nil, &msgs)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

type summarizeRequest struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

// Summarize compacts the session context using the given model.
func (c *Client) Summarize(sessionID string, providerID, modelID string) error {
	body := summarizeRequest{
		ProviderID: providerID,
		ModelID:    modelID,
	}
	resp, err := c.do(context.Background(), http.MethodPost, "/session/"+sessionID+"/summarize", &body, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Health returns the daemon health status.
func (c *Client) Health() (*HealthStatus, error) {
	var h HealthStatus
	_, err := c.do(context.Background(), http.MethodGet, "/api/health", nil, &h)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// SessionStatus returns a map of session IDs to their current state.
func (c *Client) SessionStatus() (map[string]SessionState, error) {
	var st map[string]SessionState
	_, err := c.do(context.Background(), http.MethodGet, "/session/status", nil, &st)
	if err != nil {
		return nil, err
	}
	return st, nil
}

// SubscribeEvents opens an SSE stream, returning event and error channels.
func (c *Client) SubscribeEvents(ctx context.Context, sessionID string) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errs := make(chan error, 1)

	go c.subscribeEvents(ctx, sessionID, events, errs)
	return events, errs
}

func (c *Client) subscribeEvents(ctx context.Context, sessionID string, events chan<- Event, errs chan<- error) {
	defer close(events)

	// SSE needs a long-lived connection, so use a client without a fixed timeout.
	sseClient := &http.Client{Transport: c.hc.Transport}

	url := c.baseURL + "/api/session/" + sessionID + "/event"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		errs <- fmt.Errorf("opencode: sse request: %w", err)
		return
	}

	resp, err := sseClient.Do(req)
	if err != nil {
		errs <- fmt.Errorf("opencode: sse: %w", err)
		return
	}

	// Close the response body when the context is cancelled so the scanner
	// goroutine unblocks quickly.
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

	lineCh := make(chan string, 16)
	go func() {
		defer close(lineCh)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	heartbeat := time.NewTimer(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lineCh:
			if !ok {
				return // scanner finished
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(line[5:])
			if payload == "" {
				continue
			}
			var evt Event
			if err := json.Unmarshal([]byte(payload), &evt); err != nil {
				continue // skip malformed events
			}
			heartbeat.Reset(heartbeatInterval)
			select {
			case events <- evt:
			case <-ctx.Done():
				return
			}
		case <-heartbeat.C:
			status, err := c.SessionStatus()
			if err != nil {
				heartbeat.Reset(heartbeatInterval)
				continue
			}
			raw, _ := json.Marshal(status)
			rm := json.RawMessage(raw)
			select {
			case events <- Event{Type: "heartbeat", Data: &rm}:
			case <-ctx.Done():
				return
			}
			heartbeat.Reset(heartbeatInterval)
		}
	}
}

// PendingPermissions lists all pending permission requests.
func (c *Client) PendingPermissions() ([]PermissionRequest, error) {
	var reqs []PermissionRequest
	_, err := c.do(context.Background(), http.MethodGet, "/api/permission/request", nil, &reqs)
	if err != nil {
		return nil, err
	}
	return reqs, nil
}

type permissionReply struct {
	Action string `json:"action"`
}

// ReplyToPermission responds to a permission request. Acceptable actions:
// "allow_once", "allow_always", "deny".
func (c *Client) ReplyToPermission(sessionID, requestID, action string) error {
	body := permissionReply{Action: action}
	path := fmt.Sprintf("/api/session/%s/permission/%s/reply", sessionID, requestID)
	resp, err := c.do(context.Background(), http.MethodPost, path, &body, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

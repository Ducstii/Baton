package daemon

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ConversationStore persists brain chat messages per run as JSONL files.
type ConversationStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewConversationStore creates a ConversationStore that stores files under baseDir.
func NewConversationStore(baseDir string) *ConversationStore {
	return &ConversationStore{baseDir: baseDir}
}

// AppendMessage adds a message to the run's conversation file.
// The file is created on the first append for a given run.
func (s *ConversationStore) AppendMessage(runID string, msg ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(s.baseDir, runID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(msg)
}

// LoadConversation returns all messages for a run, oldest first.
// Returns an empty slice if the run has no messages yet.
func (s *ConversationStore) LoadConversation(runID string) ([]ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	f, err := os.Open(filepath.Join(s.baseDir, runID+".jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return []ChatMessage{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var messages []ChatMessage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var msg ChatMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // skip malformed lines
		}
		messages = append(messages, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

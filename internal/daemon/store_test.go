package daemon

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConversationStore_AppendAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewConversationStore(dir)

	msg := ChatMessage{Role: "user", Content: "hello", Timestamp: time.Now()}
	if err := store.AppendMessage("run1", msg); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	msgs, err := store.LoadConversation("run1")
	if err != nil {
		t.Fatalf("LoadConversation failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("unexpected message: %+v", msgs[0])
	}
}

func TestConversationStore_EmptyConversation(t *testing.T) {
	dir := t.TempDir()
	store := NewConversationStore(dir)

	msgs, err := store.LoadConversation("nonexistent")
	if err != nil {
		t.Fatalf("LoadConversation for nonexistent run failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestConversationStore_OrderPreserved(t *testing.T) {
	dir := t.TempDir()
	store := NewConversationStore(dir)

	messages := []ChatMessage{
		{Role: "user", Content: "first", Timestamp: time.Now()},
		{Role: "brain", Content: "second", Timestamp: time.Now()},
		{Role: "user", Content: "third", Timestamp: time.Now()},
	}

	for _, msg := range messages {
		if err := store.AppendMessage("run_order", msg); err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	loaded, err := store.LoadConversation("run_order")
	if err != nil {
		t.Fatalf("LoadConversation failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}
	for i, msg := range loaded {
		if msg.Role != messages[i].Role || msg.Content != messages[i].Content {
			t.Errorf("message %d mismatch: got %+v, want %+v", i, msg, messages[i])
		}
	}
}

func TestConversationStore_ConcurrentAppends(t *testing.T) {
	dir := t.TempDir()
	store := NewConversationStore(dir)

	var wg sync.WaitGroup
	n := 10
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := ChatMessage{Role: "user", Content: fmt.Sprintf("msg %d", i), Timestamp: time.Now()}
			if err := store.AppendMessage("run_conc", msg); err != nil {
				t.Errorf("AppendMessage failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	loaded, err := store.LoadConversation("run_conc")
	if err != nil {
		t.Fatalf("LoadConversation failed: %v", err)
	}
	if len(loaded) != n {
		t.Errorf("expected %d messages, got %d", n, len(loaded))
	}
}

func TestConversationStore_MissingRunReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewConversationStore(dir)

	msgs, err := store.LoadConversation("run_does_not_exist")
	if err != nil {
		t.Fatalf("LoadConversation failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty slice, got %d messages", len(msgs))
	}
}

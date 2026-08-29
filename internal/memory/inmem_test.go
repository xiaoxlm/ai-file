package memory_test

import (
	"strings"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/llm"
	"github.com/xiaoxlm/ai-file/internal/memory"
)

func TestInMemory_AppendOrder(t *testing.T) {
	t.Parallel()

	m := memory.NewInMemory()
	if err := m.Append(llm.Message{Role: llm.RoleUser, Content: "a"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := m.Append(llm.Message{Role: llm.RoleAssistant, Content: "b"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	msgs := m.Messages()
	if len(msgs) != 2 {
		t.Fatalf("len(Messages()) = %d, want 2", len(msgs))
	}
	if msgs[0].Content != "a" || msgs[1].Content != "b" {
		t.Fatalf("Messages() = %+v, want ordered a then b", msgs)
	}
}

func TestInMemory_MessagesDeepCopy(t *testing.T) {
	t.Parallel()

	m := memory.NewInMemory()
	if err := m.Append(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "hello",
		ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "x", ArgumentsJSON: "{}"},
		},
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	msgs := m.Messages()
	msgs[0].Content = "mutated"
	msgs[0].ToolCalls[0].Name = "y"

	again := m.Messages()
	if again[0].Content != "hello" {
		t.Errorf("Content = %q, want hello after mutation", again[0].Content)
	}
	if again[0].ToolCalls[0].Name != "x" {
		t.Errorf("ToolCalls[0].Name = %q, want x after mutation", again[0].ToolCalls[0].Name)
	}
}

func TestInMemory_KVAndClear(t *testing.T) {
	t.Parallel()

	m := memory.NewInMemory()
	m.Set("goal_path", "/tmp/foo")

	v, ok := m.Get("goal_path")
	if !ok || v != "/tmp/foo" {
		t.Errorf("Get(goal_path) = (%q, %v), want (/tmp/foo, true)", v, ok)
	}
	if _, ok := m.Get("missing"); ok {
		t.Error("Get(missing) ok = true, want false")
	}

	m.Set("paragraph_count", "3")
	m.Clear()

	if len(m.Messages()) != 0 {
		t.Errorf("len(Messages()) after Clear = %d, want 0", len(m.Messages()))
	}
	if _, ok := m.Get("goal_path"); ok {
		t.Error("Get(goal_path) after Clear ok = true, want false")
	}
	if _, ok := m.Get("paragraph_count"); ok {
		t.Error("Get(paragraph_count) after Clear ok = true, want false")
	}
}

func TestInMemory_TruncatesToolObservation(t *testing.T) {
	t.Parallel()

	const maxObs = 32 * 1024
	big := strings.Repeat("x", maxObs+100)

	m := memory.NewInMemory()
	if err := m.Append(llm.Message{
		Role:       llm.RoleTool,
		Content:    big,
		ToolCallID: "call-123",
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	msgs := m.Messages()
	if len(msgs) != 1 {
		t.Fatalf("len(Messages()) = %d, want 1", len(msgs))
	}
	msg := msgs[0]
	if msg.ToolCallID != "call-123" {
		t.Errorf("ToolCallID = %q, want call-123", msg.ToolCallID)
	}

	const suffix = "\n[observation truncated]"
	if !strings.HasSuffix(msg.Content, suffix) {
		t.Fatalf("Content missing suffix %q", suffix)
	}
	prefix := strings.TrimSuffix(msg.Content, suffix)
	if len(prefix) != maxObs {
		t.Errorf("truncated prefix len = %d, want %d", len(prefix), maxObs)
	}
	if prefix != big[:maxObs] {
		t.Error("truncated prefix does not match original prefix")
	}
}

func TestInMemory_DoesNotTruncateNonTool(t *testing.T) {
	t.Parallel()

	const maxObs = 32 * 1024
	big := strings.Repeat("x", maxObs+100)

	m := memory.NewInMemory()
	if err := m.Append(llm.Message{Role: llm.RoleAssistant, Content: big}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	msgs := m.Messages()
	if len(msgs[0].Content) != maxObs+100 {
		t.Errorf("Content len = %d, want %d", len(msgs[0].Content), maxObs+100)
	}
}

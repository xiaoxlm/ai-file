package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/llm"
	"github.com/xiaoxlm/ai-file/internal/memory"
)

type scriptedClient struct {
	responses []llm.ChatResponse
	errs      []error
	requests  []llm.ChatRequest
}

func (c *scriptedClient) Chat(
	_ context.Context,
	request llm.ChatRequest,
) (llm.ChatResponse, error) {
	c.requests = append(c.requests, request)
	index := len(c.requests) - 1
	if index < len(c.errs) && c.errs[index] != nil {
		return llm.ChatResponse{}, c.errs[index]
	}
	if index >= len(c.responses) {
		return llm.ChatResponse{}, fmt.Errorf("unexpected chat call %d", index+1)
	}
	return c.responses[index], nil
}

type executeResult struct {
	observation string
	err         error
	items       []string
	complete    bool
}

type fakeExecutor struct {
	results []executeResult
	calls   []llm.ToolCall
	index   int
	items   []string
	done    bool
}

func (e *fakeExecutor) List() []llm.ToolSpec {
	return []llm.ToolSpec{{Name: "read_file"}, {Name: "finish"}}
}

func (e *fakeExecutor) Execute(
	_ context.Context,
	name string,
	argumentsJSON string,
) (string, error) {
	e.calls = append(e.calls, llm.ToolCall{
		Name:          name,
		ArgumentsJSON: argumentsJSON,
	})
	if e.index >= len(e.results) {
		return "", fmt.Errorf("unexpected tool call %d", e.index+1)
	}
	result := e.results[e.index]
	e.index++
	if result.complete {
		e.items = append([]string(nil), result.items...)
		e.done = true
	}
	return result.observation, result.err
}

func (e *fakeExecutor) Completion() ([]string, bool) {
	return append([]string(nil), e.items...), e.done
}

func toolCall(id, name, arguments string) llm.ToolCall {
	return llm.ToolCall{
		ID:            id,
		Name:          name,
		ArgumentsJSON: arguments,
	}
}

func newLoop(
	client llm.Client,
	executor *fakeExecutor,
	maxSteps int,
	verbose *bytes.Buffer,
) (*Loop, *memory.InMemory) {
	mem := memory.NewInMemory()
	_ = mem.Append(llm.Message{
		Role:    llm.RoleSystem,
		Content: NewGoal("/tmp/input.txt").Prompt(),
	})
	loop := &Loop{
		Client:   client,
		Tools:    executor,
		Memory:   mem,
		MaxSteps: maxSteps,
	}
	if verbose != nil {
		loop.Verbose = verbose
	}
	return loop, mem
}

func TestLoopRunReadThenFinish(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{
		{
			Content:   "先读取文件",
			ToolCalls: []llm.ToolCall{toolCall("read-1", "read_file", `{"path":"/tmp/input.txt"}`)},
		},
		{
			Content:   "提交摘要",
			ToolCalls: []llm.ToolCall{toolCall("finish-1", "finish", `{"items":["第一段"]}`)},
		},
	}}
	executor := &fakeExecutor{results: []executeResult{
		{observation: "paragraph_count: 1\n[1] 内容"},
		{observation: "finish accepted", items: []string{"第一段"}, complete: true},
	}}
	loop, mem := newLoop(client, executor, 3, nil)

	result, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0] != "第一段" {
		t.Fatalf("Run() result = %#v", result)
	}
	if got := len(client.requests); got != 2 {
		t.Fatalf("Chat calls = %d, want 2", got)
	}
	messages := mem.Messages()
	if got := messages[len(messages)-1]; got.Role != llm.RoleTool ||
		got.ToolCallID != "finish-1" ||
		got.Content != "finish accepted" {
		t.Fatalf("last message = %#v", got)
	}
}

func TestLoopRunAddsCorrectionWhenNoToolCall(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{
		{Content: "直接回答"},
		{
			ToolCalls: []llm.ToolCall{toolCall("finish-1", "finish", `{}`)},
		},
	}}
	executor := &fakeExecutor{results: []executeResult{
		{observation: "finish accepted", complete: true},
	}}
	loop, _ := newLoop(client, executor, 2, nil)

	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	secondRequest := client.requests[1]
	if len(secondRequest.Messages) < 2 {
		t.Fatalf("second request messages = %#v", secondRequest.Messages)
	}
	correction := secondRequest.Messages[len(secondRequest.Messages)-1]
	if correction.Role != llm.RoleUser ||
		!strings.Contains(correction.Content, "必须调用工具或 finish") {
		t.Fatalf("correction = %#v", correction)
	}
}

func TestLoopRunContinuesAfterRejectedFinish(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{toolCall("finish-1", "finish", `{"items":[]}`)},
		},
		{
			ToolCalls: []llm.ToolCall{toolCall("finish-2", "finish", `{"items":["摘要"]}`)},
		},
	}}
	executor := &fakeExecutor{results: []executeResult{
		{observation: "error: item count 0 does not match paragraph count 1"},
		{observation: "finish accepted", items: []string{"摘要"}, complete: true},
	}}
	loop, _ := newLoop(client, executor, 2, nil)

	result, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0] != "摘要" {
		t.Fatalf("Run() result = %#v", result)
	}
}

func TestLoopRunTurnsToolErrorIntoObservation(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{toolCall("bad-1", "missing", `{}`)},
		},
		{
			ToolCalls: []llm.ToolCall{toolCall("finish-1", "finish", `{}`)},
		},
	}}
	executor := &fakeExecutor{results: []executeResult{
		{err: errors.New("unknown tool")},
		{observation: "finish accepted", complete: true},
	}}
	loop, _ := newLoop(client, executor, 2, nil)

	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	messages := client.requests[1].Messages
	observation := messages[len(messages)-1]
	if observation.Role != llm.RoleTool ||
		observation.ToolCallID != "bad-1" ||
		observation.Content != "error: unknown tool" {
		t.Fatalf("tool observation = %#v", observation)
	}
}

func TestLoopRunReturnsLLMError(t *testing.T) {
	client := &scriptedClient{
		responses: []llm.ChatResponse{{}},
		errs:      []error{errors.New("service unavailable")},
	}
	loop, _ := newLoop(client, &fakeExecutor{}, 1, nil)

	_, err := loop.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "LLM chat: service unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestLoopRunReturnsErrMaxSteps(t *testing.T) {
	client := &scriptedClient{responses: []llm.ChatResponse{
		{Content: "没有调用"},
		{Content: "仍未调用"},
	}}
	loop, _ := newLoop(client, &fakeExecutor{}, 2, nil)

	_, err := loop.Run(context.Background())
	if !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("Run() error = %v, want ErrMaxSteps", err)
	}
	if got := len(client.requests); got != 2 {
		t.Fatalf("Chat calls = %d, want 2", got)
	}
}

func TestLoopRunWritesVerboseTrace(t *testing.T) {
	var verbose bytes.Buffer
	client := &scriptedClient{responses: []llm.ChatResponse{{
		ToolCalls: []llm.ToolCall{toolCall("finish-1", "finish", `{"items":[]}`)},
	}}}
	executor := &fakeExecutor{results: []executeResult{{
		observation: "finish accepted",
		complete:    true,
	}}}
	loop, _ := newLoop(client, executor, 1, &verbose)

	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := verbose.String()
	for _, want := range []string{
		`step=1 thought="(empty)"`,
		`step=1 action=finish arguments={"items":[]}`,
		`step=1 observation="finish accepted"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("verbose output %q does not contain %q", output, want)
		}
	}
}

func TestGoalPromptContainsPathAndRequiredInstructions(t *testing.T) {
	prompt := NewGoal("/tmp/input.txt").Prompt()
	for _, want := range []string{
		"/tmp/input.txt",
		"必须先调用 read_file",
		"每段用一句",
		"finish.items",
		"paragraph_count",
		"禁止编造",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Prompt() = %q, want substring %q", prompt, want)
		}
	}
}

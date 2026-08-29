package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/xiaoxlm/ai-file/internal/llm"
	"github.com/xiaoxlm/ai-file/internal/memory"
	"github.com/xiaoxlm/ai-file/internal/tools"
)

const (
	correctionMessage   = "必须调用工具或 finish，继续完成目标。"
	maxVerboseObsBytes  = 2 * 1024
	emptyThoughtDisplay = "(empty)"
)

// ErrMaxSteps indicates that the model did not complete within the step limit.
var ErrMaxSteps = errors.New("agent did not complete within max steps")

// Result is the complete, structured result accepted by the tool executor.
type Result struct {
	Items []string
}

// Loop drives vendor-neutral chat and tool calls until completion.
type Loop struct {
	Client   llm.Client
	Tools    tools.Executor
	Memory   memory.Memory
	MaxSteps int
	Verbose  io.Writer
}

// Run executes the ReAct loop.
func (l *Loop) Run(ctx context.Context) (Result, error) {
	for step := 1; step <= l.MaxSteps; step++ {
		response, err := l.Client.Chat(ctx, llm.ChatRequest{
			Messages: l.Memory.Messages(),
			Tools:    l.Tools.List(),
		})
		if err != nil {
			return Result{}, fmt.Errorf("LLM chat: %w", err)
		}

		if err := l.Memory.Append(llm.Message{
			Role:      llm.RoleAssistant,
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		}); err != nil {
			return Result{}, fmt.Errorf("append assistant message: %w", err)
		}
		l.writeThought(step, response.Content)

		if len(response.ToolCalls) == 0 {
			if err := l.Memory.Append(llm.Message{
				Role:    llm.RoleUser,
				Content: correctionMessage,
			}); err != nil {
				return Result{}, fmt.Errorf("append correction message: %w", err)
			}
			continue
		}

		for _, call := range response.ToolCalls {
			l.writeAction(step, call)
			observation, executeErr := l.Tools.Execute(
				ctx,
				call.Name,
				call.ArgumentsJSON,
			)
			if executeErr != nil {
				observation = "error: " + executeErr.Error()
			}
			if err := l.Memory.Append(llm.Message{
				Role:       llm.RoleTool,
				Content:    observation,
				ToolCallID: call.ID,
			}); err != nil {
				return Result{}, fmt.Errorf("append tool observation: %w", err)
			}
			l.writeObservation(step, observation)

			if items, ok := l.Tools.Completion(); ok {
				return Result{Items: items}, nil
			}
		}
	}

	return Result{}, ErrMaxSteps
}

func (l *Loop) writeThought(step int, thought string) {
	if l.Verbose == nil {
		return
	}
	if thought == "" {
		thought = emptyThoughtDisplay
	}
	_, _ = fmt.Fprintf(l.Verbose, "step=%d thought=%q\n", step, thought)
}

func (l *Loop) writeAction(step int, call llm.ToolCall) {
	if l.Verbose == nil {
		return
	}
	_, _ = fmt.Fprintf(
		l.Verbose,
		"step=%d action=%s arguments=%s\n",
		step,
		call.Name,
		call.ArgumentsJSON,
	)
}

func (l *Loop) writeObservation(step int, observation string) {
	if l.Verbose == nil {
		return
	}
	if len(observation) > maxVerboseObsBytes {
		observation = observation[:maxVerboseObsBytes]
	}
	_, _ = fmt.Fprintf(
		l.Verbose,
		"step=%d observation=%q\n",
		step,
		observation,
	)
}

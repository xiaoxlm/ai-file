package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/xiaoxlm/ai-file/internal/memory"
)

const finishInputSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["items"],
  "properties": {
    "items": {
      "type": "array",
      "items": { "type": "string", "minLength": 1 }
    }
  }
}`

type finishTool struct {
	registry *Registry
	mem      memory.Memory
}

// NewFinish returns a finish tool bound to the registry and session memory.
func NewFinish(registry *Registry, mem memory.Memory) Tool {
	return &finishTool{
		registry: registry,
		mem:      mem,
	}
}

func (t *finishTool) Name() string { return "finish" }

func (t *finishTool) Description() string {
	return "Submit one summary per paragraph in original order."
}

func (t *finishTool) InputSchema() json.RawMessage {
	return json.RawMessage(finishInputSchema)
}

func (t *finishTool) Execute(
	_ context.Context,
	argumentsJSON string,
) (string, error) {
	countStr, ok := t.mem.Get(keyParagraphCount)
	if !ok {
		return "error: read_file must succeed before finish", nil
	}

	paragraphCount, err := strconv.Atoi(countStr)
	if err != nil {
		return "", fmt.Errorf("parse paragraph_count %q: %w", countStr, err)
	}

	var args struct {
		Items []string `json:"items"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", fmt.Errorf("parse finish arguments: %w", err)
	}
	if args.Items == nil {
		return "error: items is required", nil
	}

	if len(args.Items) != paragraphCount {
		return fmt.Sprintf(
			"error: item count %d does not match paragraph count %d",
			len(args.Items),
			paragraphCount,
		), nil
	}

	for i, item := range args.Items {
		if strings.TrimSpace(item) == "" {
			return fmt.Sprintf("error: items[%d] must not be empty", i), nil
		}
	}

	items := append([]string(nil), args.Items...)
	t.registry.setCompletion(items)
	return "finish accepted", nil
}

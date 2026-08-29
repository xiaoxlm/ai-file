package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xiaoxlm/ai-file/internal/memory"
	"github.com/xiaoxlm/ai-file/internal/split"
)

const readFileInputSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path"],
  "properties": {
    "path": { "type": "string", "description": "待读取的目标文件路径" }
  }
}`

const (
	keyGoalPath              = "goal_path"
	keyParagraphCount        = "paragraph_count"
	keyReadFileObservation   = "read_file_observation"
	keyTruncatedParagraphs   = "truncated_paragraphs"
	truncatedParagraphSuffix = "\n[truncated]"
)

type readFileTool struct {
	mem          memory.Memory
	maxParaChars int
}

// NewReadFile returns a read_file tool bound to session memory.
func NewReadFile(mem memory.Memory, maxParaChars int) Tool {
	return &readFileTool{
		mem:          mem,
		maxParaChars: maxParaChars,
	}
}

func (t *readFileTool) Name() string { return "read_file" }

func (t *readFileTool) Description() string {
	return "Read the goal file and return paragraphs split on blank lines."
}

func (t *readFileTool) InputSchema() json.RawMessage {
	return json.RawMessage(readFileInputSchema)
}

func (t *readFileTool) Execute(
	_ context.Context,
	argumentsJSON string,
) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", fmt.Errorf("parse read_file arguments: %w", err)
	}
	if strings.TrimSpace(args.Path) == "" {
		return "error: path is required", nil
	}

	resolved, err := normalizePath(args.Path)
	if err != nil {
		return "", fmt.Errorf("resolve read_file path: %w", err)
	}

	goalPath, ok := t.mem.Get(keyGoalPath)
	if !ok || resolved != goalPath {
		return "error: path not allowed", nil
	}

	if cached, ok := t.mem.Get(keyReadFileObservation); ok {
		return cached, nil
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	paragraphs := split.Paragraphs(string(content))
	display, truncated := truncateParagraphs(paragraphs, t.maxParaChars)
	observation := formatReadFileObservation(resolved, display)

	t.mem.Set(keyParagraphCount, strconv.Itoa(len(paragraphs)))
	t.mem.Set(keyReadFileObservation, observation)
	if len(truncated) == 0 {
		t.mem.Set(keyTruncatedParagraphs, "")
	} else {
		parts := make([]string, len(truncated))
		for i, n := range truncated {
			parts[i] = strconv.Itoa(n)
		}
		t.mem.Set(keyTruncatedParagraphs, strings.Join(parts, ","))
	}

	return observation, nil
}

func normalizePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func truncateParagraphs(
	paragraphs []string,
	maxRunes int,
) (display []string, truncated []int) {
	display = make([]string, len(paragraphs))
	for i, paragraph := range paragraphs {
		text, wasTruncated := truncateRunes(paragraph, maxRunes)
		display[i] = text
		if wasTruncated {
			truncated = append(truncated, i+1)
		}
	}
	return display, truncated
}

func truncateRunes(text string, maxRunes int) (string, bool) {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text, false
	}
	return string(runes[:maxRunes]) + truncatedParagraphSuffix, true
}

func formatReadFileObservation(path string, paragraphs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "path: %s\nparagraph_count: %d\nparagraphs:\n", path, len(paragraphs))
	for i, paragraph := range paragraphs {
		fmt.Fprintf(&b, "[%d] %s\n", i+1, paragraph)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

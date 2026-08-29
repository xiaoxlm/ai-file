// Package app composes input validation, tools, memory, LLM, and the agent loop.
package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xiaoxlm/ai-file/internal/agent"
	"github.com/xiaoxlm/ai-file/internal/config"
	"github.com/xiaoxlm/ai-file/internal/llm"
	"github.com/xiaoxlm/ai-file/internal/memory"
	"github.com/xiaoxlm/ai-file/internal/split"
	"github.com/xiaoxlm/ai-file/internal/tools"
)

// ExitCode is the process status returned by Run.
type ExitCode int

const (
	// ExitSuccess indicates complete output was produced.
	ExitSuccess ExitCode = 0
	// ExitFailure indicates an LLM, output, or internal failure.
	ExitFailure ExitCode = 1
	// ExitUsage indicates invalid input or configuration.
	ExitUsage ExitCode = 2
)

const (
	keyGoalPath            = "goal_path"
	keyTruncatedParagraphs = "truncated_paragraphs"
)

// Options contains one application run's inputs and injectable dependencies.
type Options struct {
	Path    string
	OutPath string
	Config  config.Config
	Client  llm.Client
	Stdout  io.Writer
	Stderr  io.Writer
}

// Run validates one file and produces its complete summary.
func Run(ctx context.Context, opts Options) ExitCode {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	path, content, code, err := preflight(opts.Path, opts.Config.MaxBytes)
	if err != nil {
		writeDiagnostic(stderr, err)
		return code
	}
	if opts.OutPath != "" {
		same, err := outputTargetsSource(path, opts.OutPath)
		if err != nil {
			writeDiagnostic(stderr, fmt.Errorf("validate output path: %w", err))
			return ExitFailure
		}
		if same {
			writeDiagnostic(stderr, fmt.Errorf("output path must not overwrite source file"))
			return ExitFailure
		}
	}

	if len(split.Paragraphs(string(content))) == 0 {
		return publish(stdout, stderr, opts.OutPath, agent.Render(path, agent.Result{}, nil))
	}

	mem := memory.NewInMemory()
	mem.Set(keyGoalPath, path)
	goal := agent.NewGoal(path)
	if err := mem.Append(llm.Message{
		Role:    llm.RoleSystem,
		Content: goal.Prompt(),
	}); err != nil {
		writeDiagnostic(stderr, fmt.Errorf("initialize memory: %w", err))
		return ExitFailure
	}

	registry := tools.NewRegistry()
	if err := registry.RegisterCoreTools(mem, opts.Config.MaxParaChars); err != nil {
		writeDiagnostic(stderr, fmt.Errorf("register core tools: %w", err))
		return ExitFailure
	}

	client := opts.Client
	if client == nil {
		var err error
		client, err = llm.New(opts.Config)
		if err != nil {
			writeDiagnostic(stderr, fmt.Errorf("create LLM client: %w", err))
			return ExitUsage
		}
	}

	loop := agent.Loop{
		Client:   client,
		Tools:    registry,
		Memory:   mem,
		MaxSteps: opts.Config.MaxSteps,
	}
	if opts.Config.Verbose {
		loop.Verbose = stderr
	}

	result, err := loop.Run(ctx)
	if err != nil {
		writeDiagnostic(stderr, err)
		return ExitFailure
	}
	truncated, err := truncatedParagraphs(mem)
	if err != nil {
		writeDiagnostic(stderr, err)
		return ExitFailure
	}

	return publish(
		stdout,
		stderr,
		opts.OutPath,
		agent.Render(path, result, truncated),
	)
}

func preflight(path string, maxBytes int64) (string, []byte, ExitCode, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil, ExitUsage, fmt.Errorf("file path is required")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, ExitUsage, fmt.Errorf("resolve file path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, ExitUsage, fmt.Errorf("resolve file path: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, ExitUsage, fmt.Errorf("stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, ExitUsage, fmt.Errorf("path is not a regular file: %s", resolved)
	}
	if info.Size() > maxBytes {
		return "", nil, ExitUsage, fmt.Errorf(
			"file size %d exceeds limit %d",
			info.Size(),
			maxBytes,
		)
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", nil, ExitUsage, fmt.Errorf("read file: %w", err)
	}
	if int64(len(content)) > maxBytes {
		return "", nil, ExitUsage, fmt.Errorf(
			"file size %d exceeds limit %d",
			len(content),
			maxBytes,
		)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return "", nil, ExitUsage, fmt.Errorf("file contains NUL byte")
	}
	if !utf8.Valid(content) {
		return "", nil, ExitUsage, fmt.Errorf("file is not valid UTF-8")
	}

	return resolved, content, ExitSuccess, nil
}

func outputTargetsSource(sourcePath, outputPath string) (bool, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return false, err
	}
	outputInfo, err := os.Stat(outputPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(sourceInfo, outputInfo), nil
}

func truncatedParagraphs(mem memory.Memory) ([]int, error) {
	value, ok := mem.Get(keyTruncatedParagraphs)
	if !ok || value == "" {
		return []int{}, nil
	}

	parts := strings.Split(value, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		paragraph, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("parse truncated paragraph %q: %w", part, err)
		}
		result = append(result, paragraph)
	}
	return result, nil
}

func publish(
	stdout io.Writer,
	stderr io.Writer,
	outPath string,
	rendered string,
) ExitCode {
	if outPath != "" {
		if err := writeFileAtomic(outPath, []byte(rendered)); err != nil {
			writeDiagnostic(stderr, fmt.Errorf("write output: %w", err))
			return ExitFailure
		}
	}
	if _, err := io.WriteString(stdout, rendered); err != nil {
		if outPath != "" {
			_ = os.Remove(outPath)
		}
		writeDiagnostic(stderr, fmt.Errorf("write stdout: %w", err))
		return ExitFailure
	}
	return ExitSuccess
}

func writeFileAtomic(path string, content []byte) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(abs), ".ai-file-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, abs)
}

func writeDiagnostic(stderr io.Writer, err error) {
	_, _ = fmt.Fprintln(stderr, "错误:", err)
}

package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/memory"
)

func canonicalPath(t *testing.T, path string) string {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%q) error = %v", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", abs, err)
	}
	return resolved
}

func TestReadFile_MissingPath(t *testing.T) {
	t.Parallel()

	mem := memory.NewInMemory()
	tool := NewReadFile(mem, 1000)

	obs, err := tool.Execute(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if obs != "error: path is required" {
		t.Errorf("Execute() observation = %q, want error: path is required", obs)
	}
}

func TestReadFile_PathNotAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	goal := filepath.Join(dir, "goal.txt")
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(goal, []byte("goal"), 0o644); err != nil {
		t.Fatalf("WriteFile(goal) error = %v", err)
	}
	if err := os.WriteFile(other, []byte("other"), 0o644); err != nil {
		t.Fatalf("WriteFile(other) error = %v", err)
	}

	mem := memory.NewInMemory()
	mem.Set("goal_path", canonicalPath(t, goal))
	tool := NewReadFile(mem, 1000)

	obs, err := tool.Execute(
		context.Background(),
		`{"path":`+mustJSON(t, other)+`}`,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if obs != "error: path not allowed" {
		t.Errorf("Execute() observation = %q, want error: path not allowed", obs)
	}
}

func TestReadFile_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	content := "first paragraph\n\nsecond paragraph"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	goal := canonicalPath(t, path)
	mem := memory.NewInMemory()
	mem.Set("goal_path", goal)
	tool := NewReadFile(mem, 1000)

	obs, err := tool.Execute(
		context.Background(),
		`{"path":`+mustJSON(t, path)+`}`,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantPrefix := "path: " + goal + "\nparagraph_count: 2\nparagraphs:\n"
	if !strings.HasPrefix(obs, wantPrefix) {
		t.Fatalf("Execute() observation prefix = %q, want prefix %q", obs, wantPrefix)
	}
	if !strings.Contains(obs, "[1] first paragraph") {
		t.Errorf("Execute() observation missing first paragraph: %q", obs)
	}
	if !strings.Contains(obs, "[2] second paragraph") {
		t.Errorf("Execute() observation missing second paragraph: %q", obs)
	}

	count, ok := mem.Get("paragraph_count")
	if !ok || count != "2" {
		t.Errorf("paragraph_count = (%q, %v), want (2, true)", count, ok)
	}
	cached, ok := mem.Get("read_file_observation")
	if !ok || cached != obs {
		t.Errorf("read_file_observation = (%q, %v), want cached observation", cached, ok)
	}
	if truncated, ok := mem.Get("truncated_paragraphs"); ok && truncated != "" {
		t.Errorf("truncated_paragraphs = %q, want empty or absent", truncated)
	}
}

func TestReadFile_CachesObservation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("only"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	goal := canonicalPath(t, path)
	mem := memory.NewInMemory()
	mem.Set("goal_path", goal)
	tool := NewReadFile(mem, 1000)

	first, err := tool.Execute(
		context.Background(),
		`{"path":`+mustJSON(t, path)+`}`,
	)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}

	if err := os.WriteFile(path, []byte("changed"), 0o644); err != nil {
		t.Fatalf("WriteFile(changed) error = %v", err)
	}

	second, err := tool.Execute(
		context.Background(),
		`{"path":`+mustJSON(t, path)+`}`,
	)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second != first {
		t.Errorf("cached observation = %q, want %q", second, first)
	}
}

func TestReadFile_TruncatesByRune(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	content := "你好世界\n\na"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	goal := canonicalPath(t, path)
	mem := memory.NewInMemory()
	mem.Set("goal_path", goal)
	tool := NewReadFile(mem, 2)

	obs, err := tool.Execute(
		context.Background(),
		`{"path":`+mustJSON(t, path)+`}`,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(obs, "[1] 你好\n[truncated]") {
		t.Errorf("Execute() observation = %q, want rune-truncated first paragraph", obs)
	}
	if !strings.Contains(obs, "[2] a") {
		t.Errorf("Execute() observation = %q, want untruncated second paragraph", obs)
	}

	truncated, ok := mem.Get("truncated_paragraphs")
	if !ok || truncated != "1" {
		t.Errorf("truncated_paragraphs = (%q, %v), want (1, true)", truncated, ok)
	}
}

func TestReadFile_ResolvesRelativePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.txt")
	if err := os.WriteFile(path, []byte("rel"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	goal := canonicalPath(t, path)
	mem := memory.NewInMemory()
	mem.Set("goal_path", goal)
	tool := NewReadFile(mem, 1000)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	obs, err := tool.Execute(context.Background(), `{"path":"doc.txt"}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(obs, "path: "+goal) {
		t.Errorf("Execute() observation = %q, want path %q", obs, goal)
	}
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(%q) error = %v", s, err)
	}
	return string(b)
}

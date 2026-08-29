package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/config"
	"github.com/xiaoxlm/ai-file/internal/llm"
)

type fakeClient struct {
	path      string
	summaries []string
	fail      error
	requests  []llm.ChatRequest
}

func (c *fakeClient) Chat(
	_ context.Context,
	request llm.ChatRequest,
) (llm.ChatResponse, error) {
	c.requests = append(c.requests, request)
	if c.fail != nil {
		return llm.ChatResponse{}, c.fail
	}

	switch len(c.requests) {
	case 1:
		return llm.ChatResponse{
			ToolCalls: []llm.ToolCall{{
				ID:            "read-1",
				Name:          "read_file",
				ArgumentsJSON: fmt.Sprintf(`{"path":%q}`, c.path),
			}},
		}, nil
	case 2:
		items := make([]string, len(c.summaries))
		for i, summary := range c.summaries {
			items[i] = fmt.Sprintf("%q", summary)
		}
		return llm.ChatResponse{
			ToolCalls: []llm.ToolCall{{
				ID:            "finish-1",
				Name:          "finish",
				ArgumentsJSON: `{"items":[` + strings.Join(items, ",") + `]}`,
			}},
		}, nil
	default:
		return llm.ChatResponse{}, fmt.Errorf("unexpected Chat call %d", len(c.requests))
	}
}

func TestRun_EmptyFileDoesNotCallLLM(t *testing.T) {
	t.Parallel()

	path := writeInput(t, []byte(" \n\t\n"))
	client := &fakeClient{fail: errors.New("must not be called")}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), Options{
		Path:   path,
		Config: testConfig(),
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if code != ExitSuccess {
		t.Fatalf("Run() code = %d, want %d; stderr = %q", code, ExitSuccess, stderr.String())
	}
	if len(client.requests) != 0 {
		t.Fatalf("Chat calls = %d, want 0", len(client.requests))
	}
	resolved := canonicalPath(t, path)
	want := "文件: " + resolved + "\n段数: 0\n\n无有效段落\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_InvalidInputsReturnExitUsage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	oversize := filepath.Join(dir, "oversize.txt")
	if err := os.WriteFile(oversize, []byte("12345"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	nul := filepath.Join(dir, "nul.txt")
	if err := os.WriteFile(nul, []byte("a\x00b"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	nonUTF8 := filepath.Join(dir, "non-utf8.txt")
	if err := os.WriteFile(nonUTF8, []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name     string
		path     string
		maxBytes int64
	}{
		{name: "no path", path: "", maxBytes: 4},
		{name: "missing", path: filepath.Join(dir, "missing.txt"), maxBytes: 4},
		{name: "directory", path: dir, maxBytes: 4},
		{name: "oversize", path: oversize, maxBytes: 4},
		{name: "NUL", path: nul, maxBytes: 4},
		{name: "non UTF-8", path: nonUTF8, maxBytes: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig()
			cfg.MaxBytes = tt.maxBytes
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			client := &fakeClient{fail: errors.New("must not be called")}

			code := Run(context.Background(), Options{
				Path:   tt.path,
				Config: cfg,
				Client: client,
				Stdout: &stdout,
				Stderr: &stderr,
			})

			if code != ExitUsage {
				t.Errorf("Run() code = %d, want %d", code, ExitUsage)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if stderr.Len() == 0 {
				t.Error("stderr is empty, want diagnostic")
			}
			if len(client.requests) != 0 {
				t.Errorf("Chat calls = %d, want 0", len(client.requests))
			}
		})
	}
}

func TestRun_ReadsThenFinishesAndRenders(t *testing.T) {
	t.Parallel()

	path := writeInput(t, []byte("第一段\n\n第二段\n\n第三段"))
	resolved := canonicalPath(t, path)
	client := &fakeClient{
		path:      resolved,
		summaries: []string{"摘要一", "摘要二", "摘要三"},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), Options{
		Path:   path,
		Config: testConfig(),
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if code != ExitSuccess {
		t.Fatalf("Run() code = %d, want %d; stderr = %q", code, ExitSuccess, stderr.String())
	}
	if len(client.requests) != 2 {
		t.Fatalf("Chat calls = %d, want 2", len(client.requests))
	}
	if got := client.requests[0].Messages; len(got) != 1 ||
		got[0].Role != llm.RoleSystem ||
		!strings.Contains(got[0].Content, resolved) {
		t.Errorf("first request messages = %#v, want system Goal with canonical path", got)
	}
	if got := client.requests[0].Tools; len(got) != 2 ||
		got[0].Name != "read_file" ||
		got[1].Name != "finish" {
		t.Errorf("first request tools = %#v, want read_file then finish", got)
	}
	for _, want := range []string{
		"文件: " + resolved,
		"段数: 3",
		"1. 摘要一",
		"2. 摘要二",
		"3. 摘要三",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want to contain %q", stdout.String(), want)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestRun_WritesOutOnlyAfterCompleteSuccess(t *testing.T) {
	t.Parallel()

	path := writeInput(t, []byte("一段"))
	resolved := canonicalPath(t, path)
	outPath := filepath.Join(t.TempDir(), "summary.txt")
	client := &fakeClient{
		path:      resolved,
		summaries: []string{"摘要"},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), Options{
		Path:    path,
		OutPath: outPath,
		Config:  testConfig(),
		Client:  client,
		Stdout:  &stdout,
		Stderr:  &stderr,
	})

	if code != ExitSuccess {
		t.Fatalf("Run() code = %d, want %d; stderr = %q", code, ExitSuccess, stderr.String())
	}
	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(out) error = %v", err)
	}
	if string(written) != stdout.String() {
		t.Errorf("out = %q, want stdout %q", string(written), stdout.String())
	}
}

func TestRun_DoesNotOverwriteSourceFile(t *testing.T) {
	t.Parallel()

	original := []byte("一段")
	path := writeInput(t, original)
	resolved := canonicalPath(t, path)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), Options{
		Path:    path,
		OutPath: path,
		Config:  testConfig(),
		Client: &fakeClient{
			path:      resolved,
			summaries: []string{"摘要"},
		},
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if code != ExitFailure {
		t.Errorf("Run() code = %d, want %d", code, ExitFailure)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(source) error = %v", err)
	}
	if !bytes.Equal(content, original) {
		t.Errorf("source = %q, want unchanged %q", content, original)
	}
}

func TestRun_LLMFailureDoesNotWriteOutput(t *testing.T) {
	t.Parallel()

	path := writeInput(t, []byte("一段"))
	outPath := filepath.Join(t.TempDir(), "summary.txt")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(context.Background(), Options{
		Path:    path,
		OutPath: outPath,
		Config:  testConfig(),
		Client:  &fakeClient{fail: errors.New("service unavailable")},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})

	if code != ExitFailure {
		t.Errorf("Run() code = %d, want %d", code, ExitFailure)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "service unavailable") {
		t.Errorf("stderr = %q, want LLM diagnostic", stderr.String())
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("Stat(out) error = %v, want not exist", err)
	}
}

func testConfig() config.Config {
	return config.Config{
		MaxSteps:     4,
		MaxBytes:     512 * 1024,
		MaxParaChars: 8000,
	}
}

func writeInput(t *testing.T, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return resolved
}

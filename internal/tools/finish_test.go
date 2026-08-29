package tools

import (
	"context"
	"testing"

	"github.com/xiaoxlm/ai-file/internal/memory"
)

func TestFinish_BeforeReadFile(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	mem := memory.NewInMemory()
	tool := NewFinish(reg, mem)

	obs, err := tool.Execute(
		context.Background(),
		`{"items":["one"]}`,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "error: read_file must succeed before finish"
	if obs != want {
		t.Errorf("Execute() observation = %q, want %q", obs, want)
	}
	if _, ok := reg.Completion(); ok {
		t.Error("Completion() ok = true before successful finish, want false")
	}
}

func TestFinish_MissingItems(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	mem := memory.NewInMemory()
	mem.Set("paragraph_count", "1")
	tool := NewFinish(reg, mem)

	obs, err := tool.Execute(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if obs != "error: items is required" {
		t.Errorf("Execute() observation = %q, want error: items is required", obs)
	}
}

func TestFinish_ItemCountMismatch(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	mem := memory.NewInMemory()
	mem.Set("paragraph_count", "2")
	tool := NewFinish(reg, mem)

	obs, err := tool.Execute(
		context.Background(),
		`{"items":["only one"]}`,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "error: item count 1 does not match paragraph count 2"
	if obs != want {
		t.Errorf("Execute() observation = %q, want %q", obs, want)
	}
	if _, ok := reg.Completion(); ok {
		t.Error("Completion() ok = true after mismatch, want false")
	}
}

func TestFinish_ZeroParagraphsEmptyItems(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	mem := memory.NewInMemory()
	mem.Set("paragraph_count", "0")
	tool := NewFinish(reg, mem)

	obs, err := tool.Execute(context.Background(), `{"items":[]}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if obs != "finish accepted" {
		t.Errorf("Execute() observation = %q, want finish accepted", obs)
	}

	items, ok := reg.Completion()
	if !ok {
		t.Fatal("Completion() ok = false, want true")
	}
	if len(items) != 0 {
		t.Errorf("Completion() items = %v, want empty slice", items)
	}
}

func TestFinish_SuccessThreeItems(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	mem := memory.NewInMemory()
	mem.Set("paragraph_count", "3")
	tool := NewFinish(reg, mem)

	wantItems := []string{"a", "b", "c"}
	args := `{"items":["a","b","c"]}`
	obs, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if obs != "finish accepted" {
		t.Errorf("Execute() observation = %q, want finish accepted", obs)
	}

	items, ok := reg.Completion()
	if !ok {
		t.Fatal("Completion() ok = false, want true")
	}
	if len(items) != 3 || items[0] != "a" || items[1] != "b" || items[2] != "c" {
		t.Fatalf("Completion() items = %v, want %v", items, wantItems)
	}
}

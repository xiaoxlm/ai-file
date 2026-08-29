package tools

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeTool struct {
	name string
}

func (f fakeTool) Name() string { return f.name }

func (f fakeTool) Description() string { return "desc for " + f.name }

func (f fakeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (f fakeTool) Execute(context.Context, string) (string, error) {
	return "ok", nil
}

func TestRegistry_DuplicateRegisterFails(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.Register(fakeTool{name: "t"}); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	if err := r.Register(fakeTool{name: "t"}); err == nil {
		t.Fatal("second Register() error = nil, want error")
	}
}

func TestRegistry_ExecuteUnknownTool(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.Execute(context.Background(), "missing", `{}`)
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	const want = `unknown tool "missing"`
	if err.Error() != want {
		t.Errorf("Execute() error = %q, want %q", err.Error(), want)
	}
}

func TestRegistry_ListSchemas(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.Register(fakeTool{name: "alpha"}); err != nil {
		t.Fatalf("Register(alpha) error = %v", err)
	}
	if err := r.Register(fakeTool{name: "beta"}); err != nil {
		t.Fatalf("Register(beta) error = %v", err)
	}

	specs := r.List()
	if len(specs) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(specs))
	}
	if specs[0].Name != "alpha" || specs[1].Name != "beta" {
		t.Fatalf("List() names = [%q, %q], want [alpha, beta]", specs[0].Name, specs[1].Name)
	}
	for _, spec := range specs {
		if spec.Description == "" {
			t.Errorf("spec %q has empty Description", spec.Name)
		}
		if len(spec.InputSchema) == 0 {
			t.Errorf("spec %q has empty InputSchema", spec.Name)
		}
	}
}

func TestRegistry_CompletionBeforeSet(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	items, ok := r.Completion()
	if ok {
		t.Errorf("Completion() ok = true, want false")
	}
	if items != nil {
		t.Errorf("Completion() items = %v, want nil", items)
	}
}

func TestRegistry_CompletionReturnsCopy(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	r.setCompletion([]string{"one", "two"})

	items, ok := r.Completion()
	if !ok {
		t.Fatal("Completion() ok = false, want true")
	}
	if len(items) != 2 || items[0] != "one" || items[1] != "two" {
		t.Fatalf("Completion() items = %v, want [one two]", items)
	}

	items[0] = "mutated"
	again, ok := r.Completion()
	if !ok {
		t.Fatal("second Completion() ok = false, want true")
	}
	if again[0] != "one" {
		t.Errorf("stored item after mutation = %q, want one", again[0])
	}
}

func TestRegistry_ExecuteRegisteredTool(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.Register(fakeTool{name: "t"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	obs, err := r.Execute(context.Background(), "t", `{}`)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if obs != "ok" {
		t.Errorf("Execute() observation = %q, want ok", obs)
	}
}

package skills_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/runtime/skills"
)

func TestLazyLoadViaRegistry(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", "ext")

	writeSkill(t, filepath.Join(dir, "SKILL.md"), "ext", "body from registry")

	calls := map[string]int{}
	var mu sync.Mutex
	restore := skills.SetReadFileForTest(func(path string) ([]byte, error) {
		mu.Lock()
		calls[path]++
		mu.Unlock()
		return os.ReadFile(path)
	})
	defer restore()

	regs, errs := skills.LoadFromFS(skills.LoaderOptions{ProjectRoot: root})
	if len(errs) != 0 {
		t.Fatalf("unexpected load errs: %v", errs)
	}

	registry := skills.NewRegistry()
	for _, reg := range regs {
		if err := registry.Register(reg.Definition, reg.Handler); err != nil {
			t.Fatalf("register: %v", err)
		}
	}

	if len(calls) != 0 {
		t.Fatalf("expected no reads at startup, got %v", calls)
	}

	res, err := registry.Execute(context.Background(), "ext", skills.ActivationContext{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	output, ok := res.Output.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", res.Output)
	}
	if output["body"] != "body from registry" {
		t.Fatalf("unexpected body: %#v", output["body"])
	}

	mu.Lock()
	defer mu.Unlock()
	if calls[filepath.Join(dir, "SKILL.md")] != 1 {
		t.Fatalf("expected SKILL.md to be read once, got %d", calls[filepath.Join(dir, "SKILL.md")])
	}
}

func TestLazyLoadErrorPropagates(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", "err")
	writeSkill(t, filepath.Join(dir, "SKILL.md"), "err", "body")

	restore := skills.SetReadFileForTest(func(path string) ([]byte, error) {
		if filepath.Base(path) == "SKILL.md" {
			return nil, errors.New("io failure")
		}
		return os.ReadFile(path)
	})
	defer restore()

	regs, errs := skills.LoadFromFS(skills.LoaderOptions{ProjectRoot: root})
	if len(errs) != 0 {
		t.Fatalf("unexpected load errs: %v", errs)
	}

	registry := skills.NewRegistry()
	if err := registry.Register(regs[0].Definition, regs[0].Handler); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := registry.Execute(context.Background(), "err", skills.ActivationContext{}); err == nil {
		t.Fatalf("expected execute error")
	}
}

// writeSkill duplicates the helper from pkg/runtime/skills for external tests.
func writeSkill(t *testing.T, path, name, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: desc\n---\n" + body
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

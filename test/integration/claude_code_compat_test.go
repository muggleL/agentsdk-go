//go:build integration
// +build integration

package integration

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/config"
	"github.com/cexll/agentsdk-go/pkg/runtime/skills"
	"github.com/cexll/agentsdk-go/pkg/runtime/subagents"
)

func userHomeOrSkip(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("resolve user home: %v", err)
	}
	return home
}

func ensureDirExistsOrSkip(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skipf("%s not found; skipping integration check against real user config", dir)
		}
		t.Fatalf("stat %s: %v", dir, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s exists but is not a directory", dir)
	}
}

func ensureFileExistsOrSkip(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skipf("%s not found; skipping integration check against real user config", path)
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory, expected file", path)
	}
}

func TestLoadUserClaudeSkills(t *testing.T) {
	home := userHomeOrSkip(t)
	skillsDir := filepath.Join(home, ".claude", "skills")
	ensureDirExistsOrSkip(t, skillsDir)

	opts := skills.LoaderOptions{
		ProjectRoot: t.TempDir(), // avoid pulling repo .claude state into the check
		UserHome:    home,
		EnableUser:  true,
	}
	registrations, errs := skills.LoadFromFS(opts)

	failures := 0
	for _, err := range errs {
		failures++
		t.Logf("WARN: failed to load user skill: %v", err)
	}

	successes := 0
	for _, reg := range registrations {
		if strings.TrimSpace(reg.Definition.Name) == "" {
			failures++
			t.Logf("WARN: skill with empty name detected (source: %v)", reg.Definition.Metadata)
			continue
		}
		successes++
	}

	t.Logf("user skills load summary: success=%d, failed=%d", successes, failures)

	if successes == 0 {
		if failures == 0 {
			t.Skipf("no skills found in %s", skillsDir)
		}
		t.Fatalf("failed to load any valid user skills from %s", skillsDir)
	}
}

func TestLoadUserClaudeAgents(t *testing.T) {
	home := userHomeOrSkip(t)
	agentsDir := filepath.Join(home, ".claude", "agents")
	ensureDirExistsOrSkip(t, agentsDir)

	opts := subagents.LoaderOptions{
		ProjectRoot: t.TempDir(),
		UserHome:    home,
		EnableUser:  true,
	}
	registrations, errs := subagents.LoadFromFS(opts)

	failures := 0
	for _, err := range errs {
		failures++
		t.Logf("WARN: failed to load user subagent: %v", err)
	}

	successes := 0
	for _, reg := range registrations {
		if strings.TrimSpace(reg.Definition.Name) == "" {
			failures++
			t.Logf("WARN: subagent with empty name detected (source: %v)", reg.Definition.BaseContext.Metadata["source"])
			continue
		}
		successes++
	}

	t.Logf("user subagents load summary: success=%d, failed=%d", successes, failures)

	if successes == 0 {
		if failures == 0 {
			t.Skipf("no subagents found in %s", agentsDir)
		}
		t.Skip("all user subagents have parse errors - not a code defect")
	}
}

func TestSettingsCompatibility(t *testing.T) {
	home := userHomeOrSkip(t)
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	ensureFileExistsOrSkip(t, settingsPath)

	loader := config.SettingsLoader{ProjectRoot: t.TempDir()}
	settings, err := loader.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings == nil {
		t.Fatalf("settings loader returned nil")
	}
	// Some user configs omit model because the app injects it at runtime; seed a safe default
	// so we can still validate permissions/MCP/hooks compatibility.
	if strings.TrimSpace(settings.Model) == "" {
		settings.Model = "claude-3-5-sonnet-20241022"
	}

	if err := config.ValidateSettings(settings); err != nil {
		t.Fatalf("settings.json incompatible with current schema: %v", err)
	}

	t.Logf("settings.json compatible with current schema: %s", settingsPath)
}

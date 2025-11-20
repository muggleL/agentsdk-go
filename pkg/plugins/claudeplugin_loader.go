package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ClaudePlugin represents a fully resolved plugin in .claude-plugin format.
type ClaudePlugin struct {
	Name        string
	Version     string
	Description string
	Author      string
	RootDir     string
	Commands    []string
	Agents      []string
	Skills      []string
	Hooks       map[string][]string
	MCPConfig   *MCPConfig
}

// MCPConfig holds parsed .mcp.json content.
type MCPConfig struct {
	Path string
	Data map[string]any
}

// LoadPluginFromDir loads a plugin using the official .claude-plugin layout.
// The provided dir should be the repository root that contains the .claude-plugin
// folder; the manifest search falls back to dir/.claude-plugin/plugin.json.
func LoadPluginFromDir(dir string) (*ClaudePlugin, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("plugin directory is required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("plugin path %s is not a directory", dir)
	}

	manifestPath, err := FindManifest(dir)
	if err != nil {
		return nil, err
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	plugin := &ClaudePlugin{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Author:      manifest.Author,
		RootDir:     manifest.PluginDir,
		Commands:    manifest.Commands,
		Agents:      manifest.Agents,
		Skills:      manifest.Skills,
		Hooks:       manifest.Hooks,
	}

	plugin.Commands, err = populateMarkdownList(plugin.Commands, filepath.Join(manifest.PluginDir, ".claude-plugin", "commands"))
	if err != nil {
		return nil, err
	}
	plugin.Agents, err = populateMarkdownList(plugin.Agents, filepath.Join(manifest.PluginDir, ".claude-plugin", "agents"))
	if err != nil {
		return nil, err
	}
	plugin.Skills, err = populateSkills(plugin.Skills, filepath.Join(manifest.PluginDir, ".claude-plugin", "skills"))
	if err != nil {
		return nil, err
	}
	if len(plugin.Hooks) == 0 {
		plugin.Hooks, err = loadHookFile(filepath.Join(manifest.PluginDir, ".claude-plugin", "hooks", "hooks.json"))
		if err != nil {
			return nil, err
		}
	}
	plugin.MCPConfig, err = loadMCPConfig(filepath.Join(manifest.PluginDir, ".claude-plugin", ".mcp.json"))
	if err != nil {
		return nil, err
	}

	return plugin, nil
}

// ScanPluginsInProject looks for a .claude-plugin manifest under projectRoot.
// Missing manifests are not treated as an error to allow projects without plugins.
func ScanPluginsInProject(projectRoot string) ([]*ClaudePlugin, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, errors.New("project root is required")
	}
	manifestPath, err := FindManifest(root)
	if err != nil {
		if errors.Is(err, ErrManifestNotFound) {
			return nil, nil
		}
		return nil, err
	}
	plug, err := LoadPluginFromDir(filepath.Dir(filepath.Dir(manifestPath)))
	if err != nil {
		// LoadPluginFromDir expects the repo root. If manifest sits directly under
		// root without .claude-plugin/, fall back to that directory.
		plug, err = LoadPluginFromDir(root)
	}
	if err != nil {
		return nil, err
	}
	return []*ClaudePlugin{plug}, nil
}

// FilterEnabledPlugins keeps plugins whose name is marked true in enabledPlugins.
// When enabledPlugins is empty or nil, all plugins are returned. A false entry
// explicitly disables a plugin.
func FilterEnabledPlugins(plugins []*ClaudePlugin, enabledPlugins map[string]bool) []*ClaudePlugin {
	if len(enabledPlugins) == 0 {
		return plugins
	}
	filtered := make([]*ClaudePlugin, 0, len(plugins))
	for _, p := range plugins {
		if p == nil {
			continue
		}
		allowed, ok := enabledPlugins[p.Name]
		if !ok || allowed {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func populateMarkdownList(existing []string, dir string) ([]string, error) {
	if len(existing) > 0 {
		for _, name := range existing {
			path := filepath.Join(dir, name+".md")
			info, err := os.Stat(path)
			if err != nil {
				return nil, fmt.Errorf("missing %s: %w", path, err)
			}
			if info.IsDir() {
				return nil, fmt.Errorf("expected file, got directory: %s", path)
			}
		}
		return sortedCopy(existing), nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if name != "" {
			names = append(names, name)
		}
	}
	return sortedCopy(names), nil
}

func populateSkills(existing []string, dir string) ([]string, error) {
	if len(existing) > 0 {
		for _, name := range existing {
			skillFile := filepath.Join(dir, name, "SKILL.md")
			info, err := os.Stat(skillFile)
			if err != nil {
				return nil, fmt.Errorf("missing %s: %w", skillFile, err)
			}
			if info.IsDir() {
				return nil, fmt.Errorf("expected skill file, got directory: %s", skillFile)
			}
		}
		return sortedCopy(existing), nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
		if info, err := os.Stat(skillFile); err == nil && !info.IsDir() {
			skills = append(skills, entry.Name())
		}
	}
	return sortedCopy(skills), nil
}

func loadHookFile(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var hooks map[string][]string
	if err := json.Unmarshal(data, &hooks); err != nil {
		return nil, fmt.Errorf("decode hooks: %w", err)
	}
	return normalizeHookMap(hooks), nil
}

func loadMCPConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode mcp config: %w", err)
	}
	return &MCPConfig{Path: path, Data: parsed}, nil
}

func sortedCopy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

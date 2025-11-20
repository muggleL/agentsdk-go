package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadPluginFromDirCollectsComponents(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin", "commands"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin", "agents"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin", "skills", "alpha"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin", "hooks"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "commands", "hello.md"), []byte("/hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "agents", "helper.md"), []byte("agent"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "skills", "alpha", "SKILL.md"), []byte("skill"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "hooks", "hooks.json"), []byte(`{"PreToolUse":["Bash"]}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", ".mcp.json"), []byte(`{"servers":[]}`), 0o600))

	manifest := Manifest{Name: "demo", Version: "1.0.0"}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), data, 0o600))

	plug, err := LoadPluginFromDir(pluginRoot)
	require.NoError(t, err)
	require.Equal(t, "demo", plug.Name)
	require.Contains(t, plug.Commands, "hello")
	require.Contains(t, plug.Agents, "helper")
	require.Contains(t, plug.Skills, "alpha")
	require.Contains(t, plug.Hooks["PreToolUse"], "Bash")
	require.NotNil(t, plug.MCPConfig)
	require.False(t, plug.MCPConfig.Data == nil)
}

func TestScanPluginsInProjectMissingManifest(t *testing.T) {
	plugs, err := ScanPluginsInProject(t.TempDir())
	require.NoError(t, err)
	require.Nil(t, plugs)
}

func TestScanPluginsInProjectFallbackManifest(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "plugin.json")
	data, err := json.Marshal(Manifest{Name: "solo", Version: "1.0.0"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, data, 0o600))

	plugs, err := ScanPluginsInProject(root)
	require.NoError(t, err)
	require.Len(t, plugs, 1)
	require.Equal(t, "solo", plugs[0].Name)
}

func TestFilterEnabledPlugins(t *testing.T) {
	plugins := []*ClaudePlugin{{Name: "a"}, {Name: "b"}}
	enabled := map[string]bool{"a": true, "b": false}
	filtered := FilterEnabledPlugins(plugins, enabled)
	require.Len(t, filtered, 1)
	require.Equal(t, "a", filtered[0].Name)
}

func TestLoadPluginFromDirErrors(t *testing.T) {
	tests := []struct {
		name string
		dir  func(t *testing.T) string
	}{
		// missing manifest should surface ErrManifestNotFound
		{
			name: "missing manifest",
			dir: func(t *testing.T) string {
				root := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755))
				return root
			},
		},
		// malformed plugin.json should bubble up decode errors
		{
			name: "invalid manifest json",
			dir: func(t *testing.T) string {
				root := t.TempDir()
				pluginDir := filepath.Join(root, "demo")
				require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), []byte("{"), 0o600))
				return pluginDir
			},
		},
		// permission denied when reading commands directory should fail load
		{
			name: "commands directory unreadable",
			dir: func(t *testing.T) string {
				root := t.TempDir()
				pluginDir := filepath.Join(root, "demo")
				commandsDir := filepath.Join(pluginDir, ".claude-plugin", "commands")
				require.NoError(t, os.MkdirAll(filepath.Dir(commandsDir), 0o755))
				require.NoError(t, os.Mkdir(commandsDir, 0o000))

				data, err := json.Marshal(Manifest{Name: "demo", Version: "1.0.0"})
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), data, 0o600))
				return pluginDir
			},
		},
		// missing directory entirely should error immediately
		{
			name: "nonexistent directory",
			dir: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
		},
		// blank path should be rejected before filesystem access
		{
			name: "empty path",
			dir: func(t *testing.T) string {
				return "   "
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadPluginFromDir(tc.dir(t))
			require.Error(t, err)
		})
	}
}

func TestPopulateMarkdownListExistingValidation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "commands")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "as-directory.md"), 0o755))

	tests := []struct {
		name     string
		existing []string
	}{
		// missing referenced markdown should error
		{name: "missing referenced file", existing: []string{"ghost"}},
		// referenced path that is a directory should error
		{name: "referenced directory", existing: []string{"as-directory"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := populateMarkdownList(tc.existing, dir)
			require.Error(t, err)
		})
	}
}

func TestPopulateSkillsExistingValidation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "empty"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "directory", "SKILL.md"), 0o755))

	tests := []struct {
		name     string
		existing []string
	}{
		// missing SKILL.md should be reported
		{name: "missing skill file", existing: []string{"absent"}},
		// SKILL path that is a directory should be rejected
		{name: "skill file is directory", existing: []string{"directory"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := populateSkills(tc.existing, dir)
			require.Error(t, err)
		})
	}
}

func TestAuxLoadersErrorCases(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		// malformed hooks.json should return a decode error
		{
			name: "invalid hooks json",
			run: func(t *testing.T) {
				path := filepath.Join(root, "hooks.json")
				require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
				_, err := loadHookFile(path)
				require.Error(t, err)
			},
		},
		// malformed .mcp.json should surface decode error
		{
			name: "invalid mcp json",
			run: func(t *testing.T) {
				path := filepath.Join(root, ".mcp.json")
				require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
				_, err := loadMCPConfig(path)
				require.Error(t, err)
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, tc.run)
	}
}

func TestFilterEnabledPluginsPassthrough(t *testing.T) {
	plugins := []*ClaudePlugin{{Name: "a"}, nil}
	// empty enable map should return original slice excluding nils only through caller
	got := FilterEnabledPlugins(plugins, nil)
	require.Len(t, got, 2)
}

func TestScanPluginsInProjectValidation(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		// empty root should be rejected
		{name: "empty root", root: "   "},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := ScanPluginsInProject(tc.root)
			require.Error(t, err)
		})
	}
}

func TestSortedCopyReturnsNilForEmpty(t *testing.T) {
	require.Nil(t, sortedCopy(nil))
	require.Nil(t, sortedCopy([]string{}))
}

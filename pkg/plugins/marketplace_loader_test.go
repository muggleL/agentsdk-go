package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMarketplaceDirectorySource(t *testing.T) {
	root := t.TempDir()
	marketRoot := filepath.Join(root, "market")
	pluginRoot := filepath.Join(root, "plugin")
	require.NoError(t, os.MkdirAll(filepath.Join(marketRoot, ".claude-plugin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin"), 0o755))

	pluginManifest := Manifest{Name: "demo", Version: "1.0.0"}
	pluginBytes, err := json.Marshal(pluginManifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), pluginBytes, 0o600))

	marketManifest := MarketplaceManifest{
		Name:    "local",
		Plugins: []MarketplacePluginEntry{{Name: "demo", Source: MarketplaceSource{Source: "directory", Path: pluginRoot}}},
	}
	marketBytes, err := json.Marshal(marketManifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(marketRoot, ".claude-plugin", "marketplace.json"), marketBytes, 0o600))

	cfg := &MarketplaceConfig{
		EnabledPlugins: map[string]bool{"demo@local": true},
		ExtraKnownMarketplaces: map[string]MarketplaceSource{
			"local": {Source: "directory", Path: marketRoot},
		},
	}
	plugs, err := LoadMarketplace(cfg)
	require.NoError(t, err)
	require.Len(t, plugs, 1)
	require.Equal(t, "demo", plugs[0].Name)
}

func TestLoadMarketplaceRejectsInvalidKey(t *testing.T) {
	cfg := &MarketplaceConfig{EnabledPlugins: map[string]bool{"badkey": true}}
	_, err := LoadMarketplace(cfg)
	require.Error(t, err)
}

func TestMarketplaceUnsupportedSource(t *testing.T) {
	root := t.TempDir()
	marketRoot := filepath.Join(root, "market")
	require.NoError(t, os.MkdirAll(filepath.Join(marketRoot, ".claude-plugin"), 0o755))
	manifest := MarketplaceManifest{
		Name:    "bad",
		Plugins: []MarketplacePluginEntry{{Name: "demo", Source: MarketplaceSource{Source: "unknown"}}},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(marketRoot, ".claude-plugin", "marketplace.json"), data, 0o600))

	cfg := &MarketplaceConfig{
		EnabledPlugins:         map[string]bool{"demo@bad": true},
		ExtraKnownMarketplaces: map[string]MarketplaceSource{"bad": {Source: "directory", Path: marketRoot}},
	}
	_, err = LoadMarketplace(cfg)
	require.Error(t, err)
}

func TestLoadPluginFromSourceErrors(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		source  MarketplaceSource
		prepare func(t *testing.T)
	}{
		// unsupported source should be rejected immediately
		{name: "unsupported source type", source: MarketplaceSource{Source: "zip"}},
		// empty directory path should fail validation
		{name: "empty directory path", source: MarketplaceSource{Source: "directory", Path: " "}},
		// non-existent directory should bubble up filesystem error from loader
		{name: "missing directory", source: MarketplaceSource{Source: "directory", Path: filepath.Join(root, "absent")}},
		// git clone failures should be exposed to callers
		{
			name:   "git clone failure",
			source: MarketplaceSource{Source: "git", URL: "https://example.com/does-not-matter.git"},
			prepare: func(t *testing.T) {
				path := writeFakeGit(t, "#!/bin/sh\nexit 1\n")
				t.Setenv("PATH", path+string(os.PathListSeparator)+os.Getenv("PATH"))
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepare != nil {
				tc.prepare(t)
			}
			_, err := loadPluginFromSource(tc.source, root)
			require.Error(t, err)
		})
	}
}

func TestLoadPluginFromSourceGitSuccess(t *testing.T) {
	path := writeFakeGit(t, "#!/bin/sh\nmkdir -p \"$4/.claude-plugin\"\nprintf '{\"name\":\"remote\",\"version\":\"1.0.0\"}' > \"$4/.claude-plugin/plugin.json\"\nexit 0\n")
	t.Setenv("PATH", path+string(os.PathListSeparator)+os.Getenv("PATH"))

	tests := []struct {
		name   string
		source MarketplaceSource
	}{
		// github source should be materialized via git clone
		{name: "github source", source: MarketplaceSource{Source: "github", Repo: "demo/repo"}},
		// generic git source should also clone successfully
		{name: "git source", source: MarketplaceSource{Source: "git", URL: "https://example.com/repo.git"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			plugin, err := loadPluginFromSource(tc.source, t.TempDir())
			require.NoError(t, err)
			require.Equal(t, "remote", plugin.Name)
		})
	}
}

func TestLoadPluginFromSourceRelativeDirectory(t *testing.T) {
	base := t.TempDir()
	pluginDir := filepath.Join(base, "plugins", "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), []byte(`{"name":"demo","version":"1.0.0"}`), 0o600))

	plugin, err := loadPluginFromSource(MarketplaceSource{Source: "directory", Path: "plugins/demo"}, base)
	require.NoError(t, err)
	require.Equal(t, "demo", plugin.Name)
}

func TestMaterializeSourceErrors(t *testing.T) {
	tests := []struct {
		name string
		src  MarketplaceSource
	}{
		// unsupported source should fail fast
		{name: "unsupported source", src: MarketplaceSource{Source: "invalid"}},
		// missing repo must error for github sources
		{name: "missing github repo", src: MarketplaceSource{Source: "github", Repo: ""}},
		// empty git url should be rejected
		{name: "missing git url", src: MarketplaceSource{Source: "git"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := materializeSource(tc.src)
			require.Error(t, err)
		})
	}
}

func TestMaterializeSourceRelativeDirectory(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		// relative directory path should be absolutized
		{name: "relative path", path: "relative/market"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			src := MarketplaceSource{Source: "directory", Path: tc.path}
			resolved, err := materializeSource(src)
			require.NoError(t, err)
			require.True(t, filepath.IsAbs(resolved))
		})
	}
}

func TestMaterializeSourceGitSuccess(t *testing.T) {
	path := writeFakeGit(t, "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", path+string(os.PathListSeparator)+os.Getenv("PATH"))

	// git source should be cloned using the provided URL
	resolved, err := materializeSource(MarketplaceSource{Source: "git", URL: "https://example.com/repo.git"})
	require.NoError(t, err)
	require.DirExists(t, resolved)
}

func TestMaterializeSourceGitHubSuccess(t *testing.T) {
	path := writeFakeGit(t, "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", path+string(os.PathListSeparator)+os.Getenv("PATH"))

	// github source should delegate to git clone
	resolved, err := materializeSource(MarketplaceSource{Source: "github", Repo: "owner/repo"})
	require.NoError(t, err)
	require.DirExists(t, resolved)
}

func TestLoadMarketplaceManifestVariants(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) MarketplaceSource
		shouldFail bool
	}{
		// manifest at project root should be accepted when .claude-plugin missing
		{
			name: "manifest at root fallback",
			setup: func(t *testing.T) MarketplaceSource {
				root := t.TempDir()
				mf := MarketplaceManifest{Name: "plain"}
				data, err := json.Marshal(mf)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(root, "marketplace.json"), data, 0o600))
				return MarketplaceSource{Source: "directory", Path: root}
			},
		},
		// missing manifest should return error
		{
			name: "missing manifest error",
			setup: func(t *testing.T) MarketplaceSource {
				return MarketplaceSource{Source: "directory", Path: t.TempDir()}
			},
			shouldFail: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := loadMarketplaceManifest(tc.setup(t))
			if tc.shouldFail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMarketplaceEntryUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		// string source should be translated to directory source
		{name: "string source path", payload: `{"name":"demo","source":"./local"}`},
		// missing source should trigger error
		{name: "missing source field", payload: `{"name":"demo"}`, wantErr: true},
		// unsupported source object should fail validation
		{name: "unsupported source object", payload: `{"name":"demo","source":{"source":"zip"}}`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var entry MarketplacePluginEntry
			err := json.Unmarshal([]byte(tc.payload), &entry)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "demo", entry.Name)
			require.Equal(t, "directory", entry.Source.Source)
		})
	}
}

func TestValidateMarketplaceSourceNil(t *testing.T) {
	require.Error(t, validateMarketplaceSource(nil))
}

func TestLoadMarketplaceInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *MarketplaceConfig
		wantErr bool
	}{
		// nil config should error
		{name: "nil config", cfg: nil, wantErr: true},
		// empty enabled plugins should short-circuit with nil result
		{name: "no enabled plugins", cfg: &MarketplaceConfig{EnabledPlugins: map[string]bool{}}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			plugs, err := LoadMarketplace(tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Nil(t, plugs)
		})
	}
}

func TestLoadMarketplaceUnknownRegistry(t *testing.T) {
	// requesting marketplace not in config should error
	cfg := &MarketplaceConfig{EnabledPlugins: map[string]bool{"demo@missing": true}}
	_, err := LoadMarketplace(cfg)
	require.Error(t, err)
}

func TestPluginByNameAndMergeSources(t *testing.T) {
	entry := MarketplaceManifest{
		Plugins: []MarketplacePluginEntry{{Name: "demo"}, {Name: "other"}},
	}
	// plugin should be found
	found, ok := entry.PluginByName("other")
	require.True(t, ok)
	require.Equal(t, "other", found.Name)
	// missing plugin should return false
	_, ok = entry.PluginByName("absent")
	require.False(t, ok)
	// merge should allow extra to override base
	base := map[string]MarketplaceSource{"a": {Source: "directory", Path: "/base"}}
	extra := map[string]MarketplaceSource{"a": {Source: "directory", Path: "/override"}}
	merged := mergeMarketplaceSources(base, extra)
	require.Equal(t, "/override", merged["a"].Path)
}

func TestLoadMarketplaceManifestDecodeError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude-plugin", "marketplace.json"), []byte("{"), 0o600))

	_, _, err := loadMarketplaceManifest(MarketplaceSource{Source: "directory", Path: root})
	require.Error(t, err)
}

func TestValidateMarketplaceSourceSupported(t *testing.T) {
	// supported source types should pass validation
	for _, src := range []MarketplaceSource{
		{Source: "github", Repo: "repo/name"},
		{Source: "git", URL: "https://example.com/repo.git"},
		{Source: "directory", Path: "/abs"},
	} {
		require.NoError(t, validateMarketplaceSource(&src))
	}
}

func writeFakeGit(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700)) //nolint:gosec // executable helper script for tests
	return dir
}

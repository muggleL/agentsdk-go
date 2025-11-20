package plugins

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadManifestWithSignature(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	mf := Manifest{
		Name:        "demo",
		Version:     "1.0.0",
		Description: "Demo plugin",
		Author:      "tester",
		Commands:    []string{"hello"},
		Hooks:       map[string][]string{"PreToolUse": {"Bash"}},
		Signer:      "dev",
	}
	digest, err := computeManifestDigest(&mf)
	require.NoError(t, err)
	mf.Digest = digest
	sig, err := SignManifest(&mf, priv)
	require.NoError(t, err)
	mf.Signature = sig

	manifestPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	data, err := json.MarshalIndent(&mf, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, data, 0o600))

	store := NewTrustStore()
	store.Register("dev", pub)

	loaded, err := LoadManifest(manifestPath, WithTrustStore(store), WithRoot(pluginDir))
	require.NoError(t, err)
	require.True(t, loaded.Trusted)
	require.Equal(t, pluginDir, loaded.PluginDir)
	require.Equal(t, manifestPath, loaded.ManifestPath)
}

func TestLoadManifestComputesDigestWhenMissing(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
	mf := Manifest{Name: "demo", Version: "1.0.0", Description: "demo"}
	data, err := json.Marshal(mf)
	require.NoError(t, err)
	manifestPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	require.NoError(t, os.WriteFile(manifestPath, data, 0o600))

	loaded, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	require.NotEmpty(t, loaded.Digest)
	require.True(t, loaded.Trusted)
}

func TestFindManifestPrefersClaudePluginFolder(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude-plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), []byte(`{"name":"a","version":"1.0.0"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.json"), []byte(`{"name":"b","version":"1.0.0"}`), 0o600))

	path, err := FindManifest(root)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, ".claude-plugin", "plugin.json"), path)
}

func TestDiscoverManifestsLoadsChildren(t *testing.T) {
	root := t.TempDir()
	dirs := []string{"one", "two"}
	for _, name := range dirs {
		pluginDir := filepath.Join(root, name, ".claude-plugin")
		require.NoError(t, os.MkdirAll(pluginDir, 0o755))
		mf := Manifest{Name: name, Version: "1.0.0"}
		data, err := json.Marshal(mf)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o600))
	}

	store := NewTrustStore()
	store.AllowUnsigned(true)
	manifests, err := DiscoverManifests(root, store)
	require.NoError(t, err)
	require.Len(t, manifests, 2)
	require.Equal(t, "one", manifests[0].Name)
	require.Equal(t, "two", manifests[1].Name)
}

func TestDiscoverManifestsSkipsMissingChildren(t *testing.T) {
	root := t.TempDir()
	validDir := filepath.Join(root, "has-manifest", ".claude-plugin")
	require.NoError(t, os.MkdirAll(validDir, 0o755))
	data, err := json.Marshal(Manifest{Name: "kept", Version: "1.0.0"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(validDir, "plugin.json"), data, 0o600))

	emptyDir := filepath.Join(root, "missing")
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))

	store := NewTrustStore()
	store.AllowUnsigned(true)
	manifests, err := DiscoverManifests(root, store)
	require.NoError(t, err)
	require.Len(t, manifests, 1)
	require.Equal(t, "kept", manifests[0].Name)
}

func TestValidateManifestFieldsErrors(t *testing.T) {
	require.Error(t, validateManifestFields(nil))
	require.Error(t, validateManifestFields(&Manifest{Name: "BAD", Version: "1.0.0"}))
	require.Error(t, validateManifestFields(&Manifest{Name: "ok", Version: "not-semver"}))
	require.Error(t, validateManifestFields(&Manifest{Name: "ok", Version: "1.0.0", Commands: []string{""}}))
	require.Error(t, validateManifestFields(&Manifest{Name: "ok", Version: "1.0.0", Agents: []string{""}}))
	require.Error(t, validateManifestFields(&Manifest{Name: "ok", Version: "1.0.0", Skills: []string{""}}))
	require.Error(t, validateManifestFields(&Manifest{Name: "ok", Version: "1.0.0", Digest: "short"}))
}

func TestIsSemVerHelper(t *testing.T) {
	require.True(t, IsSemVer("1.2.3"))
	require.True(t, IsSemVer("v1.2.3-beta"))
	require.False(t, IsSemVer(""))
	require.False(t, IsSemVer("not-semver"))
}

func TestComputeManifestDigestNil(t *testing.T) {
	_, err := computeManifestDigest(nil)
	require.Error(t, err)
}

func TestNormalizeHookMapFiltersEmptyEntries(t *testing.T) {
	input := map[string][]string{
		"":         {"noop"},
		"PreTool":  {" ", "Run"},
		"EmptyVal": {},
	}
	out := normalizeHookMap(input)
	require.Equal(t, map[string][]string{"PreTool": {"Run"}}, out)
}

func TestLoadManifestSignatureFailures(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) (string, []ManifestOption)
	}{
		// mismatched signer key should fail verification
		{
			name: "signature mismatch",
			prepare: func(t *testing.T) (string, []ManifestOption) {
				root := t.TempDir()
				pluginDir := filepath.Join(root, "plugin")

				_, privForSig, err := ed25519.GenerateKey(nil)
				require.NoError(t, err)
				pubRegistered, _, err := ed25519.GenerateKey(nil)
				require.NoError(t, err)

				mf := Manifest{Name: "demo", Version: "1.0.0", Signer: "dev"}
				digest, err := computeManifestDigest(&mf)
				require.NoError(t, err)
				mf.Digest = digest
				sig, err := SignManifest(&mf, privForSig)
				require.NoError(t, err)
				mf.Signature = sig

				path := writeManifest(t, pluginDir, mf)
				store := NewTrustStore()
				store.Register("dev", pubRegistered)
				return path, []ManifestOption{WithTrustStore(store), WithRoot(pluginDir)}
			},
		},
		// digest mismatch must be caught before trust validation
		{
			name: "digest mismatch",
			prepare: func(t *testing.T) (string, []ManifestOption) {
				root := t.TempDir()
				pluginDir := filepath.Join(root, "plugin")
				mf := Manifest{Name: "demo", Version: "1.0.0", Digest: strings.Repeat("0", 64)}
				path := writeManifest(t, pluginDir, mf)
				return path, []ManifestOption{WithRoot(pluginDir)}
			},
		},
		// missing signature fields should be rejected when unsigned plugins disallowed
		{
			name: "missing signature",
			prepare: func(t *testing.T) (string, []ManifestOption) {
				root := t.TempDir()
				pluginDir := filepath.Join(root, "plugin")
				mf := Manifest{Name: "demo", Version: "1.0.0"}
				digest, err := computeManifestDigest(&mf)
				require.NoError(t, err)
				mf.Digest = digest
				path := writeManifest(t, pluginDir, mf)
				store := NewTrustStore()
				return path, []ManifestOption{WithTrustStore(store), WithRoot(pluginDir)}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			path, opts := tc.prepare(t)
			_, err := LoadManifest(path, opts...)
			require.Error(t, err)
		})
	}
}

func TestLoadManifestRootEnforcement(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugin")
	manifest := Manifest{Name: "demo", Version: "1.0.0"}
	path := writeManifest(t, pluginDir, manifest)
	// root is intentionally outside pluginDir to simulate traversal
	_, err := LoadManifest(path, WithRoot(filepath.Join(root, "other")))
	require.Error(t, err)
}

func TestLoadManifestRefusesDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadManifest(dir)
	require.Error(t, err)
}

func writeManifest(t *testing.T, pluginDir string, mf Manifest) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
	data, err := json.Marshal(mf)
	require.NoError(t, err)
	path := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestDiscoverManifestsMissingRoot(t *testing.T) {
	manifests, err := DiscoverManifests(filepath.Join(t.TempDir(), "nope"), NewTrustStore())
	require.NoError(t, err)
	require.Nil(t, manifests)
}

func TestDiscoverManifestsWithUnreadableEntry(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-dir")
	require.NoError(t, os.WriteFile(filePath, []byte("noop"), 0o600))

	_, err := DiscoverManifests(filePath, NewTrustStore())
	require.Error(t, err)
}

func TestDiscoverManifestsSkipsFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("doc"), 0o600))

	store := NewTrustStore()
	store.AllowUnsigned(true)
	manifests, err := DiscoverManifests(root, store)
	require.NoError(t, err)
	require.Nil(t, manifests)
}

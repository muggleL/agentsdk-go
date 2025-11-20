package config

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/plugins"
	"github.com/stretchr/testify/require"
)

// nolint:unused // retained for future plugin-signing tests.
func makeTrustStore(t *testing.T) (*plugins.TrustStore, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	ts := plugins.NewTrustStore()
	ts.Register("dev", pub)
	ts.AllowUnsigned(false)
	return ts, priv
}

// nolint:unused // retained for future plugin-signing tests.
func writePlugin(t *testing.T, projectRoot, name string, signer ed25519.PrivateKey) string {
	t.Helper()
	pluginDir := filepath.Join(projectRoot, ".claude-plugin")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte("demo"), 0o600))

	mf := &plugins.Manifest{
		Name:    name,
		Version: "1.2.3",
		Signer:  "dev",
	}
	mf.Digest = manifestDigestForTest(t, mf)
	sig, err := plugins.SignManifest(mf, signer)
	require.NoError(t, err)
	mf.Signature = sig

	manifestBytes, err := json.MarshalIndent(mf, "", "  ")
	require.NoError(t, err)
	manifestPath := filepath.Join(pluginDir, "plugin.json")
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0o600))
	return manifestPath
}

// nolint:unused // retained for future plugin-signing tests.
func manifestDigestForTest(t *testing.T, mf *plugins.Manifest) string {
	t.Helper()
	payload := struct {
		Name        string              `json:"name"`
		Version     string              `json:"version"`
		Description string              `json:"description"`
		Author      string              `json:"author"`
		Commands    []string            `json:"commands,omitempty"`
		Agents      []string            `json:"agents,omitempty"`
		Skills      []string            `json:"skills,omitempty"`
		Hooks       map[string][]string `json:"hooks,omitempty"`
	}{
		Name:        mf.Name,
		Version:     mf.Version,
		Description: mf.Description,
		Author:      mf.Author,
		Commands:    mf.Commands,
		Agents:      mf.Agents,
		Skills:      mf.Skills,
		Hooks:       mf.Hooks,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

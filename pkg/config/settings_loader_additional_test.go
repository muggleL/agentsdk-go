package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsLoader_EmptyProjectRoot(t *testing.T) {
	loader := SettingsLoader{ProjectRoot: "  "}
	_, err := loader.Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "project root")
}

func TestLoadJSONFile_BlankPath(t *testing.T) {
	cfg, err := loadJSONFile("")
	require.NoError(t, err)
	require.Nil(t, cfg)
}

func TestLoadJSONFile_PermissionDenied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"model":"claude"}`), 0o600))
	require.NoError(t, os.Chmod(path, 0o000))

	cfg, err := loadJSONFile(path)
	require.Nil(t, cfg)
	require.Error(t, err)
	require.True(t, errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrInvalid))
}

func TestApplySettingsLayer_EmptyPath(t *testing.T) {
	dst := GetDefaultSettings()
	require.NoError(t, applySettingsLayer(&dst, "skip", ""))
	require.Equal(t, "askBeforeRunningTools", dst.Permissions.DefaultMode)
}

func TestGetUserSettingsPath_HomeUnset(t *testing.T) {
	envMu.Lock()
	original := os.Getenv("HOME")
	require.NoError(t, os.Unsetenv("HOME"))
	t.Cleanup(func() {
		if original == "" {
			require.NoError(t, os.Unsetenv("HOME"))
		} else {
			require.NoError(t, os.Setenv("HOME", original))
		}
		envMu.Unlock()
	})

	path := getUserSettingsPath()
	require.Empty(t, path)
}

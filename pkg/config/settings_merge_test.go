package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeSettingsDeepCopyAndOverrides(t *testing.T) {
	lower := &Settings{
		APIKeyHelper:         "lower-helper",
		CleanupPeriodDays:    30,
		CompanyAnnouncements: []string{"a"},
		Env:                  map[string]string{"K1": "V1", "shared": "low"},
		IncludeCoAuthoredBy:  boolPtr(true),
		Model:                "claude-3",
		Permissions: &PermissionsConfig{
			Allow:                 []string{"fs"},
			DefaultMode:           "askBeforeRunningTools",
			Ask:                   []string{"net"},
			AdditionalDirectories: []string{"/data"},
		},
		Hooks: &HooksConfig{
			PreToolUse: map[string]string{"bash": "echo low"},
		},
		Sandbox: &SandboxConfig{
			Enabled:          boolPtr(false),
			ExcludedCommands: []string{"rm"},
			Network: &SandboxNetworkConfig{
				AllowUnixSockets: []string{"/tmp/agent.sock"},
			},
		},
		EnabledPlugins: map[string]bool{"p1": true},
	}

	higher := &Settings{
		CleanupPeriodDays:    7,
		CompanyAnnouncements: []string{"b", "a"},
		Env:                  map[string]string{"K2": "V2", "shared": "high"},
		IncludeCoAuthoredBy:  boolPtr(false),
		Model:                "claude-3-5",
		Permissions: &PermissionsConfig{
			Allow:       []string{"fs", "net"},
			DefaultMode: "acceptEdits",
			Ask:         []string{"net"},
		},
		Hooks: &HooksConfig{
			PreToolUse:  map[string]string{"bash": "echo high"},
			PostToolUse: map[string]string{"bash": "echo done"},
		},
		Sandbox: &SandboxConfig{
			Enabled:          boolPtr(true),
			ExcludedCommands: []string{"sudo"},
			Network: &SandboxNetworkConfig{
				AllowUnixSockets: []string{"/tmp/agent.sock", "/var/run/docker.sock"},
				HTTPProxyPort:    intPtr(8080),
			},
		},
		EnabledPlugins: map[string]bool{"p2": true, "p1": false},
	}

	merged := MergeSettings(lower, higher)
	require.NotNil(t, merged)
	require.Equal(t, "claude-3-5", merged.Model)
	require.Equal(t, 7, merged.CleanupPeriodDays)
	require.Equal(t, map[string]string{"K1": "V1", "K2": "V2", "shared": "high"}, merged.Env)
	require.Equal(t, []string{"a", "b"}, merged.CompanyAnnouncements)
	require.NotSame(t, lower.Env, merged.Env)
	require.NotSame(t, higher.Env, merged.Env)
	require.False(t, *merged.IncludeCoAuthoredBy)

	require.Equal(t, []string{"fs", "net"}, merged.Permissions.Allow)
	require.Equal(t, []string{"net"}, merged.Permissions.Ask)
	require.Equal(t, "acceptEdits", merged.Permissions.DefaultMode)
	require.Equal(t, []string{"/data"}, merged.Permissions.AdditionalDirectories)

	require.Equal(t, map[string]string{"bash": "echo high"}, merged.Hooks.PreToolUse)
	require.Equal(t, map[string]string{"bash": "echo done"}, merged.Hooks.PostToolUse)

	require.True(t, *merged.Sandbox.Enabled)
	require.Equal(t, []string{"rm", "sudo"}, merged.Sandbox.ExcludedCommands)
	require.Equal(t, []string{"/tmp/agent.sock", "/var/run/docker.sock"}, merged.Sandbox.Network.AllowUnixSockets)
	require.Equal(t, 8080, *merged.Sandbox.Network.HTTPProxyPort)

	require.Equal(t, map[string]bool{"p1": false, "p2": true}, merged.EnabledPlugins)

	// Ensure inputs untouched.
	require.Equal(t, "claude-3", lower.Model)
	require.Equal(t, map[string]string{"K1": "V1", "shared": "low"}, lower.Env)
	require.Nil(t, higher.Sandbox.Network.SocksProxyPort)
}

func TestMergePermissionsDeduplication(t *testing.T) {
	lower := &PermissionsConfig{
		Allow: []string{"a", "b"},
		Deny:  []string{"c"},
	}
	higher := &PermissionsConfig{
		Allow: []string{"b", "c"},
		Deny:  []string{"d"},
	}
	out := mergePermissions(lower, higher)
	require.Equal(t, []string{"a", "b", "c"}, out.Allow)
	require.Equal(t, []string{"c", "d"}, out.Deny)
}

func TestMergeStringSlicesHandlesNil(t *testing.T) {
	out := mergeStringSlices(nil, []string{"a", "a", "b"})
	require.Equal(t, []string{"a", "b"}, out)
	require.Nil(t, mergeStringSlices(nil, nil))
}

func intPtr(v int) *int { return &v }

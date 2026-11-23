package permissionhook

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const toggleConfig = `# rules
hooks:
  permissionRequest:
    allow:
      tools:
        - list_repositories # keep me
`

func TestSetDisabledRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(toggleConfig), 0o600))

	res, err := SetDisabled(p, true)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	data, _ := os.ReadFile(p)
	assert.Contains(t, string(data), "# keep me")
	cfg, err := ParseConfig(data)
	require.NoError(t, err)
	assert.True(t, cfg.Hooks.Disabled)
	assert.Equal(t, []string{"list_repositories"}, cfg.Hooks.PermissionRequest.Allow.Tools)
	info, _ := os.Stat(p)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	res, err = SetDisabled(p, true)
	require.NoError(t, err)
	assert.False(t, res.Changed)

	res, err = SetDisabled(p, false)
	require.NoError(t, err)
	assert.True(t, res.Changed)
	data, _ = os.ReadFile(p)
	assert.NotContains(t, string(data), "disabled")
	cfg, err = ParseConfig(data)
	require.NoError(t, err)
	assert.False(t, cfg.Hooks.Disabled)
}

func TestSetDisabledMissingConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.yaml")
	res, err := SetDisabled(p, false)
	require.NoError(t, err)
	assert.False(t, res.Changed)
	assert.NoFileExists(t, p)

	res, err = SetDisabled(p, true)
	require.NoError(t, err)
	assert.True(t, res.Created)
	cfg, found, err := LoadConfig(p)
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, cfg.Hooks.Disabled)
}

func TestDisabledConfigSkipsValidationAndDecisions(t *testing.T) {
	invalid := "hooks:\n  disabled: true\n  permissionRequest:\n    allow:\n      commands: [\" :* \"]\n"
	p := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(invalid), 0o644))

	var out bytes.Buffer
	in := `{"toolName":"view","toolArgs":{"path":"/tmp"}}`
	require.NoError(t, Run(strings.NewReader(in), &out, p))
	assert.Empty(t, out.String())

	_, err := SetDisabled(p, false)
	assert.Error(t, err)
	data, _ := os.ReadFile(p)
	assert.Equal(t, invalid, string(data))
}

func TestInspectAndUninstallHook(t *testing.T) {
	dir := t.TempDir()
	hookPath := filepath.Join(dir, "hooks", HookFileName)
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(toggleConfig), 0o644))

	st, err := InspectHook(hookPath, configPath)
	require.NoError(t, err)
	assert.Equal(t, StateNotInstalled, st.State())

	_, err = InstallHook(InstallOptions{Path: hookPath, ConfigPath: configPath, TimeoutSec: 5, ScopedTools: true})
	require.NoError(t, err)
	other := filepath.Join(dir, "hooks", "other.json")
	require.NoError(t, os.WriteFile(other, []byte(`{"version":1,"hooks":{"permissionRequest":[]}}`), 0o644))

	st, err = InspectHook(hookPath, "")
	require.NoError(t, err)
	assert.Equal(t, StateActive, st.State())
	assert.Equal(t, ModeScopedPreToolUse, st.Mode)
	assert.True(t, st.ConfigPinned)
	assert.Equal(t, 1, st.Config.AllowTools)
	assert.Equal(t, []string{other}, st.ConflictingHooks)

	p, err := ResolveHookConfigPath(hookPath, "")
	require.NoError(t, err)
	assert.Equal(t, configPath, p)

	_, err = SetDisabled(configPath, true)
	require.NoError(t, err)
	st, err = InspectHook(hookPath, "")
	require.NoError(t, err)
	assert.Equal(t, StateDisabled, st.State())

	var out bytes.Buffer
	require.NoError(t, WriteStatus(&out, st))
	assert.Contains(t, out.String(), "State:   disabled")

	_, removed, err := UninstallHook(other, false)
	assert.Error(t, err)
	assert.False(t, removed)
	assert.FileExists(t, other)

	_, removed, err = UninstallHook(hookPath, false)
	require.NoError(t, err)
	assert.True(t, removed)
	assert.NoFileExists(t, hookPath)
	assert.FileExists(t, configPath)

	_, removed, err = UninstallHook(hookPath, false)
	require.NoError(t, err)
	assert.False(t, removed)
}

func TestInspectAutomaticallySelectedPreToolHook(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(toggleConfig), 0o600))
	dest := filepath.Join(dir, "hooks", HookFileName)
	_, err := InstallHook(InstallOptions{Path: dest, ConfigPath: config, TimeoutSec: 5})
	require.NoError(t, err)
	st, err := InspectHook(dest, "")
	require.NoError(t, err)
	assert.Equal(t, ModeAllPreToolUse, st.Mode)
	assert.Equal(t, StateActive, st.State())
	assert.True(t, st.ConfigPinned)
	_, removed, err := UninstallHook(dest, false)
	require.NoError(t, err)
	assert.True(t, removed)
}

func TestInspectWarnsWhenRulesOutgrowPermissionMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(toggleConfig), 0o600))
	dest := filepath.Join(dir, "hook.json")
	_, err := InstallHook(InstallOptions{Path: dest, ConfigPath: config, TimeoutSec: 5, ScopedToolsSet: true})
	require.NoError(t, err)
	st, err := InspectHook(dest, "")
	require.NoError(t, err)
	assert.True(t, st.NeedsReinstall)
	var out bytes.Buffer
	require.NoError(t, WriteStatus(&out, st))
	assert.Contains(t, out.String(), "reinstall the hook with --force")
}

func TestInspectGHReadPreset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`
hooks:
  permissionRequest:
    allow:
      ghRead: true
      gitRead: true
      shellDisplay: true
`), 0o600))
	dest := filepath.Join(dir, "hook.json")
	_, err := InstallHook(InstallOptions{Path: dest, ConfigPath: config, TimeoutSec: 5, ScopedToolsSet: true})
	require.NoError(t, err)
	st, err := InspectHook(dest, "")
	require.NoError(t, err)
	assert.True(t, st.Config.AllowGHRead)
	assert.True(t, st.Config.AllowGitRead)
	assert.True(t, st.Config.AllowShellDisplay)
	assert.True(t, st.NeedsReinstall)
	var out bytes.Buffer
	require.NoError(t, WriteStatus(&out, st))
	assert.Contains(t, out.String(), "ghRead=true")
	assert.Contains(t, out.String(), "gitRead=true")
	assert.Contains(t, out.String(), "shellDisplay=true")
	assert.Contains(t, out.String(), "reinstall the hook with --force")
}

func TestUninstallKeepsUnrelatedHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hook.json")
	content := `{"version":1,"hooks":{"preToolUse":[{"type":"command","exec":"other-tool","args":["hook","pre-tool-use","--all-rules"],"timeoutSec":5}]}}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	_, removed, err := UninstallHook(path, false)
	assert.ErrorContains(t, err, "not generated by hook install")
	assert.False(t, removed)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

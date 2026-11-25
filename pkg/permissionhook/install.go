package permissionhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// HookFileName is the file name used for the generated user-level hook config.
const HookFileName = "gh-copilot-attendant.json"

// DefaultHookTimeoutSec is the default timeoutSec of the generated hook entry.
const DefaultHookTimeoutSec = 10

// InstallOptions controls how the hook config file is generated.
type InstallOptions struct {
	// Path is the destination file. Empty means DefaultHookPath().
	Path string
	// ConfigPath is passed to the hook as --config when non-empty.
	ConfigPath string
	// TimeoutSec is the hook timeout in seconds.
	TimeoutSec int
	// Force overwrites an existing file.
	Force bool
	// ScopedTools installs preToolUse instead of permissionRequest.
	ScopedTools bool
	// ScopedToolsSet distinguishes an explicit false from automatic selection.
	ScopedToolsSet bool
}

type hookFile struct {
	Version int                    `json:"version"`
	Hooks   map[string][]hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type       string   `json:"type"`
	Exec       string   `json:"exec"`
	Args       []string `json:"args"`
	Matcher    string   `json:"matcher,omitempty"`
	TimeoutSec int      `json:"timeoutSec"`
}

// DefaultHookPath returns $COPILOT_HOME/hooks/gh-copilot-attendant.json,
// falling back to ~/.copilot/hooks/gh-copilot-attendant.json.
func DefaultHookPath() (string, error) {
	base := os.Getenv("COPILOT_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".copilot")
	}
	return filepath.Join(base, "hooks", HookFileName), nil
}

// BuildHookConfig returns the Copilot CLI permissionRequest hook config.
func BuildHookConfig(configPath string, timeoutSec int) ([]byte, error) {
	return buildHookConfig(configPath, timeoutSec, ModePermissionRequest)
}

func buildHookConfig(configPath string, timeoutSec int, mode string) ([]byte, error) {
	if timeoutSec <= 0 {
		return nil, errors.New("timeout must be positive")
	}
	var configArgs []string
	if configPath != "" {
		abs, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		configArgs = []string{"--config", abs}
	}
	entry := func(command, matcher string, flags ...string) hookEntry {
		args := append([]string{"copilot-attendant", "hook", command}, flags...)
		args = append(args, configArgs...)
		return hookEntry{Type: "command", Exec: "gh", Args: args, Matcher: matcher, TimeoutSec: timeoutSec}
	}
	hooks := map[string][]hookEntry{
		"permissionRequest": {entry("permission-request", "")},
	}
	switch mode {
	case ModePermissionRequest:
	case ModeScopedPreToolUse:
		hooks = map[string][]hookEntry{
			"preToolUse": {entry("pre-tool-use", "^(view|rg|grep|glob|web_fetch|list_repositories)$")},
		}
	case ModeAllPreToolUse:
		hooks = map[string][]hookEntry{
			"preToolUse": {entry("pre-tool-use", "", "--all-rules")},
		}
	default:
		return nil, fmt.Errorf("unsupported hook mode %q", mode)
	}
	f := hookFile{Version: 1, Hooks: hooks}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// InstallHook writes the hook config file and returns its path.
func InstallHook(opts InstallOptions) (string, error) {
	dest := opts.Path
	if dest == "" {
		p, err := DefaultHookPath()
		if err != nil {
			return "", err
		}
		dest = p
	}
	configPath, err := ResolveConfigPath(opts.ConfigPath)
	if err != nil {
		return "", err
	}
	cfg, found, err := LoadConfig(configPath)
	if err != nil {
		return "", err
	}
	mode := ModePermissionRequest
	if opts.ScopedTools {
		mode = ModeScopedPreToolUse
		if found {
			if err := validatePreToolConfig(cfg); err != nil {
				return "", err
			}
		}
	} else if !opts.ScopedToolsSet && found && needsPreToolUse(cfg) {
		mode = ModeAllPreToolUse
	}
	data, err := buildHookConfig(opts.ConfigPath, opts.TimeoutSec, mode)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if opts.Force {
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return "", err
		}
	} else {
		f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%s already exists (use --force to overwrite)", dest)
		}
		if err != nil {
			return "", err
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return dest, nil
}

func needsPreToolUse(cfg *Config) bool {
	rules := cfg.Hooks.PermissionRequest
	if rules.Allow.GHRead || rules.Allow.GitRead || rules.Allow.ShellDisplay || len(rules.Allow.ScopedTools) > 0 {
		return true
	}
	for _, tool := range []string{"view", "rg", "grep", "glob", "web_fetch", "list_repositories"} {
		if _, ok := matchToolRules(rules.Allow.Tools, tool); ok {
			return true
		}
		if _, ok := matchToolRules(rules.Deny.Tools, tool); ok {
			return true
		}
	}
	return false
}

// UninstallHook removes the hook config file at path (default DefaultHookPath()) and
// returns its path. removed is false when the file does not exist. A file not
// generated by InstallHook is kept unless force is true.
func UninstallHook(path string, force bool) (dest string, removed bool, err error) {
	dest = path
	if dest == "" {
		if dest, err = DefaultHookPath(); err != nil {
			return "", false, err
		}
	}
	data, err := os.ReadFile(dest)
	if errors.Is(err, os.ErrNotExist) {
		return dest, false, nil
	}
	if err != nil {
		return dest, false, err
	}
	if !force {
		h, err := parseManagedHook(data)
		if err != nil || h == nil {
			return dest, false, fmt.Errorf("%s was not generated by hook install (use --force to remove it)", dest)
		}
	}
	if err := os.Remove(dest); err != nil {
		return dest, false, err
	}
	return dest, true, nil
}

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
}

type hookFile struct {
	Version int                    `json:"version"`
	Hooks   map[string][]hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type       string   `json:"type"`
	Exec       string   `json:"exec"`
	Args       []string `json:"args"`
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

// BuildHookConfig returns the Copilot CLI hook config JSON that runs
// "gh copilot-attendant hook permission-request".
func BuildHookConfig(configPath string, timeoutSec int) ([]byte, error) {
	if timeoutSec <= 0 {
		return nil, errors.New("timeout must be positive")
	}
	args := []string{"copilot-attendant", "hook", "permission-request"}
	if configPath != "" {
		abs, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		args = append(args, "--config", abs)
	}
	f := hookFile{
		Version: 1,
		Hooks: map[string][]hookEntry{
			"permissionRequest": {{Type: "command", Exec: "gh", Args: args, TimeoutSec: timeoutSec}},
		},
	}
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
	data, err := BuildHookConfig(opts.ConfigPath, opts.TimeoutSec)
	if err != nil {
		return "", err
	}
	if !opts.Force {
		if _, err := os.Stat(dest); err == nil {
			return "", fmt.Errorf("%s already exists (use --force to overwrite)", dest)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

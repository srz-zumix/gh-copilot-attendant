// Package permissionhook implements GitHub Copilot CLI hooks that approve or
// deny tool calls based on rules loaded from a config file.
package permissionhook

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigEnvVar is the environment variable that overrides the default config path.
const ConfigEnvVar = "GH_COPILOT_ATTENDANT_CONFIG"

// Config is the root of the gh-copilot-attendant config file.
type Config struct {
	Hooks HooksConfig `yaml:"hooks"`
}

// HooksConfig holds settings for each supported Copilot CLI hook event.
type HooksConfig struct {
	// Disabled makes every hook abstain without removing the rules or the hook registration.
	Disabled          bool                    `yaml:"disabled"`
	Logging           LoggingConfig           `yaml:"logging"`
	PermissionRequest PermissionRequestConfig `yaml:"permissionRequest"`
}

// LoggingConfig controls local hook invocation logging.
type LoggingConfig struct {
	Disabled bool `yaml:"disabled"`
}

// PermissionRequestConfig holds permission and shell environment assignment rules.
type PermissionRequestConfig struct {
	Allow                  AllowRuleSet                `yaml:"allow"`
	Deny                   RuleSet                     `yaml:"deny"`
	EnvironmentAssignments EnvironmentAssignmentConfig `yaml:"environmentAssignments"`
}

// AllowRuleSet includes command rules, read presets, and scoped tool rules.
type AllowRuleSet struct {
	RuleSet      `yaml:",inline"`
	GHRead       bool             `yaml:"ghRead"`
	GitRead      bool             `yaml:"gitRead"`
	ShellDisplay bool             `yaml:"shellDisplay"`
	ScopedTools  []ScopedToolRule `yaml:"scopedTools"`
}

// RuleSet is a group of shell command rules and tool name rules.
type RuleSet struct {
	// Commands are shell command rules such as "git status" or "go test:*".
	Commands []string `yaml:"commands"`
	// Tools are glob patterns matched against the hook toolName.
	Tools []string `yaml:"tools"`
}

// ScopedToolRule approves the named tools only for a configured path or URL scope.
type ScopedToolRule struct {
	Tools                  []string `yaml:"tools"`
	Paths                  []string `yaml:"paths"`
	URLs                   []string `yaml:"urls"`
	CurrentRepository      bool     `yaml:"currentRepository"`
	AllowDespiteNativeDeny bool     `yaml:"allowDespiteNativeDeny"`
}

// ResolveConfigPath returns the config file path to use. An explicit path wins,
// then the GH_COPILOT_ATTENDANT_CONFIG environment variable, then the XDG default.
func ResolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if p := os.Getenv(ConfigEnvVar); p != "" {
		return p, nil
	}
	return DefaultConfigPath()
}

// DefaultConfigPath returns $XDG_CONFIG_HOME/gh-copilot-attendant/config.yaml,
// falling back to ~/.config/gh-copilot-attendant/config.yaml.
func DefaultConfigPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "gh-copilot-attendant", "config.yaml"), nil
}

// LoadConfig reads and validates the config file. found is false when the file does not exist.
func LoadConfig(p string) (cfg *Config, found bool, err error) {
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, false, nil
		}
		return nil, false, err
	}
	cfg, err = ParseConfig(data)
	if err != nil {
		return nil, true, fmt.Errorf("invalid config %s: %w", p, err)
	}
	return cfg, true, nil
}

// ParseConfig decodes YAML config data, rejecting unknown fields and invalid rules.
func ParseConfig(data []byte) (*Config, error) {
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if cfg.Hooks.Disabled {
		return cfg, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks that every rule in the config is well formed.
func (c *Config) Validate() error {
	pr := c.Hooks.PermissionRequest
	sets := []struct {
		name  string
		rules RuleSet
	}{{"allow", pr.Allow.RuleSet}, {"deny", pr.Deny}}
	for _, s := range sets {
		for _, r := range s.rules.Commands {
			if _, err := ParseCommandRule(r); err != nil {
				return fmt.Errorf("hooks.permissionRequest.%s.commands: %w", s.name, err)
			}
		}
		for _, t := range s.rules.Tools {
			if strings.TrimSpace(t) == "" {
				return fmt.Errorf("hooks.permissionRequest.%s.tools: empty pattern", s.name)
			}
			if _, err := path.Match(t, ""); err != nil {
				return fmt.Errorf("hooks.permissionRequest.%s.tools: invalid pattern %q: %w", s.name, t, err)
			}
		}
	}
	for i, rule := range pr.Allow.ScopedTools {
		if err := validateScopedToolRule(rule); err != nil {
			return fmt.Errorf("hooks.permissionRequest.allow.scopedTools[%d]: %w", i, err)
		}
		for _, tool := range rule.Tools {
			if pattern, ok := matchToolRules(pr.Allow.Tools, tool); ok {
				return fmt.Errorf("hooks.permissionRequest.allow.scopedTools[%d]: tool %q also matches unscoped allow rule %q", i, tool, pattern)
			}
		}
	}
	if err := pr.EnvironmentAssignments.validate(); err != nil {
		return fmt.Errorf("hooks.permissionRequest.environmentAssignments: %w", err)
	}
	return nil
}

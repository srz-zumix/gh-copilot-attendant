package hook

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/permissionhook"
)

// NewPreToolUseCmd creates the "pre-tool-use" command.
func NewPreToolUseCmd() *cobra.Command {
	var configPath string
	var allRules bool

	cmd := &cobra.Command{
		Use:   "pre-tool-use",
		Short: "Decide scoped Copilot CLI tool calls",
		Long: `Read a Copilot CLI preToolUse hook payload from stdin and approve scoped
view, rg, grep, or glob calls that match path rules, web_fetch calls that match
exact URLs, and configured list_repositories calls. Tool deny rules take
precedence over allow rules in this config. Hook approvals can override native
CLI denials. Scoped URL rules require allowDespiteNativeDeny: true. Unmatched
calls fall through to normal permission handling.
With --all-rules, also evaluate command and unscoped tool rules.
Configuration and input errors produce a progress warning without a decision.

The config file is read from --config, then $GH_COPILOT_ATTENDANT_CONFIG, then
$XDG_CONFIG_HOME/gh-copilot-attendant/config.yaml (default
~/.config/gh-copilot-attendant/config.yaml). A missing config produces no decision.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			run := permissionhook.RunPreToolUse
			if allRules {
				run = permissionhook.RunAllRulesPreToolUse
			}
			if err := run(cmd.InOrStdin(), cmd.OutOrStdout(), configPath); err != nil {
				return fmt.Errorf("failed to evaluate pre-tool-use request: %w", err)
			}
			return nil
		},
	}
	cmd.SilenceUsage = true
	cmd.Flags().StringVar(&configPath, "config", "", "Path to the config file")
	cmd.Flags().BoolVar(&allRules, "all-rules", false, "Evaluate all configured permission rules")
	return cmd
}

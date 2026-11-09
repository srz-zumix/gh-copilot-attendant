package hook

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/permissionhook"
)

// NewPermissionRequestCmd creates the "permission-request" command.
func NewPermissionRequestCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "permission-request",
		Short: "Decide a Copilot CLI permissionRequest hook from config rules",
		Long: `Read a GitHub Copilot CLI permissionRequest hook payload from stdin and print
{"behavior":"allow"} or {"behavior":"deny","message":"..."} according to the
hooks.permissionRequest rules in the config file. Nothing is printed when no rule
decides the request, so Copilot CLI continues with its normal permission flow.

The config file is read from --config, then $GH_COPILOT_ATTENDANT_CONFIG, then
$XDG_CONFIG_HOME/gh-copilot-attendant/config.yaml (default
~/.config/gh-copilot-attendant/config.yaml). A missing config file produces no decision.

Deny rules win over allow rules. A bash command is allowed only when every simple
command separated by ";", "&", "&&", "||", "|", or newlines matches an allow command rule.
Commands with expansions, substitutions, subshells, comments, input redirection, or
output redirection other than /dev/null or a file descriptor are never allowed.
PowerShell commands are never allowed, and tool rules never allow shell tools.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := permissionhook.Run(cmd.InOrStdin(), cmd.OutOrStdout(), configPath); err != nil {
				return fmt.Errorf("failed to evaluate permission request: %w", err)
			}
			return nil
		},
	}
	cmd.SilenceUsage = true
	f := cmd.Flags()
	f.StringVar(&configPath, "config", "", "Path to the config file")
	return cmd
}

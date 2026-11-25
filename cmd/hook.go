package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/hook"
)

// NewHookCmd creates the "hook" command and its subcommands.
func NewHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Run, install, and manage GitHub Copilot CLI hooks",
	}
	cmd.AddCommand(hook.NewPermissionRequestCmd())
	cmd.AddCommand(hook.NewPreToolUseCmd())
	cmd.AddCommand(hook.NewInstallCmd())
	cmd.AddCommand(hook.NewUninstallCmd())
	cmd.AddCommand(hook.NewStatusCmd())
	cmd.AddCommand(hook.NewDisableCmd())
	cmd.AddCommand(hook.NewEnableCmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewHookCmd())
}

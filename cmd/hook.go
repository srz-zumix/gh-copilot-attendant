package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/hook"
)

// NewHookCmd creates the "hook" command and its subcommands.
func NewHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Run and install GitHub Copilot CLI hooks",
	}
	cmd.AddCommand(hook.NewPermissionRequestCmd())
	cmd.AddCommand(hook.NewInstallCmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewHookCmd())
}

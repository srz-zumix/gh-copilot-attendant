package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/vscode"
)

// NewVSCodeCmd creates the "vscode" command and its subcommands.
func NewVSCodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vscode",
		Short: "Inspect local VS Code GitHub Copilot Chat session history",
	}
	cmd.AddCommand(vscode.NewStatsCmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewVSCodeCmd())
}

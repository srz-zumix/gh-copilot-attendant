package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/session"
)

// NewSessionCmd creates the "session" command and its subcommands.
func NewSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Inspect local GitHub Copilot CLI session history",
	}
	cmd.AddCommand(session.NewStatsCmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewSessionCmd())
}

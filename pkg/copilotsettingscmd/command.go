package copilotsettingscmd

import "github.com/spf13/cobra"

// NewGroupCmd creates a command group that shows help and rejects unknown arguments.
func NewGroupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/metrics"
)

// NewMetricsCmd creates the "metrics" command and its subcommands.
func NewMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Summarize GitHub Copilot usage metrics of an organization or enterprise",
	}
	cmd.AddCommand(metrics.NewReportCmd())
	return cmd
}

func init() {
	rootCmd.AddCommand(NewMetricsCmd())
}

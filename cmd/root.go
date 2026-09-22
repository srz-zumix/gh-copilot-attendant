package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/version"
)

var rootCmd = &cobra.Command{
	Use:     "gh-copilot-attendant",
	Short:   "GitHub CLI extension with basic Copilot attendant commands",
	Version: version.Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

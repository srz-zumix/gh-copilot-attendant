package cmd

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/version"
	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

// NewCopilotCmd creates the "copilot" command and its subcommands.
func NewCopilotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copilot",
		Short: "Manage GitHub Copilot CLI integrations bundled with this extension",
	}
	cmd.AddCommand(copilotext.NewExtensionCmd(copilotext.Config{
		ToolName:    "gh-copilot-attendant",
		ToolVersion: version.Version,
		Extensions: []copilotext.Extension{
			{
				Name: "attendant-dashboard",
				URL:  "https://github.com/srz-zumix/gh-copilot-attendant/tree/main/.github/extensions/attendant-dashboard",
			},
		},
	}))
	return cmd
}

func init() {
	rootCmd.AddCommand(NewCopilotCmd())
}

package hook

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/permissionhook"
)

// NewInstallCmd creates the "install" command.
func NewInstallCmd() *cobra.Command {
	var opts permissionhook.InstallOptions

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Register the permissionRequest hook with GitHub Copilot CLI",
		Long: `Write a user-level Copilot CLI hook config that runs
"gh copilot-attendant hook permission-request" for every permissionRequest event.

The file is written to $COPILOT_HOME/hooks/gh-copilot-attendant.json (default
~/.copilot/hooks/gh-copilot-attendant.json) unless --path is given. Use --config to pin
the config file passed to the hook, --timeout to change the hook timeout, and --force to
overwrite an existing file. Copilot CLI loads hooks when a session starts.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := permissionhook.InstallHook(opts)
			if err != nil {
				return fmt.Errorf("failed to install permissionRequest hook: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed permissionRequest hook to %s\n", path)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.ConfigPath, "config", "", "Config file path to pass to the hook")
	f.BoolVar(&opts.Force, "force", false, "Overwrite an existing hook config file")
	f.StringVar(&opts.Path, "path", "", "Destination hook config file path")
	f.IntVar(&opts.TimeoutSec, "timeout", permissionhook.DefaultHookTimeoutSec, "Hook timeout in seconds")
	return cmd
}

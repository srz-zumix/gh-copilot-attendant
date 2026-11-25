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
		Short: "Register Copilot CLI permission hooks",
		Long: `Write a user-level Copilot CLI hook config. By default, select
preToolUse for the gh, Git read, or shell display presets, scoped rules, and supported read-tool rules,
or permissionRequest otherwise. Automatic preToolUse evaluates all configured rules. With
--scoped-tools, register the strict scoped preToolUse mode instead.

The file is written to $COPILOT_HOME/hooks/gh-copilot-attendant.json (default
~/.copilot/hooks/gh-copilot-attendant.json) unless --path is given. Use --config to pin
the config file passed to the hook, --timeout to change the hook timeout, and --force to
overwrite an existing file. Copilot CLI loads hooks when a session starts.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ScopedToolsSet = cmd.Flags().Changed("scoped-tools")
			path, err := permissionhook.InstallHook(opts)
			if err != nil {
				return fmt.Errorf("failed to install permission hooks: %w", err)
			}
			status, err := permissionhook.InspectHook(path, opts.ConfigPath)
			if err != nil {
				return fmt.Errorf("hook installed to %s but failed to inspect it: %w", path, err)
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed %s hook to %s\n", status.Mode, path); err != nil {
				return err
			}
			if status.Mode != permissionhook.ModePermissionRequest {
				_, err = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: other permissionRequest hooks can prevent scoped approvals")
			}
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.ConfigPath, "config", "", "Config file path to pass to the hook")
	f.BoolVar(&opts.Force, "force", false, "Overwrite an existing hook config file")
	f.StringVar(&opts.Path, "path", "", "Destination hook config file path")
	f.BoolVar(&opts.ScopedTools, "scoped-tools", false, "Explicitly select strict scoped preToolUse (false selects permissionRequest)")
	f.IntVar(&opts.TimeoutSec, "timeout", permissionhook.DefaultHookTimeoutSec, "Hook timeout in seconds")
	return cmd
}

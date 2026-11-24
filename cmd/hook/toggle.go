package hook

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/permissionhook"
)

// NewDisableCmd creates the "disable" command.
func NewDisableCmd() *cobra.Command {
	return newToggleCmd(true)
}

// NewEnableCmd creates the "enable" command.
func NewEnableCmd() *cobra.Command {
	return newToggleCmd(false)
}

func newToggleCmd(disable bool) *cobra.Command {
	var configPath, hookPath string

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Re-enable all hook rules in the config",
		Long: `Remove hooks.disabled from the config file so the installed hook applies its
rules again. Other rules and comments are kept. The command fails without changing
the file when the resulting config is invalid. A missing config is left unchanged.

The config file is selected by --config, then the config pinned by the hook
installed at --path (default ~/.copilot/hooks/gh-copilot-attendant.json), then
$GH_COPILOT_ATTENDANT_CONFIG, then $XDG_CONFIG_HOME/gh-copilot-attendant/config.yaml
(default ~/.config/gh-copilot-attendant/config.yaml). The change applies to the
next hook call.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := permissionhook.ResolveHookConfigPath(hookPath, configPath)
			if err != nil {
				return fmt.Errorf("failed to resolve config path: %w", err)
			}
			res, err := permissionhook.SetDisabled(p, disable)
			if err != nil {
				return fmt.Errorf("failed to update config: %w", err)
			}
			state := "enabled"
			if disable {
				state = "disabled"
			}
			msg := fmt.Sprintf("Hook rules %s in %s\n", state, res.Path)
			if !res.Changed {
				if !disable {
					if _, statErr := os.Stat(res.Path); errors.Is(statErr, os.ErrNotExist) {
						_, err = fmt.Fprintf(cmd.OutOrStdout(), "No config found at %s; nothing to enable\n", res.Path)
						return err
					}
				}
				msg = fmt.Sprintf("Hook rules already %s in %s\n", state, res.Path)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), msg)
			return err
		},
	}
	if disable {
		cmd.Use = "disable"
		cmd.Short = "Disable all hook rules without uninstalling the hook"
		cmd.Long = `Set hooks.disabled: true in the config file. The installed hook then makes no
decision for any request, so Copilot CLI uses its normal permission flow. Other
rules and comments are kept, and "hook enable" restores them. A missing config is
created with only this flag.

The config file is selected by --config, then the config pinned by the hook
installed at --path (default ~/.copilot/hooks/gh-copilot-attendant.json), then
$GH_COPILOT_ATTENDANT_CONFIG, then $XDG_CONFIG_HOME/gh-copilot-attendant/config.yaml
(default ~/.config/gh-copilot-attendant/config.yaml). The change applies to the
next hook call.`
	}
	f := cmd.Flags()
	f.StringVar(&configPath, "config", "", "Path to the config file")
	f.StringVar(&hookPath, "path", "", "Hook config file used to find a pinned config")
	return cmd
}

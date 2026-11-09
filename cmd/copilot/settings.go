package copilot

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/copilot/settings/policy"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/copilot/settings/runner"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// NewSettingsCmd creates the "settings" command.
func NewSettingsCmd() *cobra.Command {
	cmd := copilotsettingscmd.NewGroupCmd("settings", "Manage Copilot code review policies and runner settings")
	service := copilotsettings.NewService(gh.NewGitHubClientWithRepo)
	cmd.AddCommand(policy.NewPolicyCmd(service), runner.NewRunnerCmd(service))
	return cmd
}

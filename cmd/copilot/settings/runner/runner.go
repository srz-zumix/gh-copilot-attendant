package runner

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
)

// NewRunnerCmd creates the "runner" command.
func NewRunnerCmd(service *copilotsettings.Service) *cobra.Command {
	cmd := copilotsettingscmd.NewGroupCmd("runner", "Manage the organization Copilot code review runner")
	cmd.AddCommand(NewViewCmd(service), NewSetCmd(service))
	return cmd
}

package policy

import (
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
)

// NewPolicyCmd creates the "policy" command.
func NewPolicyCmd(service *copilotsettings.Service) *cobra.Command {
	cmd := copilotsettingscmd.NewGroupCmd("policy", "Manage the Copilot code review policy")
	cmd.AddCommand(NewViewCmd(service), NewSetCmd(service))
	return cmd
}

package policy

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// NewSetCmd creates the "policy set" command.
func NewSetCmd(service *copilotsettings.Service) *cobra.Command {
	var target copilotsettings.TargetInput
	var state string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set an organization or enterprise code review policy",
		Long: `Set the Copilot code review policy of an organization or enterprise.
Use --org or --repo to select an organization, or --enterprise to select an enterprise.
--repo selects its owner; without a target flag, the current repository owner is used.
Organization states are enabled and disabled. Enterprise states are
enabled_for_all_organizations, disabled_for_all_organizations, and
configured_by_organization_admins. Enterprise updates propagate asynchronously.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := service.SetPolicy(cmd.Context(), copilotsettingscmd.TargetInput(cmd, target), state)
			if err != nil {
				return fmt.Errorf("set Copilot code review policy: %w", err)
			}
			message := "Updated Copilot code review policy for %s\n"
			if resolved.Enterprise {
				message = "Accepted Copilot code review policy update for %s; propagation is asynchronous\n"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), message, resolved); err != nil {
				return fmt.Errorf("policy update for %s succeeded but writing confirmation failed: %w", resolved, err)
			}
			return nil
		},
	}
	copilotsettingscmd.AddTargetFlags(cmd, &target, true)
	cmd.Flags().StringVarP(&state, "state", "s", "", "policy state (required; choices depend on the target)")
	if err := cmd.MarkFlagRequired("state"); err != nil {
		panic(fmt.Errorf("mark --state as required: %w", err))
	}
	if err := cmd.RegisterFlagCompletionFunc("state", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		states := gh.CopilotCodeReviewOrganizationPolicyStates
		if cmd.Flags().Changed("enterprise") {
			states = gh.CopilotCodeReviewEnterprisePolicyStates
		}
		return cobra.FixedCompletions(states, cobra.ShellCompDirectiveNoFileComp)(cmd, args, toComplete)
	}); err != nil {
		panic(fmt.Errorf("register --state completion: %w", err))
	}
	return cmd
}

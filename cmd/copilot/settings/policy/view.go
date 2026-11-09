package policy

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/render"
)

// NewViewCmd creates the "policy view" command.
func NewViewCmd(service *copilotsettings.Service) *cobra.Command {
	var target copilotsettings.TargetInput
	var exporter cmdutil.Exporter

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View an organization or enterprise code review policy",
		Long: `View the configured Copilot code review policy of an organization or enterprise.
Use --org or --repo to select an organization, or --enterprise to select an enterprise.
--repo selects its owner; without a target flag, the current repository owner is used.
Use --format json with --jq or --template to format the result.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := copilotsettingscmd.ValidateOutputFlags(cmd); err != nil {
				return fmt.Errorf("validate policy output: %w", err)
			}
			policy, err := service.ViewPolicy(cmd.Context(), copilotsettingscmd.TargetInput(cmd, target))
			if err != nil {
				return fmt.Errorf("view Copilot code review policy: %w", err)
			}
			r := render.NewOutputRenderer(exporter, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err := render.RenderCopilotCodeReviewPolicy(r, policy); err != nil {
				return fmt.Errorf("render Copilot code review policy: %w", err)
			}
			return nil
		},
	}
	copilotsettingscmd.AddTargetFlags(cmd, &target, true)
	cmdutil.AddFormatFlags(cmd, &exporter)
	return cmd
}

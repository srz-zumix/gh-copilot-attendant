package runner

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/render"
)

// NewViewCmd creates the "runner view" command.
func NewViewCmd(service *copilotsettings.Service) *cobra.Command {
	var target copilotsettings.TargetInput
	var exporter cmdutil.Exporter

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View the organization code review runner configuration",
		Long: `View the runner configuration used by Copilot code review in an organization.
Use --org or --repo to select an organization. --repo selects its owner; without a
target flag, the current repository owner is used. If code review has no runner
configuration of its own, the effective coding agent configuration is shown.
Use --format json with --jq or --template to format the result.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := copilotsettingscmd.ValidateOutputFlags(cmd); err != nil {
				return fmt.Errorf("validate runner output: %w", err)
			}
			runner, err := service.ViewRunner(cmd.Context(), copilotsettingscmd.TargetInput(cmd, target))
			if err != nil {
				return fmt.Errorf("view Copilot code review runner: %w", err)
			}
			r := render.NewOutputRenderer(exporter, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err := render.RenderCopilotCodeReviewRunner(r, runner); err != nil {
				return fmt.Errorf("render Copilot code review runner: %w", err)
			}
			return nil
		},
	}
	copilotsettingscmd.AddTargetFlags(cmd, &target, false)
	cmdutil.AddFormatFlags(cmd, &exporter)
	return cmd
}

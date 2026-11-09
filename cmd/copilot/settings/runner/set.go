package runner

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettingscmd"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// NewSetCmd creates the "runner set" command.
func NewSetCmd(service *copilotsettings.Service) *cobra.Command {
	var target copilotsettings.TargetInput
	var options copilotsettings.RunnerOptions
	var allowCustomization bool
	var exporter cmdutil.Exporter

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the organization code review runner configuration",
		Long: `Set the runner configuration used by Copilot code review in an organization.
Use --org or --repo to select an organization. --repo selects its owner; without a
target flag, the current repository owner is used. --type labeled requires --labels
or --group; --type standard does not accept either. --labels may be repeated or
comma separated. Omitting --allow-repository-customization keeps its current
effective value. A successful update creates a code review-specific configuration.
Use --format json with --jq or --template to format the updated configuration.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := copilotsettingscmd.ValidateOutputFlags(cmd); err != nil {
				return fmt.Errorf("validate runner output: %w", err)
			}
			options.LabelsSet = cmd.Flags().Changed("labels")
			options.GroupSet = cmd.Flags().Changed("group")
			if cmd.Flags().Changed("allow-repository-customization") {
				options.AllowRepositoryCustomization = &allowCustomization
			}
			resolved, runner, err := service.SetRunner(cmd.Context(), copilotsettingscmd.TargetInput(cmd, target), options)
			if err != nil {
				return fmt.Errorf("set Copilot code review runner: %w", err)
			}
			r := render.NewOutputRenderer(exporter, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err := render.RenderCopilotCodeReviewRunner(r, runner); err != nil {
				return fmt.Errorf("runner update for %s succeeded but rendering its result failed: %w", resolved, err)
			}
			return nil
		},
	}
	copilotsettingscmd.AddTargetFlags(cmd, &target, false)
	flags := cmd.Flags()
	flags.StringVar(&options.Type, "type", "", "runner type: standard or labeled (required)")
	flags.StringArrayVar(&options.LabelValues, "labels", nil, "runner labels; may be repeated or comma separated")
	flags.StringVar(&options.Group, "group", "", "runner group name")
	flags.BoolVar(&allowCustomization, "allow-repository-customization", false, "allow repositories to use their own runner configuration (omitted: keep the current value)")
	if err := cmd.MarkFlagRequired("type"); err != nil {
		panic(fmt.Errorf("mark --type as required: %w", err))
	}
	if err := cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(gh.CopilotCodeReviewRunnerTypes, cobra.ShellCompDirectiveNoFileComp)); err != nil {
		panic(fmt.Errorf("register --type completion: %w", err))
	}
	cmdutil.AddFormatFlags(cmd, &exporter)
	return cmd
}

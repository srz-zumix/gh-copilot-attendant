package copilotsettingscmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
)

// AddTargetFlags registers the target flags for a settings command.
func AddTargetFlags(cmd *cobra.Command, input *copilotsettings.TargetInput, allowEnterprise bool) {
	flags := cmd.Flags()
	flags.StringVar(&input.Organization, "org", "", "organization as [HOST/]ORG (defaults to the current repository owner)")
	flags.StringVarP(&input.Repository, "repo", "R", "", "use the owner of [HOST/]OWNER/REPO as the organization")
	if allowEnterprise {
		flags.StringVar(&input.Enterprise, "enterprise", "", "enterprise as [HOST/]ENTERPRISE")
		cmd.MarkFlagsMutuallyExclusive("org", "repo", "enterprise")
	} else {
		cmd.MarkFlagsMutuallyExclusive("org", "repo")
	}
}

// TargetInput records whether optional target flags were supplied.
func TargetInput(cmd *cobra.Command, input copilotsettings.TargetInput) copilotsettings.TargetInput {
	input.OrganizationSet = cmd.Flags().Changed("org")
	input.RepositorySet = cmd.Flags().Changed("repo")
	input.EnterpriseSet = cmd.Flags().Changed("enterprise")
	return input
}

// ValidateOutputFlags validates the expression selected by cmdutil's exporter.
func ValidateOutputFlags(cmd *cobra.Command) error {
	jq, err := cmd.Flags().GetString("jq")
	if err != nil {
		return fmt.Errorf("read --jq: %w", err)
	}
	template, err := cmd.Flags().GetString("template")
	if err != nil {
		return fmt.Errorf("read --template: %w", err)
	}
	return copilotsettings.ValidateOutputExpression(jq, template)
}

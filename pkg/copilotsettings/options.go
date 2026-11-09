package copilotsettings

import (
	"encoding/csv"
	"fmt"
	"slices"
	"strings"

	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// ValidatePolicyState checks the states supported by the selected target.
func ValidatePolicyState(enterprise bool, state string) error {
	states := gh.CopilotCodeReviewOrganizationPolicyStates
	scope := "organization"
	if enterprise {
		states = gh.CopilotCodeReviewEnterprisePolicyStates
		scope = "enterprise"
	}
	if !slices.Contains(states, state) {
		return fmt.Errorf("invalid --state %q for %s (valid: %s)", state, scope, strings.Join(states, ", "))
	}
	return nil
}

// RunnerOptions preserves the supplied runner flags before constructing the API body.
type RunnerOptions struct {
	Type                         string
	LabelValues                  []string
	LabelsSet                    bool
	Group                        string
	GroupSet                     bool
	AllowRepositoryCustomization *bool
}

// Update validates runner options and constructs the request body.
func (o RunnerOptions) Update() (*gh.OrganizationCopilotCodeReviewRunnerUpdate, error) {
	if !slices.Contains(gh.CopilotCodeReviewRunnerTypes, o.Type) {
		return nil, fmt.Errorf("invalid --type %q (valid: %s)", o.Type, strings.Join(gh.CopilotCodeReviewRunnerTypes, ", "))
	}
	labelsSet := o.LabelsSet || len(o.LabelValues) > 0
	groupSet := o.GroupSet || o.Group != ""
	if o.Type == "standard" && (labelsSet || groupSet) {
		return nil, fmt.Errorf("--labels and --group cannot be used with --type standard")
	}

	update := &gh.OrganizationCopilotCodeReviewRunnerUpdate{
		RunnerType:                   o.Type,
		AllowRepositoryCustomization: o.AllowRepositoryCustomization,
	}
	if o.Type == "standard" {
		return update, nil
	}
	if !labelsSet && !groupSet {
		return nil, fmt.Errorf("--type labeled requires at least one label or a group")
	}
	if labelsSet {
		if len(o.LabelValues) == 0 {
			return nil, fmt.Errorf("--labels must not be empty")
		}
		for _, value := range o.LabelValues {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("--labels must not contain an empty label")
			}
			if strings.ContainsAny(value, "\r\n") {
				return nil, fmt.Errorf("--labels must not contain line breaks")
			}
			reader := csv.NewReader(strings.NewReader(value))
			reader.FieldsPerRecord = -1
			records, err := reader.ReadAll()
			if err != nil {
				return nil, fmt.Errorf("parse --labels %q: %w", value, err)
			}
			if len(records) != 1 || len(records[0]) == 0 {
				return nil, fmt.Errorf("--labels must contain a single comma-separated list")
			}
			for _, label := range records[0] {
				if strings.TrimSpace(label) == "" {
					return nil, fmt.Errorf("--labels must not contain an empty label")
				}
				update.RunnerLabels = append(update.RunnerLabels, label)
			}
		}
	}
	if groupSet {
		if strings.TrimSpace(o.Group) == "" {
			return nil, fmt.Errorf("--group must not be empty")
		}
		group := o.Group
		update.RunnerGroupName = &group
	}
	return update, nil
}

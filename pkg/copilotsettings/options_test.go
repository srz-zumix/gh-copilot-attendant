package copilotsettings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePolicyState(t *testing.T) {
	for _, state := range []string{"enabled", "disabled"} {
		require.NoError(t, ValidatePolicyState(false, state))
		require.Error(t, ValidatePolicyState(true, state))
	}
	for _, state := range []string{
		"enabled_for_all_organizations",
		"disabled_for_all_organizations",
		"configured_by_organization_admins",
	} {
		require.NoError(t, ValidatePolicyState(true, state))
		require.Error(t, ValidatePolicyState(false, state))
	}
	require.ErrorContains(t, ValidatePolicyState(false, ""), "invalid --state")
	require.ErrorContains(t, ValidatePolicyState(true, "ENABLED"), "invalid --state")
}

func TestRunnerOptionsUpdate(t *testing.T) {
	tests := []struct {
		name    string
		options RunnerOptions
		labels  []string
		group   string
		err     string
	}{
		{"missing type", RunnerOptions{}, nil, "", "invalid --type"},
		{"unknown type", RunnerOptions{Type: "other"}, nil, "", "invalid --type"},
		{"standard", RunnerOptions{Type: "standard"}, nil, "", ""},
		{"standard with empty labels", RunnerOptions{Type: "standard", LabelsSet: true}, nil, "", "cannot be used"},
		{"standard with group", RunnerOptions{Type: "standard", Group: "runner-pool"}, nil, "", "cannot be used"},
		{"standard with empty group", RunnerOptions{Type: "standard", GroupSet: true}, nil, "", "cannot be used"},
		{"labeled without selectors", RunnerOptions{Type: "labeled"}, nil, "", "requires at least one"},
		{"labeled with empty labels", RunnerOptions{Type: "labeled", LabelsSet: true}, nil, "", "must not be empty"},
		{"labeled with blank group", RunnerOptions{Type: "labeled", Group: "  ", GroupSet: true}, nil, "", "--group must not be empty"},
		{"empty repeated value", RunnerOptions{Type: "labeled", LabelValues: []string{"", "runner-amber"}, LabelsSet: true}, nil, "", "empty label"},
		{"empty comma element", RunnerOptions{Type: "labeled", LabelValues: []string{"runner-amber,,runner-cobalt"}}, nil, "", "empty label"},
		{"blank label with group", RunnerOptions{Type: "labeled", LabelValues: []string{" "}, Group: "runner-pool"}, nil, "", "empty label"},
		{"empty group with label", RunnerOptions{Type: "labeled", LabelValues: []string{"runner-amber"}, GroupSet: true}, nil, "", "--group must not be empty"},
		{"malformed labels", RunnerOptions{Type: "labeled", LabelValues: []string{`"unfinished`}}, nil, "", "parse --labels"},
		{"labels with line break", RunnerOptions{Type: "labeled", LabelValues: []string{"runner-amber\nrunner-cobalt"}}, nil, "", "line breaks"},
		{"group only", RunnerOptions{Type: "labeled", Group: "runner-pool"}, nil, "runner-pool", ""},
		{"repeated and comma-separated labels", RunnerOptions{Type: "labeled", LabelValues: []string{"runner-amber,runner-cobalt", "runner-copper"}}, []string{"runner-amber", "runner-cobalt", "runner-copper"}, "", ""},
		{"labels and group", RunnerOptions{Type: "labeled", LabelValues: []string{"runner-amber"}, Group: "runner-pool"}, []string{"runner-amber"}, "runner-pool", ""},
		{"quoted comma in label", RunnerOptions{Type: "labeled", LabelValues: []string{`"runner,amber",runner-cobalt`}}, []string{"runner,amber", "runner-cobalt"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := tt.options.Update()
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.options.Type, update.RunnerType)
			assert.Equal(t, tt.labels, update.RunnerLabels)
			if tt.group == "" {
				assert.Nil(t, update.RunnerGroupName)
			} else {
				require.NotNil(t, update.RunnerGroupName)
				assert.Equal(t, tt.group, *update.RunnerGroupName)
			}
		})
	}
}

func TestRunnerCustomizationIsTriState(t *testing.T) {
	no := false
	yes := true
	tests := []struct {
		name    string
		value   *bool
		present bool
	}{
		{"omitted", nil, false},
		{"false", &no, true},
		{"true", &yes, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := (RunnerOptions{Type: "standard", AllowRepositoryCustomization: tt.value}).Update()
			require.NoError(t, err)
			data, err := json.Marshal(update)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(data, &body))
			value, found := body["allow_repository_customization"]
			assert.Equal(t, tt.present, found)
			if tt.present {
				assert.Equal(t, *tt.value, value)
			}
			assert.NotContains(t, body, "runner_labels")
			assert.NotContains(t, body, "runner_group_name")
		})
	}
}

func TestValidateOutputExpression(t *testing.T) {
	tests := []struct {
		name     string
		jq       string
		template string
		err      string
	}{
		{"plain", "", "", ""},
		{"valid jq", `.runner_type`, "", ""},
		{"valid jq env function", `env.FIXTURE_VALUE`, "", ""},
		{"invalid jq syntax", `[.`, "", "invalid --jq"},
		{"invalid jq function", `fixture_unknown_function`, "", "invalid --jq"},
		{"valid template", "", `{{.runner_type | hasPrefix "stand"}}`, ""},
		{"invalid template", "", `{{`, "invalid --template"},
		{"jq takes precedence", `.runner_type`, `{{`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputExpression(tt.jq, tt.template)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

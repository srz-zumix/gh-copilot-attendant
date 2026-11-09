package render

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	sharedrender "github.com/srz-zumix/go-gh-extension/pkg/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderCopilotCodeReviewPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy *copilotsettings.Policy
		fields []string
	}{
		{
			name: "organization",
			policy: &copilotsettings.Policy{Organization: &gh.OrganizationCopilotCodeReviewPolicy{
				ConfiguredState: nil, EffectiveState: "enabled", Editable: false, ManagedBy: "enterprise",
			}},
			fields: []string{"configured_state", "null", "effective_state", "enabled", "editable", "false", "managed_by", "enterprise"},
		},
		{
			name:   "enterprise",
			policy: &copilotsettings.Policy{Enterprise: &gh.EnterpriseCopilotCodeReviewPolicy{PolicyState: nil}},
			fields: []string{"policy_state", "null"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := sharedrender.NewStringRenderer(nil)
			require.NoError(t, RenderCopilotCodeReviewPolicy(&output.Renderer, tt.policy))
			for _, field := range tt.fields {
				assert.Contains(t, output.Stdout.String(), field)
			}
			assert.NotContains(t, output.Stdout.String(), "disabled")
		})
	}
	require.Error(t, RenderCopilotCodeReviewPolicy(&sharedrender.NewStringRenderer(nil).Renderer, nil))
}

func TestRenderCopilotCodeReviewPolicyJSONPreservesNull(t *testing.T) {
	tests := []struct {
		name   string
		policy *copilotsettings.Policy
		key    string
	}{
		{"organization", &copilotsettings.Policy{Organization: &gh.OrganizationCopilotCodeReviewPolicy{EffectiveState: "enabled"}}, "configured_state"},
		{"enterprise", &copilotsettings.Policy{Enterprise: &gh.EnterpriseCopilotCodeReviewPolicy{}}, "policy_state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := sharedrender.NewStringRenderer(cmdutil.NewJSONExporter())
			require.NoError(t, RenderCopilotCodeReviewPolicy(&output.Renderer, tt.policy))
			var body map[string]any
			require.NoError(t, json.Unmarshal(output.Stdout.Bytes(), &body))
			value, exists := body[tt.key]
			assert.True(t, exists)
			assert.Nil(t, value)
			assert.NotContains(t, body, "organization")
			assert.NotContains(t, body, "enterprise")
		})
	}
}

func TestRenderCopilotCodeReviewRunner(t *testing.T) {
	runner := &gh.OrganizationCopilotCodeReviewRunner{
		RunnerType: "labeled", RunnerLabels: []string{"runner-amber"},
		RunnerGroupName: nil, AllowRepositoryCustomization: false, ConfiguredBy: "coding_agent",
	}
	output := sharedrender.NewStringRenderer(nil)
	require.NoError(t, RenderCopilotCodeReviewRunner(&output.Renderer, runner))
	for _, value := range []string{"runner_type", "labeled", "runner-amber", "runner_group_name", "null", "false", "coding_agent"} {
		assert.Contains(t, output.Stdout.String(), value)
	}

	jsonOutput := sharedrender.NewStringRenderer(cmdutil.NewJSONExporter())
	require.NoError(t, RenderCopilotCodeReviewRunner(&jsonOutput.Renderer, runner))
	var body map[string]any
	require.NoError(t, json.Unmarshal(jsonOutput.Stdout.Bytes(), &body))
	assert.Equal(t, false, body["allow_repository_customization"])
	assert.Nil(t, body["runner_group_name"])
	assert.Equal(t, "coding_agent", body["configured_by"])
	require.ErrorContains(t, RenderCopilotCodeReviewRunner(&output.Renderer, nil), "non-nil Copilot code review runner")
}

type shortOutputWriter struct{}

func (shortOutputWriter) Write([]byte) (int, error) {
	return 0, nil
}

func TestRenderCopilotCodeReviewReportsShortWrites(t *testing.T) {
	tests := []struct {
		name   string
		render func(*sharedrender.Renderer) error
	}{
		{
			name: "policy",
			render: func(r *sharedrender.Renderer) error {
				return RenderCopilotCodeReviewPolicy(r, &copilotsettings.Policy{
					Organization: &gh.OrganizationCopilotCodeReviewPolicy{EffectiveState: "enabled"},
				})
			},
		},
		{
			name: "runner",
			render: func(r *sharedrender.Renderer) error {
				return RenderCopilotCodeReviewRunner(r, &gh.OrganizationCopilotCodeReviewRunner{RunnerType: "standard"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewOutputRenderer(nil, shortOutputWriter{}, io.Discard)
			original := r.IO.Out
			require.ErrorIs(t, tt.render(r), io.ErrShortWrite)
			assert.Equal(t, original, r.IO.Out)
		})
	}
}

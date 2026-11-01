package render

import (
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderIDEActiveUserEstimate(t *testing.T) {
	tests := []struct {
		name      string
		users     copilotmetrics.Counter
		missing   []string
		value     string
		isPartial bool
	}{
		{name: "complete", users: copilotmetrics.Counter{Value: 3, Valid: true}, value: "3"},
		{name: "zero", users: copilotmetrics.Counter{Valid: true}, value: "0"},
		{name: "partial", users: copilotmetrics.Counter{Value: 2, Valid: true}, missing: []string{"2030-01-02"}, value: "2", isPartial: true},
		{name: "all missing", missing: []string{"2030-01-01", "2030-01-02", "2030-01-03"}, value: "n/a", isPartial: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testCopilotMetricsSummary()
			s.Partial = nil
			s.Warnings = []string{"existing warning"}
			s.Rich = &copilotmetrics.RichMetrics{
				Days:                    []string{"2030-01-03"},
				IDEActiveUserTotalDaily: []copilotmetrics.Counter{{Value: 999, Valid: true}},
				IDEActiveUserMonth: &copilotmetrics.IDEActiveUserEstimate{
					StartDay: "2030-01-01", EndDay: "2030-01-03", Users: tt.users, MissingDays: tt.missing,
				},
			}

			out, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: true})
			require.NoError(t, err)
			_, card, found := strings.Cut(out, `<article class="kpi-card"><p>IDE active users (estimate)</p>`)
			require.True(t, found)
			card, _, found = strings.Cut(card, `</article>`)
			require.True(t, found)
			assert.Contains(t, card, `<strong>`+tt.value+`</strong>`)
			assert.Contains(t, card, "calendar month: 2030-01-01 .. 2030-01-03; reported IDEs only")
			assert.NotContains(t, card, "999", "the card never falls back to the latest daily count")
			if tt.isPartial {
				assert.Contains(t, card, "; partial")
				assert.Contains(t, out, "IDE active user estimate is partial; users reports are missing for "+strings.Join(tt.missing, ", "))
			} else {
				assert.NotContains(t, card, "; partial")
				assert.NotContains(t, out, "IDE active user estimate is partial")
			}
			assert.Equal(t, []string{"existing warning"}, s.Warnings, "rendering does not modify the summary")

			normal, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{})
			require.NoError(t, err)
			assert.NotContains(t, normal, "IDE active users (estimate)")
			assert.NotContains(t, normal, "IDE active user estimate is partial")
		})
	}
}

func TestIDEActiveUserEstimateDoesNotExposeUserLogins(t *testing.T) {
	ds := &copilotmetrics.Dataset{
		Days: []string{"2030-01-01", "2030-01-02"},
		Users: []copilotmetrics.UserDay{
			{UserID: 1, Login: "example-private-login", Day: "2030-01-01", IDEs: []string{"Editor A"}},
			{UserID: 2, Login: "example-other-login", Day: "2030-01-02", IDEs: []string{"Editor B"}},
		},
	}
	s := copilotmetrics.BuildSummary(ds, copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionIDE: true}, IncludeRichMetrics: true,
	})
	out, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, out, `<p>IDE active users (estimate)</p><strong>2</strong>`)
	assert.NotContains(t, out, "example-private-login")
	assert.NotContains(t, out, "example-other-login")
}

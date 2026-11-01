package render

import (
	"strings"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamUsagePiesFromDailyReports(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03"}
	ds := &copilotmetrics.Dataset{
		Days: days,
		OrgDays: []*github.CopilotDailyMetrics{
			{Day: days[0], UserInitiatedInteractionCount: new(10), CodeGenerationActivityCount: new(10)},
			{Day: days[1], UserInitiatedInteractionCount: new(5), CodeGenerationActivityCount: new(24)},
		},
		Users: []copilotmetrics.UserDay{
			{UserID: 1, Login: "fictional-user-a", Day: days[0], UserInitiatedInteractions: new(10), CodeGenerationActivities: new(2)},
			{UserID: 1, Login: "fictional-user-a", Day: days[1], UserInitiatedInteractions: new(5), CodeGenerationActivities: new(20)},
			{UserID: 2, Login: "fictional-user-b", Day: days[0], UserInitiatedInteractions: new(0), CodeGenerationActivities: new(8)},
			{UserID: 2, Login: "fictional-user-b", Day: days[1], CodeGenerationActivities: new(4)},
			{UserID: 3, Login: "fictional-user-c", Day: days[1]},
		},
		UserTeams: []*github.CopilotUserTeamsDailyMetrics{
			{UserID: 1, TeamID: 10, Slug: "team-alpha", Day: days[0]},
			{UserID: 1, TeamID: 20, Slug: "team-beta", Day: days[0]},
			{UserID: 1, TeamID: 20, Slug: "team-beta", Day: days[1]},
			{UserID: 2, TeamID: 10, Slug: "team-alpha", Day: days[0]},
			{UserID: 2, TeamID: 10, Slug: "team-alpha", Day: days[1]},
			{UserID: 3, TeamID: 30, Slug: "team-missing", Day: days[1]},
		},
		Missing: map[copilotmetrics.Family][]string{
			copilotmetrics.FamilyUsers:     {days[2]},
			copilotmetrics.FamilyUserTeams: {days[2]},
		},
	}
	opts := copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionTeam: true},
		Top:      1, IncludeRichMetrics: true,
	}
	summary := copilotmetrics.BuildSummary(ds, opts)
	require.Len(t, summary.Teams, 1)
	require.Len(t, summary.AllTeams, 3)
	assert.Equal(t, "team-beta", summary.Teams[0].Slug)
	html, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, html, `<div class="rich-panels rich-panels-paired">`)
	assert.Contains(t, html, `<figure class="rich-panel rich-panel-half"><figcaption>Team usage (interactions)</figcaption>`)
	assert.Contains(t, html, `<figure class="rich-panel rich-panel-half"><figcaption>Team usage (code-generation activities)</figcaption>`)
	assert.Less(t, strings.Index(html, "<figcaption>Team usage (interactions)</figcaption>"), strings.Index(html, "<figcaption>Team usage (code-generation activities)</figcaption>"))
	assert.Equal(t, 2, strings.Count(html, `<svg class="pie-plot"`))
	assert.Contains(t, html, `data-tooltip="team-alpha: 10 (40.0%)"`)
	assert.Contains(t, html, `data-tooltip="team-beta: 15 (60.0%)"`)
	assert.Contains(t, html, `data-tooltip="team-alpha: 14 (38.9%)"`)
	assert.Contains(t, html, `data-tooltip="team-beta: 22 (61.1%)"`)
	assert.NotContains(t, html, "team-missing", "unavailable counts are not turned into zero-valued slices")
	assert.NotContains(t, html, "fictional-user-", "aggregated team charts do not expose user logins")
	assert.Contains(t, html, "team totals do not add up to the organization total")
	assert.Contains(t, html, "Shares of reported team counts; missing values excluded.")
	assert.Contains(t, html, "users reports are missing for 1 day(s)")
	assert.Contains(t, html, "user-teams reports are missing for 1 day(s)")
	assert.NotContains(t, html, "src=")
	assert.NotContains(t, html, "<link")

	sections := copilotMetricsHTMLSections(summary, true)
	require.Len(t, sections, 1)
	require.Len(t, sections[0].Table.Rows, 1)
	assert.Equal(t, "team-beta", sections[0].Table.Rows[0][0])
	opts.Top = 0
	unlimited := copilotmetrics.BuildSummary(ds, opts)
	assert.Equal(t, sections[0].RichPanels, copilotMetricsHTMLSections(unlimited, true)[0].RichPanels)

	plain, err := RenderCopilotMetricsHTML(summary)
	require.NoError(t, err)
	assert.NotContains(t, plain, "<figcaption>Team usage")
	assert.Contains(t, plain, "By team")
	assert.NotContains(t, RenderCopilotMetricsMarkdown(summary), "Team usage")

	opts.Sections = copilotmetrics.SectionSet{copilotmetrics.SectionUser: true}
	withoutTeam := copilotmetrics.BuildSummary(ds, opts)
	assert.Empty(t, withoutTeam.AllTeams)
	html, err = RenderCopilotMetricsHTMLWithOptions(withoutTeam, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.NotContains(t, html, "<figcaption>Team usage")
	assert.NotContains(t, html, "By team")
}

func TestTeamUsagePieFallbackAndSinglePanel(t *testing.T) {
	summary := &copilotmetrics.Summary{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionTeam: true},
		Teams: []copilotmetrics.TeamRow{{
			TeamID: 1, Slug: "sample-team",
			ActivityTotals: copilotmetrics.ActivityTotals{
				CodeGeneration: copilotmetrics.Counter{Value: 7, Valid: true},
			},
		}},
	}
	html, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.NotContains(t, html, "<figcaption>Team usage (interactions)</figcaption>")
	assert.Contains(t, html, `<figure class="rich-panel"><figcaption>Team usage (code-generation activities)</figcaption>`)
	assert.Contains(t, html, `data-tooltip="sample-team: 7 (100.0%)"`)
	assert.NotContains(t, html, `<figure class="rich-panel rich-panel-half">`)
}

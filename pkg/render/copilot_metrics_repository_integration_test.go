package render

import (
	"strings"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryChartsFromDailyReports(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03"}
	pr := func(reviewed, generated, applied int) *github.CopilotMetricsPullRequests {
		return &github.CopilotMetricsPullRequests{
			TotalCreated: new(1), TotalCreatedByCopilot: new(0),
			TotalReviewed: new(reviewed), TotalReviewedByCopilot: new(reviewed),
			TotalMerged: new(1), TotalMergedCreatedByCopilot: new(0),
			TotalCopilotSuggestions: new(generated), TotalCopilotAppliedSuggestions: new(applied),
		}
	}
	ds := &copilotmetrics.Dataset{
		Days:    days,
		Missing: map[copilotmetrics.Family][]string{copilotmetrics.FamilyRepositories: {days[1]}},
		Repositories: []*github.CopilotRepositoryDailyMetrics{
			{Day: days[0], RepoID: 1, RepoOwnerName: "sample", RepoName: "alpha", PullRequests: pr(1, 10, 12)},
			{Day: days[2], RepoID: 2, RepoOwnerName: "sample", RepoName: "beta", PullRequests: pr(4, 3, 1)},
		},
	}
	summary := copilotmetrics.BuildSummary(ds, copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionRepository: true},
		Top:      1, IncludeRichMetrics: true,
	})
	require.Len(t, summary.Repositories, 1)
	require.Len(t, summary.Rich.Repositories, 2)

	html, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	reviewedTitle := `<figure class="rich-panel rich-panel-half"><figcaption>Pull requests reviewed by repository</figcaption>`
	suggestionsTitle := `<figure class="rich-panel rich-panel-half"><figcaption>Copilot review suggestions by repository</figcaption>`
	reviewedIndex := strings.Index(html, reviewedTitle)
	suggestionsIndex := strings.Index(html, suggestionsTitle)
	createdIndex := strings.Index(html, `<figcaption>Pull requests created by repository</figcaption>`)
	require.GreaterOrEqual(t, reviewedIndex, 0)
	require.Greater(t, suggestionsIndex, reviewedIndex)
	require.Greater(t, createdIndex, suggestionsIndex)
	assert.NotContains(t, html[reviewedIndex+len(reviewedTitle):suggestionsIndex], "<figcaption>")
	assert.Contains(t, html, `<figcaption>Copilot reviews per day by repository</figcaption>`)
	assert.Contains(t, html, "sample/alpha\nGenerated suggestions: 10 (partial)\nApplied suggestions: 12 (partial)")
	assert.Contains(t, html, "2030-01-01\nsample/beta: 0 (0.0%)\nsample/alpha: 1 (100.0%)\nTotal: 1 (100.0%)")
	assert.Contains(t, html, "2030-01-02\nsample/beta: n/a\nsample/alpha: n/a\nDisplayed subtotal: n/a")
	assert.Contains(t, html, `class="repository-incomplete-marker"`)
	assert.Contains(t, html, "repositories reports are missing for 1 day(s)")
	assert.Equal(t, 1, strings.Count(html, `<figcaption>Copilot reviews per day by repository</figcaption>`))

	plain, err := RenderCopilotMetricsHTML(summary)
	require.NoError(t, err)
	assert.NotContains(t, plain, `<figcaption>Copilot review suggestions by repository</figcaption>`)
	assert.NotContains(t, plain, `<figcaption>Copilot reviews per day by repository</figcaption>`)
}

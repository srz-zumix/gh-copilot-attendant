package copilotmetrics

import (
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRichRepositories(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04", "2030-01-05"}
	ds := &Dataset{
		Days:    days,
		Missing: map[Family][]string{FamilyRepositories: {days[3]}},
		Repositories: []*github.CopilotRepositoryDailyMetrics{
			{Day: days[4], RepoID: 1, RepoOwnerName: "sample", RepoName: "renamed", PullRequests: &github.CopilotMetricsPullRequests{
				TotalReviewedByCopilot: new(4), TotalCopilotSuggestions: new(4),
			}},
			{Day: days[1], RepoID: 2, RepoOwnerName: "sample", RepoName: "beta", PullRequests: &github.CopilotMetricsPullRequests{
				TotalReviewedByCopilot: new(1), TotalCopilotAppliedSuggestions: new(0),
			}},
			{Day: days[0], RepoID: 1, RepoOwnerName: "sample", RepoName: "alpha", PullRequests: &github.CopilotMetricsPullRequests{
				TotalReviewedByCopilot: new(2), TotalCopilotSuggestions: new(3), TotalCopilotAppliedSuggestions: new(5),
			}},
			{Day: days[1], RepoID: 1, RepoOwnerName: "sample", RepoName: "alpha", PullRequests: &github.CopilotMetricsPullRequests{
				TotalCopilotAppliedSuggestions: new(2),
			}},
		},
	}
	summary := BuildSummary(ds, SummaryOptions{Sections: sections(SectionRepository), Top: 1, IncludeRichMetrics: true})
	require.Len(t, summary.Repositories, 1)
	require.Len(t, summary.Rich.Repositories, 2, "rich charts use the full repository set")

	alpha := summary.Rich.Repositories[0]
	assert.Equal(t, "sample/alpha", alpha.Name, "one repository ID retains one deterministic name")
	assert.Equal(t, []Counter{
		{Value: 2, Valid: true}, {}, {Valid: true}, {}, {Value: 4, Valid: true},
	}, alpha.Reviews)
	assert.Equal(t, Counter{Value: 7, Valid: true}, alpha.Suggestions)
	assert.False(t, alpha.SuggestionsComplete)
	assert.Equal(t, Counter{Value: 7, Valid: true}, alpha.AppliedSuggestions)
	assert.False(t, alpha.AppliedSuggestionsComplete)

	beta := summary.Rich.Repositories[1]
	assert.Equal(t, "sample/beta", beta.Name)
	assert.Equal(t, Counter{Valid: true}, beta.Suggestions, "available days without a repository record contribute zero")
	assert.False(t, beta.SuggestionsComplete, "an omitted field and an unavailable report are not zero")
	assert.Equal(t, Counter{Valid: true}, beta.AppliedSuggestions)
	assert.False(t, beta.AppliedSuggestionsComplete, "an unavailable report makes the period incomplete")

	withoutRepository := BuildSummary(ds, SummaryOptions{Sections: sections(SectionTrend), IncludeRichMetrics: true})
	assert.Empty(t, withoutRepository.Rich.Repositories)
}

func TestBuildRichRepositoriesCompletenessAndNames(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02"}
	ds := &Dataset{
		Days: days,
		Repositories: []*github.CopilotRepositoryDailyMetrics{
			{Day: days[0], RepoID: 1, RepoName: "Other", PullRequests: &github.CopilotMetricsPullRequests{
				TotalReviewedByCopilot: new(0), TotalCopilotSuggestions: new(10),
			}},
			{Day: days[0], RepoID: 2, RepoName: "same", PullRequests: &github.CopilotMetricsPullRequests{}},
			{Day: days[1], RepoID: 3, RepoName: "same", PullRequests: &github.CopilotMetricsPullRequests{
				TotalReviewedByCopilot: new(1), TotalCopilotSuggestions: new(0), TotalCopilotAppliedSuggestions: new(0),
			}},
		},
	}
	repositories := buildRichRepositories(ds)
	require.Len(t, repositories, 3)
	assert.Equal(t, "Other (repository 1)", repositories[0].Name)
	assert.Equal(t, Counter{Value: 10, Valid: true}, repositories[0].Suggestions)
	assert.True(t, repositories[0].SuggestionsComplete)
	assert.Equal(t, Counter{Valid: true}, repositories[0].AppliedSuggestions)
	assert.False(t, repositories[0].AppliedSuggestionsComplete)
	assert.Equal(t, []Counter{{Valid: true}, {Valid: true}}, repositories[0].Reviews)
	assert.Equal(t, "same (repository 2)", repositories[1].Name)
	assert.Equal(t, "same (repository 3)", repositories[2].Name)
	assert.False(t, repositories[1].SuggestionsComplete)
	assert.True(t, repositories[2].SuggestionsComplete)
	assert.True(t, repositories[2].AppliedSuggestionsComplete)

	ds.Days = days[:1]
	ds.Repositories = ds.Repositories[1:2]
	onlyUnknown := buildRichRepositories(ds)
	require.Len(t, onlyUnknown, 1)
	assert.False(t, onlyUnknown[0].Suggestions.Valid)
	assert.False(t, onlyUnknown[0].AppliedSuggestions.Valid)
	assert.False(t, onlyUnknown[0].Reviews[0].Valid)
	assert.Empty(t, buildRichRepositories(&Dataset{Days: days}))
	assert.Empty(t, buildRichRepositories(&Dataset{Days: days, Missing: map[Family][]string{FamilyRepositories: days}}))
}

package copilotmetrics

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	latest      *gh.CopilotLatestMetrics
	latestUsers *gh.CopilotLatestUserMetrics
	org         map[string][]*github.CopilotDailyMetrics
	users       map[string][]*github.CopilotUserDailyMetrics
	repos       map[string][]*github.CopilotRepositoryDailyMetrics
	teams       map[string][]*github.CopilotUserTeamsDailyMetrics
	errDay      string
	calls       atomic.Int32
}

func (f *fakeFetcher) LatestMetrics(context.Context) (*gh.CopilotLatestMetrics, error) {
	f.calls.Add(1)
	if f.latest == nil {
		return nil, errors.New("no latest report")
	}
	return f.latest, nil
}

func (f *fakeFetcher) LatestUserMetrics(context.Context) (*gh.CopilotLatestUserMetrics, error) {
	f.calls.Add(1)
	if f.latestUsers == nil {
		return nil, errors.New("no latest users report")
	}
	return f.latestUsers, nil
}

func fakeDaily[T any](f *fakeFetcher, m map[string][]T, day string) ([]T, bool, error) {
	f.calls.Add(1)
	if f.errDay == "*" || day == f.errDay {
		return nil, false, errors.New("boom")
	}
	v, ok := m[day]
	return v, ok, nil
}

func (f *fakeFetcher) DailyMetrics(_ context.Context, day string) ([]*github.CopilotDailyMetrics, bool, error) {
	return fakeDaily(f, f.org, day)
}

func (f *fakeFetcher) DailyUserMetrics(_ context.Context, day string) ([]*github.CopilotUserDailyMetrics, bool, error) {
	return fakeDaily(f, f.users, day)
}

func (f *fakeFetcher) DailyRepositoryMetrics(_ context.Context, day string) ([]*github.CopilotRepositoryDailyMetrics, bool, error) {
	return fakeDaily(f, f.repos, day)
}

func (f *fakeFetcher) DailyUserTeamsMetrics(_ context.Context, day string) ([]*github.CopilotUserTeamsDailyMetrics, bool, error) {
	return fakeDaily(f, f.teams, day)
}

func orgDay(day string, dau, interactions int) *github.CopilotDailyMetrics {
	return &github.CopilotDailyMetrics{Day: day, DailyActiveUsers: new(dau), UserInitiatedInteractionCount: new(interactions)}
}

func sections(s ...Section) SectionSet {
	set := SectionSet{}
	for _, sec := range s {
		set[sec] = true
	}
	return set
}

func TestCollectLatest(t *testing.T) {
	f := &fakeFetcher{
		latest: &gh.CopilotLatestMetrics{
			Window: gh.CopilotMetricsWindow{StartDay: "2030-01-01", EndDay: "2030-01-03"},
			DayTotals: []*github.CopilotDailyMetrics{
				orgDay("2030-01-03", 3, 30),
				orgDay("2030-01-01", 1, 10),
				orgDay("2030-01-01", 9, 90),
				orgDay("2029-12-31", 5, 50),
			},
		},
		latestUsers: &gh.CopilotLatestUserMetrics{
			Window: gh.CopilotMetricsWindow{StartDay: "2030-01-01", EndDay: "2030-01-03"},
			Records: []*github.CopilotUserPeriodicMetrics{
				{UserID: 1, UserLogin: "user-a", Day: "2030-01-01"},
				{UserID: 1, UserLogin: "user-a", Day: "2030-01-01"},
			},
		},
	}
	ds, err := Collect(t.Context(), f, CollectOptions{Sections: sections(SectionKPI, SectionUser), Period: Period{Latest: true}})
	require.NoError(t, err)
	assert.Equal(t, []string{"2030-01-01", "2030-01-02", "2030-01-03"}, ds.Days)
	require.Len(t, ds.OrgDays, 2)
	assert.Equal(t, "2030-01-01", ds.OrgDays[0].Day)
	assert.Equal(t, 1, *ds.OrgDays[0].DailyActiveUsers, "first record of a duplicated day wins")
	assert.Equal(t, []string{"2030-01-02"}, ds.Missing[FamilyOrganization])
	assert.Len(t, ds.Users, 1)
	assert.Len(t, ds.Warnings, 3)
}

func TestCollectLatestWindowMismatch(t *testing.T) {
	f := &fakeFetcher{
		latest: &gh.CopilotLatestMetrics{
			Window:    gh.CopilotMetricsWindow{StartDay: "2030-01-01", EndDay: "2030-01-02"},
			DayTotals: []*github.CopilotDailyMetrics{orgDay("2030-01-01", 1, 1), orgDay("2030-01-02", 1, 1)},
		},
		latestUsers: &gh.CopilotLatestUserMetrics{
			Window: gh.CopilotMetricsWindow{StartDay: "2030-01-02", EndDay: "2030-01-03"},
			Records: []*github.CopilotUserPeriodicMetrics{
				{UserID: 1, Day: "2030-01-02", TotalsByIDE: []*github.CopilotUserMetricsIDE{{IDE: "Editor A"}}},
				{UserID: 1, Day: "2030-01-03", TotalsByIDE: []*github.CopilotUserMetricsIDE{{IDE: "Editor A"}}},
			},
		},
	}
	opts := CollectOptions{Sections: sections(SectionIDE), IncludeIDEActiveUsers: true, Period: Period{Latest: true}}
	ds, err := Collect(t.Context(), f, opts)
	require.NoError(t, err)
	assert.Len(t, ds.Users, 1)
	assert.Contains(t, ds.Warnings[0], "differs")
	assert.Equal(t, []string{"2030-01-01"}, ds.Missing[FamilyUsers], "dates outside the user report window are unavailable")
	summary := BuildSummary(ds, SummaryOptions{Sections: opts.Sections, IncludeRichMetrics: true})
	require.NotNil(t, summary.Rich)
	require.Len(t, summary.Rich.IDEActiveUsers, 1)
	assert.False(t, summary.Rich.IDEActiveUsers[0].Daily[0].Valid, "a date outside the users report window is not shown as zero")
	assert.True(t, summary.Rich.IDEActiveUsers[0].Daily[1].Valid)

	opts.Strict = true
	_, err = Collect(t.Context(), f, opts)
	require.Error(t, err)
}

func TestCollectDaily(t *testing.T) {
	f := &fakeFetcher{
		org: map[string][]*github.CopilotDailyMetrics{
			"2030-01-01": {orgDay("2030-01-01", 2, 20)},
			"2030-01-03": {orgDay("2030-01-03", 4, 40)},
		},
		repos: map[string][]*github.CopilotRepositoryDailyMetrics{
			"2030-01-01": {{Day: "2030-01-01", RepoID: 7, RepoName: "repo-x"}},
		},
	}
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03"}
	opts := CollectOptions{Sections: sections(SectionKPI, SectionRepository), Period: Period{Days: days}, Concurrency: 2}
	ds, err := Collect(t.Context(), f, opts)
	require.NoError(t, err)
	assert.Len(t, ds.OrgDays, 2)
	assert.Len(t, ds.Repositories, 1)
	assert.Equal(t, []string{"2030-01-02"}, ds.Missing[FamilyOrganization])
	assert.Equal(t, []string{"2030-01-02", "2030-01-03"}, ds.Missing[FamilyRepositories])

	opts.Strict = true
	_, err = Collect(t.Context(), f, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "organization (2030-01-02)")
}

func TestCollectDailyAllMissing(t *testing.T) {
	f := &fakeFetcher{}
	_, err := Collect(t.Context(), f, CollectOptions{Sections: sections(SectionKPI), Period: Period{Days: []string{"2030-01-01"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usage metrics reports were found")
}

func TestCollectDailyError(t *testing.T) {
	days := DayRange("2030-01-01", "2030-01-20")
	f := &fakeFetcher{errDay: "*"}
	_, err := Collect(t.Context(), f, CollectOptions{Sections: sections(SectionKPI), Period: Period{Days: days}, Concurrency: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), ": boom")
	assert.Equal(t, int32(1), f.calls.Load(), "remaining requests are cancelled after the first error")
}

func TestCollectUsersAndTeamsOnly(t *testing.T) {
	f := &fakeFetcher{
		users: map[string][]*github.CopilotUserDailyMetrics{
			"2030-01-01": {{UserID: 1, UserLogin: "user-a", Day: "2030-01-01"}},
		},
		teams: map[string][]*github.CopilotUserTeamsDailyMetrics{
			"2030-01-01": {{UserID: 1, TeamID: 10, Slug: "team-a", Day: "2030-01-01"}},
		},
	}
	ds, err := Collect(t.Context(), f, CollectOptions{Sections: sections(SectionTeam), Period: Period{Days: []string{"2030-01-01"}}})
	require.NoError(t, err)
	assert.Empty(t, ds.OrgDays)
	assert.Len(t, ds.Users, 1)
	assert.Len(t, ds.UserTeams, 1)
	assert.Empty(t, ds.Missing)
}

func TestCollectIDEUsersWithoutUserSection(t *testing.T) {
	f := &fakeFetcher{
		org: map[string][]*github.CopilotDailyMetrics{
			"2030-01-01": {orgDay("2030-01-01", 2, 4)},
		},
		users: map[string][]*github.CopilotUserDailyMetrics{
			"2030-01-01": {{UserID: 1, UserLogin: "private-login", Day: "2030-01-01"}},
		},
	}
	ds, err := Collect(t.Context(), f, CollectOptions{
		Sections: sections(SectionKPI), Period: Period{Days: []string{"2030-01-01"}},
		IncludeIDEActiveUsers: true,
	})
	require.NoError(t, err)
	require.Len(t, ds.Users, 1)
	assert.Equal(t, "private-login", ds.Users[0].Login, "the report uses the record for aggregation; renderers must not expose it in rich mode")
	assert.Equal(t, int32(2), f.calls.Load(), "one organization and one user report are fetched")

	f.calls.Store(0)
	_, err = Collect(t.Context(), f, CollectOptions{
		Sections: sections(SectionKPI), Period: Period{Days: []string{"2030-01-01"}},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), f.calls.Load(), "user reports are not fetched when IDE counts are not requested")
}

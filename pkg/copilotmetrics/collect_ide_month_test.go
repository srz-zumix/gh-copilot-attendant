package copilotmetrics

import (
	"context"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dailyIDEUser(day string, id int, login string) *github.CopilotUserDailyMetrics {
	return &github.CopilotUserDailyMetrics{
		Day: day, UserID: id, UserLogin: login,
		TotalsByIDE: []*github.CopilotUserMetricsIDE{{IDE: "Editor A"}},
	}
}

func TestCollectIDEMonthSupplementForLatestReport(t *testing.T) {
	days := DayRange("2030-01-04", "2030-01-31")
	orgDays := make([]*github.CopilotDailyMetrics, 0, len(days))
	for _, day := range days {
		orgDays = append(orgDays, orgDay(day, 1, 1))
	}
	f := &fakeFetcher{
		latest: &gh.CopilotLatestMetrics{
			Window: gh.CopilotMetricsWindow{StartDay: days[0], EndDay: days[len(days)-1]}, DayTotals: orgDays,
		},
		latestUsers: &gh.CopilotLatestUserMetrics{
			Window: gh.CopilotMetricsWindow{StartDay: days[0], EndDay: days[len(days)-1]},
			Records: []*github.CopilotUserPeriodicMetrics{
				{Day: "2030-01-04", UserID: 1, UserLogin: "example-user-a", TotalsByIDE: []*github.CopilotUserMetricsIDE{{IDE: "Editor A"}}},
				{Day: "2030-01-31", UserID: 2, UserLogin: "example-user-b", TotalsByIDE: []*github.CopilotUserMetricsIDE{{IDE: "Editor B"}}},
			},
		},
		users: map[string][]*github.CopilotUserDailyMetrics{
			"2030-01-01": {
				dailyIDEUser("2030-01-01", 1, "example-user-a"),
				dailyIDEUser("2030-01-01", 3, "example-user-c"),
			},
			"2030-01-02": {},
			"2030-01-03": {dailyIDEUser("2030-01-03", 4, "example-user-d")},
		},
	}
	opts := CollectOptions{
		Sections: sections(SectionIDE, SectionUser), Period: Period{Latest: true}, IncludeIDEActiveUsers: true,
	}
	ds, err := Collect(context.Background(), f, opts)
	require.NoError(t, err)
	assert.Equal(t, int32(5), f.calls.Load(), "two latest reports and three additional daily reports")
	require.NotNil(t, ds.ideMonthSupplement)
	assert.Equal(t, DayRange("2030-01-01", "2030-01-03"), ds.ideMonthSupplement.days)
	assert.Empty(t, ds.ideMonthSupplement.missingDays)
	assert.Len(t, ds.Users, 2, "supplemental records do not extend the selected user window")

	summary := BuildSummary(ds, SummaryOptions{Sections: opts.Sections, IncludeRichMetrics: true})
	require.NotNil(t, summary.Rich.IDEActiveUserMonth)
	assert.Equal(t, Counter{Value: 4, Valid: true}, summary.Rich.IDEActiveUserMonth.Users)
	assert.Empty(t, summary.Rich.IDEActiveUserMonth.MissingDays)
	var logins []string
	for _, user := range summary.Users {
		logins = append(logins, user.Login)
	}
	assert.ElementsMatch(t, []string{"example-user-a", "example-user-b"}, logins)
}

func TestCollectIDEMonthSupplementMissingAndStrict(t *testing.T) {
	f := &fakeFetcher{
		org: map[string][]*github.CopilotDailyMetrics{
			"2030-01-04": {orgDay("2030-01-04", 1, 1)},
		},
		users: map[string][]*github.CopilotUserDailyMetrics{
			"2030-01-01": {dailyIDEUser("2030-01-01", 1, "example-user-a")},
			"2030-01-03": {},
			"2030-01-04": {dailyIDEUser("2030-01-04", 2, "example-user-b")},
		},
	}
	opts := CollectOptions{
		Sections: sections(SectionIDE), Period: Period{Days: []string{"2030-01-04"}}, IncludeIDEActiveUsers: true,
	}
	ds, err := Collect(context.Background(), f, opts)
	require.NoError(t, err)
	require.NotNil(t, ds.ideMonthSupplement)
	assert.Equal(t, []string{"2030-01-02"}, ds.ideMonthSupplement.missingDays)
	assert.Empty(t, ds.Missing[FamilyUsers], "lookback gaps do not change the selected report window")
	estimate := BuildSummary(ds, SummaryOptions{Sections: opts.Sections, IncludeRichMetrics: true}).Rich.IDEActiveUserMonth
	require.NotNil(t, estimate)
	assert.Equal(t, Counter{Value: 2, Valid: true}, estimate.Users)
	assert.Equal(t, []string{"2030-01-02"}, estimate.MissingDays)

	opts.Strict = true
	_, err = Collect(context.Background(), f, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "users (2030-01-02)")
}

func TestCollectIDEMonthSupplementEmptyAndUnavailable(t *testing.T) {
	tests := []struct {
		name        string
		users       map[string][]*github.CopilotUserDailyMetrics
		want        Counter
		missingDays []string
	}{
		{
			name: "empty reports are available",
			users: map[string][]*github.CopilotUserDailyMetrics{
				"2030-01-01": {},
				"2030-01-02": {},
				"2030-01-03": {},
			},
			want: Counter{Valid: true},
		},
		{
			name:        "all reports are unavailable",
			missingDays: DayRange("2030-01-01", "2030-01-03"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeFetcher{
				org:   map[string][]*github.CopilotDailyMetrics{"2030-01-03": {orgDay("2030-01-03", 0, 0)}},
				users: tt.users,
			}
			opts := CollectOptions{
				Sections: sections(SectionIDE), Period: Period{Days: []string{"2030-01-03"}}, IncludeIDEActiveUsers: true,
			}
			ds, err := Collect(context.Background(), f, opts)
			require.NoError(t, err)
			estimate := BuildSummary(ds, SummaryOptions{Sections: opts.Sections, IncludeRichMetrics: true}).Rich.IDEActiveUserMonth
			require.NotNil(t, estimate)
			assert.Equal(t, tt.want, estimate.Users)
			assert.Equal(t, tt.missingDays, estimate.MissingDays)
			if len(tt.missingDays) > 0 {
				opts.Strict = true
				_, err = Collect(context.Background(), f, opts)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "users (2030-01-01, 2030-01-02, 2030-01-03)")
			}
		})
	}
}

func TestCollectIDEMonthSupplementDoesNotRefetchCoveredDays(t *testing.T) {
	days := DayRange("2030-01-30", "2030-02-02")
	f := &fakeFetcher{
		org:   map[string][]*github.CopilotDailyMetrics{},
		users: map[string][]*github.CopilotUserDailyMetrics{},
	}
	for _, day := range days {
		f.org[day] = []*github.CopilotDailyMetrics{orgDay(day, 0, 0)}
		f.users[day] = []*github.CopilotUserDailyMetrics{}
	}
	opts := CollectOptions{
		Sections: sections(SectionIDE), Period: Period{Days: days}, IncludeIDEActiveUsers: true,
	}
	ds, err := Collect(context.Background(), f, opts)
	require.NoError(t, err)
	require.NotNil(t, ds.ideMonthSupplement)
	assert.Empty(t, ds.ideMonthSupplement.days)
	assert.Equal(t, int32(8), f.calls.Load(), "only the requested daily organization and users reports are fetched")
	estimate := BuildSummary(ds, SummaryOptions{Sections: opts.Sections, IncludeRichMetrics: true}).Rich.IDEActiveUserMonth
	require.NotNil(t, estimate)
	assert.Equal(t, "2030-02-01", estimate.StartDay)
	assert.Equal(t, "2030-02-02", estimate.EndDay)
	assert.Equal(t, Counter{Valid: true}, estimate.Users)
}

func TestCollectIDEMonthSupplementErrorsAndOptIn(t *testing.T) {
	f := &fakeFetcher{
		org: map[string][]*github.CopilotDailyMetrics{
			"2030-01-04": {orgDay("2030-01-04", 1, 1)},
		},
		users: map[string][]*github.CopilotUserDailyMetrics{
			"2030-01-04": {dailyIDEUser("2030-01-04", 1, "example-user-a")},
		},
		errDay: "2030-01-02",
	}
	opts := CollectOptions{
		Sections: sections(SectionIDE, SectionUser), Period: Period{Days: []string{"2030-01-04"}}, IncludeIDEActiveUsers: true,
	}
	_, err := Collect(context.Background(), f, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "additional users reports for the IDE active-user estimate")
	assert.Contains(t, err.Error(), "2030-01-02: boom")

	f.calls.Store(0)
	opts.IncludeIDEActiveUsers = false
	ds, err := Collect(context.Background(), f, opts)
	require.NoError(t, err)
	assert.Nil(t, ds.ideMonthSupplement)
	assert.Equal(t, int32(2), f.calls.Load(), "no additional reports are fetched without IDE active users")
}

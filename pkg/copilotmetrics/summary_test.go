package copilotmetrics

import (
	"fmt"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func kpiByLabel(t *testing.T, kpis []KPI, label string) KPI {
	t.Helper()
	for _, k := range kpis {
		if k.Label == label {
			return k
		}
	}
	t.Fatalf("KPI %q not found", label)
	return KPI{}
}

func TestBuildSummaryKPIs(t *testing.T) {
	d1 := orgDay("2030-01-01", 10, 100)
	d1.WeeklyActiveUsers = new(20)
	d1.MonthlyActiveUsers = new(30)
	d1.CodeGenerationActivityCount = new(50)
	d1.CodeAcceptanceActivityCount = new(10)
	d2 := orgDay("2030-01-02", 20, 200)
	d2.WeeklyActiveUsers = new(25)
	d2.MonthlyActiveUsers = new(35)
	d2.CodeGenerationActivityCount = new(50)
	d2.CodeAcceptanceActivityCount = new(30)
	d3 := &github.CopilotDailyMetrics{Day: "2030-01-03"}

	ds := &Dataset{
		Period:  Period{Days: []string{"2030-01-01", "2030-01-02", "2030-01-03"}},
		Days:    []string{"2030-01-01", "2030-01-02", "2030-01-03"},
		OrgDays: []*github.CopilotDailyMetrics{d1, d2, d3},
	}
	s := BuildSummary(ds, SummaryOptions{Source: "octo (organization)", Sections: sections(SectionKPI, SectionTrend)})

	avg := kpiByLabel(t, s.KPIs, "Daily active users (average)")
	assert.True(t, avg.Valid)
	assert.InDelta(t, 15.0, avg.Value, 1e-9, "DAU is averaged over days with data, never summed")
	assert.Equal(t, 20.0, kpiByLabel(t, s.KPIs, "Daily active users (peak)").Value)

	mau := kpiByLabel(t, s.KPIs, "Monthly active users")
	assert.Equal(t, 35.0, mau.Value, "MAU is taken from the last day with data")
	assert.Contains(t, mau.Window, "2030-01-02")

	assert.Equal(t, 300.0, kpiByLabel(t, s.KPIs, "User-initiated interactions").Value)
	rate := kpiByLabel(t, s.KPIs, "Code acceptance rate")
	assert.True(t, rate.Valid)
	assert.InDelta(t, 0.4, rate.Value, 1e-9, "rate is derived from summed values")

	assert.False(t, kpiByLabel(t, s.KPIs, "Monthly active chat users").Valid)
	assert.False(t, kpiByLabel(t, s.KPIs, "CLI sessions").Valid, "missing values are n/a, not zero")

	require.Len(t, s.Trend, 3)
	assert.False(t, s.Trend[2].DailyActiveUsers.Valid)
	assert.Equal(t, "2030-01-01", s.StartDay)
	assert.Equal(t, "2030-01-03", s.EndDay)
	assert.Equal(t, 3, s.Days)
}

func TestBuildSummaryBreakdownTop(t *testing.T) {
	feature := func(name string, n int) *github.CopilotMetricsFeature {
		return &github.CopilotMetricsFeature{Feature: name, UserInitiatedInteractionCount: new(n)}
	}
	d1 := &github.CopilotDailyMetrics{Day: "2030-01-01", TotalsByFeature: []*github.CopilotMetricsFeature{feature("chat", 5), feature("agent", 1), feature("", 2)}}
	d2 := &github.CopilotDailyMetrics{Day: "2030-01-02", TotalsByFeature: []*github.CopilotMetricsFeature{feature("agent", 6)}}
	ds := &Dataset{Days: []string{"2030-01-01", "2030-01-02"}, OrgDays: []*github.CopilotDailyMetrics{d1, d2}}
	s := BuildSummary(ds, SummaryOptions{Sections: sections(SectionFeature), Top: 2})
	require.Len(t, s.Features, 2)
	assert.Equal(t, "agent", s.Features[0].Key)
	assert.Equal(t, int64(7), s.Features[0].UserInitiatedInteraction.Value)
	assert.Equal(t, "chat", s.Features[1].Key)
	assert.Equal(t, 1, s.Omitted[SectionFeature])
	assert.False(t, s.Features[0].CodeGeneration.Valid)
}

func TestBuildSummaryModelDimensionsForAllModes(t *testing.T) {
	first := &github.CopilotDailyMetrics{Day: "2030-01-01"}
	second := &github.CopilotDailyMetrics{Day: "2030-01-02"}
	first.TotalsByModelFeature = []*github.CopilotMetricsModelFeature{
		{Model: "model-a", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(10)},
		{Model: "model-a", Feature: "chat_panel_ask_mode"},
		{Model: "model-a", Feature: "chat_inline", UserInitiatedInteractionCount: new(20)},
		{Model: "", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(1)},
		{Model: "unknown", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(2)},
		{Model: "Other", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(3)},
		{Model: "unkown", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(4)},
		{Model: "Others", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(5)},
		{Model: "model-null", Feature: "chat_panel_agent_mode"},
		{Model: "model-zero", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(0)},
	}
	second.TotalsByModelFeature = []*github.CopilotMetricsModelFeature{
		{Model: "model-a", Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(20)},
		{Model: "model-a", Feature: "chat_panel_ask_mode", UserInitiatedInteractionCount: new(8)},
	}
	first.TotalsByLanguageFeature = []*github.CopilotMetricsLanguageFeature{
		{Language: "lang-a", CodeGenerationActivityCount: new(4)},
	}
	first.TotalsByLanguageModel = []*github.CopilotMetricsLanguageModel{
		{Language: "lang-a", Model: "model-a", CodeGenerationActivityCount: new(4)},
		{Language: "", Model: "", CodeGenerationActivityCount: new(1)},
		{Language: "unknown", Model: "unknown", CodeGenerationActivityCount: new(2)},
		{Language: "Other", Model: "Other", CodeGenerationActivityCount: new(3)},
		{Language: "unkown", Model: "unkown", CodeGenerationActivityCount: new(4)},
		{Language: "Others", Model: "Others", CodeGenerationActivityCount: new(5)},
		{Language: "lang-null", Model: "model-null"},
		{Language: "lang-zero", Model: "model-zero", CodeGenerationActivityCount: new(0)},
	}
	second.TotalsByLanguageModel = []*github.CopilotMetricsLanguageModel{
		{Language: "lang-a", Model: "model-a", CodeGenerationActivityCount: new(6)},
	}
	for i := 1; i <= 24; i++ {
		name := fmt.Sprintf("item-%02d", i)
		first.TotalsByModelFeature = append(first.TotalsByModelFeature, &github.CopilotMetricsModelFeature{
			Model: name, Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(1),
		})
		first.TotalsByLanguageFeature = append(first.TotalsByLanguageFeature, &github.CopilotMetricsLanguageFeature{
			Language: name, CodeGenerationActivityCount: new(1),
		})
		first.TotalsByLanguageModel = append(first.TotalsByLanguageModel, &github.CopilotMetricsLanguageModel{
			Language: name, Model: name, CodeGenerationActivityCount: new(1),
		})
	}
	ds := &Dataset{
		Days: []string{"2030-01-01", "2030-01-02"}, OrgDays: []*github.CopilotDailyMetrics{first, second},
	}
	find := func(t *testing.T, rows []RichValue, group, name string) Counter {
		t.Helper()
		for _, row := range rows {
			if row.Group == group && row.Name == name {
				return row.Value
			}
		}
		t.Fatalf("missing dimension %q / %q", group, name)
		return Counter{}
	}
	for _, rich := range []bool{false, true} {
		t.Run(fmt.Sprintf("rich=%t", rich), func(t *testing.T) {
			s := BuildSummary(ds, SummaryOptions{
				Sections: sections(SectionModel, SectionLanguage), Top: 1, IncludeRichMetrics: rich,
			})
			assert.Len(t, s.Models, 1)
			assert.Len(t, s.Languages, 1)
			assert.Len(t, s.AllLanguages, 25)
			assert.Len(t, s.ModelRequestsByFeature, 34, "detail dimensions are not subject to --top")
			assert.Len(t, s.ModelCodeGenerationByLanguage, 32)
			assert.Equal(t, Counter{Value: 30, Valid: true}, find(t, s.ModelRequestsByFeature, "chat_panel_agent_mode", "model-a"))
			assert.Equal(t, Counter{Value: 8, Valid: true}, find(t, s.ModelRequestsByFeature, "chat_panel_ask_mode", "model-a"))
			assert.Equal(t, Counter{}, find(t, s.ModelRequestsByFeature, "chat_panel_agent_mode", "model-null"))
			assert.Equal(t, Counter{Valid: true}, find(t, s.ModelRequestsByFeature, "chat_panel_agent_mode", "model-zero"))
			assert.Equal(t, Counter{Value: 10, Valid: true}, find(t, s.ModelCodeGenerationByLanguage, "lang-a", "model-a"))
			for _, name := range []string{"", "unknown", "Other", "Others", "unkown"} {
				assert.True(t, find(t, s.ModelRequestsByFeature, "chat_panel_agent_mode", name).Valid)
				assert.True(t, find(t, s.ModelCodeGenerationByLanguage, name, name).Valid)
			}
			if rich {
				require.NotNil(t, s.Rich)
				assert.Equal(t, s.ModelRequestsByFeature, s.Rich.ModelRequestsByFeature)
				assert.Equal(t, s.ModelCodeGenerationByLanguage, s.Rich.ModelCodeGenerationByLanguage)
				assert.Equal(t, int64(58), find(t, s.Rich.ModelRequests, "", "model-a").Value,
					"non-chat features remain in the model total")
			} else {
				assert.Nil(t, s.Rich)
			}
		})
	}
	for _, tc := range []struct {
		name         string
		sections     map[Section]bool
		wantModel    bool
		wantLanguage bool
	}{
		{name: "model", sections: sections(SectionModel), wantModel: true},
		{name: "language", sections: sections(SectionLanguage), wantLanguage: true},
		{name: "neither", sections: sections(SectionFeature)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := BuildSummary(ds, SummaryOptions{Sections: tc.sections, Top: 1})
			assert.Equal(t, tc.wantModel, len(s.ModelRequestsByFeature) > 0)
			assert.Equal(t, tc.wantLanguage, len(s.ModelCodeGenerationByLanguage) > 0)
			assert.Equal(t, tc.wantLanguage, len(s.AllLanguages) > 0)
		})
	}
}

func TestBuildSummaryTeamsAndUsers(t *testing.T) {
	ds := &Dataset{
		Days: []string{"2030-01-01", "2030-01-02"},
		Users: []UserDay{
			{UserID: 1, Login: "user-a", Day: "2030-01-01", UserInitiatedInteractions: new(10), AICreditsUsed: new(1.5), UsedChat: true},
			{UserID: 1, Login: "user-a", Day: "2030-01-02", UserInitiatedInteractions: new(5), UsedAgent: true},
			{UserID: 2, Login: "user-b", Day: "2030-01-02", UserInitiatedInteractions: new(3)},
		},
		UserTeams: []*github.CopilotUserTeamsDailyMetrics{
			{UserID: 1, TeamID: 10, Slug: "team-a", Day: "2030-01-01"},
			{UserID: 1, TeamID: 20, Slug: "team-b", Day: "2030-01-01"},
			// User 1 left team-a before day 2, so day 2 activity only counts for team-b.
			{UserID: 1, TeamID: 20, Slug: "team-b", Day: "2030-01-02"},
			{UserID: 2, TeamID: 10, Slug: "team-a", Day: "2030-01-02"},
			{UserID: 3, TeamID: 10, Slug: "team-a", Day: "2030-01-02"},
		},
		Missing: map[Family][]string{FamilyUserTeams: {"2030-01-03"}, FamilyRepositories: {"2030-01-03"}},
	}
	s := BuildSummary(ds, SummaryOptions{Sections: sections(SectionTeam, SectionUser)})

	require.Len(t, s.Teams, 2)
	assert.Equal(t, "team-b", s.Teams[0].Slug)
	assert.Equal(t, int64(15), s.Teams[0].UserInitiatedInteraction.Value)
	assert.Equal(t, 1, s.Teams[0].Members)
	assert.Equal(t, "team-a", s.Teams[1].Slug)
	assert.Equal(t, int64(13), s.Teams[1].UserInitiatedInteraction.Value)
	assert.Equal(t, 3, s.Teams[1].Members)
	assert.Equal(t, 2, s.Teams[1].ActiveUsers)
	assert.InDelta(t, 1.5, s.Teams[1].AICredits.Value, 1e-9)
	assert.Equal(t, s.Teams, s.AllTeams)

	limited := BuildSummary(ds, SummaryOptions{Sections: sections(SectionTeam), Top: 1})
	require.Len(t, limited.Teams, 1)
	assert.Equal(t, s.AllTeams, limited.AllTeams)
	assert.Equal(t, 1, limited.Omitted[SectionTeam])

	require.Len(t, s.Users, 2)
	assert.Equal(t, "user-a", s.Users[0].Login)
	assert.Equal(t, 2, s.Users[0].ActiveDays)
	assert.Equal(t, []string{"agent", "chat"}, s.Users[0].Surfaces)
	assert.False(t, s.Users[1].AICredits.Valid)
	assert.Equal(t, s.Users, s.AllUsers)
	limitedUsers := BuildSummary(ds, SummaryOptions{Sections: sections(SectionUser), Top: 1})
	require.Len(t, limitedUsers.Users, 1)
	assert.Equal(t, s.AllUsers, limitedUsers.AllUsers)

	require.Len(t, s.Partial, 1, "only families affecting selected sections are reported")
	assert.Equal(t, FamilyUserTeams, s.Partial[0].Family)
	assert.Equal(t, []Section{SectionTeam}, s.Partial[0].Sections)
}

func TestBuildSummaryRepositories(t *testing.T) {
	pr := func(created, byCopilot int) *github.CopilotMetricsPullRequests {
		return &github.CopilotMetricsPullRequests{TotalCreated: new(created), TotalCreatedByCopilot: new(byCopilot)}
	}
	ds := &Dataset{
		Days: []string{"2030-01-01", "2030-01-02"},
		Repositories: []*github.CopilotRepositoryDailyMetrics{
			{Day: "2030-01-01", RepoID: 1, RepoOwnerName: "octo", RepoName: "repo-x", PullRequests: pr(3, 0)},
			{Day: "2030-01-01", RepoID: 2, RepoOwnerName: "octo", RepoName: "repo-y", PullRequests: pr(1, 1)},
			{Day: "2030-01-02", RepoID: 2, RepoOwnerName: "octo", RepoName: "repo-y"},
		},
	}
	s := BuildSummary(ds, SummaryOptions{Sections: sections(SectionRepository)})
	require.Len(t, s.Repositories, 2)
	require.Len(t, s.AllRepositories, 2)
	assert.Equal(t, "octo/repo-y", s.Repositories[0].Name)
	assert.Equal(t, "octo/repo-y", s.AllRepositories[0].Name)
	assert.Equal(t, 2, s.Repositories[0].ActiveDays)
	assert.False(t, s.Repositories[0].Merged.Valid)
	assert.Equal(t, int64(3), s.Repositories[1].Created.Value)
}

func TestBuildSummaryRepositoriesKeepsUntrimmedRowsForCharts(t *testing.T) {
	pr := func(created, byCopilot int) *github.CopilotMetricsPullRequests {
		return &github.CopilotMetricsPullRequests{TotalCreated: new(created), TotalCreatedByCopilot: new(byCopilot)}
	}
	ds := &Dataset{
		Days: []string{"2030-01-01"},
		Repositories: []*github.CopilotRepositoryDailyMetrics{
			{Day: "2030-01-01", RepoID: 1, RepoOwnerName: "octo", RepoName: "repo-a", PullRequests: pr(8, 1)},
			{Day: "2030-01-01", RepoID: 2, RepoOwnerName: "octo", RepoName: "repo-b", PullRequests: pr(2, 2)},
		},
	}
	s := BuildSummary(ds, SummaryOptions{Sections: sections(SectionRepository), Top: 1})
	require.Len(t, s.Repositories, 1)
	require.Len(t, s.AllRepositories, 2)
	assert.Equal(t, "octo/repo-b", s.Repositories[0].Name)
	assert.Equal(t, "octo/repo-a", s.AllRepositories[1].Name)
}

func TestRate(t *testing.T) {
	_, ok := Rate(Counter{Value: 1, Valid: true}, Counter{Value: 0, Valid: true})
	assert.False(t, ok)
	_, ok = Rate(Counter{Value: 1, Valid: true}, Counter{})
	assert.False(t, ok)
	r, ok := Rate(Counter{Value: 1, Valid: true}, Counter{Value: 4, Valid: true})
	assert.True(t, ok)
	assert.InDelta(t, 0.25, r, 1e-9)
}

func TestBuildUsersRatioCountsUseMatchingDays(t *testing.T) {
	ds := &Dataset{
		Days: []string{"2030-01-01", "2030-01-02", "2030-01-03"},
		Users: []UserDay{
			{UserID: 1, Login: "user-a", Day: "2030-01-01", UserInitiatedInteractions: new(4), CodeGenerationActivities: new(8), CodeAcceptanceActivities: new(2)},
			{UserID: 1, Login: "user-a", Day: "2030-01-02", UserInitiatedInteractions: new(6)},
			{UserID: 1, Login: "user-a", Day: "2030-01-03", CodeGenerationActivities: new(10)},
		},
	}
	s := BuildSummary(ds, SummaryOptions{Sections: sections(SectionUser)})
	require.Len(t, s.AllUsers, 1)
	user := s.AllUsers[0]
	assert.Equal(t, int64(10), user.UserInitiatedInteraction.Value)
	assert.Equal(t, int64(18), user.CodeGeneration.Value)
	assert.Equal(t, 3, user.ActiveDays)
	rate, ok := user.Acceptance.Rate()
	require.True(t, ok)
	assert.InDelta(t, .25, rate, 1e-9)
	assert.Equal(t, RatioCounts{Numerator: Counter{2, true}, Denominator: Counter{8, true}}, user.Acceptance)

	_, ok = RatioCounts{}.Rate()
	assert.False(t, ok)
}

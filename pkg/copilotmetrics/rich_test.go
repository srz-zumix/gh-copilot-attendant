package copilotmetrics

import (
	"fmt"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIDEActiveUsers(t *testing.T) {
	days := make([]string, 10)
	for i := range days {
		days[i] = fmt.Sprintf("2030-01-%02d", i+1)
	}
	ds := &Dataset{
		Days:    days,
		Missing: map[Family][]string{FamilyUsers: {days[2]}},
		Users: []UserDay{
			{UserID: 1, Login: "private-login", Day: days[0], IDEs: []string{"Editor A", "Editor B"}},
			{UserID: 1, Login: "private-login", Day: days[0], IDEs: []string{"Editor A"}},
			{UserID: 2, Day: days[0], IDEs: []string{"Editor A"}},
			{UserID: 1, Day: days[1], IDEs: []string{"Editor A"}},
			{UserID: 3, Day: days[1], IDEs: []string{"Editor B"}},
			{UserID: 2, Day: days[8], IDEs: []string{"Editor A"}},
			{UserID: 1, Day: days[9], IDEs: []string{"Editor B"}},
		},
	}
	dayIndex := make(map[string]int, len(days))
	for i, day := range days {
		dayIndex[day] = i
	}

	ides, dailyTotal, weeklyTotal := buildIDEActiveUsers(ds, dayIndex)
	require.Len(t, ides, 2)
	assert.Equal(t, "Editor A", ides[0].IDE)
	assert.Equal(t, int64(2), ides[0].Daily[0].Value)
	assert.True(t, ides[0].Daily[0].Valid)
	assert.False(t, ides[0].Daily[2].Valid, "daily counts are unavailable for missing reports")
	assert.False(t, ides[0].Weekly[8].Valid, "rolling seven-day count is unavailable if its window has a missing report")
	assert.Equal(t, int64(1), ides[0].Weekly[9].Value)
	assert.True(t, ides[0].Weekly[9].Valid)
	assert.False(t, dailyTotal[2].Valid)
	assert.Equal(t, int64(2), dailyTotal[0].Value, "the total deduplicates users appearing in multiple IDEs")
	assert.True(t, weeklyTotal[9].Valid)
	assert.Equal(t, int64(2), weeklyTotal[9].Value, "the rolling total deduplicates users across IDEs")
}

func TestBuildSummaryRichMetrics(t *testing.T) {
	dau, interactions, mau, agentUsers := 10, 30, 20, 5
	codeGeneration, codeAcceptance := 40, 10
	ds := &Dataset{
		Days: []string{"2030-01-01"},
		OrgDays: []*github.CopilotDailyMetrics{{
			Day:                           "2030-01-01",
			DailyActiveUsers:              &dau,
			UserInitiatedInteractionCount: &interactions,
			MonthlyActiveUsers:            &mau,
			MonthlyActiveAgentUsers:       &agentUsers,
			CodeGenerationActivityCount:   &codeGeneration,
			CodeAcceptanceActivityCount:   &codeAcceptance,
			TotalsByFeature: []*github.CopilotMetricsFeature{
				{Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(3)},
				{Feature: "chat_panel_ask_mode", UserInitiatedInteractionCount: new(7)},
			},
			TotalsByModelFeature: []*github.CopilotMetricsModelFeature{
				{Model: "model-a", Feature: "chat", UserInitiatedInteractionCount: new(20)},
			},
			TotalsByLanguageFeature: []*github.CopilotMetricsLanguageFeature{
				{Language: "go", CodeGenerationActivityCount: new(40)},
			},
			TotalsByLanguageModel: []*github.CopilotMetricsLanguageModel{
				{Language: "go", Model: "model-a", CodeGenerationActivityCount: new(40)},
			},
		}},
	}

	summary := BuildSummary(ds, SummaryOptions{
		Sections:           sections(SectionKPI, SectionTrend, SectionIDE, SectionModel, SectionLanguage),
		IncludeRichMetrics: true,
	})
	require.NotNil(t, summary.Rich)
	assert.Equal(t, int64(10), summary.Rich.DailyActiveUsers[0].Value)
	assert.InDelta(t, 3.0, summary.Rich.AverageChatRequestsPerActiveUser[0].Value, 1e-9)
	require.Len(t, summary.Rich.ChatModeRequests, 2)
	assert.Equal(t, []string{"Agent", "Ask"}, []string{summary.Rich.ChatModeRequests[0].Name, summary.Rich.ChatModeRequests[1].Name})
	assert.InDelta(t, 0.25, summary.Rich.AcceptanceRate[0].Value, 1e-9)
	require.Len(t, summary.Rich.ModelRequests, 1)
	assert.Equal(t, "model-a", summary.Rich.ModelRequests[0].Name)
	require.Len(t, summary.Rich.LanguageCodeGeneration, 1)
	assert.Equal(t, "go", summary.Rich.LanguageCodeGeneration[0].Name)
	assert.Empty(t, summary.Rich.IDEActiveUsers, "IDE metrics require user-level reports only when selected")
}

func TestRichCodeCompletions(t *testing.T) {
	feature := func(name string, suggested, accepted *int) *github.CopilotMetricsFeature {
		return &github.CopilotMetricsFeature{
			Feature: name,
			CopilotMetricsCodeActivity: github.CopilotMetricsCodeActivity{
				CodeGenerationActivityCount: suggested,
				CodeAcceptanceActivityCount: accepted,
			},
		}
	}
	ds := &Dataset{
		Days: DayRange("2030-01-01", "2030-01-05"),
		OrgDays: []*github.CopilotDailyMetrics{
			{Day: "2030-01-01", TotalsByFeature: []*github.CopilotMetricsFeature{
				nil,
				feature("code_completion", new(80), new(20)),
				feature("code_completion", new(20), new(5)),
				feature("chat_inline", new(900), new(800)),
				feature("agent_edit", new(500), new(400)),
			}},
			{Day: "2030-01-02", TotalsByFeature: []*github.CopilotMetricsFeature{
				feature("code_completion", new(0), new(0)),
			}},
			{Day: "2030-01-03", TotalsByFeature: []*github.CopilotMetricsFeature{
				feature("code_completion", new(10), nil),
			}},
			{Day: "2030-01-04", CodeGenerationActivityCount: new(1000)},
		},
	}
	rich := buildRichMetrics(ds, false, nil, nil)
	assert.Equal(t, Counter{Value: 100, Valid: true}, rich.CodeCompletionsSuggested[0])
	assert.Equal(t, Counter{Value: 25, Valid: true}, rich.CodeCompletionsAccepted[0])
	assert.Equal(t, FloatCounter{Value: .25, Valid: true}, rich.CodeCompletionsAcceptanceRate[0])
	assert.Equal(t, Counter{Valid: true}, rich.CodeCompletionsSuggested[1])
	assert.Equal(t, Counter{Valid: true}, rich.CodeCompletionsAccepted[1])
	assert.False(t, rich.CodeCompletionsAcceptanceRate[1].Valid, "zero suggestions have no defined acceptance rate")
	assert.False(t, rich.CodeCompletionsAcceptanceRate[2].Valid, "missing acceptance is not zero")
	for _, i := range []int{3, 4} {
		assert.False(t, rich.CodeCompletionsSuggested[i].Valid, "missing completion data must not fall back to all-feature totals")
		assert.False(t, rich.CodeCompletionsAcceptanceRate[i].Valid)
	}
}

package copilotmetrics

import (
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRichChatModeRequestsFromDailyFeatureBreakdowns(t *testing.T) {
	feature := func(name string, count *int) *github.CopilotMetricsFeature {
		return &github.CopilotMetricsFeature{Feature: name, UserInitiatedInteractionCount: count}
	}
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04"}
	ds := &Dataset{
		Days: days,
		OrgDays: []*github.CopilotDailyMetrics{
			{
				Day: days[0],
				CopilotMetricsChatPanel: github.CopilotMetricsChatPanel{
					ChatPanelAgentMode: new(900), ChatPanelAskMode: new(900),
				},
				TotalsByFeature: []*github.CopilotMetricsFeature{
					nil,
					feature("chat_panel_agent_mode", new(10)),
					feature("chat_panel_agent_mode", new(2)),
					feature("chat_panel_ask_mode", new(0)),
					feature("chat_panel_custom_mode", new(3)),
					feature("chat_panel_edit_mode", new(4)),
					feature("chat_panel_plan_mode", new(5)),
					feature("chat_panel_unknown_mode", nil),
					feature("chat_inline", new(500)),
					feature("code_completion", new(500)),
					feature("chat_panel_unrecognized_mode", new(500)),
				},
				TotalsByModelFeature: []*github.CopilotMetricsModelFeature{{
					Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(999),
				}},
			},
			{
				Day: days[1],
				TotalsByFeature: []*github.CopilotMetricsFeature{
					feature("chat_panel_agent_mode", nil),
					feature("chat_panel_ask_mode", new(7)),
					feature("chat_panel_custom_mode", nil),
					feature("chat_panel_plan_mode", new(0)),
					feature("chat_panel_unknown_mode", new(1)),
				},
			},
			{
				Day: days[2],
				CopilotMetricsChatPanel: github.CopilotMetricsChatPanel{
					ChatPanelAgentMode: new(800),
				},
				TotalsByFeature: []*github.CopilotMetricsFeature{feature("chat_inline", new(30))},
			},
		},
	}
	rich := buildRichMetrics(ds, false, nil, nil)
	want := []RichSeries{
		{Name: "Agent", Values: []Counter{{Value: 12, Valid: true}, {}, {}, {}}},
		{Name: "Ask", Values: []Counter{{Valid: true}, {Value: 7, Valid: true}, {}, {}}},
		{Name: "Custom", Values: []Counter{{Value: 3, Valid: true}, {}, {}, {}}},
		{Name: "Edit", Values: []Counter{{Value: 4, Valid: true}, {}, {}, {}}},
		{Name: "Plan", Values: []Counter{{Value: 5, Valid: true}, {Valid: true}, {}, {}}},
		{Name: "Unknown", Values: []Counter{{}, {Value: 1, Valid: true}, {}, {}}},
	}
	assert.Equal(t, want, rich.ChatModeRequests)
}

func TestRichChatModeRequestsPreserveUnavailableSeries(t *testing.T) {
	ds := &Dataset{
		Days: []string{"2030-01-01"},
		OrgDays: []*github.CopilotDailyMetrics{{
			Day: "2030-01-01",
			TotalsByFeature: []*github.CopilotMetricsFeature{
				{Feature: "chat_panel_agent_mode", UserInitiatedInteractionCount: new(3)},
				{Feature: "chat_panel_plan_mode"},
			},
		}},
	}
	rich := buildRichMetrics(ds, false, nil, nil)
	require.Len(t, rich.ChatModeRequests, 2)
	assert.Equal(t, RichSeries{Name: "Agent", Values: []Counter{{Value: 3, Valid: true}}}, rich.ChatModeRequests[0])
	assert.Equal(t, RichSeries{Name: "Plan", Values: []Counter{{}}}, rich.ChatModeRequests[1])

	ds.OrgDays[0].TotalsByFeature = nil
	ds.OrgDays[0].ChatPanelAgentMode = new(25)
	assert.Empty(t, buildRichMetrics(ds, false, nil, nil).ChatModeRequests)
}

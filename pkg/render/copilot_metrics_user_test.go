package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func copilotMetricsUserTestCounter(value int64) copilotmetrics.Counter {
	return copilotmetrics.Counter{Value: value, Valid: true}
}

func TestCopilotMetricsUserRatiosEligibilityAndSorting(t *testing.T) {
	user := func(login string, interactions int64, days int) copilotmetrics.UserRow {
		return copilotmetrics.UserRow{
			Login: login, ActiveDays: days,
			ActivityTotals: copilotmetrics.ActivityTotals{UserInitiatedInteraction: copilotMetricsUserTestCounter(interactions)},
		}
	}
	users := []copilotmetrics.UserRow{
		user("fictional-user-a", 4, 2),
		user("fictional-user-c", 8, 4),
		user("fictional-user-b", 8, 4),
		user("fictional-user-zero", 0, 3),
		user("fictional-user-no-days", 10, 0),
		user("fictional-user-negative-days", 10, -1),
		user("fictional-user-negative", -1, 3),
		{Login: "fictional-user-missing", ActiveDays: 3},
	}
	ratios := copilotMetricsUserRatios(users)
	require.Len(t, ratios, 4)
	assert.Equal(t, []string{"fictional-user-b", "fictional-user-c", "fictional-user-a", "fictional-user-zero"},
		[]string{ratios[0].Login, ratios[1].Login, ratios[2].Login, ratios[3].Login})
	assert.Equal(t, 2.0, ratios[0].Rate)
	assert.Equal(t, 0.0, ratios[3].Rate)
}

func copilotMetricsUserTestRatio(numerator, denominator int64) copilotmetrics.RatioCounts {
	return copilotmetrics.RatioCounts{
		Numerator:   copilotMetricsUserTestCounter(numerator),
		Denominator: copilotMetricsUserTestCounter(denominator),
	}
}

func copilotMetricsUserPanelTitles(panels []copilotMetricsHTMLPanel) ([]string, []bool) {
	titles := make([]string, len(panels))
	half := make([]bool, len(panels))
	for index, panel := range panels {
		titles[index] = panel.Title
		half[index] = panel.HalfWidth
	}
	return titles, half
}

func TestCopilotMetricsUserPanelsPairing(t *testing.T) {
	full := copilotmetrics.UserRow{
		Login: "fictional-user-a", ActiveDays: 3, Surfaces: []string{"chat"},
		ActivityTotals: copilotmetrics.ActivityTotals{UserInitiatedInteraction: copilotMetricsUserTestCounter(4)},
		Acceptance:     copilotMetricsUserTestRatio(1, 4),
	}
	for _, tc := range []struct {
		name   string
		users  []copilotmetrics.UserRow
		titles []string
		half   []bool
	}{
		{name: "none"},
		{
			name:   "all",
			users:  []copilotmetrics.UserRow{full},
			titles: []string{copilotMetricsUserRatioTitle, copilotMetricsUserSurfaceTitle, copilotMetricsUserDaysTitle, copilotMetricsUserAcceptanceTitle},
			half:   []bool{true, true, true, true},
		},
		{
			name:   "days only",
			users:  []copilotmetrics.UserRow{{Login: "fictional-user-b", ActiveDays: 1}},
			titles: []string{copilotMetricsUserDaysTitle},
			half:   []bool{false},
		},
		{
			name: "missing ratio and acceptance",
			users: []copilotmetrics.UserRow{{
				Login: "fictional-user-c", ActiveDays: 2, Surfaces: []string{"cli"},
			}},
			titles: []string{copilotMetricsUserSurfaceTitle, copilotMetricsUserDaysTitle},
			half:   []bool{false, false},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			titles, half := copilotMetricsUserPanelTitles(copilotMetricsUserPanels(tc.users))
			if tc.titles == nil {
				assert.Empty(t, titles)
				return
			}
			assert.Equal(t, tc.titles, titles)
			assert.Equal(t, tc.half, half)
			if tc.name == "all" {
				panels := copilotMetricsUserPanels(tc.users)
				for _, panel := range panels[:2] {
					assert.Contains(t, string(panel.Chart), copilotMetricsThemeGradientColor(copilotMetricsThemeRose, 0, 1).hex())
				}
				for _, panel := range panels[2:] {
					slices := copilotMetricsTeamPieSlices(t, panel)
					require.Len(t, slices, 1)
					assert.Equal(t, copilotMetricsThemeGradientColor(copilotMetricsThemeRose, 1, 5).hex(), slices[0].Fill)
				}
			}
		})
	}
}

func TestCopilotMetricsUserBucketBoundaries(t *testing.T) {
	classify := func(buckets []copilotMetricsUserBucket, value float64) string {
		for _, bucket := range buckets {
			if bucket.Contains(value) {
				return bucket.Name
			}
		}
		return ""
	}
	for value, want := range map[float64]string{1: "1 day", 2: "2–3 days", 3: "2–3 days", 4: "4–7 days", 7: "4–7 days", 8: "8–14 days", 14: "8–14 days", 15: "15+ days", 40: "15+ days"} {
		assert.Equal(t, want, classify(copilotMetricsUserDayBuckets, value), "days %v", value)
	}
	for value, want := range map[float64]string{0: "<20%", .1999: "<20%", .2: "20–<40%", .3999: "20–<40%", .4: "40–<60%", .6: "60–<80%", .7999: "60–<80%", .8: "≥80%", 1: "≥80%", 1.5: "≥80%"} {
		assert.Equal(t, want, classify(copilotMetricsUserAcceptanceBuckets, value), "rate %v", value)
	}
}

func TestCopilotMetricsUserBucketColorsStayStable(t *testing.T) {
	users := []copilotmetrics.UserRow{
		{Login: "fictional-user-a", ActiveDays: 1},
		{Login: "fictional-user-b", ActiveDays: 20},
	}
	panels := copilotMetricsUserPanels(users)
	require.Len(t, panels, 1)
	slices := copilotMetricsTeamPieSlices(t, panels[0])
	require.Len(t, slices, 2)
	assert.Equal(t, copilotMetricsThemeGradientColor(copilotMetricsThemeRose, 0, 5).hex(), slices[0].Fill)
	assert.Equal(t, copilotMetricsThemeGradientColor(copilotMetricsThemeRose, 4, 5).hex(), slices[1].Fill)
	assert.NotContains(t, string(panels[0].Chart), "2–3 days", "empty buckets are omitted")

	single := copilotMetricsUserPanels(users[:1])
	require.Len(t, single, 1)
	assert.Contains(t, string(single[0].Chart), `<circle class="pie-slice"`)
}

func TestCopilotMetricsUserRatioPanel(t *testing.T) {
	users := make([]copilotmetrics.UserRow, 0, 10)
	for index := range 9 {
		users = append(users, copilotmetrics.UserRow{
			Login:          fmt.Sprintf("fictional-user-%d", index),
			ActiveDays:     2,
			ActivityTotals: copilotmetrics.ActivityTotals{UserInitiatedInteraction: copilotMetricsUserTestCounter(int64(index))},
		})
	}
	users = append(users, copilotmetrics.UserRow{
		Login: "fictional-<user>", ActiveDays: 2,
		ActivityTotals: copilotmetrics.ActivityTotals{UserInitiatedInteraction: copilotMetricsUserTestCounter(100)},
	})
	ratios := copilotMetricsUserRatios(users)
	require.Len(t, ratios, copilotMetricsUserRatioChartLimit)
	assert.Equal(t, "fictional-<user>", ratios[0].Login)
	assert.Equal(t, "fictional-user-8", ratios[1].Login)
	panel := copilotMetricsUserRatioPanel(users)
	chart := string(panel.Chart)
	assert.Contains(t, chart, "fictional-&lt;user&gt;")
	assert.NotContains(t, chart, "fictional-<user>")
	assert.Contains(t, chart, "Interactions per active day: 50.00")
	assert.Contains(t, chart, "Interactions: 100")
	assert.Contains(t, chart, "Active days: 2")
	assert.NotContains(t, chart, "Code generation")
	assert.NotContains(t, chart, "Other")

	zero := copilotMetricsUserRatioPanel([]copilotmetrics.UserRow{{
		Login: "fictional-user-z", ActiveDays: 5,
		ActivityTotals: copilotmetrics.ActivityTotals{UserInitiatedInteraction: copilotMetricsUserTestCounter(0)},
	}})
	require.NotEmpty(t, zero.Chart)
	assert.NotContains(t, string(zero.Chart), "NaN")
	assert.NotContains(t, string(zero.Chart), "Inf")
}

func TestCopilotMetricsUserSurfacePanel(t *testing.T) {
	panel := copilotMetricsUserSurfacePanel([]copilotmetrics.UserRow{
		{Login: "fictional-user-a", Surfaces: []string{"chat", "cli"}},
		{Login: "fictional-user-b", Surfaces: []string{"chat"}},
		{Login: "fictional-user-c"},
		{Login: "fictional-user-d"},
	})
	chart := string(panel.Chart)
	assert.Contains(t, chart, `data-tooltip-fields="heading=chat&amp;label=Users&amp;note=Share+of+reported+users.&amp;share=50.0%25&amp;value=2"`)
	assert.Contains(t, chart, `data-tooltip-fields="heading=cli&amp;label=Users&amp;note=Share+of+reported+users.&amp;share=25.0%25&amp;value=1"`)
	assert.Less(t, strings.Index(chart, ">chat</text>"), strings.Index(chart, ">code-review</text>"))
	assert.Empty(t, copilotMetricsUserSurfacePanel([]copilotmetrics.UserRow{{Login: "fictional-user-e"}}).Chart)

	odd := string(copilotMetricsUserSurfacePanel([]copilotmetrics.UserRow{
		{Surfaces: []string{"chat"}}, {Surfaces: []string{"chat"}}, {Surfaces: []string{"chat"}},
	}).Chart)
	assert.Contains(t, odd, `text-anchor="end">2</text>`, "odd maxima round up so the midpoint tick is exact")
	assert.Contains(t, odd, `text-anchor="end">4</text>`)
}

func TestUserRichPanelsInReports(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04"}
	ds := &copilotmetrics.Dataset{
		Days: days,
		Users: []copilotmetrics.UserDay{
			{UserID: 1, Login: "fictional-user-a", Day: days[0], UserInitiatedInteractions: new(4), CodeGenerationActivities: new(8), CodeAcceptanceActivities: new(2), UsedChat: true},
			{UserID: 1, Login: "fictional-user-a", Day: days[1], UserInitiatedInteractions: new(2), CodeGenerationActivities: new(2), CodeAcceptanceActivities: new(2), UsedAgent: true},
			{UserID: 2, Login: "fictional-user-b", Day: days[0], UserInitiatedInteractions: new(5), CodeGenerationActivities: new(5), CodeAcceptanceActivities: new(5), UsedCLI: true},
			{UserID: 1, Login: "fictional-user-a", Day: days[2], UserInitiatedInteractions: new(0)},
			{UserID: 1, Login: "fictional-user-a", Day: days[3]},
		},
	}
	summary := copilotmetrics.BuildSummary(ds, copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionUser: true}, Top: 1, IncludeRichMetrics: true,
	})
	require.Len(t, summary.Users, 1)
	html, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, html, `<div class="rich-panels rich-panels-paired">`)
	order := []string{copilotMetricsUserRatioTitle, copilotMetricsUserSurfaceTitle, copilotMetricsUserDaysTitle, copilotMetricsUserAcceptanceTitle}
	last := -1
	for _, title := range order {
		index := strings.Index(html, `<figure class="rich-panel rich-panel-half"><figcaption>`+title+`</figcaption>`)
		require.Greater(t, index, last, title)
		last = index
	}
	assert.Contains(t, html, "fictional-user-b", "charts include users beyond --top")
	ratios := copilotMetricsUserRatios(summary.AllUsers)
	require.Len(t, ratios, 2)
	assert.Equal(t, "fictional-user-b", ratios[0].Login)
	assert.Equal(t, 5.0, ratios[0].Rate)
	assert.Equal(t, "fictional-user-a", ratios[1].Login)
	assert.Equal(t, 1.5, ratios[1].Rate, "zero and missing interaction days still count as active days")
	assert.Equal(t, int64(6), ratios[1].Interactions)
	assert.Equal(t, 4, ratios[1].ActiveDays)
	assert.Contains(t, html, "Interactions per active day: 1.50")
	assert.NotContains(t, html, "Code generation per interaction")
	assert.Contains(t, html, `data-tooltip="40–&lt;60%: 1 (50.0%)"`)
	assert.Contains(t, html, `data-tooltip="≥80%: 1 (50.0%)"`)

	plain, err := RenderCopilotMetricsHTML(summary)
	require.NoError(t, err)
	assert.NotContains(t, plain, "<figcaption>"+copilotMetricsUserRatioTitle)
	assert.NotContains(t, RenderCopilotMetricsMarkdown(summary), copilotMetricsUserRatioTitle)
}

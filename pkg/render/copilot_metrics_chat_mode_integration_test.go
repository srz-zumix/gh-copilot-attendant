package render

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatModeChartFromDailyFeatureReports(t *testing.T) {
	feature := func(name string, count *int) *github.CopilotMetricsFeature {
		return &github.CopilotMetricsFeature{Feature: name, UserInitiatedInteractionCount: count}
	}
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04"}
	var firstDay github.CopilotDailyMetrics
	require.NoError(t, json.Unmarshal([]byte(`{
		"day":"2030-01-01",
		"daily_active_users":2,
		"user_initiated_interaction_count":118,
		"totals_by_feature":[
			{"feature":"chat_panel_agent_mode","user_initiated_interaction_count":10},
			{"feature":"chat_panel_ask_mode","user_initiated_interaction_count":4},
			{"feature":"chat_panel_custom_mode","user_initiated_interaction_count":0},
			{"feature":"chat_panel_edit_mode","user_initiated_interaction_count":2},
			{"feature":"chat_panel_plan_mode","user_initiated_interaction_count":1},
			{"feature":"chat_panel_unknown_mode","user_initiated_interaction_count":1},
			{"feature":"chat_inline","user_initiated_interaction_count":100}
		]
	}`), &firstDay))
	ds := &copilotmetrics.Dataset{
		Days: days,
		OrgDays: []*github.CopilotDailyMetrics{
			&firstDay,
			{
				Day: days[1],
				TotalsByFeature: []*github.CopilotMetricsFeature{
					feature("chat_panel_agent_mode", new(3)),
					feature("chat_panel_ask_mode", nil),
					feature("chat_panel_custom_mode", new(0)),
					feature("chat_panel_edit_mode", new(0)),
					feature("chat_panel_plan_mode", new(0)),
					feature("chat_panel_unknown_mode", new(0)),
				},
			},
			{Day: days[2]},
		},
	}
	summary := copilotmetrics.BuildSummary(ds, copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionTrend: true}, IncludeRichMetrics: true,
	})
	html, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)

	average := strings.Index(html, "<figcaption>Average chat requests per active user</figcaption>")
	start := strings.Index(html, "<figcaption>Requests per chat mode</figcaption>")
	require.GreaterOrEqual(t, average, 0)
	require.Greater(t, start, average)
	assert.Equal(t, 1, strings.Count(html[average:start], "<figcaption>"))
	end := strings.Index(html[start:], "</figure>")
	require.Greater(t, end, 0)
	figure := html[start : start+end]
	assert.Contains(t, figure, "* marks incomplete days; bars show known counts only.")
	assert.Contains(t, html, `.repository-incomplete-marker, .daily-stacked-incomplete-marker { fill: var(--warn); font-size: 10px; font-weight: 700; opacity: .75; }`)
	assert.Equal(t, 3, strings.Count(figure, `class="daily-stacked-incomplete-marker"`))
	var viewWidth, viewHeight float64
	_, err = fmt.Sscanf(figure[strings.Index(figure, `viewBox="`)+len(`viewBox="`):], "0 0 %g %g", &viewWidth, &viewHeight)
	require.NoError(t, err)
	markers := regexp.MustCompile(`class="daily-stacked-incomplete-marker" x="[^"]+" y="([^"]+)"`).FindAllStringSubmatch(figure, -1)
	require.Len(t, markers, 3)
	for _, marker := range markers {
		markerY, parseErr := strconv.ParseFloat(marker[1], 64)
		require.NoError(t, parseErr)
		assert.Greater(t, markerY, viewHeight-38, "marker should sit below the x-axis")
		assert.Less(t, markerY, viewHeight-10, "marker should sit above the date labels")
	}

	svgStart := strings.Index(figure, "<svg ")
	svgEnd := strings.Index(figure, "</svg>")
	require.GreaterOrEqual(t, svgStart, 0)
	require.Greater(t, svgEnd, svgStart)
	var document struct {
		Days []struct {
			Rects []struct {
				Class  string `xml:"class,attr"`
				Fill   string `xml:"fill,attr"`
				Fields string `xml:"data-tooltip-fields,attr"`
			} `xml:"rect"`
		} `xml:"g"`
	}
	require.NoError(t, xml.Unmarshal([]byte(figure[svgStart:svgEnd+len("</svg>")]), &document))
	require.Len(t, document.Days, len(days))
	first := document.Days[0].Rects
	require.Len(t, first, 6)
	assert.Equal(t, copilotMetricsThemeRamps[copilotMetricsThemeGreen][0].hex(), first[0].Fill)
	assert.Equal(t, copilotMetricsOtherColor.hex(), first[len(first)-2].Fill)
	fields, err := url.ParseQuery(first[len(first)-1].Fields)
	require.NoError(t, err)
	assert.Equal(t, days[0], fields.Get("heading"))
	assert.Equal(t, []string{"Agent", "Ask", "Edit", "Plan", "Custom", "Other", "Total"}, fields["label"])
	assert.Equal(t, []string{"10", "4", "2", "1", "0", "1", "18"}, fields["value"])
	assert.Equal(t, []string{"55.6%", "22.2%", "11.1%", "5.6%", "0.0%", "5.6%", "100.0%"}, fields["share"])
	assert.Empty(t, fields.Get("note"))

	partial := document.Days[1].Rects
	require.Len(t, partial, 2)
	fields, err = url.ParseQuery(partial[len(partial)-1].Fields)
	require.NoError(t, err)
	assert.Equal(t, days[1], fields.Get("heading"))
	assert.Equal(t, []string{"Agent", "Ask", "Edit", "Plan", "Custom", "Other", "Displayed subtotal"}, fields["label"])
	assert.Equal(t, []string{"3", "n/a", "0", "0", "0", "0", "3"}, fields["value"])
	assert.Equal(t, []string{"", "", "", "", "", "", ""}, fields["share"])
	assert.Equal(t, "Some chat mode counts are unavailable; the total and shares are not calculated.", fields.Get("note"))

	for _, day := range document.Days[2:] {
		require.Len(t, day.Rects, 1, "an unavailable day retains its tooltip")
		fields, err = url.ParseQuery(day.Rects[0].Fields)
		require.NoError(t, err)
		assert.Equal(t, []string{"n/a", "n/a", "n/a", "n/a", "n/a", "n/a", "n/a"}, fields["value"])
		assert.Equal(t, "Displayed subtotal", fields["label"][6])
	}

	plain, err := RenderCopilotMetricsHTML(summary)
	require.NoError(t, err)
	assert.NotContains(t, plain, "<figcaption>Requests per chat mode</figcaption>")
}

func TestChatModeChartWithOnlyUnavailableCounts(t *testing.T) {
	ds := &copilotmetrics.Dataset{
		Days: []string{"2030-01-01"},
		OrgDays: []*github.CopilotDailyMetrics{{
			Day: "2030-01-01",
			TotalsByFeature: []*github.CopilotMetricsFeature{{
				Feature: "chat_panel_plan_mode",
			}},
		}},
	}
	summary := copilotmetrics.BuildSummary(ds, copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionTrend: true}, IncludeRichMetrics: true,
	})
	html, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, html, "<figcaption>Requests per chat mode</figcaption>")
	assert.Contains(t, html, "Plan: n/a\nDisplayed subtotal: n/a")
	assert.Contains(t, html, `class="daily-stacked-incomplete-marker"`)

	ds.OrgDays[0].TotalsByFeature = nil
	summary = copilotmetrics.BuildSummary(ds, copilotmetrics.SummaryOptions{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionTrend: true}, IncludeRichMetrics: true,
	})
	html, err = RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.NotContains(t, html, "<figcaption>Requests per chat mode</figcaption>")
}

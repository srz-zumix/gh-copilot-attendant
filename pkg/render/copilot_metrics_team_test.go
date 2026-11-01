package render

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type copilotMetricsTeamPieSlice struct {
	Fill   string `xml:"fill,attr"`
	Fields string `xml:"data-tooltip-fields,attr"`
}

func copilotMetricsTeamPieSlices(t *testing.T, panel copilotMetricsHTMLPanel) []copilotMetricsTeamPieSlice {
	t.Helper()
	var document struct {
		Div struct {
			SVG struct {
				Paths   []copilotMetricsTeamPieSlice `xml:"path"`
				Circles []copilotMetricsTeamPieSlice `xml:"circle"`
			} `xml:"svg"`
		} `xml:"div"`
	}
	require.NoError(t, xml.Unmarshal([]byte("<root>"+string(panel.Chart)+"</root>"), &document))
	return append(document.Div.SVG.Paths, document.Div.SVG.Circles...)
}

func TestCopilotMetricsTeamPanelsAvailability(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	for _, tc := range []struct {
		name         string
		interactions copilotmetrics.Counter
		generation   copilotmetrics.Counter
		titles       []string
	}{
		{name: "missing"},
		{name: "zero", interactions: valid(0), generation: valid(0)},
		{name: "negative", interactions: valid(-1), generation: valid(-2)},
		{name: "interactions only", interactions: valid(5), titles: []string{copilotMetricsTeamInteractionsTitle}},
		{name: "generation only", generation: valid(7), titles: []string{copilotMetricsTeamCodeGenerationTitle}},
		{name: "zero generation", interactions: valid(5), generation: valid(0), titles: []string{copilotMetricsTeamInteractionsTitle}},
		{name: "both", interactions: valid(5), generation: valid(7), titles: []string{copilotMetricsTeamInteractionsTitle, copilotMetricsTeamCodeGenerationTitle}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panels := copilotMetricsTeamPanels([]copilotmetrics.TeamRow{{
				TeamID: 1, Slug: "sample-team", ActiveUsers: 100,
				ActivityTotals: copilotmetrics.ActivityTotals{
					UserInitiatedInteraction: tc.interactions, CodeGeneration: tc.generation,
					CodeAcceptance: valid(999), LOCAdded: valid(1000),
				},
			}})
			require.Len(t, panels, len(tc.titles))
			for index, panel := range panels {
				assert.Equal(t, tc.titles[index], panel.Title)
				assert.Equal(t, len(panels) == 2, panel.HalfWidth)
				assert.Contains(t, string(panel.Chart), "Shares of reported team counts; missing values excluded.")
				slices := copilotMetricsTeamPieSlices(t, panel)
				require.Len(t, slices, 1)
				assert.Equal(t, copilotMetricsThemeGradientColor(copilotMetricsThemePurple, 0, 1).hex(), slices[0].Fill)
				fields, err := url.ParseQuery(slices[0].Fields)
				require.NoError(t, err)
				assert.Equal(t, []string{"sample-team"}, fields["label"])
				assert.Equal(t, []string{"100.0%"}, fields["share"])
				want := tc.interactions.Value
				if panel.Title == copilotMetricsTeamCodeGenerationTitle {
					want = tc.generation.Value
				}
				assert.Equal(t, []string{strconv.FormatInt(want, 10)}, fields["value"])
				assert.Empty(t, fields.Get("heading"), "tooltips do not repeat chart titles")
				assert.Contains(t, string(panel.Chart), `tabindex="0" aria-label=`)
			}
		})
	}
	assert.Empty(t, copilotMetricsTeamPanels(nil))
}

func TestCopilotMetricsTeamPiesRankAndGroupIndependently(t *testing.T) {
	values := []int64{500, 200, 100, 60, 40, 30, 20, 10, 6, 4}
	teams := make([]copilotmetrics.TeamRow, len(values))
	for index, value := range values {
		teams[index] = copilotmetrics.TeamRow{
			TeamID: int64(index + 1), Slug: fmt.Sprintf("team-%c", 'a'+index),
			ActivityTotals: copilotmetrics.ActivityTotals{
				UserInitiatedInteraction: copilotmetrics.Counter{Value: value, Valid: true},
				CodeGeneration:           copilotmetrics.Counter{Value: values[len(values)-1-index], Valid: true},
			},
		}
	}
	panels := copilotMetricsTeamPanels(teams)
	require.Len(t, panels, 2)
	for index, panel := range panels {
		slices := copilotMetricsTeamPieSlices(t, panel)
		require.Len(t, slices, len(copilotMetricsChartColors))
		var total int64
		var totalShare float64
		for rank, slice := range slices {
			fields, err := url.ParseQuery(slice.Fields)
			require.NoError(t, err)
			require.Len(t, fields["label"], 1)
			require.Len(t, fields["value"], 1)
			require.Len(t, fields["share"], 1)
			value, err := strconv.ParseInt(strings.ReplaceAll(fields["value"][0], ",", ""), 10, 64)
			require.NoError(t, err)
			share, err := strconv.ParseFloat(strings.TrimSuffix(fields["share"][0], "%"), 64)
			require.NoError(t, err)
			total += value
			totalShare += share
			if rank == 0 {
				want := "team-a"
				if index == 1 {
					want = "team-j"
				}
				assert.Equal(t, want, fields["label"][0])
				assert.Equal(t, int64(500), value)
			}
			if fields["label"][0] == "Other" {
				assert.Equal(t, int64(40), value, "small shares and excess categories are counted exactly once")
				assert.Equal(t, copilotMetricsOtherColor.hex(), slice.Fill)
			}
		}
		assert.Equal(t, int64(970), total)
		assert.InDelta(t, 100, totalShare, .35)
		assert.Equal(t, 1, strings.Count(string(panel.Chart), "<span>Other</span>"))
	}
	for left, right := 0, len(teams)-1; left < right; left, right = left+1, right-1 {
		teams[left], teams[right] = teams[right], teams[left]
	}
	assert.Equal(t, panels, copilotMetricsTeamPanels(teams), "ranking does not depend on input order")
}

func TestCopilotMetricsTeamPieDisplayNames(t *testing.T) {
	names := []string{"same", "same", "", copilotMetricsUnreportedDimension, `<script>"unsafe"</script>`}
	teams := make([]copilotmetrics.TeamRow, len(names))
	for index, name := range names {
		teams[index] = copilotmetrics.TeamRow{
			TeamID: int64(index + 1), Slug: name,
			ActivityTotals: copilotmetrics.ActivityTotals{
				UserInitiatedInteraction: copilotmetrics.Counter{Value: 100, Valid: true},
			},
		}
	}
	panels := copilotMetricsTeamPanels(teams)
	require.Len(t, panels, 1)
	slices := copilotMetricsTeamPieSlices(t, panels[0])
	require.Len(t, slices, len(names))
	var labels []string
	for _, slice := range slices {
		fields, err := url.ParseQuery(slice.Fields)
		require.NoError(t, err)
		labels = append(labels, fields["label"]...)
	}
	assert.ElementsMatch(t, []string{
		"same (team 1)", "same (team 2)",
		"(not reported) (team 3)", "(not reported) (team 4)", names[4],
	}, labels)
	assert.NotContains(t, string(panels[0].Chart), "<script>")
	assert.Contains(t, string(panels[0].Chart), "&lt;script&gt;&#34;unsafe&#34;&lt;/script&gt;")

	for _, name := range []string{"Other", "others", "UNKNOWN", "unkown", " unknown "} {
		t.Run(name, func(t *testing.T) {
			team := teams[0]
			team.Slug = name
			panels := copilotMetricsTeamPanels([]copilotmetrics.TeamRow{team})
			require.Len(t, panels, 1)
			slices := copilotMetricsTeamPieSlices(t, panels[0])
			require.Len(t, slices, 1)
			fields, err := url.ParseQuery(slices[0].Fields)
			require.NoError(t, err)
			assert.Equal(t, []string{name + " (team 1)"}, fields["label"])
			assert.NotEqual(t, copilotMetricsOtherColor.hex(), slices[0].Fill)
		})
	}
}

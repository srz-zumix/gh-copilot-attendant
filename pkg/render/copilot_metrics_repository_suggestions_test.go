package render

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotMetricsRepositorySuggestionBars(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	repositories := []copilotmetrics.RichRepository{
		{
			ID: 1, Name: `sample/repository-with-a-very-long-name-<script>`,
			Suggestions: counter(3), SuggestionsComplete: false,
			AppliedSuggestions: counter(9), AppliedSuggestionsComplete: true,
		},
		{
			ID: 2, Name: "sample/zero",
			Suggestions: counter(0), SuggestionsComplete: true,
		},
	}
	bars := copilotMetricsRepositorySuggestionBars(repositories)
	chart := string(copilotMetricsSVGRepositorySuggestionBars(bars))
	assert.Contains(t, chart, `<p class="chart-color-key">Ranked by known generated counts. * marks partial totals. Generated and applied suggestions may differ.</p>`)
	assert.Contains(t, chart, "Generated suggestions: 3 (partial)\nApplied suggestions: 9")
	assert.Contains(t, chart, "Applied suggestions: n/a")
	assert.Contains(t, chart, "may refer to different suggestions")
	assert.Contains(t, chart, `class="repository-incomplete-marker"`)
	assert.Contains(t, chart, `class="repository-chart-hit"`)
	assert.Contains(t, chart, `tabindex="0" role="img"`)
	assert.Contains(t, chart, "&lt;script&gt;")
	assert.NotContains(t, chart, "<script>")
	assert.NotContains(t, chart, "Acceptance rate")

	type rect struct {
		Class string  `xml:"class,attr"`
		Width float64 `xml:"width,attr"`
		Fill  string  `xml:"fill,attr"`
	}
	var document struct {
		SVG struct {
			Bars []struct {
				Fields string `xml:"data-tooltip-fields,attr"`
				Rects  []rect `xml:"rect"`
			} `xml:"g"`
		} `xml:"svg"`
	}
	require.NoError(t, xml.Unmarshal([]byte("<root>"+chart+"</root>"), &document))
	require.Len(t, document.SVG.Bars, 2)
	first := document.SVG.Bars[0]
	require.Len(t, first.Rects, 3, "a hit target and two independent bars are drawn")
	assert.Equal(t, "repository-chart-hit", first.Rects[0].Class)
	assert.InDelta(t, 162, first.Rects[1].Width, .1)
	assert.InDelta(t, 486, first.Rects[2].Width, .1, "applied can exceed generated")
	assert.Equal(t, copilotMetricsThemeRamps[copilotMetricsThemeGreen][0].hex(), first.Rects[1].Fill)
	assert.Equal(t, copilotMetricsThemeRamps[copilotMetricsThemeGreen][1].hex(), first.Rects[2].Fill)
	fields, err := url.ParseQuery(first.Fields)
	require.NoError(t, err)
	assert.Equal(t, repositories[0].Name, fields.Get("heading"))
	assert.Equal(t, []string{"Generated suggestions", "Applied suggestions"}, fields["label"])
	assert.Equal(t, []string{"3 (partial)", "9"}, fields["value"])
	assert.Equal(t, []string{"", ""}, fields["share"])
	assert.Equal(t, "Generated and applied suggestions are counted on their respective days and may refer to different suggestions.", fields.Get("note"))
}

func TestCopilotMetricsRepositorySuggestionsTopAndPartialOther(t *testing.T) {
	var repositories []copilotmetrics.RichRepository
	for index := 1; index <= copilotMetricsRepositoryChartLimit+2; index++ {
		repository := copilotmetrics.RichRepository{
			ID: int64(index), Name: fmt.Sprintf("sample/repo-%02d", index),
			Suggestions:                copilotmetrics.Counter{Value: int64(index), Valid: true},
			SuggestionsComplete:        true,
			AppliedSuggestions:         copilotmetrics.Counter{Value: int64(index * 10), Valid: true},
			AppliedSuggestionsComplete: true,
		}
		if index == 1 {
			repository.Suggestions = copilotmetrics.Counter{}
			repository.SuggestionsComplete = false
		}
		if index == 2 {
			repository.AppliedSuggestionsComplete = false
		}
		repositories = append(repositories, repository)
	}
	bars := copilotMetricsRepositorySuggestionBars(repositories)
	require.Len(t, bars, copilotMetricsRepositoryChartLimit+1)
	assert.Equal(t, "sample/repo-09", bars[0].Name)
	other := bars[len(bars)-1]
	assert.Equal(t, "Other repositories", other.Name)
	assert.Equal(t, copilotmetrics.Counter{Value: 2, Valid: true}, other.Generated)
	assert.False(t, other.GeneratedFull)
	assert.Equal(t, copilotmetrics.Counter{Value: 30, Valid: true}, other.Applied)
	assert.False(t, other.AppliedFull)
	chart := string(copilotMetricsSVGRepositorySuggestionBars(bars))
	assert.Contains(t, chart, "Other repositories\nGenerated suggestions: 2 (partial)\nApplied suggestions: 30 (partial)")
	assert.Empty(t, copilotMetricsSVGRepositorySuggestionBars(nil))
	assert.Empty(t, copilotMetricsRepositorySuggestionsPanel(nil).Chart)
	assert.Equal(t, 0, strings.Count(chart, "Acceptance rate"))
}

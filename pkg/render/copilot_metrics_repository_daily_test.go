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

func TestCopilotMetricsRepositoryDailyPanelCompleteness(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	rich := &copilotmetrics.RichMetrics{
		Days: []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04"},
		Repositories: []copilotmetrics.RichRepository{
			{ID: 1, Name: "sample/alpha<script>", Reviews: []copilotmetrics.Counter{counter(4), counter(0), {}, {}}},
			{ID: 2, Name: "sample/beta", Reviews: []copilotmetrics.Counter{counter(2), counter(0), counter(3), {}}},
		},
	}
	panel := copilotMetricsRepositoryDailyPanel(rich)
	assert.Equal(t, copilotMetricsRepositoryDailyReviewsTitle, panel.Title)
	chart := string(panel.Chart)
	assert.Contains(t, chart, `class="stacked-legend"`)
	assert.Contains(t, chart, "sample/alpha&lt;script&gt;")
	assert.NotContains(t, chart, "<script>")
	assert.NotContains(t, chart, `<text class="legend"`, "repository names use the wrapping HTML legend")
	assert.Equal(t, 2, strings.Count(chart, `class="daily-stacked-incomplete-marker"`))
	assert.Contains(t, chart, "bars omit categories with missing values")

	type rect struct {
		Class  string `xml:"class,attr"`
		Fill   string `xml:"fill,attr"`
		Fields string `xml:"data-tooltip-fields,attr"`
		Label  string `xml:"aria-label,attr"`
	}
	var document struct {
		SVG struct {
			Days []struct {
				Rects []rect `xml:"rect"`
			} `xml:"g"`
		} `xml:"svg"`
	}
	require.NoError(t, xml.Unmarshal([]byte("<root>"+chart+"</root>"), &document))
	require.Len(t, document.SVG.Days, 4)
	first := document.SVG.Days[0].Rects
	require.Len(t, first, 3)
	assert.Equal(t, copilotMetricsThemeRamps[copilotMetricsThemeGreen][0].hex(), first[0].Fill)
	assert.Equal(t, copilotMetricsThemeRamps[copilotMetricsThemeGreen][1].hex(), first[1].Fill)
	fields, err := url.ParseQuery(first[len(first)-1].Fields)
	require.NoError(t, err)
	assert.Equal(t, []string{"sample/beta", "sample/alpha<script>", "Total"}, fields["label"])
	assert.Equal(t, []string{"2", "4", "6"}, fields["value"])
	assert.Equal(t, []string{"33.3%", "66.7%", "100.0%"}, fields["share"])
	assert.NotContains(t, first[len(first)-1].Label, copilotMetricsRepositoryDailyReviewsTitle)

	zero := document.SVG.Days[1].Rects
	require.Len(t, zero, 1)
	fields, err = url.ParseQuery(zero[0].Fields)
	require.NoError(t, err)
	assert.Equal(t, []string{"0", "0", "0"}, fields["value"])
	assert.Equal(t, []string{"n/a", "n/a", "n/a"}, fields["share"])

	partial := document.SVG.Days[2].Rects
	require.Len(t, partial, 2)
	assert.Equal(t, first[0].Fill, partial[0].Fill, "a repository keeps one color across dates")
	fields, err = url.ParseQuery(partial[len(partial)-1].Fields)
	require.NoError(t, err)
	assert.Equal(t, []string{"3", "n/a", "3"}, fields["value"])
	assert.Equal(t, []string{"", "", ""}, fields["share"])
	assert.Equal(t, "Displayed subtotal", fields["label"][2])
	assert.Contains(t, fields.Get("note"), "total and shares are not calculated")

	missing := document.SVG.Days[3].Rects
	require.Len(t, missing, 1, "an unavailable day still has a focusable tooltip")
	fields, err = url.ParseQuery(missing[0].Fields)
	require.NoError(t, err)
	assert.Equal(t, []string{"n/a", "n/a", "n/a"}, fields["value"])
	assert.Contains(t, missing[0].Label, "total and shares are not calculated")
}

func TestCopilotMetricsRepositoryDailyTopAndOther(t *testing.T) {
	var repositories []copilotmetrics.RichRepository
	for index := 1; index <= copilotMetricsRepositoryChartLimit+2; index++ {
		repository := copilotmetrics.RichRepository{
			ID: int64(index), Name: fmt.Sprintf("sample/repo-%02d", index),
			Reviews: []copilotmetrics.Counter{
				{Value: int64(index), Valid: true}, {Value: int64(index), Valid: true},
			},
		}
		if index == 2 {
			repository.Reviews[1] = copilotmetrics.Counter{}
		}
		repositories = append(repositories, repository)
	}
	series := copilotMetricsRepositoryReviewSeries(repositories, 2)
	require.Len(t, series, copilotMetricsRepositoryChartLimit+1)
	assert.Equal(t, "sample/repo-09", series[0].Name)
	other := series[len(series)-1]
	assert.Equal(t, "Other", other.Name)
	assert.Equal(t, []float64{3, 1}, other.Values)
	assert.Equal(t, []bool{true, false}, other.Valid, "Other is incomplete if any contributor is missing")

	panel := copilotMetricsRepositoryDailyPanel(&copilotmetrics.RichMetrics{
		Days: []string{"2030-01-01", "2030-01-02"}, Repositories: repositories,
	})
	chart := string(panel.Chart)
	assert.Contains(t, chart, `fill="`+copilotMetricsOtherColor.hex()+`"`)
	assert.Contains(t, chart, "Other: 3 (6.7%)")
	assert.Contains(t, chart, "Other: n/a\nDisplayed subtotal: 42")

	for left, right := 0, len(repositories)-1; left < right; left, right = left+1, right-1 {
		repositories[left], repositories[right] = repositories[right], repositories[left]
	}
	assert.Equal(t, series, copilotMetricsRepositoryReviewSeries(repositories, 2), "ranking is deterministic")
}

func TestCopilotMetricsRepositoryDailyPanelEmptyAndAllUnknown(t *testing.T) {
	assert.Empty(t, copilotMetricsRepositoryDailyPanel(nil).Chart)
	assert.Empty(t, copilotMetricsRepositoryDailyPanel(&copilotmetrics.RichMetrics{
		Days: []string{"2030-01-01"},
	}).Chart)
	panel := copilotMetricsRepositoryDailyPanel(&copilotmetrics.RichMetrics{
		Days: []string{"2030-01-01"}, Repositories: []copilotmetrics.RichRepository{{ID: 1, Name: "sample/repo", Reviews: []copilotmetrics.Counter{{}}}},
	})
	assert.Contains(t, string(panel.Chart), "sample/repo: n/a\nDisplayed subtotal: n/a")
	assert.Contains(t, string(panel.Chart), `class="daily-stacked-incomplete-marker"`)
}

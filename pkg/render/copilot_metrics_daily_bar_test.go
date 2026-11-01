package render

import (
	"encoding/xml"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotMetricsSVGDailyStackedBarChart(t *testing.T) {
	type rect struct {
		Class  string  `xml:"class,attr"`
		X      float64 `xml:"x,attr"`
		Y      float64 `xml:"y,attr"`
		Width  float64 `xml:"width,attr"`
		Height float64 `xml:"height,attr"`
		Fill   string  `xml:"fill,attr"`
		Fields string  `xml:"data-tooltip-fields,attr"`
		Label  string  `xml:"aria-label,attr"`
	}
	type day struct {
		Class string `xml:"class,attr"`
		Rects []rect `xml:"rect"`
	}
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03"}
	series := []copilotMetricsPlotSeries{
		{Name: "mode-a", Values: []float64{30, 0, 0}, Valid: []bool{true, false, false}},
		{Name: "mode-b", Values: []float64{10, 0, 0}, Valid: []bool{true, true, false}},
		{Name: "Other", Values: []float64{0, 0, 0}, Valid: []bool{true, true, false}},
	}
	chart := string(copilotMetricsDailyStackedBarChart("Requests & modes", days, series, copilotMetricsThemeGreen, copilotMetricsDailyStackedBarOptions{}))
	var svg struct {
		Role      string `xml:"role,attr"`
		AriaLabel string `xml:"aria-label,attr"`
		Days      []day  `xml:"g"`
	}
	require.NoError(t, xml.Unmarshal([]byte(chart), &svg))
	assert.Equal(t, "group", svg.Role)
	assert.Equal(t, "Requests & modes", svg.AriaLabel)
	require.Len(t, svg.Days, 2, "days without any value have no bar")

	first := svg.Days[0]
	assert.Equal(t, "chart-day daily-stacked-bar", first.Class)
	require.Len(t, first.Rects, 3, "zero values add no segment")
	green := copilotMetricsThemeRamps[copilotMetricsThemeGreen]
	assert.Equal(t, green[0].hex(), first.Rects[0].Fill)
	assert.Equal(t, green[1].hex(), first.Rects[1].Fill)
	assert.InDelta(t, first.Rects[0].Y, first.Rects[1].Y+first.Rects[1].Height, .11, "segments stack from the bottom up")
	assert.InDelta(t, 3*first.Rects[1].Height, first.Rects[0].Height, .2)
	hit := first.Rects[2]
	assert.Equal(t, "metric-day-hit", hit.Class)
	fields, err := url.ParseQuery(hit.Fields)
	require.NoError(t, err)
	assert.Equal(t, "2030-01-01", fields.Get("heading"))
	assert.Equal(t, []string{"mode-a", "mode-b", "Other", "Total"}, fields["label"])
	assert.Equal(t, []string{"30", "10", "0", "40"}, fields["value"])
	assert.Equal(t, []string{"75.0%", "25.0%", "0.0%", "100.0%"}, fields["share"])
	assert.NotContains(t, hit.Label, "Requests & modes", "tooltips omit the chart title")

	second, err := url.ParseQuery(svg.Days[1].Rects[len(svg.Days[1].Rects)-1].Fields)
	require.NoError(t, err)
	assert.Equal(t, []string{"n/a", "0", "0", "0"}, second["value"])
	assert.Equal(t, []string{"", "n/a", "n/a", "n/a"}, second["share"])

	assert.Contains(t, chart, `fill="`+copilotMetricsOtherColor.hex()+`" aria-hidden="true"/><text class="legend"`)
	assert.Equal(t, 3, strings.Count(chart, `class="area-legend-key"`))
}

func TestCopilotMetricsSVGDailyStackedBarChartEmpty(t *testing.T) {
	assert.Empty(t, copilotMetricsDailyStackedBarChart("Requests", nil, nil, copilotMetricsThemeGreen, copilotMetricsDailyStackedBarOptions{}))
	assert.Empty(t, copilotMetricsDailyStackedBarChart("Requests", []string{"2030-01-01"},
		[]copilotMetricsPlotSeries{{Name: "mode-a", Values: []float64{0}, Valid: []bool{false}}}, copilotMetricsThemeGreen, copilotMetricsDailyStackedBarOptions{}))
}

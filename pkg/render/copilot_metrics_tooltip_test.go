package render

import (
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotMetricsChartsHaveNoNativeTitleTooltip(t *testing.T) {
	const title = "Chart & title"
	days := []string{"2030-01-01"}
	series := []copilotMetricsPlotSeries{{Name: "series-a", Values: []float64{100}, Valid: []bool{true}}}
	values := []copilotmetrics.RichValue{
		{Name: "model-a", Group: "language-a", Value: copilotmetrics.Counter{Value: 3, Valid: true}},
		{Name: "model-b", Group: "language-a", Value: copilotmetrics.Counter{Value: 1, Valid: true}},
	}
	tests := []struct {
		name  string
		chart template.HTML
		role  string
	}{
		{"normal line", copilotMetricsSVGChart(title, "line", days, []int64{3}, []bool{true}), "group"},
		{"normal filled line", copilotMetricsSVGChart(title, "area", days, []int64{3}, []bool{true}), "group"},
		{"normal bar", copilotMetricsSVGChart(title, "bar", days, []int64{3}, []bool{true}), "img"},
		{"rich line", copilotMetricsSVGMultiLineChart(title, days, series), "group"},
		{"stacked area", copilotMetricsSVGStackedAreaChart(title, days, series, nil), "group"},
		{"stacked bar", copilotMetricsHTMLRichStackedPanel(title, values, "Language").Chart, "group"},
		{"pie slices", copilotMetricsHTMLRichPiePanel(title, values).Chart, "img"},
		{"pie circle", copilotMetricsHTMLRichPiePanel(title, values[:1]).Chart, "img"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := xml.NewDecoder(strings.NewReader(string(tt.chart)))
			depth, svgDepth, svgCount, pointTitles := 0, -1, 0, 0
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				switch element := token.(type) {
				case xml.StartElement:
					if element.Name.Local == "svg" {
						svgDepth = depth
						svgCount++
						attributes := make(map[string]string, len(element.Attr))
						for _, attr := range element.Attr {
							attributes[attr.Name.Local] = attr.Value
						}
						assert.Equal(t, title, attributes["aria-label"])
						assert.Equal(t, tt.role, attributes["role"])
					}
					if element.Name.Local == "title" && svgDepth >= 0 {
						pointTitles++
					}
					depth++
				case xml.EndElement:
					depth--
					if element.Name.Local == "svg" {
						svgDepth = -1
					}
				}
			}
			assert.Equal(t, 1, svgCount)
			assert.Zero(t, pointTitles, "native titles would duplicate the custom tooltip")
		})
	}
}

func TestCopilotMetricsSVGChartAreaFillsUnderEachRun(t *testing.T) {
	chart := string(copilotMetricsSVGChart("Interactions", "area",
		[]string{"01", "02", "03", "04", "05"},
		[]int64{10, 20, 0, 30, 40},
		[]bool{true, true, false, true, true}))
	assert.Contains(t, chart, `<svg class="chart chart-line"`)
	assert.Equal(t, 1, strings.Count(chart, `<path class="line-area"`))
	assert.Equal(t, 1, strings.Count(chart, `<path class="line"`))
	assert.Equal(t, 2, strings.Count(chart, "Z"), "each contiguous run is closed separately so gaps stay blank")
	assert.Equal(t, 4, strings.Count(chart, `<circle class="dot"`))
	assert.NotContains(t, chart, `<rect class="bar"`)
}

func TestCopilotMetricsSVGChartDayTargets(t *testing.T) {
	type target struct {
		Class     string  `xml:"class,attr"`
		X         float64 `xml:"x,attr"`
		Y         float64 `xml:"y,attr"`
		Width     float64 `xml:"width,attr"`
		Height    float64 `xml:"height,attr"`
		Tooltip   string  `xml:"data-tooltip,attr"`
		TabIndex  string  `xml:"tabindex,attr"`
		Role      string  `xml:"role,attr"`
		AriaLabel string  `xml:"aria-label,attr"`
		Title     string  `xml:"title"`
	}
	type marker struct {
		X          float64 `xml:"cx,attr"`
		Y          float64 `xml:"cy,attr"`
		AriaHidden string  `xml:"aria-hidden,attr"`
		Title      string  `xml:"title"`
	}
	type day struct {
		Class  string `xml:"class,attr"`
		Target target `xml:"rect"`
		Marker marker `xml:"circle"`
	}
	tests := []struct {
		name        string
		labels      []string
		values      []int64
		valid       []bool
		wantIndices []int
	}{
		{
			name:        "missing days and zero",
			labels:      []string{"day 1", "day & 2", "day 3", "day 4", "day 5"},
			values:      []int64{0, 0, 20, 0, 10},
			valid:       []bool{false, true, true, false, true},
			wantIndices: []int{1, 2, 4},
		},
		{
			name:        "single day",
			labels:      []string{"day 1"},
			values:      []int64{0},
			valid:       []bool{true},
			wantIndices: []int{0},
		},
		{
			name:   "no data",
			labels: []string{"day 1", "day 2"},
			values: []int64{0, 0},
			valid:  []bool{false, false},
		},
	}
	for _, kind := range []string{"line", "area"} {
		for _, tt := range tests {
			t.Run(kind+"/"+tt.name, func(t *testing.T) {
				const title = "Interactions & <daily>"
				chart := string(copilotMetricsSVGChart(title, kind, tt.labels, tt.values, tt.valid))
				var svg struct {
					Role      string `xml:"role,attr"`
					AriaLabel string `xml:"aria-label,attr"`
					Days      []day  `xml:"g"`
				}
				require.NoError(t, xml.Unmarshal([]byte(chart), &svg))
				assert.Equal(t, "group", svg.Role)
				assert.Equal(t, title, svg.AriaLabel)
				require.Len(t, svg.Days, len(tt.wantIndices))
				step := float64(copilotMetricsChartWidth-copilotMetricsChartPadLeft-copilotMetricsChartPadRgt) / float64(len(tt.labels))
				for i, index := range tt.wantIndices {
					got := svg.Days[i]
					wantTooltip := tt.labels[index] + ": " + formatCopilotMetricsInt(tt.values[index])
					start := float64(copilotMetricsChartPadLeft) + step*float64(index)
					end := start + step
					if index == 0 {
						start -= 3
					}
					if index == len(tt.labels)-1 {
						end += 3
					}
					assert.Equal(t, "chart-day", got.Class)
					assert.Equal(t, "chart-day-hit", got.Target.Class)
					assert.InDelta(t, start, got.Target.X, 0.001)
					assert.InDelta(t, end-start, got.Target.Width, 0.001)
					assert.Equal(t, float64(copilotMetricsChartPadTop)-3, got.Target.Y)
					assert.Equal(t, 124.0, got.Target.Height)
					assert.InDelta(t, float64(copilotMetricsChartPadLeft)+step*(float64(index)+0.5), got.Marker.X, 0.051)
					assert.Equal(t, wantTooltip, got.Target.Tooltip)
					assert.Equal(t, wantTooltip, got.Target.AriaLabel)
					assert.Empty(t, got.Target.Title, "native titles would duplicate the custom tooltip")
					assert.Equal(t, "0", got.Target.TabIndex)
					assert.Equal(t, "img", got.Target.Role)
					assert.Equal(t, "true", got.Marker.AriaHidden)
					assert.Empty(t, got.Marker.Title, "the decorative marker must not create another tooltip target")
					if tt.values[index] == 0 {
						assert.Equal(t, 134.0, got.Marker.Y)
					}
				}
			})
		}
	}
}

func TestCopilotMetricsSVGTimeSeriesDayTargets(t *testing.T) {
	type target struct {
		X         float64 `xml:"x,attr"`
		Y         float64 `xml:"y,attr"`
		Width     float64 `xml:"width,attr"`
		Height    float64 `xml:"height,attr"`
		Tooltip   string  `xml:"data-tooltip,attr"`
		TabIndex  string  `xml:"tabindex,attr"`
		Role      string  `xml:"role,attr"`
		AriaLabel string  `xml:"aria-label,attr"`
		Title     string  `xml:"title"`
	}
	type marker struct {
		X          float64 `xml:"cx,attr"`
		Y          float64 `xml:"cy,attr"`
		Fill       string  `xml:"fill,attr"`
		AriaHidden string  `xml:"aria-hidden,attr"`
		Title      string  `xml:"title"`
	}
	type day struct {
		Class   string   `xml:"class,attr"`
		Target  target   `xml:"rect"`
		Markers []marker `xml:"circle"`
	}
	type expectedMarker struct {
		series int
		value  float64
	}
	type expectedDay struct {
		index   int
		markers []expectedMarker
	}

	days := []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04", "2030-01-05"}
	series := []copilotMetricsPlotSeries{
		{Name: "First & <A>", Values: []float64{3, 0, 0, 0, 6}, Valid: []bool{true, true, false, true, true}},
		{Name: "Second", Values: []float64{7, 10, 10, 0, 6}, Valid: []bool{true, true, true, true, true}},
	}
	percentages := copilotMetricsDailyPercentageSeries(series, len(days))
	areaColors := copilotMetricsAreaHueColors(percentages)
	const title = "Usage & <all>"
	tests := []struct {
		name        string
		chart       template.HTML
		max         float64
		colors      []string
		want        []expectedDay
		wantMissing string
	}{
		{
			name:   "line",
			chart:  copilotMetricsSVGMultiLineChart(title, days, series),
			max:    10,
			colors: []string{copilotMetricsChartColors[0].hex(), copilotMetricsChartColors[1].hex()},
			want: []expectedDay{
				{0, []expectedMarker{{0, 3}, {1, 7}}},
				{1, []expectedMarker{{0, 0}, {1, 10}}},
				{2, []expectedMarker{{1, 10}}},
				{3, []expectedMarker{{0, 0}, {1, 0}}},
				{4, []expectedMarker{{0, 6}, {1, 6}}},
			},
			wantMissing: "First & <A>: n/a\nSecond: 10.0",
		},
		{
			name:   "stacked area",
			chart:  copilotMetricsSVGStackedAreaChart(title, days, percentages, nil),
			max:    100,
			colors: []string{areaColors[series[0].Name].hex(), areaColors[series[1].Name].hex()},
			want: []expectedDay{
				{0, []expectedMarker{{0, 30}, {1, 100}}},
				{1, []expectedMarker{{0, 0}, {1, 100}}},
				{2, []expectedMarker{{1, 100}}},
				{4, []expectedMarker{{0, 50}, {1, 100}}},
			},
			wantMissing: "First & <A>: n/a\nSecond: 100.0%",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svg struct {
				Role      string `xml:"role,attr"`
				AriaLabel string `xml:"aria-label,attr"`
				Days      []day  `xml:"g"`
			}
			require.NoError(t, xml.Unmarshal([]byte(tt.chart), &svg))
			assert.Equal(t, "group", svg.Role)
			assert.Equal(t, title, svg.AriaLabel)
			require.Len(t, svg.Days, len(tt.want))
			x := func(index int) float64 { return 72 + 613*float64(index)/float64(len(days)-1) }
			for i, expected := range tt.want {
				got := svg.Days[i]
				start, end := 69.0, 688.0
				if expected.index > 0 {
					start = (x(expected.index-1) + x(expected.index)) / 2
				}
				if expected.index < len(days)-1 {
					end = (x(expected.index) + x(expected.index+1)) / 2
				}
				assert.Equal(t, "chart-day", got.Class)
				assert.InDelta(t, start, got.Target.X, 0.001)
				assert.InDelta(t, end-start, got.Target.Width, 0.001)
				assert.Equal(t, 33.0, got.Target.Y)
				assert.Equal(t, 137.0, got.Target.Height)
				assert.Equal(t, "0", got.Target.TabIndex)
				assert.Equal(t, "img", got.Target.Role)
				assert.Empty(t, got.Target.Title, "native titles would duplicate the tooltip")
				assert.Contains(t, got.Target.Tooltip, days[expected.index]+"\n")
				assert.NotContains(t, got.Target.Tooltip, title)
				assert.Equal(t, got.Target.Tooltip, got.Target.AriaLabel)
				if expected.index == 2 {
					assert.Contains(t, got.Target.Tooltip, tt.wantMissing)
				}
				require.Len(t, got.Markers, len(expected.markers))
				for j, wantMarker := range expected.markers {
					marker := got.Markers[j]
					assert.InDelta(t, x(expected.index), marker.X, 0.051)
					assert.InDelta(t, 36+131*(1-wantMarker.value/tt.max), marker.Y, 0.051)
					assert.Equal(t, tt.colors[wantMarker.series], marker.Fill)
					assert.Equal(t, "true", marker.AriaHidden)
					assert.Empty(t, marker.Title, "the decorative marker must not create another tooltip target")
				}
			}
			assert.NotContains(t, string(tt.chart), "<all>")
		})
	}
}

func TestCopilotMetricsSVGTimeSeriesSingleDayTarget(t *testing.T) {
	days := []string{"2030-01-01"}
	series := []copilotMetricsPlotSeries{{Name: "first", Values: []float64{100}, Valid: []bool{true}}}
	for _, chart := range []template.HTML{
		copilotMetricsSVGMultiLineChart("Daily", days, series),
		copilotMetricsSVGStackedAreaChart("Daily", days, series, nil),
	} {
		var svg struct {
			Days []struct {
				Target struct {
					X     float64 `xml:"x,attr"`
					Width float64 `xml:"width,attr"`
				} `xml:"rect"`
				Markers []struct {
					X float64 `xml:"cx,attr"`
				} `xml:"circle"`
			} `xml:"g"`
		}
		require.NoError(t, xml.Unmarshal([]byte(chart), &svg))
		require.Len(t, svg.Days, 1)
		assert.Equal(t, 69.0, svg.Days[0].Target.X)
		assert.Equal(t, 619.0, svg.Days[0].Target.Width)
		require.Len(t, svg.Days[0].Markers, 1)
		assert.Equal(t, 378.5, svg.Days[0].Markers[0].X)
	}
}

func TestCopilotMetricsTooltipFieldsRoundTrip(t *testing.T) {
	tooltip := copilotMetricsTooltip{
		Heading: "2030-01-01 & <date>",
		Rows: []copilotMetricsTooltipRow{
			{Label: "model: \"<item>&\nsecond line", Value: "1,234", Share: "12.3%"},
			{Label: "missing + model", Value: "n/a"},
		},
		Note: "Percentages: all activities",
	}
	var target struct {
		Text   string `xml:"data-tooltip,attr"`
		Fields string `xml:"data-tooltip-fields,attr"`
	}
	require.NoError(t, xml.Unmarshal([]byte("<rect "+tooltip.attributes()+"/>"), &target))
	assert.Equal(t, tooltip.text(), target.Text)
	fields, err := url.ParseQuery(target.Fields)
	require.NoError(t, err)
	assert.Equal(t, tooltip.Heading, fields.Get("heading"))
	assert.Equal(t, []string{tooltip.Rows[0].Label, tooltip.Rows[1].Label}, fields["label"])
	assert.Equal(t, []string{"1,234", "n/a"}, fields["value"])
	assert.Equal(t, []string{"12.3%", ""}, fields["share"])
	assert.Equal(t, tooltip.Note, fields.Get("note"))
	assert.NotContains(t, tooltip.attributes(), "<item>")
}

func TestCopilotMetricsTooltipsOmitChartTitles(t *testing.T) {
	const title = "Unique chart title"
	days := []string{"2030-01-01"}
	series := []copilotMetricsPlotSeries{{Name: "series-a", Values: []float64{2}, Valid: []bool{true}}}
	values := []copilotmetrics.RichValue{{Name: "model-a", Group: "language-a", Value: copilotmetrics.Counter{Value: 2, Valid: true}}}
	charts := []template.HTML{
		copilotMetricsSVGChart(title, "line", days, []int64{2}, []bool{true}),
		copilotMetricsSVGMultiLineChart(title, days, series),
		copilotMetricsSVGStackedAreaChart(title, days, series, nil),
		copilotMetricsHTMLRichStackedBarPanel(title, values).Chart,
		copilotMetricsHTMLRichPiePanel(title, values).Chart,
		copilotMetricsHTMLRichBarPanel(title, values).Chart,
	}
	for _, chart := range charts {
		decoder := xml.NewDecoder(strings.NewReader("<root>" + string(chart) + "</root>"))
		tooltips := 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if element, ok := token.(xml.StartElement); ok {
				for _, attr := range element.Attr {
					if attr.Name.Local == "data-tooltip" {
						tooltips++
						assert.NotContains(t, attr.Value, title)
					}
				}
			}
		}
		assert.Positive(t, tooltips)
		assert.Contains(t, string(chart), `aria-label="`+title+`"`, "the chart remains accessible by name")
	}
	normalBar := string(copilotMetricsSVGChart(title, "bar", days, []int64{2}, []bool{true}))
	assert.Contains(t, normalBar, `aria-label="2030-01-01: 2"`)
	assert.NotContains(t, normalBar, "<title>", "native titles would duplicate the tooltip")
}

func TestCopilotMetricsStackedTooltipsListAllSeries(t *testing.T) {
	count := func(n int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: n, Valid: true} }
	const group = "lang<&:alpha"
	const model = "model-b: \"<item>&"
	values := []copilotmetrics.RichValue{
		{Name: "model-a", Group: group, Value: count(60)},
		{Name: model, Group: group, Value: count(25)},
		{Name: "model-zero", Group: group, Value: count(0)},
		{Name: "model-missing", Group: group},
		{Name: "model-missing", Group: "lang-b", Value: count(15)},
	}
	chart := string(copilotMetricsHTMLRichStackedBarPanel("Unique chart title", values).Chart)
	type target struct {
		Class       string `xml:"class,attr"`
		Tooltip     string `xml:"data-tooltip,attr"`
		Fields      string `xml:"data-tooltip-fields,attr"`
		Role        string `xml:"role,attr"`
		AriaLabel   string `xml:"aria-label,attr"`
		NativeTitle string `xml:"title,attr"`
		Title       string `xml:"title"`
	}
	var document struct {
		SVG struct {
			Role     string   `xml:"role,attr"`
			Segments []target `xml:"rect"`
			Axes     []struct {
				Label target `xml:"div"`
			} `xml:"foreignObject"`
		} `xml:"div>svg"`
	}
	require.NoError(t, xml.Unmarshal([]byte("<root>"+chart+"</root>"), &document))
	assert.Equal(t, "group", document.SVG.Role)
	require.Len(t, document.SVG.Segments, 4)
	require.Len(t, document.SVG.Axes, 2)

	wantText := "Language: " + group + "\nmodel-a: 60 (60.0%)\n" + model +
		": 25 (25.0%)\nmodel-missing: n/a\nmodel-zero: 0 (0.0%)\n" +
		"Language total: 85 (85.0%)\nPercentages are shares of all activities."
	seenGroup := 0
	for _, segment := range document.SVG.Segments {
		assert.Equal(t, "img", segment.Role)
		assert.Empty(t, segment.Title, "native titles would duplicate the custom tooltip")
		assert.NotContains(t, segment.Tooltip, "Unique chart title")
		fields, err := url.ParseQuery(segment.Fields)
		require.NoError(t, err)
		if fields.Get("heading") != "Language: "+group {
			continue
		}
		seenGroup++
		assert.Equal(t, wantText, segment.Tooltip)
		assert.Contains(t, segment.AriaLabel, "\n"+wantText)
		assert.Equal(t, []string{"model-a", model, "model-missing", "model-zero", "Language total"}, fields["label"])
		assert.Equal(t, []string{"60", "25", "n/a", "0", "85"}, fields["value"])
		assert.Equal(t, []string{"60.0%", "25.0%", "", "0.0%", "85.0%"}, fields["share"])
	}
	assert.Equal(t, 3, seenGroup)
	assert.Equal(t, wantText, document.SVG.Axes[0].Label.Tooltip)
	assert.Empty(t, document.SVG.Axes[0].Label.NativeTitle, "native titles would duplicate the custom tooltip")
	assert.Equal(t, wantText, document.SVG.Axes[0].Label.AriaLabel)
	assert.NotContains(t, chart, "<item>")
}

func TestCopilotMetricsStackedZeroTotalTooltip(t *testing.T) {
	chart := string(copilotMetricsHTMLRichStackedBarPanel("Chart", []copilotmetrics.RichValue{
		{Name: "model-zero", Group: "language-zero", Value: copilotmetrics.Counter{Valid: true}},
	}).Chart)
	want := "Language: language-zero\nmodel-zero: 0 (n/a)\nLanguage total: 0 (n/a)"
	assert.Contains(t, chart, `data-tooltip="`+want)
	assert.Contains(t, chart, `class="stacked-axis-label" data-tooltip="`+want, "the zero-height bar remains reachable from its axis label")
	assert.NotContains(t, chart, "NaN")
	assert.NotContains(t, chart, "Inf")
}

func TestCopilotMetricsStackedTooltipAggregatesOther(t *testing.T) {
	var values []copilotmetrics.RichValue
	for i := 1; i <= 8; i++ {
		values = append(values, copilotmetrics.RichValue{
			Name: fmt.Sprintf("model-%02d", i), Group: "language-a",
			Value: copilotmetrics.Counter{Value: int64(i), Valid: true},
		})
	}
	chart := string(copilotMetricsHTMLRichStackedBarPanel("Chart", values).Chart)
	assert.Contains(t, chart, "Other: 3 (8.3%)\nLanguage total: 36 (100.0%)")
	assert.NotContains(t, chart, "model-01")
	assert.NotContains(t, chart, "model-02")
}

package render

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotMetricsStackedAreaPoints(t *testing.T) {
	series := []copilotMetricsPlotSeries{
		{Name: "first", Values: []float64{75, 25, 0, 100, 0}, Valid: []bool{true, true, false, true, true}},
		{Name: "second", Values: []float64{25, 75, 0, 0, 100}, Valid: []bool{true, true, false, false, true}},
	}
	points := copilotMetricsStackedAreaPoints(series, 5)
	require.Len(t, points, 2)
	assert.Equal(t, []copilotMetricsAreaPoint{
		{Lower: 0, Upper: 75, Valid: true},
		{Lower: 0, Upper: 25, Valid: true},
		{},
		{Lower: 0, Upper: 100, Valid: true},
		{Lower: 0, Upper: 0, Valid: true},
	}, points[0])
	assert.Equal(t, []copilotMetricsAreaPoint{
		{Lower: 75, Upper: 100, Valid: true},
		{Lower: 25, Upper: 100, Valid: true},
		{},
		{Lower: 100, Upper: 100},
		{Lower: 0, Upper: 100, Valid: true},
	}, points[1])
	assert.Equal(t, float64(75), series[0].Values[0], "stacking does not modify the source percentages")
}

type testCopilotMetricsCubic struct {
	x0, y0, x1, y1, x2, y2, x3, y3 float64
}

func testCopilotMetricsAreaCurves(t *testing.T, path string) []testCopilotMetricsCubic {
	t.Helper()
	fields := strings.Fields(path)
	readFloat := func(value string) float64 {
		result, err := strconv.ParseFloat(value, 64)
		require.NoError(t, err)
		return result
	}
	var curves []testCopilotMetricsCubic
	var x, y float64
	for i := 0; i < len(fields); {
		command := fields[i][0]
		if command == 'Z' {
			break
		}
		first := readFloat(fields[i][1:])
		i++
		switch command {
		case 'M', 'L':
			x, y = first, readFloat(fields[i])
			i++
		case 'C':
			curve := testCopilotMetricsCubic{
				x0: x, y0: y, x1: first, y1: readFloat(fields[i]),
				x2: readFloat(fields[i+1]), y2: readFloat(fields[i+2]),
				x3: readFloat(fields[i+3]), y3: readFloat(fields[i+4]),
			}
			i += 5
			curves = append(curves, curve)
			x, y = curve.x3, curve.y3
		default:
			t.Fatalf("unexpected path command %q", command)
		}
	}
	return curves
}

func (c testCopilotMetricsCubic) at(t float64) (float64, float64) {
	u := 1 - t
	return u*u*u*c.x0 + 3*u*u*t*c.x1 + 3*u*t*t*c.x2 + t*t*t*c.x3,
		u*u*u*c.y0 + 3*u*u*t*c.y1 + 3*u*t*t*c.y2 + t*t*t*c.y3
}

func TestCopilotMetricsStackedAreaCurvesShareBoundaries(t *testing.T) {
	series := []copilotMetricsPlotSeries{
		{Name: "first", Values: []float64{50, 0, 20}, Valid: []bool{true, false, true}},
		{Name: "second", Values: []float64{20, 40, 0}, Valid: []bool{true, true, false}},
		{Name: "third", Values: []float64{30, 60, 80}, Valid: []bool{true, true, true}},
	}
	points := copilotMetricsStackedAreaPoints(series, 3)
	x := func(day int) float64 { return float64(day * 100) }
	y := func(value float64) float64 { return value }

	assert.Equal(t, copilotMetricsAreaPoint{Lower: 20, Upper: 20}, points[1][2], "a missing middle layer collapses at its cumulative boundary")
	curves := make([][]testCopilotMetricsCubic, len(series))
	for i := range series {
		path := copilotMetricsStackedAreaPath(points, i, 0, 2, x, y, 0, 200)
		curves[i] = testCopilotMetricsAreaCurves(t, path)
		require.Len(t, curves[i], 4)
	}
	assert.Contains(t, copilotMetricsStackedAreaPath(points, 0, 0, 2, x, y, 0, 200),
		"C33.3 50.0 66.7 0.0 100.0 0.0", "a missing series tapers smoothly")
	for layer := range series {
		for day := 0; day < 2; day++ {
			upper := curves[layer][day]
			lower := curves[layer][3-day]
			if day > 0 {
				previous := curves[layer][day-1]
				incomingSlope := (previous.y3 - previous.y2) / (previous.x3 - previous.x2)
				outgoingSlope := (upper.y1 - upper.y0) / (upper.x1 - upper.x0)
				assert.InDelta(t, incomingSlope, outgoingSlope, 0.001, "adjacent curves meet smoothly")
			}
			for step := 0; step <= 20; step++ {
				progress := float64(step) / 20
				upperX, upperY := upper.at(progress)
				lowerX, lowerY := lower.at(1 - progress)
				smooth := progress * progress * (3 - 2*progress)
				assert.InDelta(t, upperX, lowerX, 0.001)
				assert.InDelta(t, upper.y0+(upper.y3-upper.y0)*smooth, upperY, 0.001)
				assert.GreaterOrEqual(t, lowerY, -0.001)
				assert.LessOrEqual(t, upperY, 100.001)
				assert.LessOrEqual(t, lowerY, upperY+0.001, "layers must not cross")
				if layer == 0 {
					assert.InDelta(t, 0, lowerY, 0.001)
				}
				if layer == len(series)-1 {
					assert.InDelta(t, 100, upperY, 0.001)
				}
				if layer+1 < len(series) {
					nextX, nextY := curves[layer+1][3-day].at(1 - progress)
					assert.InDelta(t, upperX, nextX, 0.001)
					assert.InDelta(t, upperY, nextY, 0.001, "adjacent layers share exactly one boundary")
				}
			}
		}
	}
	assert.False(t, series[0].Valid[1], "the source still records missing data")
}

func TestCopilotMetricsStackedAreaMissingAndSingleDays(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03"}
	series := []copilotMetricsPlotSeries{
		{Name: "first", Values: []float64{40, 0, 30}, Valid: []bool{true, false, true}},
		{Name: "second", Values: []float64{60, 0, 70}, Valid: []bool{true, false, true}},
	}
	chart := string(copilotMetricsSVGStackedAreaChart("Daily", days, series, nil))
	assert.Equal(t, 4, strings.Count(chart, `<path class="metric-area"`))
	assert.Equal(t, 2, strings.Count(chart, `<rect class="metric-day-hit"`))
	assert.NotContains(t, chart, `data-tooltip="2030-01-02`, "a missing day is not bridged or assigned a tooltip")
	assert.Contains(t, chart, `tabindex="0" role="img" aria-label="2030-01-01`+"\n")

	single := []copilotMetricsPlotSeries{
		{Name: `item"><script>`, Values: []float64{100}, Valid: []bool{true}},
	}
	singleChart := string(copilotMetricsSVGStackedAreaChart(`Title"><script>`, days[:1], single, nil))
	assert.Contains(t, singleChart, `<path class="metric-area"`)
	assert.Contains(t, singleChart, `M72.0 36.0`, "a single day fills the available plot width")
	assert.Contains(t, singleChart, `L685.0 36.0`)
	assert.Contains(t, singleChart, `item&#34;&gt;&lt;script&gt;`)
	assert.NotContains(t, singleChart, `<script>`)
	assert.Empty(t, copilotMetricsSVGStackedAreaChart("Missing", days[:1], []copilotMetricsPlotSeries{{
		Name: "missing", Values: []float64{0}, Valid: []bool{false},
	}}, nil))
}

func TestCopilotMetricsStackedAreaDoesNotBridgeMissingDays(t *testing.T) {
	days := []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04", "2030-01-05"}
	series := []copilotMetricsPlotSeries{
		{Name: "first", Values: []float64{20, 0, 0, 60, 0}, Valid: []bool{true, false, false, true, false}},
		{Name: "second", Values: []float64{80, 0, 0, 40, 100}, Valid: []bool{true, false, false, true, true}},
	}
	chart := string(copilotMetricsSVGStackedAreaChart("Daily", days, series, nil))
	assert.Equal(t, 4, strings.Count(chart, `<path class="metric-area"`))
	assert.Equal(t, 3, strings.Count(chart, `<rect class="metric-day-hit"`))
	assert.NotContains(t, chart, `data-tooltip="2030-01-02`)
	assert.NotContains(t, chart, `data-tooltip="2030-01-03`)
	assert.Contains(t, chart, "2030-01-05\nfirst: n/a\nsecond: 100.0%")
	paths := regexp.MustCompile(`d="([^"]+)" aria-hidden="true"`).FindAllStringSubmatch(chart, -1)
	require.Len(t, paths, 4)
	for _, path := range paths {
		curves := testCopilotMetricsAreaCurves(t, path[1])
		for _, curve := range curves {
			assert.GreaterOrEqual(t, curve.x0, 531.7, "curves must not cross a missing day")
			assert.GreaterOrEqual(t, curve.x3, 531.7, "curves must not cross a missing day")
		}
	}
}

func TestCopilotMetricsAreaGradientFollowsDisplayedTotals(t *testing.T) {
	count := func(n int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: n, Valid: true} }
	daily := []copilotmetrics.RichSeries{
		{Name: "d", Values: []copilotmetrics.Counter{count(5), count(5)}},
		{Name: "a", Values: []copilotmetrics.Counter{count(30), count(30)}},
		{Name: "e", Values: []copilotmetrics.Counter{count(5), count(5)}},
		{Name: "b", Values: []copilotmetrics.Counter{count(20), count(20)}},
		{Name: "c", Values: []copilotmetrics.Counter{count(10), count(10)}},
		{Name: "f", Values: []copilotmetrics.Counter{count(2), count(2)}},
		{Name: "g", Values: []copilotmetrics.Counter{count(1), count(1)}},
		{Name: "Others", Values: []copilotmetrics.Counter{count(100), count(100)}},
	}
	s := &copilotmetrics.Summary{
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionModel: true, copilotmetrics.SectionLanguage: true},
		Rich: &copilotmetrics.RichMetrics{
			Days:               []string{"2030-01-01", "2030-01-02"},
			ModelRequestsByDay: daily, LanguageCodeGenerationByDay: daily,
		},
		ModelRequestsByFeature: []copilotmetrics.RichValue{
			{Name: "g", Group: "chat_panel_agent_mode", Value: count(1000)},
			{Name: "a", Group: "chat_panel_agent_mode", Value: count(1)},
		},
	}
	for _, name := range []string{"g", "a", "b", "c", "d", "e", "f", "table-only"} {
		s.Models = append(s.Models, copilotmetrics.BreakdownRow{Key: name})
		s.Languages = append(s.Languages, copilotmetrics.BreakdownRow{
			Key: name, ActivityTotals: copilotmetrics.ActivityTotals{CodeGeneration: count(1000)},
		})
	}
	s.AllLanguages = s.Languages
	names := []string{"a", "b", "c", "d", "e", "f"}
	for _, section := range copilotMetricsHTMLSections(s, true) {
		t.Run(string(section.ID), func(t *testing.T) {
			scheme := copilotMetricsColorsForSection(s, section.ID)
			require.NotNil(t, scheme)
			require.NotEmpty(t, section.RichPanels)
			chart := string(section.RichPanels[0].Chart)
			areaPaths := regexp.MustCompile(`<path class="metric-area" fill="([^"]+)"`).FindAllStringSubmatch(chart, -1)
			legend := regexp.MustCompile(`<rect class="area-legend-key"[^>]* fill="([^"]+)"[^>]*/><text class="legend"[^>]*>([^<]*)</text>`).FindAllStringSubmatch(chart, -1)
			require.Len(t, areaPaths, len(names)+1)
			require.Len(t, legend, len(names)+1)
			var previousHue float64
			for rank, name := range names {
				color := copilotMetricsHueGradientColor(rank, len(names))
				themeColor := copilotMetricsThemeGradientColor(scheme.theme, rank, len(names))
				assert.Equal(t, themeColor.hex(), scheme.group(name), "tables keep the section theme")
				assert.Equal(t, color.hex(), areaPaths[rank][1])
				assert.Equal(t, []string{color.hex(), name}, legend[rank][1:])
				hue := 215 - 280*float64(rank)/float64(len(names)-1)
				if rank > 0 {
					assert.Less(t, hue, previousHue)
				}
				previousHue = hue
				assert.Contains(t, chart, `class="metric-area" fill="`+color.hex()+`"`)
				for row, cells := range section.Table.Rows {
					if cells[0] == name {
						assert.Equal(t, themeColor.hex(), section.Table.RowColors[row].Group)
					}
				}
			}
			assert.Equal(t, "#6e7781", scheme.group("Other"))
			assert.Equal(t, "#6e7781", areaPaths[len(names)][1])
			assert.Equal(t, []string{"#6e7781", "Other"}, legend[len(names)][1:])
			assert.Contains(t, chart, "Other: ")
			assert.NotEqual(t, "#000000", scheme.group("table-only"), "table-only categories retain a color")
			seen := map[copilotMetricsColor]bool{}
			for _, color := range scheme.groups {
				assert.False(t, seen[color], "table group colors remain distinct")
				seen[color] = true
			}
		})
	}
}

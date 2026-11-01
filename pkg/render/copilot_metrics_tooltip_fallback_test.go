package render

import (
	"encoding/xml"
	"fmt"
	"html/template"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type copilotMetricsTooltipTestNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr                      `xml:",any,attr"`
	Text     string                          `xml:",chardata"`
	Children []copilotMetricsTooltipTestNode `xml:",any"`
}

func (node copilotMetricsTooltipTestNode) attr(name string) (string, bool) {
	for _, attribute := range node.Attrs {
		if attribute.Name.Local == name {
			return attribute.Value, true
		}
	}
	return "", false
}

func (node copilotMetricsTooltipTestNode) walk(visit func(copilotMetricsTooltipTestNode)) {
	visit(node)
	for _, child := range node.Children {
		child.walk(visit)
	}
}

func TestCopilotMetricsFallbacksCoverChartTargets(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	days := []string{"2030-01-01", "2030-01-02"}
	series := []copilotMetricsPlotSeries{{Name: `series "&<a>`, Values: []float64{2, 0}, Valid: []bool{true, true}}}
	values := []copilotmetrics.RichValue{
		{Name: `model "&<a>`, Group: "language-a", Value: counter(2)},
		{Name: "model-b", Group: "language-a", Value: counter(1)},
	}
	users := []copilotmetrics.UserRow{{
		Login: `fictional-"<user>&`, ActiveDays: 2, Surfaces: []string{"chat"},
		ActivityTotals: copilotmetrics.ActivityTotals{UserInitiatedInteraction: counter(0)},
	}}
	bars := []copilotMetricsRepositoryBar{
		{Name: `fictional/"<zero>&`, Total: counter(0), Copilot: counter(0), Other: counter(0)},
		{Name: "fictional/repo-b", Total: counter(3), Copilot: counter(1), Other: counter(2)},
	}
	charts := []struct {
		name  string
		chart template.HTML
	}{
		{"normal line", copilotMetricsSVGChart("Line", "line", days, []int64{2, 0}, []bool{true, true})},
		{"normal area", copilotMetricsSVGChart("Area", "area", days, []int64{2, 0}, []bool{true, true})},
		{"normal bar", copilotMetricsSVGChart("Bar", "bar", days, []int64{2, 0}, []bool{true, true})},
		{"rich line", copilotMetricsSVGMultiLineChart("Line", days, series)},
		{"stacked area", copilotMetricsSVGStackedAreaChart("Area", days, series, nil)},
		{"daily stacked bar", copilotMetricsDailyStackedBarChart("Daily", days, series, copilotMetricsThemeGreen, copilotMetricsDailyStackedBarOptions{})},
		{"model and language", copilotMetricsHTMLRichStackedPanel("Stacked", values, "Language").Chart},
		{"pie slices and legend", copilotMetricsHTMLRichPiePanel("Pie", values).Chart},
		{"pie circle and legend", copilotMetricsHTMLRichPiePanel("Pie", values[:1]).Chart},
		{"html bars", copilotMetricsHTMLRichBarPanel("Bars", values).Chart},
		{"repository bars", copilotMetricsSVGRepositoryStackedBar("Repositories", bars, "Copilot", "Other", false)},
		{"repository suggestions", copilotMetricsSVGRepositorySuggestionBars([]copilotMetricsRepositorySuggestionBar{{
			Name: "fictional/repo-a", Generated: counter(0), GeneratedFull: true,
		}})},
		{"user ratio", copilotMetricsUserRatioPanel(users).Chart},
		{"user surface", copilotMetricsUserSurfacePanel(users).Chart},
	}

	for _, chart := range charts {
		t.Run(chart.name, func(t *testing.T) {
			require.NotEmpty(t, chart.chart)
			next := 0
			rewritten, tooltips, fallbacks, err := copilotMetricsChartTooltips(chart.chart, &next)
			require.NoError(t, err)
			require.NotEmpty(t, tooltips)
			assert.Equal(t, len(fallbacks), next)
			fallbackText := make(map[string]string, len(fallbacks))
			fallbackSVG := make(map[string]string, len(fallbacks))
			for _, fallback := range fallbacks {
				fallbackText[fallback.Target] = fallback.Text
				fallbackSVG[fallback.Target] = fallback.SVG
				if fallback.SVG == "" {
					continue
				}
				for _, value := range fallback.Box {
					assert.True(t, value >= 0 && value <= 1, "box fraction %v is inside the SVG", value)
				}
				assert.Positive(t, fallback.Box[2])
				assert.Positive(t, fallback.Box[3])
			}
			byKey := make(map[string]string, len(tooltips))
			for _, tooltip := range tooltips {
				assert.NotContains(t, byKey, tooltip.Key)
				byKey[tooltip.Key] = tooltip.Text
			}

			var root copilotMetricsTooltipTestNode
			require.NoError(t, xml.Unmarshal([]byte("<root>"+string(rewritten)+"</root>"), &root))
			targets := 0
			svgs := make(map[string]bool)
			root.walk(func(node copilotMetricsTooltipTestNode) {
				if id, ok := node.attr("data-tooltip-svg"); ok {
					assert.Equal(t, "svg", node.XMLName.Local)
					svgs[id] = true
				}
			})
			root.walk(func(node copilotMetricsTooltipTestNode) {
				text, ok := node.attr("data-tooltip")
				if !ok {
					return
				}
				targets++
				key, ok := node.attr("data-tooltip-key")
				require.True(t, ok)
				assert.Equal(t, text, byKey[key])
				assert.Equal(t, copilotMetricsTooltipKey(text), key)
				tabindex, ok := node.attr("tabindex")
				assert.True(t, ok)
				assert.Equal(t, "0", tabindex)
				label, ok := node.attr("aria-label")
				assert.True(t, ok)
				assert.Contains(t, label, text)
				target, ok := node.attr("data-tooltip-target")
				require.True(t, ok)
				assert.Equal(t, text, fallbackText[target])
				_, boxed := node.attr("data-tooltip-box")
				assert.Equal(t, boxed, fallbackSVG[target] != "", "boxed targets are anchored through their SVG")
				if boxed {
					assert.True(t, svgs[fallbackSVG[target]])
				}
				node.walk(func(descendant copilotMetricsTooltipTestNode) {
					_, hasTitle := descendant.attr("title")
					assert.False(t, hasTitle, "native title would duplicate the tooltip")
					assert.NotEqual(t, "title", descendant.XMLName.Local, "native title would duplicate the tooltip")
				})
			})
			assert.Positive(t, targets)
			assert.Equal(t, targets, len(fallbacks))
			assert.GreaterOrEqual(t, targets, len(tooltips))
		})
	}

	next := 0
	_, tooltips, fallbacks, err := copilotMetricsChartTooltips(copilotMetricsHTMLRichPiePanel("Pie", values[:1]).Chart, &next)
	require.NoError(t, err)
	assert.Len(t, tooltips, 1, "the slice and legend share one listed value")
	require.Len(t, fallbacks, 2, "each target has its own anchored popup")
	assert.NotEqual(t, fallbacks[0].Target, fallbacks[1].Target)
}

func TestCopilotMetricsFallbackExtractionRejectsMalformedTargets(t *testing.T) {
	tooltip := copilotMetricsTooltip{
		Heading: "fictional &lt; \"<heading>\"\nsecond line",
		Rows:    []copilotMetricsTooltipRow{{Label: "row & <value>", Value: "1"}},
	}
	chart := template.HTML(`<rect ` + tooltip.attributes() + `/>`)
	next := 3
	rewritten, tooltips, fallbacks, err := copilotMetricsChartTooltips(chart, &next)
	require.NoError(t, err)
	require.Len(t, tooltips, 1)
	require.Len(t, fallbacks, 1)
	assert.Equal(t, "t3", fallbacks[0].Target)
	assert.Equal(t, 4, next)
	assert.Contains(t, string(rewritten), `data-tooltip-target="t3"`)
	assert.Equal(t, tooltip.text(), tooltips[0].Text)
	assert.Equal(t, copilotMetricsTooltipKey(tooltip.text()), tooltips[0].Key)

	boxed := template.HTML(`<svg class="chart" viewBox="0 0 200 100"><rect ` + tooltip.svgAttributes(50, 25, 20, 10) + `/><rect ` + tooltip.svgAttributes(0, 0, 200, 100) + `/></svg>`)
	rewritten, _, fallbacks, err = copilotMetricsChartTooltips(boxed, &next)
	require.NoError(t, err)
	require.Len(t, fallbacks, 2)
	assert.Equal(t, "t4", fallbacks[0].SVG)
	assert.Equal(t, "t4", fallbacks[1].SVG, "targets in one SVG share its anchor")
	assert.InDeltaSlice(t, []float64{.25, .25, .1, .1}, fallbacks[0].Box[:], 1e-9)
	assert.InDeltaSlice(t, []float64{0, 0, 1, 1}, fallbacks[1].Box[:], 1e-9)
	assert.True(t, strings.HasPrefix(string(rewritten), `<svg class="chart" viewBox="0 0 200 100" data-tooltip-svg="t4"><rect `))
	css := string(copilotMetricsTooltipFallbackCSS(fallbacks))
	assert.Equal(t, 1, strings.Count(css, `[data-tooltip-svg="t4"] { anchor-name: --chart-svg-t4; }`))
	assert.Contains(t, css, `.chart-tooltip-anchor[data-tooltip-anchor="t4"] { position-anchor: --chart-svg-t4; left: calc(anchor(left) + anchor-size(width) * 0.25000); top: calc(anchor(top) + anchor-size(height) * 0.25000); width: calc(anchor-size(width) * 0.10000); height: calc(anchor-size(height) * 0.10000); anchor-name: --chart-tip-t4; }`)
	assert.Contains(t, css, copilotMetricsTooltipAnchorSupports+" {")
	_, _, _, err = copilotMetricsChartTooltips(template.HTML(`<rect `+tooltip.svgAttributes(0, 0, 1, 1)+`/>`), &next)
	assert.ErrorContains(t, err, "outside an SVG")

	_, _, _, err = copilotMetricsChartTooltips(template.HTML(`<rect data-tooltip="missing key"/>`), &next)
	assert.ErrorContains(t, err, "missing or malformed")
	corrupt := strings.Replace(string(chart), tooltips[0].Key, strings.Repeat("0", 24), 1)
	_, _, _, err = copilotMetricsChartTooltips(template.HTML(corrupt), &next)
	assert.ErrorContains(t, err, "does not match")
}

func TestCopilotMetricsHTMLRendersPanelLocalFallbacks(t *testing.T) {
	login := "fictional-\"<script>&\nsecond line"
	summary := &copilotmetrics.Summary{
		Source:   "fictional organization",
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionUser: true},
		Users: []copilotmetrics.UserRow{{
			Login: login, ActiveDays: 2, Surfaces: []string{"chat"},
			ActivityTotals: copilotmetrics.ActivityTotals{
				UserInitiatedInteraction: copilotmetrics.Counter{Value: 4, Valid: true},
			},
		}},
	}
	report, err := RenderCopilotMetricsHTMLWithOptions(summary, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.NotContains(t, report, `<script>&`)
	assert.Contains(t, report, `fictional-&#34;&lt;script&gt;&amp;`)
	_, fragment := copilotMetricsTestSection(t, report, "user")
	fragment = strings.ReplaceAll(fragment, ` open>`, ` open="open">`)
	var section copilotMetricsTooltipTestNode
	require.NoError(t, xml.Unmarshal([]byte(fragment), &section))
	figures := 0
	section.walk(func(figure copilotMetricsTooltipTestNode) {
		if figure.XMLName.Local != "figure" {
			return
		}
		figures++
		targets := make(map[string]string)
		fallbacks := make(map[string]string)
		var values copilotMetricsTooltipTestNode
		figure.walk(func(node copilotMetricsTooltipTestNode) {
			if text, ok := node.attr("data-tooltip"); ok {
				target, found := node.attr("data-tooltip-target")
				require.True(t, found)
				assert.NotContains(t, targets, target)
				targets[target] = text
			}
			class, _ := node.attr("class")
			switch class {
			case "chart-tooltip-fallback":
				target, found := node.attr("data-tooltip-target")
				require.True(t, found)
				assert.NotContains(t, fallbacks, target)
				fallbacks[target] = node.Text
				hidden, _ := node.attr("aria-hidden")
				assert.Equal(t, "true", hidden)
				tabindex, _ := node.attr("tabindex")
				assert.Equal(t, "-1", tabindex)
			case "chart-values":
				values = node
			}
		})
		require.NotEmpty(t, targets)
		assert.Equal(t, targets, fallbacks)
		require.Equal(t, "details", values.XMLName.Local)
		var listed []string
		values.walk(func(node copilotMetricsTooltipTestNode) {
			if node.XMLName.Local == "li" {
				listed = append(listed, node.Text)
			}
		})
		for target, text := range targets {
			assert.Contains(t, listed, text)
			selector := fmt.Sprintf(`figure:is(`+
				`:has([data-tooltip-target="%s"]:not([data-tooltip-dismissed]):focus-visible), `+
				`:not(:has([data-tooltip]:focus-visible)):has([data-tooltip-target="%s"]:not([data-tooltip-dismissed]):hover, > .chart-tooltip-fallback[data-tooltip-target="%s"]:hover), `+
				`:not(:has([data-tooltip]:focus-visible, [data-tooltip]:hover, > .chart-tooltip-fallback:hover)):has([data-tooltip-target="%s"]:not([data-tooltip-dismissed]):focus)`+
				`) > .chart-tooltip-fallback[data-tooltip-target="%s"]`, target, target, target, target, target)
			assert.Equal(t, 1, strings.Count(report, selector))
			popup := fmt.Sprintf(`.chart-tooltip-fallback[data-tooltip-target="%s"] { position-anchor: --chart-tip-%s; }`, target, target)
			assert.Equal(t, 1, strings.Count(report, popup))
			direct := strings.Count(report, fmt.Sprintf(`[data-tooltip-target="%s"] { anchor-name: --chart-tip-%s; }`, target, target))
			proxy := strings.Count(report, fmt.Sprintf(`<span class="chart-tooltip-anchor" data-tooltip-anchor="%s" aria-hidden="true"></span><div class="chart-tooltip-fallback" data-tooltip-target="%s"`, target, target))
			assert.Equal(t, 1, direct+proxy, "each popup has exactly one anchor")
			if proxy == 1 {
				assert.Contains(t, report, fmt.Sprintf(`.chart-tooltip-anchor[data-tooltip-anchor="%s"] { position-anchor: --chart-svg-`, target))
				assert.Contains(t, report, fmt.Sprintf(`anchor-name: --chart-tip-%s; }`, target))
			}
		}
		distinct := make(map[string]bool, len(targets))
		for _, text := range targets {
			distinct[text] = true
		}
		assert.Len(t, listed, len(distinct))
	})
	assert.Positive(t, figures)
	assert.Contains(t, report, `.js-enabled figure:not(.tooltip-failed) .chart-values { display: none; }`)
}

func TestCopilotMetricsRepositoryFallbackHitAreas(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	chart := copilotMetricsSVGRepositoryStackedBar("Repositories", []copilotMetricsRepositoryBar{
		{Name: "fictional/zero", Total: counter(0), Copilot: counter(0), Other: counter(0)},
		{Name: "fictional/nonzero", Total: counter(2), Copilot: counter(1), Other: counter(1)},
	}, "Copilot", "Other", false)
	var document struct {
		SVG struct {
			Bars []struct {
				Tooltip string `xml:"data-tooltip,attr"`
				Title   string `xml:"title"`
				Rects   []struct {
					Class  string  `xml:"class,attr"`
					X      float64 `xml:"x,attr"`
					Y      float64 `xml:"y,attr"`
					Width  float64 `xml:"width,attr"`
					Height float64 `xml:"height,attr"`
				} `xml:"rect"`
			} `xml:"g"`
		} `xml:"svg"`
	}
	require.NoError(t, xml.Unmarshal([]byte("<root>"+string(chart)+"</root>"), &document))
	require.Len(t, document.SVG.Bars, 2)
	for index, bar := range document.SVG.Bars {
		assert.NotEmpty(t, bar.Tooltip)
		assert.Empty(t, bar.Title, "native titles would duplicate the tooltip")
		require.NotEmpty(t, bar.Rects)
		hit := bar.Rects[0]
		assert.Equal(t, "repository-chart-hit", hit.Class)
		assert.Equal(t, copilotMetricsRepositoryChartLeft, hit.X)
		assert.Equal(t, copilotMetricsRepositoryChartWidth-copilotMetricsRepositoryChartLeft-copilotMetricsRepositoryChartRight, hit.Width)
		assert.Equal(t, copilotMetricsRepositoryChartTop+25*float64(index), hit.Y)
		assert.Equal(t, 14.0, hit.Height)
	}
	assert.Len(t, document.SVG.Bars[0].Rects, 1, "a zero bar still has a hit area")
	assert.LessOrEqual(t, document.SVG.Bars[0].Rects[0].Y+document.SVG.Bars[0].Rects[0].Height, document.SVG.Bars[1].Rects[0].Y)
}

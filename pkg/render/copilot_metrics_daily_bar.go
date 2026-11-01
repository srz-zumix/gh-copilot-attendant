package render

import (
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"
)

const copilotMetricsChatModeRequestsTitle = "Requests per chat mode"

type copilotMetricsDailyStackedBarOptions struct {
	externalLegend    bool
	incompleteNote    string
	incompleteCaption string
	// markerBelowAxis places incomplete-day markers between the x-axis and the date labels.
	markerBelowAxis bool
}

// copilotMetricsDailyStackedBarColors assigns theme colors to named series in
// their display order and reserves gray for Other.
func copilotMetricsDailyStackedBarColors(theme copilotMetricsColorTheme, series []copilotMetricsPlotSeries) map[string]copilotMetricsColor {
	var names []string
	for _, item := range series {
		if name := copilotMetricsNormalizeOther(item.Name); name != "Other" {
			names = append(names, name)
		}
	}
	colors := copilotMetricsThemeColors(theme, names)
	colors["Other"] = copilotMetricsOtherColor
	return colors
}

// copilotMetricsDailyStackedBarTooltip lists each series and its available share.
func copilotMetricsDailyStackedBarTooltip(day string, index int, series []copilotMetricsPlotSeries, total float64, incompleteNote string) copilotMetricsTooltip {
	tooltip := copilotMetricsTooltip{Heading: day}
	complete, hasValue := true, false
	for _, item := range series {
		if index >= len(item.Values) || index >= len(item.Valid) || !item.Valid[index] {
			complete = false
		} else {
			hasValue = true
		}
	}
	share := func(value float64) string {
		if total <= 0 {
			return copilotMetricsNA
		}
		return strconv.FormatFloat(value/total*100, 'f', 1, 64) + "%"
	}
	for _, item := range series {
		row := copilotMetricsTooltipRow{Label: item.Name, Value: copilotMetricsNA}
		if index < len(item.Values) && index < len(item.Valid) && item.Valid[index] {
			row.Value = formatCopilotMetricsInt(int64(math.Round(item.Values[index])))
			if incompleteNote == "" || complete {
				row.Share = share(item.Values[index])
			}
		}
		tooltip.Rows = append(tooltip.Rows, row)
	}
	if incompleteNote != "" && !complete {
		subtotal := copilotMetricsNA
		if hasValue {
			subtotal = formatCopilotMetricsInt(int64(math.Round(total)))
		}
		tooltip.Rows = append(tooltip.Rows, copilotMetricsTooltipRow{Label: "Displayed subtotal", Value: subtotal})
		tooltip.Note = incompleteNote
		return tooltip
	}
	tooltip.Rows = append(tooltip.Rows, copilotMetricsTooltipRow{
		Label: "Total", Value: formatCopilotMetricsInt(int64(math.Round(total))), Share: share(total),
	})
	return tooltip
}

// copilotMetricsDailyStackedBarChart renders one stacked bar per day with
// the series stacked in display order from the bottom up.
func copilotMetricsDailyStackedBarChart(title string, days []string, series []copilotMetricsPlotSeries, theme copilotMetricsColorTheme, options copilotMetricsDailyStackedBarOptions) template.HTML {
	if len(days) == 0 || len(series) == 0 {
		return ""
	}
	totals := make([]float64, len(days))
	hasValue := make([]bool, len(days))
	hasData := false
	for _, item := range series {
		for day := range days {
			if day < len(item.Values) && day < len(item.Valid) && item.Valid[day] {
				totals[day] += item.Values[day]
				hasValue[day] = true
				hasData = true
			}
		}
	}
	if !hasData && options.incompleteNote == "" {
		return ""
	}
	maxValue := 0.0
	for _, total := range totals {
		maxValue = math.Max(maxValue, total)
	}
	if maxValue <= 0 {
		maxValue = 1
	}
	const width, left, right, bottom = 720.0, 72.0, 35.0, 38.0
	legendRows := (len(series) + 2) / 3
	top := 36.0 + float64(legendRows-1)*14
	height := 205.0 + float64(legendRows-1)*14
	if options.externalLegend {
		top, height = 28, 197
	}
	plotWidth := width - left - right
	plotHeight := height - top - bottom
	step := plotWidth / float64(len(days))
	x := func(index int) float64 { return left + step*(float64(index)+.5) }
	y := func(value float64) float64 { return top + plotHeight*(1-value/maxValue) }
	colors := copilotMetricsDailyStackedBarColors(theme, series)
	seriesColor := func(name string) string { return colors[copilotMetricsNormalizeOther(name)].hex() }

	var b strings.Builder
	if options.externalLegend {
		fmt.Fprintf(&b, `<ul class="stacked-legend" aria-label="%s legend">`, template.HTMLEscapeString(title))
		for _, item := range series {
			fmt.Fprintf(&b, `<li><span class="series-key" style="background-color:%s" aria-hidden="true"></span><span class="stacked-legend-label">%s</span></li>`,
				seriesColor(item.Name), template.HTMLEscapeString(item.Name))
		}
		b.WriteString(`</ul>`)
	}
	if options.incompleteCaption != "" {
		fmt.Fprintf(&b, `<p class="chart-color-key">%s</p>`, template.HTMLEscapeString(options.incompleteCaption))
	}
	fmt.Fprintf(&b, `<svg class="metric-chart metric-daily-stacked-bar" viewBox="0 0 %.0f %.0f" role="group" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		width, height, template.HTMLEscapeString(title))
	for _, fraction := range []float64{0, 0.5, 1} {
		gy := top + plotHeight*(1-fraction)
		fmt.Fprintf(&b, `<line class="grid" x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f"/>`, left, gy, width-right, gy)
		fmt.Fprintf(&b, `<text class="axis" x="%.0f" y="%.1f" text-anchor="end">%s</text>`,
			left-7, gy+4, template.HTMLEscapeString(formatCopilotMetricsInt(int64(math.Round(maxValue*fraction)))))
	}
	labelEvery := 1
	if len(days) > 14 {
		labelEvery = (len(days) + 13) / 14
	}
	for i, day := range days {
		if i%labelEvery != 0 && i != len(days)-1 {
			continue
		}
		label := day
		if len(label) == len("2006-01-02") {
			label = label[5:]
		}
		fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.0f" text-anchor="middle">%s</text>`, x(i), height-10, template.HTMLEscapeString(label))
	}
	barWidth := step * .7
	for day := range days {
		if !hasValue[day] && options.incompleteNote == "" {
			continue
		}
		b.WriteString(`<g class="chart-day daily-stacked-bar">`)
		stacked := 0.0
		complete := true
		for _, item := range series {
			if day >= len(item.Values) || day >= len(item.Valid) || !item.Valid[day] {
				complete = false
				continue
			}
			if item.Values[day] <= 0 {
				continue
			}
			fmt.Fprintf(&b, `<rect class="daily-stacked-segment" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" aria-hidden="true"/>`,
				x(day)-barWidth/2, y(stacked+item.Values[day]), barWidth, y(stacked)-y(stacked+item.Values[day]), seriesColor(item.Name))
			stacked += item.Values[day]
		}
		if options.incompleteNote != "" && !complete {
			markerY := top - 6
			if options.markerBelowAxis {
				markerY = height - 23
			}
			fmt.Fprintf(&b, `<text class="daily-stacked-incomplete-marker" x="%.1f" y="%.1f" text-anchor="middle" aria-hidden="true">*</text>`, x(day), markerY)
		}
		tooltip := copilotMetricsDailyStackedBarTooltip(days[day], day, series, totals[day], options.incompleteNote)
		label := template.HTMLEscapeString(tooltip.text())
		fmt.Fprintf(&b, `<rect class="metric-day-hit" x="%.3f" y="%.3f" width="%.3f" height="%.3f" %s tabindex="0" role="img" aria-label="%s"></rect></g>`,
			x(day)-step/2, top-3, step, plotHeight+6, tooltip.svgAttributes(x(day)-step/2, top-3, step, plotHeight+6), label)
	}
	if !options.externalLegend {
		for seriesIndex, item := range series {
			legendX := left + float64(seriesIndex%3)*215
			legendY := 15 + float64(seriesIndex/3)*14
			fmt.Fprintf(&b, `<rect class="area-legend-key" x="%.1f" y="%.1f" width="14" height="6" fill="%s" aria-hidden="true"/>`,
				legendX, legendY-7, seriesColor(item.Name))
			fmt.Fprintf(&b, `<text class="legend" x="%.1f" y="%.1f">%s</text>`,
				legendX+19, legendY, template.HTMLEscapeString(item.Name))
		}
	}
	b.WriteString(`</svg>`)
	// #nosec G203 -- all labels are HTML-escaped; plotted coordinates and colors are generated internally.
	return template.HTML(b.String())
}

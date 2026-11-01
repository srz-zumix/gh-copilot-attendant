package render

import (
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const (
	copilotMetricsRepositoryChartLimit  = 7
	copilotMetricsRepositoryChartWidth  = 720.0
	copilotMetricsRepositoryChartLeft   = 218.0
	copilotMetricsRepositoryChartRight  = 16.0
	copilotMetricsRepositoryChartTop    = 22.0
	copilotMetricsRepositoryChartBottom = 26.0
)

type copilotMetricsRepositoryLegendItem struct {
	Label string
	Color string
}

type copilotMetricsRepositoryBar struct {
	Name    string
	Total   copilotmetrics.Counter
	Copilot copilotmetrics.Counter
	Other   copilotmetrics.Counter
	// Partial marks totals that omit days reporting only one of the paired counts.
	Partial bool
}

type copilotMetricsRepositoryChartSpec struct {
	title       string
	copilotName string
	otherName   string
	pair        func(copilotmetrics.RepositoryRow) copilotmetrics.CounterPair
	review      bool
}

func copilotMetricsRepositoryPanels(rows []copilotmetrics.RepositoryRow, rich *copilotmetrics.RichMetrics) []copilotMetricsHTMLPanel {
	reviewed := copilotMetricsRepositoryPanel(rows, copilotMetricsRepositoryChartSpec{
		title: "Pull requests reviewed by repository", copilotName: "Reviewed with Copilot",
		otherName: "Reviewed without Copilot",
		pair:      func(row copilotmetrics.RepositoryRow) copilotmetrics.CounterPair { return row.ReviewedPair },
		review:    true,
	})
	suggestions := copilotMetricsRepositorySuggestionsPanel(rich)
	if reviewed.Chart != "" && suggestions.Chart != "" {
		reviewed.HalfWidth = true
		suggestions.HalfWidth = true
	}
	panels := make([]copilotMetricsHTMLPanel, 0, 4)
	for _, panel := range []copilotMetricsHTMLPanel{reviewed, suggestions} {
		if panel.Chart != "" {
			panels = append(panels, panel)
		}
	}

	specs := []copilotMetricsRepositoryChartSpec{
		{
			title: "Pull requests created by repository", copilotName: "Copilot-created",
			otherName: "Other-created",
			pair:      func(row copilotmetrics.RepositoryRow) copilotmetrics.CounterPair { return row.CreatedPair },
		},
		{
			title: "Pull requests merged by repository", copilotName: "Merged Copilot-created",
			otherName: "Merged other-created",
			pair:      func(row copilotmetrics.RepositoryRow) copilotmetrics.CounterPair { return row.MergedPair },
		},
	}

	for _, spec := range specs {
		panel := copilotMetricsRepositoryPanel(rows, spec)
		if panel.Chart != "" {
			panels = append(panels, panel)
		}
	}
	createdIndex, mergedIndex := -1, -1
	for index, panel := range panels {
		switch panel.Title {
		case "Pull requests created by repository":
			createdIndex = index
		case "Pull requests merged by repository":
			mergedIndex = index
		}
	}
	if createdIndex >= 0 && mergedIndex >= 0 {
		panels[createdIndex].HalfWidth = true
		panels[mergedIndex].HalfWidth = true
	}
	return panels
}

func copilotMetricsRepositoryPanel(rows []copilotmetrics.RepositoryRow, spec copilotMetricsRepositoryChartSpec) copilotMetricsHTMLPanel {
	bars := copilotMetricsRepositoryBars(rows, spec)
	if len(bars) == 0 {
		return copilotMetricsHTMLPanel{}
	}
	return copilotMetricsHTMLPanel{
		Title: spec.title,
		Chart: copilotMetricsSVGRepositoryStackedBar(spec.title, bars, spec.copilotName, spec.otherName, spec.review),
	}
}

func copilotMetricsRepositoryBars(rows []copilotmetrics.RepositoryRow, spec copilotMetricsRepositoryChartSpec) []copilotMetricsRepositoryBar {
	bars := make([]copilotMetricsRepositoryBar, 0, len(rows))
	for _, row := range rows {
		pair := spec.pair(row)
		total, copilot := pair.Total, pair.Copilot
		if !total.Valid || !copilot.Valid || total.Value < 0 || copilot.Value < 0 {
			continue
		}
		other := total.Value - copilot.Value
		if other < 0 {
			other = 0
		}
		bars = append(bars, copilotMetricsRepositoryBar{
			Name: row.Name, Total: total, Copilot: copilot,
			Other: copilotmetrics.Counter{Value: other, Valid: true}, Partial: pair.Partial,
		})
	}
	sort.SliceStable(bars, func(i, j int) bool {
		if bars[i].Total.Value != bars[j].Total.Value {
			return bars[i].Total.Value > bars[j].Total.Value
		}
		return bars[i].Name < bars[j].Name
	})
	if len(bars) <= copilotMetricsRepositoryChartLimit {
		return bars
	}

	other := copilotMetricsRepositoryBar{
		Name:    "Other repositories",
		Total:   copilotmetrics.Counter{Valid: true},
		Copilot: copilotmetrics.Counter{Valid: true},
		Other:   copilotmetrics.Counter{Valid: true},
	}
	for _, bar := range bars[copilotMetricsRepositoryChartLimit:] {
		other.Total.AddCounter(bar.Total)
		other.Copilot.AddCounter(bar.Copilot)
		other.Other.AddCounter(bar.Other)
		other.Partial = other.Partial || bar.Partial
	}
	bars = append(bars[:copilotMetricsRepositoryChartLimit], other)
	return bars
}

func copilotMetricsSVGRepositoryStackedBar(title string, bars []copilotMetricsRepositoryBar, copilotName, otherName string, review bool) template.HTML {
	if len(bars) == 0 {
		return ""
	}
	const (
		rowHeight  = 25.0
		barHeight  = 14.0
		copilotHex = "#2da44e"
		otherHex   = "#6e7781"
	)
	maxValue := int64(0)
	for _, bar := range bars {
		maxValue = max(maxValue, bar.Total.Value, bar.Copilot.Value)
	}
	var b strings.Builder
	copilotMetricsWriteRepositoryLegend(&b, title, []copilotMetricsRepositoryLegendItem{
		{Label: copilotName, Color: copilotHex}, {Label: otherName, Color: otherHex},
	})
	scale := copilotMetricsOpenRepositoryChart(&b, title, len(bars), rowHeight, maxValue)
	for index, bar := range bars {
		y := copilotMetricsRepositoryChartTop + float64(index)*rowHeight
		copilotWidth := float64(bar.Copilot.Value) * scale
		otherWidth := float64(bar.Other.Value) * scale
		var notes []string
		if review {
			notes = append(notes, "Review counts sum daily repository reports; a pull request reviewed on multiple days can be counted more than once.")
		}
		totalLabel := "Total"
		if bar.Partial {
			totalLabel = "Displayed subtotal"
			notes = append(notes, "Days reporting only one of the paired counts are excluded; shares are not shown.")
		}
		note := strings.Join(notes, " ")
		share := func(value int64) string {
			if bar.Partial {
				return ""
			}
			if bar.Total.Value == 0 {
				return copilotMetricsNA
			}
			return fmt.Sprintf("%.1f%%", float64(value)/float64(bar.Total.Value)*100)
		}
		tooltip := copilotMetricsTooltip{
			Heading: bar.Name,
			Rows: []copilotMetricsTooltipRow{
				{Label: copilotName, Value: formatCopilotMetricsInt(bar.Copilot.Value), Share: share(bar.Copilot.Value)},
				{Label: otherName, Value: formatCopilotMetricsInt(bar.Other.Value), Share: share(bar.Other.Value)},
				{Label: totalLabel, Value: formatCopilotMetricsInt(bar.Total.Value), Share: share(bar.Total.Value)},
			},
			Note: note,
		}
		aria := template.HTMLEscapeString(tooltip.text())
		copilotMetricsWriteRepositoryLabel(&b, bar.Name, y+(barHeight-rowHeight)/2, rowHeight)
		hitWidth := copilotMetricsRepositoryChartWidth - copilotMetricsRepositoryChartLeft - copilotMetricsRepositoryChartRight
		fmt.Fprintf(&b, `<g class="repository-bar" %s tabindex="0" role="img" aria-label="%s">`,
			tooltip.svgAttributes(copilotMetricsRepositoryChartLeft, y, hitWidth, barHeight), aria)
		fmt.Fprintf(&b, `<rect class="repository-chart-hit" x="%.1f" y="%.1f" width="%.1f" height="%.1f" aria-hidden="true"/>`,
			copilotMetricsRepositoryChartLeft, y, hitWidth, barHeight)
		if copilotWidth > 0 {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" aria-hidden="true"/>`,
				copilotMetricsRepositoryChartLeft, y, copilotWidth, barHeight, copilotHex)
		}
		if otherWidth > 0 {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" aria-hidden="true"/>`,
				copilotMetricsRepositoryChartLeft+copilotWidth, y, otherWidth, barHeight, otherHex)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	// #nosec G203 -- labels are escaped; coordinates and colors are generated internally.
	return template.HTML(b.String())
}

func copilotMetricsWriteRepositoryLabel(b *strings.Builder, name string, y, height float64) {
	fmt.Fprintf(b, `<foreignObject x="0" y="%.1f" width="%.1f" height="%.1f" aria-hidden="true"><div xmlns="http://www.w3.org/1999/xhtml" class="repository-label-wrap"><div class="repository-label">%s</div></div></foreignObject>`,
		y, copilotMetricsRepositoryChartLeft-8, height, template.HTMLEscapeString(name))
}

func copilotMetricsWriteRepositoryLegend(b *strings.Builder, title string, items []copilotMetricsRepositoryLegendItem) {
	fmt.Fprintf(b, `<ul class="stacked-legend" aria-label="%s legend">`, template.HTMLEscapeString(title))
	for _, item := range items {
		fmt.Fprintf(b, `<li><span class="series-key" style="background-color:%s" aria-hidden="true"></span><span class="stacked-legend-label">%s</span></li>`,
			item.Color, template.HTMLEscapeString(item.Label))
	}
	b.WriteString(`</ul>`)
}

func copilotMetricsOpenRepositoryChart(b *strings.Builder, title string, rows int, rowHeight float64, maxValue int64) float64 {
	if maxValue <= 0 {
		maxValue = 1
	}
	plotWidth := copilotMetricsRepositoryChartWidth - copilotMetricsRepositoryChartLeft - copilotMetricsRepositoryChartRight
	plotHeight := rowHeight * float64(rows)
	height := copilotMetricsRepositoryChartTop + plotHeight + copilotMetricsRepositoryChartBottom
	fmt.Fprintf(b, `<svg class="metric-chart repository-chart" viewBox="0 0 %.0f %.1f" role="group" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		copilotMetricsRepositoryChartWidth, height, template.HTMLEscapeString(title))
	for _, fraction := range []float64{0, .5, 1} {
		x := copilotMetricsRepositoryChartLeft + plotWidth*fraction
		value := float64(maxValue) * fraction
		fmt.Fprintf(b, `<line class="grid" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			x, copilotMetricsRepositoryChartTop-3, x, copilotMetricsRepositoryChartTop+plotHeight)
		fmt.Fprintf(b, `<text class="axis" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x, height-5, formatCopilotMetricsAxisValue(value))
	}
	return plotWidth / float64(maxValue)
}

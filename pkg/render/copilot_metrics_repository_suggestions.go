package render

import (
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const copilotMetricsRepositorySuggestionsTitle = "Copilot review suggestions by repository"

type copilotMetricsRepositorySuggestionBar struct {
	ID            int64
	Name          string
	Generated     copilotmetrics.Counter
	GeneratedFull bool
	Applied       copilotmetrics.Counter
	AppliedFull   bool
}

func copilotMetricsRepositorySuggestionsPanel(rich *copilotmetrics.RichMetrics) copilotMetricsHTMLPanel {
	if rich == nil {
		return copilotMetricsHTMLPanel{}
	}
	bars := copilotMetricsRepositorySuggestionBars(rich.Repositories)
	if len(bars) == 0 {
		return copilotMetricsHTMLPanel{}
	}
	return copilotMetricsHTMLPanel{
		Title: copilotMetricsRepositorySuggestionsTitle,
		Chart: copilotMetricsSVGRepositorySuggestionBars(bars),
	}
}

func copilotMetricsRepositorySuggestionBars(repositories []copilotmetrics.RichRepository) []copilotMetricsRepositorySuggestionBar {
	bars := make([]copilotMetricsRepositorySuggestionBar, 0, len(repositories))
	for _, repository := range repositories {
		bars = append(bars, copilotMetricsRepositorySuggestionBar{
			ID: repository.ID, Name: repository.Name,
			Generated: repository.Suggestions, GeneratedFull: repository.SuggestionsComplete,
			Applied: repository.AppliedSuggestions, AppliedFull: repository.AppliedSuggestionsComplete,
		})
	}
	sort.Slice(bars, func(i, j int) bool {
		if bars[i].Generated.Valid != bars[j].Generated.Valid {
			return bars[i].Generated.Valid
		}
		if bars[i].Generated.Value != bars[j].Generated.Value {
			return bars[i].Generated.Value > bars[j].Generated.Value
		}
		if bars[i].Name != bars[j].Name {
			return bars[i].Name < bars[j].Name
		}
		return bars[i].ID < bars[j].ID
	})
	if len(bars) <= copilotMetricsRepositoryChartLimit {
		return bars
	}
	other := copilotMetricsRepositorySuggestionBar{
		Name: "Other repositories", GeneratedFull: true, AppliedFull: true,
	}
	for _, bar := range bars[copilotMetricsRepositoryChartLimit:] {
		other.Generated.AddCounter(bar.Generated)
		other.Applied.AddCounter(bar.Applied)
		other.GeneratedFull = other.GeneratedFull && bar.Generated.Valid && bar.GeneratedFull
		other.AppliedFull = other.AppliedFull && bar.Applied.Valid && bar.AppliedFull
	}
	return append(bars[:copilotMetricsRepositoryChartLimit], other)
}

func copilotMetricsRepositorySuggestionValue(value copilotmetrics.Counter, complete bool) string {
	if !value.Valid {
		return copilotMetricsNA
	}
	formatted := formatCopilotMetricsInt(value.Value)
	if !complete {
		formatted += " (partial)"
	}
	return formatted
}

func copilotMetricsSVGRepositorySuggestionBars(bars []copilotMetricsRepositorySuggestionBar) template.HTML {
	if len(bars) == 0 {
		return ""
	}
	const (
		rowHeight    = 34.0
		barHeight    = 9.0
		barTopOffset = 2.0
		barSpacing   = 14.0
	)
	generatedColor := copilotMetricsThemeRamps[copilotMetricsThemeGreen][0].hex()
	appliedColor := copilotMetricsThemeRamps[copilotMetricsThemeGreen][1].hex()
	maxValue := int64(0)
	for _, bar := range bars {
		if bar.Generated.Valid {
			maxValue = max(maxValue, bar.Generated.Value)
		}
		if bar.Applied.Valid {
			maxValue = max(maxValue, bar.Applied.Value)
		}
	}

	var b strings.Builder
	copilotMetricsWriteRepositoryLegend(&b, copilotMetricsRepositorySuggestionsTitle, []copilotMetricsRepositoryLegendItem{
		{Label: "Generated suggestions", Color: generatedColor},
		{Label: "Applied suggestions", Color: appliedColor},
	})
	b.WriteString(`<p class="chart-color-key">Ranked by known generated counts. * marks partial totals. Generated and applied suggestions may differ.</p>`)
	scale := copilotMetricsOpenRepositoryChart(&b, copilotMetricsRepositorySuggestionsTitle, len(bars), rowHeight, maxValue)
	for index, bar := range bars {
		y := copilotMetricsRepositoryChartTop + float64(index)*rowHeight
		tooltip := copilotMetricsTooltip{
			Heading: bar.Name,
			Rows: []copilotMetricsTooltipRow{
				{Label: "Generated suggestions", Value: copilotMetricsRepositorySuggestionValue(bar.Generated, bar.GeneratedFull)},
				{Label: "Applied suggestions", Value: copilotMetricsRepositorySuggestionValue(bar.Applied, bar.AppliedFull)},
			},
			Note: "Generated and applied suggestions are counted on their respective days and may refer to different suggestions.",
		}
		labelCenter := y + barTopOffset + (barSpacing+barHeight)/2
		copilotMetricsWriteRepositoryLabel(&b, bar.Name, labelCenter-rowHeight/2, rowHeight)
		fmt.Fprintf(&b, `<g class="repository-bar" %s tabindex="0" role="img" aria-label="%s">`,
			tooltip.svgAttributes(copilotMetricsRepositoryChartLeft, y, copilotMetricsRepositoryChartWidth-copilotMetricsRepositoryChartLeft-copilotMetricsRepositoryChartRight, rowHeight), template.HTMLEscapeString(tooltip.text()))
		fmt.Fprintf(&b, `<rect class="repository-chart-hit" x="%.1f" y="%.1f" width="%.1f" height="%.1f" aria-hidden="true"/>`,
			copilotMetricsRepositoryChartLeft, y, copilotMetricsRepositoryChartWidth-copilotMetricsRepositoryChartLeft-copilotMetricsRepositoryChartRight, rowHeight)
		for seriesIndex, item := range []struct {
			value    copilotmetrics.Counter
			complete bool
			color    string
		}{
			{bar.Generated, bar.GeneratedFull, generatedColor},
			{bar.Applied, bar.AppliedFull, appliedColor},
		} {
			barY := y + barTopOffset + float64(seriesIndex)*barSpacing
			if !item.value.Valid {
				fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.1f">n/a</text>`, copilotMetricsRepositoryChartLeft+4, barY+barHeight)
				continue
			}
			barWidth := float64(item.value.Value) * scale
			if barWidth > 0 {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" aria-hidden="true"/>`,
					copilotMetricsRepositoryChartLeft, barY, barWidth, barHeight, item.color)
			} else {
				fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.1f">0</text>`, copilotMetricsRepositoryChartLeft+4, barY+barHeight)
			}
			if !item.complete {
				fmt.Fprintf(&b, `<text class="repository-incomplete-marker" x="%.1f" y="%.1f" aria-hidden="true">*</text>`,
					copilotMetricsRepositoryChartLeft+barWidth+4, barY+barHeight)
			}
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	// #nosec G203 -- labels are escaped; coordinates and colors are generated internally.
	return template.HTML(b.String())
}

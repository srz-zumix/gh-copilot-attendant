package render

import (
	"sort"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const copilotMetricsRepositoryDailyReviewsTitle = "Copilot reviews per day by repository"

func copilotMetricsRepositoryDailyPanel(rich *copilotmetrics.RichMetrics) copilotMetricsHTMLPanel {
	if rich == nil {
		return copilotMetricsHTMLPanel{}
	}
	series := copilotMetricsRepositoryReviewSeries(rich.Repositories, len(rich.Days))
	chart := copilotMetricsDailyStackedBarChart(
		copilotMetricsRepositoryDailyReviewsTitle, rich.Days, series, copilotMetricsThemeGreen,
		copilotMetricsDailyStackedBarOptions{
			externalLegend:    true,
			incompleteNote:    "Some repository counts are unavailable; the total and shares are not calculated.",
			incompleteCaption: "Repositories are ranked by available counts. * marks incomplete days; bars omit categories with missing values.",
		},
	)
	if chart == "" {
		return copilotMetricsHTMLPanel{}
	}
	return copilotMetricsHTMLPanel{Title: copilotMetricsRepositoryDailyReviewsTitle, Chart: chart}
}

func copilotMetricsRepositoryReviewSeries(repositories []copilotmetrics.RichRepository, days int) []copilotMetricsPlotSeries {
	type rankedRepository struct {
		id     int64
		total  int64
		series copilotMetricsPlotSeries
	}
	ranked := make([]rankedRepository, 0, len(repositories))
	for _, repository := range repositories {
		item := rankedRepository{
			id: repository.ID,
			series: copilotMetricsPlotSeries{
				Name: repository.Name, Values: make([]float64, days), Valid: make([]bool, days),
			},
		}
		for day, value := range repository.Reviews {
			if day >= days {
				break
			}
			if value.Valid && value.Value >= 0 {
				item.total += value.Value
				item.series.Values[day] = float64(value.Value)
				item.series.Valid[day] = true
			}
		}
		ranked = append(ranked, item)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].total != ranked[j].total {
			return ranked[i].total > ranked[j].total
		}
		if ranked[i].series.Name != ranked[j].series.Name {
			return ranked[i].series.Name < ranked[j].series.Name
		}
		return ranked[i].id < ranked[j].id
	})
	count := min(len(ranked), copilotMetricsRepositoryChartLimit)
	series := make([]copilotMetricsPlotSeries, 0, count+1)
	for _, item := range ranked[:count] {
		series = append(series, item.series)
	}
	if len(ranked) <= count {
		return series
	}
	other := copilotMetricsPlotSeries{
		Name: "Other", Values: make([]float64, days), Valid: make([]bool, days),
	}
	for day := range days {
		other.Valid[day] = true
		for _, item := range ranked[count:] {
			if !item.series.Valid[day] {
				other.Valid[day] = false
				continue
			}
			other.Values[day] += item.series.Values[day]
		}
	}
	return append(series, other)
}

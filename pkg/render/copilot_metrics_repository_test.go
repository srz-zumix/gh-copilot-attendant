package render

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopilotMetricsRepositoryPanels(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	rows := []copilotmetrics.RepositoryRow{
		{
			Name: "octo/repo-b", Reviewed: counter(4), ReviewedByCopilot: counter(1),
			Created: counter(8), CreatedByCopilot: counter(2), Merged: counter(6), MergedCreatedByCopilot: counter(1),
		},
		{
			Name: "octo/repo-a", Reviewed: counter(8), ReviewedByCopilot: counter(4),
			Created: counter(3), CreatedByCopilot: counter(3), Merged: counter(4), MergedCreatedByCopilot: counter(4),
		},
	}
	panels := copilotMetricsRepositoryPanels(rows, nil)
	require.Len(t, panels, 3)
	assert.False(t, panels[0].HalfWidth)
	assert.True(t, panels[1].HalfWidth)
	assert.True(t, panels[2].HalfWidth)

	review := string(panels[0].Chart)
	assert.Less(t, strings.Index(review, "octo/repo-a"), strings.Index(review, "octo/repo-b"))
	assert.Contains(t, review, `aria-label="Pull requests reviewed by repository"`)
	assert.Contains(t, review, "Reviewed with Copilot: 4 (50.0%)")
	assert.Contains(t, review, "Reviewed without Copilot: 4 (50.0%)")
	assert.Contains(t, review, "reviewed on multiple days can be counted more than once")
	assert.Contains(t, review, `fill="#2da44e"`)
	assert.Contains(t, review, `fill="#6e7781"`)
	assert.Contains(t, review, `tabindex="0" role="img"`)

	created := string(panels[1].Chart)
	assert.Less(t, strings.Index(created, "octo/repo-b"), strings.Index(created, "octo/repo-a"))
	assert.Contains(t, created, "Copilot-created: 2 (25.0%)")
	assert.Contains(t, created, "Other-created: 6 (75.0%)")

	merged := string(panels[2].Chart)
	assert.Contains(t, merged, "Merged Copilot-created: 4 (100.0%)")
	assert.Contains(t, merged, "Merged other-created: 0 (0.0%)")
}

func TestCopilotMetricsRepositoryReviewSuggestionPairing(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	row := copilotmetrics.RepositoryRow{
		Name: "example/repo", Reviewed: counter(4), ReviewedByCopilot: counter(2),
		Created: counter(3), CreatedByCopilot: counter(1), Merged: counter(2), MergedCreatedByCopilot: counter(1),
	}
	withoutReview := row
	withoutReview.Reviewed = copilotmetrics.Counter{}
	withoutReview.ReviewedByCopilot = copilotmetrics.Counter{}
	rich := &copilotmetrics.RichMetrics{Repositories: []copilotmetrics.RichRepository{{
		ID: 1, Name: "example/repo", Suggestions: counter(4), SuggestionsComplete: true,
		AppliedSuggestions: counter(2), AppliedSuggestionsComplete: true,
	}}}
	const (
		reviewed = "Pull requests reviewed by repository"
		created  = "Pull requests created by repository"
		merged   = "Pull requests merged by repository"
	)
	tests := []struct {
		name   string
		rows   []copilotmetrics.RepositoryRow
		rich   *copilotmetrics.RichMetrics
		titles []string
		halves []bool
	}{
		{"both available", []copilotmetrics.RepositoryRow{row}, rich,
			[]string{reviewed, copilotMetricsRepositorySuggestionsTitle, created, merged}, []bool{true, true, true, true}},
		{"review unavailable", []copilotmetrics.RepositoryRow{withoutReview}, rich,
			[]string{copilotMetricsRepositorySuggestionsTitle, created, merged}, []bool{false, true, true}},
		{"suggestions unavailable", []copilotmetrics.RepositoryRow{row}, nil,
			[]string{reviewed, created, merged}, []bool{false, true, true}},
		{"suggestions only", nil, rich, []string{copilotMetricsRepositorySuggestionsTitle}, []bool{false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panels := copilotMetricsRepositoryPanels(tt.rows, tt.rich)
			require.Len(t, panels, len(tt.titles))
			for i, panel := range panels {
				assert.Equal(t, tt.titles[i], panel.Title)
				assert.Equal(t, tt.halves[i], panel.HalfWidth)
				assert.NotEmpty(t, panel.Chart)
			}
		})
	}
}

func TestCopilotMetricsRepositoryChartsAggregateRemainingRepositories(t *testing.T) {
	counter := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	var rows []copilotmetrics.RepositoryRow
	for index := 1; index <= copilotMetricsRepositoryChartLimit+2; index++ {
		rows = append(rows, copilotmetrics.RepositoryRow{
			Name:     fmt.Sprintf("octo/repo-%02d", index),
			Reviewed: counter(int64(index)), ReviewedByCopilot: counter(int64(index / 2)),
			Created: counter(int64(index)), CreatedByCopilot: counter(int64(index / 2)),
			Merged: counter(int64(index)), MergedCreatedByCopilot: counter(int64(index / 2)),
		})
	}
	bars := copilotMetricsRepositoryBars(rows, copilotMetricsRepositoryChartSpec{
		total:   func(row copilotmetrics.RepositoryRow) copilotmetrics.Counter { return row.Created },
		copilot: func(row copilotmetrics.RepositoryRow) copilotmetrics.Counter { return row.CreatedByCopilot },
	})
	require.Len(t, bars, copilotMetricsRepositoryChartLimit+1)
	assert.Equal(t, "octo/repo-09", bars[0].Name)
	assert.Equal(t, "Other repositories", bars[len(bars)-1].Name)
	assert.Equal(t, int64(3), bars[len(bars)-1].Total.Value)
	assert.Equal(t, int64(1), bars[len(bars)-1].Copilot.Value)
	assert.Equal(t, int64(2), bars[len(bars)-1].Other.Value)

	chart := string(copilotMetricsSVGRepositoryStackedBar("Repositories", bars, "Copilot", "Other", false))
	assert.Contains(t, chart, "Other repositories\nCopilot: 1 (33.3%)\nOther: 2 (66.7%)\nTotal: 3 (100.0%)")
}

func TestCopilotMetricsRepositoryChartsEmptyAndInvalid(t *testing.T) {
	assert.Empty(t, copilotMetricsRepositoryPanels(nil, nil))
	assert.Empty(t, copilotMetricsSVGRepositoryStackedBar("Empty", nil, "Copilot", "Other", false))
	assert.Empty(t, copilotMetricsRepositoryBars([]copilotmetrics.RepositoryRow{{
		Name: "octo/no-data",
	}}, copilotMetricsRepositoryChartSpec{
		total:   func(row copilotmetrics.RepositoryRow) copilotmetrics.Counter { return row.Created },
		copilot: func(row copilotmetrics.RepositoryRow) copilotmetrics.Counter { return row.CreatedByCopilot },
	}))
}

func TestCopilotMetricsRepositoryChartLabels(t *testing.T) {
	name := `example/` + strings.Repeat("W", 30) + `<script>&`
	counter := copilotmetrics.Counter{Value: 1, Valid: true}
	tests := []struct {
		name   string
		chart  string
		y      float64
		height float64
	}{
		{
			name: "reviewed",
			chart: string(copilotMetricsSVGRepositoryStackedBar("Reviewed", []copilotMetricsRepositoryBar{{
				Name: name, Total: counter, Copilot: counter,
			}}, "Copilot", "Other", true)),
			y: 16.5, height: 25,
		},
		{
			name: "suggestions",
			chart: string(copilotMetricsSVGRepositorySuggestionBars([]copilotMetricsRepositorySuggestionBar{{
				Name: name, Generated: counter, Applied: counter,
			}})),
			y: 18.5, height: 34,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var document struct {
				SVG struct {
					Labels []struct {
						X      float64 `xml:"x,attr"`
						Y      float64 `xml:"y,attr"`
						Width  float64 `xml:"width,attr"`
						Height float64 `xml:"height,attr"`
						Hidden string  `xml:"aria-hidden,attr"`
						Wrap   struct {
							Class string `xml:"class,attr"`
							Label struct {
								Class string `xml:"class,attr"`
								Text  string `xml:",chardata"`
							} `xml:"div"`
						} `xml:"div"`
					} `xml:"foreignObject"`
				} `xml:"svg"`
			}
			require.NoError(t, xml.Unmarshal([]byte("<root>"+tt.chart+"</root>"), &document))
			require.Len(t, document.SVG.Labels, 1)
			label := document.SVG.Labels[0]
			assert.Equal(t, 0.0, label.X)
			assert.Equal(t, tt.y, label.Y)
			assert.Equal(t, copilotMetricsRepositoryChartLeft-8, label.Width)
			assert.Equal(t, tt.height, label.Height)
			assert.Equal(t, "true", label.Hidden)
			assert.Equal(t, "repository-label-wrap", label.Wrap.Class)
			assert.Equal(t, "repository-label", label.Wrap.Label.Class)
			assert.Equal(t, name, label.Wrap.Label.Text)
			assert.Contains(t, tt.chart, "&lt;script&gt;&amp;")
			assert.NotContains(t, tt.chart, "<script>")
		})
	}
}

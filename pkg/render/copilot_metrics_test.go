package render

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCopilotMetricsSummary() *copilotmetrics.Summary {
	valid := func(v int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: v, Valid: true} }
	sections := copilotmetrics.SectionSet{
		copilotmetrics.SectionKPI:      true,
		copilotmetrics.SectionTrend:    true,
		copilotmetrics.SectionLanguage: true,
		copilotmetrics.SectionTeam:     true,
		copilotmetrics.SectionUser:     true,
	}
	return &copilotmetrics.Summary{
		Source:      "octo (organization)",
		PeriodLabel: "1-day reports",
		StartDay:    "2030-01-01",
		EndDay:      "2030-01-02",
		Days:        2,
		GeneratedAt: time.Date(2030, 1, 3, 4, 5, 0, 0, time.UTC),
		Sections:    sections,
		KPIs: []copilotmetrics.KPI{
			{Label: "Daily active users (average)", Value: 12.5, Valid: true, Kind: copilotmetrics.KindDecimal, Window: "average over 2 day(s)"},
			{Label: "Monthly active users", Valid: false, Kind: copilotmetrics.KindCount, Window: "n/a"},
			{Label: "Code acceptance rate", Value: 0.25, Valid: true, Kind: copilotmetrics.KindPercent, Window: "acceptance / generation"},
			{Label: "User-initiated interactions", Value: 1234567, Valid: true, Kind: copilotmetrics.KindCount, Window: "sum over 2 day(s)"},
		},
		Trend: []copilotmetrics.TrendPoint{
			{Day: "2030-01-01", DailyActiveUsers: valid(10), UserInitiatedInteraction: valid(100)},
			{Day: "2030-01-02"},
		},
		Languages: []copilotmetrics.BreakdownRow{
			{Key: "c|sharp", ActivityTotals: copilotmetrics.ActivityTotals{CodeGeneration: valid(4), CodeAcceptance: valid(1)}},
			{Key: `</span><script>alert(1)</script>`, ActivityTotals: copilotmetrics.ActivityTotals{CodeGeneration: valid(2)}},
		},
		Users: []copilotmetrics.UserRow{
			{Login: "private-login", ActiveDays: 1},
		},
		Omitted:  map[copilotmetrics.Section]int{copilotmetrics.SectionLanguage: 3},
		Partial:  []copilotmetrics.PartialNote{{Family: copilotmetrics.FamilyUsers, Sections: []copilotmetrics.Section{copilotmetrics.SectionUser}, MissingDays: []string{"2030-01-02"}}},
		Warnings: []string{"1 duplicate users record(s) were ignored"},
	}
}

func TestRenderCopilotMetricsMarkdown(t *testing.T) {
	out := RenderCopilotMetricsMarkdown(testCopilotMetricsSummary())
	assert.True(t, strings.HasPrefix(out, "# Copilot usage metrics: octo (organization)\n"))
	assert.Contains(t, out, "2030-01-01 .. 2030-01-02 (2 day(s), 1-day reports)")
	assert.Contains(t, out, "2030-01-03 04:05 UTC")
	assert.Contains(t, out, "> [!WARNING]")
	assert.Contains(t, out, "users reports are missing for 1 day(s) (2030-01-02)")
	assert.Contains(t, out, "> [!NOTE]")
	assert.Contains(t, out, "| 12.5 |")
	assert.Contains(t, out, "| n/a |")
	assert.Contains(t, out, "| 25.0% |")
	assert.Contains(t, out, "1,234,567")
	assert.Contains(t, out, "c\\|sharp", "pipes in cells are escaped")
	assert.Contains(t, out, "```mermaid\nxychart-beta\n")
	assert.Contains(t, out, `x-axis ["01-01", "01-02"]`)
	assert.Contains(t, out, "line [10, 0]")
	assert.Contains(t, out, "Days without data are plotted as 0")
	assert.Contains(t, out, "## By team")
	assert.Contains(t, out, "&lt;script&gt;", "HTML in cells is escaped")
	assert.Contains(t, out, "No data.")
	assert.True(t, strings.HasSuffix(out, "\n"))
	assert.False(t, strings.HasSuffix(out, "\n\n"))
}

func TestRenderCopilotMetricsIssueBody(t *testing.T) {
	out := RenderCopilotMetricsIssueBody(testCopilotMetricsSummary(), []string{"report (Markdown)", "report (HTML)"})
	assert.Contains(t, out, "| Daily active users (average) | 12.5 |")
	assert.Contains(t, out, "| Monthly active users | n/a |")
	assert.Contains(t, out, "- report (HTML)")
	assert.Contains(t, out, "[!WARNING]")
}

func TestRenderCopilotMetricsHTML(t *testing.T) {
	out, err := RenderCopilotMetricsHTML(testCopilotMetricsSummary())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "<!DOCTYPE html>"))
	assert.Contains(t, out, "<title>Copilot usage metrics: octo (organization)</title>")
	assert.Equal(t, 1, strings.Count(out, "<script>"), "the only script is the embedded table interaction code")
	assert.NotContains(t, out, "<script>alert(1)</script>", "user content is escaped")
	assert.Contains(t, out, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.NotContains(t, out, "<link", "no external resources")
	assert.NotContains(t, out, "src=")
	assert.Contains(t, out, `<svg class="chart chart-line"`)
	assert.Contains(t, out, `.chart-line { --text-chart: 7px; }`)
	assert.Contains(t, out, `<figure class="chart-panel"><figcaption>Daily active users</figcaption><svg`)
	assert.Contains(t, out, `<figure class="chart-panel"><figcaption>User-initiated interactions</figcaption><svg`)
	assert.Contains(t, out, `role="group" aria-label="User-initiated interactions"`)
	assert.Contains(t, out, `class="chart-day-hit"`)
	assert.Contains(t, out, `.chart-day:hover .metric-dot, .chart-day:focus-within .metric-dot { visibility: visible; }`)
	assert.Contains(t, out, `<td class="num">25.0%</td>`)
	assert.Contains(t, out, `class="num heatmap"`)
	assert.Contains(t, out, `class="sort-button"`)
	assert.Contains(t, out, `aria-sort="none"`)
	assert.Contains(t, out, `tooltip.setAttribute("role", "tooltip")`)
	assert.Contains(t, out, `tooltip.setAttribute("aria-label", item.dataset.tooltip)`)
	assert.Contains(t, out, `const fields = new URLSearchParams(item.dataset.tooltipFields)`)
	assert.Contains(t, out, `item.addEventListener("pointerenter"`)
	assert.Contains(t, out, `item.addEventListener("focus"`)
	assert.Contains(t, out, `if (event.key === "Escape")`)
	assert.Contains(t, out, `addEventListener("click"`)
	assert.Contains(t, out, `getAttribute("aria-sort") === "descending" ? "ascending" : "descending"`)
	assert.Contains(t, out, "Partial data")
	assert.Contains(t, out, `<section id="team" aria-labelledby="section-heading-team">`)
	assert.Contains(t, out, "No data.")
	// A missing trend day is not drawn as a dot in either the line or the filled line chart.
	assert.Equal(t, 0, strings.Count(out, `<rect class="bar"`))
	assert.Equal(t, 2, strings.Count(out, `<circle class="dot"`))
	assert.Equal(t, 1, strings.Count(out, `<path class="line-area"`))
}

func TestRenderCopilotMetricsHTMLRich(t *testing.T) {
	s := testCopilotMetricsSummary()
	valid := func(value int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: value, Valid: true} }
	s.Sections[copilotmetrics.SectionModel] = true
	s.Sections[copilotmetrics.SectionIDE] = true
	s.Sections[copilotmetrics.SectionRepository] = true
	s.Repositories = []copilotmetrics.RepositoryRow{
		{Name: "octo/repo-a", Created: copilotmetrics.Counter{Value: 5, Valid: true}, CreatedByCopilot: copilotmetrics.Counter{Value: 2, Valid: true}, Reviewed: copilotmetrics.Counter{Value: 4, Valid: true}, ReviewedByCopilot: copilotmetrics.Counter{Value: 3, Valid: true}, Merged: copilotmetrics.Counter{Value: 2, Valid: true}, MergedCreatedByCopilot: copilotmetrics.Counter{Value: 1, Valid: true}},
		{Name: "octo/repo-b", Created: copilotmetrics.Counter{Value: 3, Valid: true}, CreatedByCopilot: copilotmetrics.Counter{Value: 0, Valid: true}, Reviewed: copilotmetrics.Counter{Value: 2, Valid: true}, ReviewedByCopilot: copilotmetrics.Counter{Value: 0, Valid: true}, Merged: copilotmetrics.Counter{Value: 1, Valid: true}, MergedCreatedByCopilot: copilotmetrics.Counter{Value: 0, Valid: true}},
	}
	s.Repositories[0].CopilotSuggestions = valid(5)
	s.Repositories[0].CopilotAppliedSuggestions = valid(7)
	s.Repositories = withCopilotMetricsRepositoryPairs(s.Repositories)
	s.AllRepositories = s.Repositories
	s.Rich = &copilotmetrics.RichMetrics{
		Days: []string{"2030-01-01", "2030-01-02"},
		Repositories: []copilotmetrics.RichRepository{
			{ID: 1, Name: "octo/repo-a", Reviews: []copilotmetrics.Counter{valid(2), valid(3)}, Suggestions: valid(5), SuggestionsComplete: true, AppliedSuggestions: valid(7), AppliedSuggestionsComplete: true},
			{ID: 2, Name: "octo/repo-b", Reviews: []copilotmetrics.Counter{valid(1), valid(0)}, Suggestions: valid(1), SuggestionsComplete: true, AppliedSuggestions: valid(0), AppliedSuggestionsComplete: true},
		},
		DailyActiveUsers:                 []copilotmetrics.Counter{valid(10), valid(12)},
		WeeklyActiveUsers:                []copilotmetrics.Counter{valid(20), valid(22)},
		MonthlyActiveUsers:               []copilotmetrics.Counter{valid(30), valid(40)},
		MonthlyActiveAgentUsers:          []copilotmetrics.Counter{valid(5), valid(10)},
		AverageChatRequestsPerActiveUser: []copilotmetrics.FloatCounter{{Value: 2, Valid: true}, {Value: 3, Valid: true}},
		ChatModeRequests: []copilotmetrics.RichSeries{
			{Name: "Agent", Values: []copilotmetrics.Counter{valid(4), valid(6)}},
			{Name: "Ask", Values: []copilotmetrics.Counter{valid(5), valid(7)}},
		},
		CodeGeneration:                []copilotmetrics.Counter{valid(20), valid(30)},
		CodeAcceptance:                []copilotmetrics.Counter{valid(10), valid(15)},
		AcceptanceRate:                []copilotmetrics.FloatCounter{{Value: .5, Valid: true}, {Value: .5, Valid: true}},
		CodeCompletionsSuggested:      []copilotmetrics.Counter{valid(80), valid(100)},
		CodeCompletionsAccepted:       []copilotmetrics.Counter{valid(20), valid(30)},
		CodeCompletionsAcceptanceRate: []copilotmetrics.FloatCounter{{Value: .25, Valid: true}, {Value: .3, Valid: true}},
		ModelRequestsByDay: []copilotmetrics.RichSeries{
			{Name: "model-a", Values: []copilotmetrics.Counter{valid(4), valid(6)}},
		},
		ModelRequests:          []copilotmetrics.RichValue{{Name: "model-a", Value: valid(10)}},
		ModelRequestsByFeature: []copilotmetrics.RichValue{{Name: "model-a", Group: "chat_panel_ask_mode", Value: valid(10)}},
		LanguageCodeGenerationByDay: []copilotmetrics.RichSeries{
			{Name: "go", Values: []copilotmetrics.Counter{valid(20), valid(30)}},
		},
		LanguageCodeGeneration: []copilotmetrics.RichValue{{Name: "go", Value: valid(50)}},
		ModelCodeGenerationByLanguage: []copilotmetrics.RichValue{
			{Name: "model-a", Group: "go", Value: valid(50)},
		},
		IDEActiveUsers: []copilotmetrics.IDEActiveUsers{{
			IDE: "Editor A", Daily: []copilotmetrics.Counter{valid(2), valid(3)}, Weekly: []copilotmetrics.Counter{{}, valid(4)},
		}},
		IDEActiveUserTotalDaily:  []copilotmetrics.Counter{valid(2), valid(3)},
		IDEActiveUserTotalWeekly: []copilotmetrics.Counter{{}, valid(4)},
		IDEActiveUserMonth: &copilotmetrics.IDEActiveUserEstimate{
			StartDay: "2030-01-01", EndDay: "2030-01-02", Users: valid(4),
		},
	}
	for _, name := range []string{"Editor B", "Editor C", "Editor D", "Editor E", "Editor F", "Editor G"} {
		s.Rich.IDEActiveUsers = append(s.Rich.IDEActiveUsers, copilotmetrics.IDEActiveUsers{
			IDE: name, Daily: []copilotmetrics.Counter{valid(1), valid(2)}, Weekly: []copilotmetrics.Counter{{}, valid(3)},
		})
	}
	out, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, out, `<body class="rich">`)
	assert.Contains(t, out, `class="kpi-grid"`)
	assert.Contains(t, out, `<p>IDE active users (estimate)</p><strong>4</strong>`)
	assert.Contains(t, out, `calendar month: 2030-01-01 .. 2030-01-02; reported IDEs only`)
	assert.Contains(t, out, `.kpi-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 190px), 1fr));`)
	assert.Contains(t, out, `.rich-panels { display: grid; gap: 12px; grid-template-columns: minmax(0, 1fr);`)
	assert.Contains(t, out, `class="rich-panels rich-panels-paired"`)
	_, trendHTML, found := strings.Cut(out, `<section id="trend"`)
	require.True(t, found)
	trendHTML, _, found = strings.Cut(trendHTML, `</section>`)
	require.True(t, found)
	assert.Contains(t, trendHTML, `<div class="rich-panels rich-panels-paired">`)
	assert.Contains(t, trendHTML, `<figure class="rich-panel"><figcaption>Daily and weekly active users</figcaption>`)
	assert.Equal(t, 12, strings.Count(out, `<figure class="rich-panel rich-panel-half">`))
	assert.Contains(t, out, `--text-body: 14px; --text-detail: 12px; --text-heading: 18px; --text-title: 24px; --text-kpi: 24px; --text-chart: 8px;`)
	assert.Contains(t, out, `.metric-chart .axis, .metric-chart .legend, .stacked-chart .axis, .stacked-chart .legend, .chart .axis { fill: var(--muted); font-size: var(--text-chart); }`)
	assert.Contains(t, out, `.rich-panel-half .stacked-chart { --text-chart: 16px; }`)
	assert.Contains(t, out, `.rich-panel-half .stacked-chart { --text-chart: 18px; }`)
	assert.Contains(t, out, `.rich-panel-half .metric-chart { --text-chart: 13px; }`)
	assert.Contains(t, out, `.rich-panel-half .metric-chart { --text-chart: 7px; }`)
	assert.Contains(t, out, `.metric-chart, .rich-panel-half .metric-chart { --text-chart: 13px; }`)
	assert.Contains(t, out, `viewBox="0 0 720 205"`)
	assert.Contains(t, out, `.metric-chart { display: block; height: auto; width: 100%; --text-chart: 7px; }`)
	assert.Contains(t, out, `.repository-label-wrap { align-items: center; color: var(--muted); display: flex; font-size: var(--text-chart);`)
	assert.Contains(t, out, `.repository-label { min-width: 0; overflow: hidden; text-align: right; text-overflow: ellipsis; white-space: nowrap; width: 100%; }`)
	assert.Contains(t, out, `.rich .chart { max-width: none; }`)
	assert.NotContains(t, out, `width: 49%`)
	assert.NotContains(t, out, `<figure class="chart-panel"><figcaption>Daily active users</figcaption><svg`)
	assert.Contains(t, out, `<figcaption>Daily and weekly active users</figcaption>`)
	assert.Contains(t, out, `<figure class="chart-panel"><figcaption>User-initiated interactions</figcaption><svg`)
	assert.Contains(t, trendHTML, `role="group" aria-label="User-initiated interactions"`)
	assert.Contains(t, trendHTML, `class="chart-day-hit"`)
	assert.NotContains(t, out, `<figcaption></figcaption>`)
	assert.Contains(t, out, `<strong>25.0%</strong>`, "agent adoption is calculated from monthly active-user snapshots")
	assert.NotContains(t, out, `Requests by chat panel mode`)
	assert.Contains(t, trendHTML, `<figure class="rich-panel"><figcaption>Requests per chat mode</figcaption><p class="chart-color-key">* marks incomplete days; bars show known counts only.</p><svg class="metric-chart metric-daily-stacked-bar"`)
	averageIndex := strings.Index(trendHTML, `<figcaption>Average chat requests per active user</figcaption>`)
	chatModeIndex := strings.Index(trendHTML, `<figcaption>Requests per chat mode</figcaption>`)
	assert.True(t, averageIndex >= 0 && chatModeIndex > averageIndex, "requests per chat mode follows average chat requests")
	assert.Contains(t, trendHTML, `<figure class="rich-panel rich-panel-half"><figcaption>Code generation and acceptance activities</figcaption>`)
	assert.Contains(t, trendHTML, `<figure class="rich-panel rich-panel-half"><figcaption>Code acceptance rate</figcaption>`)
	assert.Contains(t, trendHTML, `<figure class="rich-panel rich-panel-half"><figcaption>Code completions</figcaption>`)
	assert.Contains(t, trendHTML, `<figure class="rich-panel rich-panel-half"><figcaption>Code completions acceptance rate</figcaption>`)
	assert.Contains(t, out, `data-tooltip="2030-01-01`+"\nSuggested: 80.0\nAccepted: 20.0")
	assert.Contains(t, out, `data-tooltip="2030-01-01`+"\nAcceptance rate (%): 25.0")
	assert.Contains(t, out, `IDE weekly active users`)
	assert.Contains(t, out, `Editor G`, "IDE charts include every series rather than summing omitted IDEs")
	assert.NotContains(t, out, `>Other</text>`)
	assert.NotContains(t, out, `<figcaption>Model usage by feature (interactions)</figcaption>`)
	assert.Contains(t, out, `<figcaption>Chat model usage (interactions)</figcaption>`)
	assert.Contains(t, out, `<svg class="pie-plot" viewBox="0 0 320 320" role="img" aria-label="Chat model usage (interactions)"`)
	assert.Contains(t, out, `data-tooltip="model-a: 10 (100.0%)"`)
	assert.Contains(t, out, `<figcaption>Model usage per chat mode</figcaption>`)
	assert.Contains(t, out, `Chat mode: Ask`)
	assert.Contains(t, out, `<figcaption>Model usage per language (code-generation activities)</figcaption>`)
	assert.Contains(t, out, `.rich-panel figcaption, .chart-panel figcaption { font-size: var(--text-body);`)
	assert.Contains(t, out, `<svg class="pie-plot" viewBox="0 0 320 320" role="img" aria-label="Language usage (code-generation activities)"`)
	assert.Contains(t, out, `class="stacked-chart"`)
	assert.Contains(t, out, `<text class="axis" x="360.0"`)
	assert.Contains(t, out, `data-tooltip="go: 50 (100.0%)"`)
	assert.Contains(t, out, `<svg class="metric-chart"`)
	assert.NotContains(t, out, `class="bar-fill"`)
	assert.Contains(t, out, `<section id="repository" aria-labelledby="section-heading-repository">`)
	assert.Contains(t, out, `<figcaption>Pull requests reviewed by repository</figcaption>`)
	assert.Contains(t, out, `<figcaption>Pull requests created by repository</figcaption>`)
	assert.Contains(t, out, `<figcaption>Pull requests merged by repository</figcaption>`)
	assert.Contains(t, out, `Reviewed with Copilot: 3 (75.0%)`)
	assert.Contains(t, out, `Reviewed without Copilot: 1 (25.0%)`)
	assert.Contains(t, out, `<figcaption>Copilot review suggestions by repository</figcaption>`)
	assert.Contains(t, out, `<figcaption>Copilot reviews per day by repository</figcaption>`)
	assert.Contains(t, out, "Generated suggestions: 5\nApplied suggestions: 7")
	assert.Contains(t, out, "2030-01-01\nocto/repo-a: 2 (66.7%)\nocto/repo-b: 1 (33.3%)\nTotal: 3 (100.0%)")
	assert.Contains(t, out, `private-login`, "rich HTML honors an explicitly selected user section")
	assert.Contains(t, out, `<section id="user" aria-labelledby="section-heading-user">`)
	assert.NotContains(t, out, "<link", "rich mode remains self-contained")

	legacy, err := RenderCopilotMetricsHTML(s)
	require.NoError(t, err)
	assert.Equal(t, legacy, mustRenderCopilotMetricsHTML(t, s, CopilotMetricsReportOptions{}))
	assert.NotContains(t, legacy, `<figcaption>Copilot review suggestions by repository</figcaption>`)
	assert.NotContains(t, legacy, `<figcaption>Copilot reviews per day by repository</figcaption>`)
}

func TestCopilotMetricsTrendPanelPairing(t *testing.T) {
	pairs := []struct {
		name       string
		countTitle string
		rateTitle  string
		setCounts  func(*copilotmetrics.RichMetrics)
		setRate    func(*copilotmetrics.RichMetrics)
	}{
		{
			name: "code activity", countTitle: "Code generation and acceptance activities", rateTitle: "Code acceptance rate",
			setCounts: func(rich *copilotmetrics.RichMetrics) {
				rich.CodeGeneration = []copilotmetrics.Counter{{Value: 10, Valid: true}}
				rich.CodeAcceptance = []copilotmetrics.Counter{{Value: 4, Valid: true}}
			},
			setRate: func(rich *copilotmetrics.RichMetrics) {
				rich.AcceptanceRate = []copilotmetrics.FloatCounter{{Value: .4, Valid: true}}
			},
		},
		{
			name: "code completions", countTitle: "Code completions", rateTitle: "Code completions acceptance rate",
			setCounts: func(rich *copilotmetrics.RichMetrics) {
				rich.CodeCompletionsSuggested = []copilotmetrics.Counter{{Value: 10, Valid: true}}
				rich.CodeCompletionsAccepted = []copilotmetrics.Counter{{Value: 4, Valid: true}}
			},
			setRate: func(rich *copilotmetrics.RichMetrics) {
				rich.CodeCompletionsAcceptanceRate = []copilotmetrics.FloatCounter{{Value: .4, Valid: true}}
			},
		},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			for _, tt := range []struct {
				name   string
				counts bool
				rate   bool
			}{
				{name: "both", counts: true, rate: true},
				{name: "counts only", counts: true},
				{name: "rate only", rate: true},
				{name: "neither"},
			} {
				t.Run(tt.name, func(t *testing.T) {
					rich := &copilotmetrics.RichMetrics{
						Days:             []string{"2030-01-01"},
						DailyActiveUsers: []copilotmetrics.Counter{{Value: 5, Valid: true}},
					}
					var wantTitles []string
					if tt.counts {
						pair.setCounts(rich)
						wantTitles = append(wantTitles, pair.countTitle)
					}
					if tt.rate {
						pair.setRate(rich)
						wantTitles = append(wantTitles, pair.rateTitle)
					}
					panels := copilotMetricsRichPanels(rich, copilotmetrics.SectionTrend, nil)
					require.Len(t, panels, 1+len(wantTitles))
					assert.Equal(t, "Daily and weekly active users", panels[0].Title)
					assert.False(t, panels[0].HalfWidth)
					for i, title := range wantTitles {
						assert.Equal(t, title, panels[i+1].Title)
						assert.Equal(t, tt.counts && tt.rate, panels[i+1].HalfWidth)
					}
				})
			}
		})
	}
}

func TestCopilotMetricsTrendUnpairedPanelsStayFullWidth(t *testing.T) {
	rich := &copilotmetrics.RichMetrics{
		Days:                          []string{"2030-01-01"},
		CodeGeneration:                []copilotmetrics.Counter{{Value: 10, Valid: true}},
		CodeCompletionsAcceptanceRate: []copilotmetrics.FloatCounter{{Value: .4, Valid: true}},
	}
	panels := copilotMetricsRichPanels(rich, copilotmetrics.SectionTrend, nil)
	require.Len(t, panels, 2)
	assert.Equal(t, "Code generation and acceptance activities", panels[0].Title)
	assert.Equal(t, "Code completions acceptance rate", panels[1].Title)
	assert.False(t, panels[0].HalfWidth)
	assert.False(t, panels[1].HalfWidth)
}

func TestCopilotMetricsSVGMultiLineChartZeroAndMissingValues(t *testing.T) {
	zeroChart := copilotMetricsSVGMultiLineChart("Zero activity", []string{"2030-01-01", "2030-01-02"}, []copilotMetricsPlotSeries{{
		Name: "Zero", Values: []float64{0, 0}, Valid: []bool{true, true},
	}})
	assert.Contains(t, string(zeroChart), `<path class="metric-line"`)
	assert.Equal(t, 2, strings.Count(string(zeroChart), `<circle class="metric-dot"`))

	missingChart := copilotMetricsSVGMultiLineChart("Unavailable", []string{"2030-01-01"}, []copilotMetricsPlotSeries{{
		Name: "Unavailable", Values: []float64{0}, Valid: []bool{false},
	}})
	assert.Empty(t, missingChart, "unavailable data must not be presented as zero")
}

func TestCopilotMetricsDailyLanguagePercentages(t *testing.T) {
	source := []copilotMetricsPlotSeries{
		{Name: "go", Values: []float64{30, 10, 0, 4}, Valid: []bool{true, true, true, false}},
		{Name: "rust", Values: []float64{10, 30, 0, 0}, Valid: []bool{true, true, true, false}},
	}
	series := copilotMetricsDailyPercentageSeries(source, 4)
	assert.Equal(t, []float64{75, 25, 0, 0}, series[0].Values)
	assert.Equal(t, []float64{25, 75, 0, 0}, series[1].Values)
	assert.Equal(t, []bool{true, true, false, false}, series[0].Valid, "zero-total and missing days have no defined percentage")
	assert.Equal(t, float64(30), source[0].Values[0], "the original counters are unchanged")
	chart := string(copilotMetricsSVGMultiLineChartWithPercentage("Languages", []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04"}, series, true))
	assert.Contains(t, chart, `>100.0%</text>`)
	assert.Contains(t, chart, "go: 75.0%\nrust: 25.0%")
	assert.Contains(t, chart, "go: 25.0%\nrust: 75.0%")
	assert.Equal(t, 4, strings.Count(chart, `<circle class="metric-dot"`))
}

func TestCopilotMetricsDailyUsagePanelsUsePercentages(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	series := []copilotmetrics.RichSeries{
		{Name: "item-a", Values: []copilotmetrics.Counter{valid(30), valid(10), valid(0), valid(20), valid(0)}},
		{Name: "item-b", Values: []copilotmetrics.Counter{valid(10), valid(30), valid(0), {}, valid(20)}},
	}
	rich := &copilotmetrics.RichMetrics{
		Days:                        []string{"2030-01-01", "2030-01-02", "2030-01-03", "2030-01-04", "2030-01-05"},
		ModelRequestsByDay:          series,
		LanguageCodeGenerationByDay: series,
	}
	summary := &copilotmetrics.Summary{Rich: rich}
	for _, tc := range []struct {
		section copilotmetrics.Section
		title   string
	}{
		{copilotmetrics.SectionModel, "Model usage per day (interactions)"},
		{copilotmetrics.SectionLanguage, "Language usage per day (code-generation activities)"},
	} {
		t.Run(string(tc.section), func(t *testing.T) {
			scheme := copilotMetricsColorsForSection(summary, tc.section)
			panels := copilotMetricsRichPanels(rich, tc.section, scheme)
			require.NotEmpty(t, panels)
			assert.Equal(t, tc.title, panels[0].Title)
			chart := string(panels[0].Chart)
			assert.Contains(t, chart, `class="metric-chart metric-stacked-area"`)
			assert.Contains(t, chart, `role="group"`)
			assert.Equal(t, 4, strings.Count(chart, `<path class="metric-area"`))
			assert.NotContains(t, chart, `<path class="metric-line"`)
			for _, label := range []string{"0.0%", "50.0%", "100.0%"} {
				assert.Contains(t, chart, `>`+label+`</text>`)
			}
			assert.Contains(t, chart, "2030-01-01\nitem-a: 75.0%\nitem-b: 25.0%")
			assert.Contains(t, chart, "2030-01-02\nitem-a: 25.0%\nitem-b: 75.0%")
			assert.NotContains(t, chart, `data-tooltip="2030-01-03`, "zero-total days have no plotted percentage")
			assert.Contains(t, chart, "2030-01-04\nitem-a: 100.0%\nitem-b: n/a")
			assert.Contains(t, chart, "2030-01-05\nitem-a: 0.0%\nitem-b: 100.0%")
			assert.Equal(t, 4, strings.Count(chart, `<rect class="metric-day-hit"`))
		})
	}
	assert.Equal(t, int64(30), rich.ModelRequestsByDay[0].Values[0].Value, "the source counts stay unchanged")
}

func TestCopilotMetricsDailyModelPercentagesIncludeOther(t *testing.T) {
	rich := &copilotmetrics.RichMetrics{Days: []string{"2030-01-01"}}
	for i := 1; i <= 8; i++ {
		rich.ModelRequestsByDay = append(rich.ModelRequestsByDay, copilotmetrics.RichSeries{
			Name: fmt.Sprintf("model-%02d", i), Values: []copilotmetrics.Counter{{Value: 1, Valid: true}},
		})
	}
	scheme := copilotMetricsColorsForSection(&copilotmetrics.Summary{Rich: rich}, copilotmetrics.SectionModel)
	panels := copilotMetricsRichPanels(rich, copilotmetrics.SectionModel, scheme)
	require.NotEmpty(t, panels)
	chart := string(panels[0].Chart)
	assert.Contains(t, chart, "model-01: 12.5%")
	assert.Contains(t, chart, "Other: 25.0%")
	assert.Equal(t, 7, strings.Count(chart, `<text class="legend"`))
}

func TestCopilotMetricsDailyTooltipIncludesAllSeries(t *testing.T) {
	chart := string(copilotMetricsSVGMultiLineChart("Daily usage", []string{"2030-01-01", "2030-01-02"}, []copilotMetricsPlotSeries{
		{Name: "First", Values: []float64{10, 20}, Valid: []bool{true, true}},
		{Name: "Second", Values: []float64{0, 30}, Valid: []bool{true, true}},
		{Name: `Missing"<script>`, Values: []float64{99}, Valid: []bool{false}},
	}))
	firstDay := "2030-01-01\nFirst: 10.0\nSecond: 0.0\nMissing&#34;&lt;script&gt;: n/a"
	secondDay := "2030-01-02\nFirst: 20.0\nSecond: 30.0\nMissing&#34;&lt;script&gt;: n/a"
	assert.Equal(t, 1, strings.Count(chart, `data-tooltip="`+firstDay+`"`), "each day has one complete tooltip")
	assert.Equal(t, 1, strings.Count(chart, `data-tooltip="`+secondDay+`"`))
	assert.NotContains(t, chart, "<script>")
}

func TestCopilotMetricsRichStackedBarPanel(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	panel := copilotMetricsHTMLRichStackedBarPanel("Model languages", []copilotmetrics.RichValue{
		{Name: "model-a", Group: "go", Value: valid(20)},
		{Name: "model-a", Group: "go", Value: valid(10)},
		{Name: "model-a", Group: "rust", Value: valid(10)},
		{Name: `model-b"<script>`, Group: "go", Value: valid(20)},
		{Name: "missing", Group: "go"},
	})
	chart := string(panel.Chart)
	assert.Equal(t, 3, strings.Count(chart, `<rect class="stacked-segment"`))
	assert.Contains(t, chart, "Language: go\nmodel-a: 30 (50.0%)\nmodel-b&#34;&lt;script&gt;: 20 (33.3%)\nLanguage total: 50 (83.3%)")
	assert.Contains(t, chart, "Language: rust\nmodel-a: 10 (16.7%)\nmodel-b&#34;&lt;script&gt;: n/a\nLanguage total: 10 (16.7%)")
	assert.Contains(t, chart, `height="107.1"`, "50% is scaled against the 84% axis maximum")
	assert.Contains(t, chart, `height="71.4"`)
	assert.Contains(t, chart, `height="35.7"`)
	assert.Contains(t, chart, `y="29.4"`, "the Go bar reaches 83.3% on the 84% axis")
	assert.Contains(t, chart, `model-b&#34;&lt;script&gt;`)
	assert.NotContains(t, chart, `<script>`)
	assert.NotContains(t, chart, `>missing</text>`)
	assert.Contains(t, chart, `>Language</text>`)
	assert.Contains(t, chart, `data-tooltip="Language: go`+"\n")
	assert.Contains(t, chart, `data-tooltip="Language: rust`+"\n")
	assert.Contains(t, chart, `>go</div>`)
	assert.Contains(t, chart, `>rust</div>`)
	assert.Contains(t, chart, `>84%</text>`)
	assert.Contains(t, chart, `>42%</text>`)
	assert.Contains(t, chart, `>%</text>`)
	assert.Empty(t, copilotMetricsHTMLRichStackedBarPanel("Empty", nil).Chart)
}

func TestCopilotMetricsStackedBarAxisMaximum(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []copilotmetrics.RichValue
		top    string
		middle string
		height string
	}{
		{"single", []copilotmetrics.RichValue{{Name: "a", Group: "go", Value: copilotmetrics.Counter{Value: 10, Valid: true}}}, "100", "50", "180.0"},
		{"balanced", []copilotmetrics.RichValue{
			{Name: "a", Group: "go", Value: copilotmetrics.Counter{Value: 1, Valid: true}},
			{Name: "a", Group: "rust", Value: copilotmetrics.Counter{Value: 1, Valid: true}},
		}, "50", "25", "180.0"},
		{"rounded", []copilotmetrics.RichValue{
			{Name: "a", Group: "go", Value: copilotmetrics.Counter{Value: 1, Valid: true}},
			{Name: "a", Group: "rust", Value: copilotmetrics.Counter{Value: 2, Valid: true}},
		}, "67", "33.5", "179.1"},
		{"zero", []copilotmetrics.RichValue{{Name: "a", Group: "go", Value: copilotmetrics.Counter{Valid: true}}}, "1", "0.5", "0.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chart := string(copilotMetricsHTMLRichStackedBarPanel("Languages", tc.values).Chart)
			assert.Contains(t, chart, ">"+tc.top+"%</text>")
			assert.Contains(t, chart, ">"+tc.middle+"%</text>")
			assert.Contains(t, chart, `height="`+tc.height+`"`)
			assert.NotContains(t, chart, "NaN")
			assert.NotContains(t, chart, "Inf")
		})
	}
}

func TestCopilotMetricsStackedOtherAliases(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	chart := string(copilotMetricsHTMLRichStackedBarPanel("Languages", []copilotmetrics.RichValue{
		{Name: "model-a", Group: "go", Value: valid(10)},
		{Name: "model-a", Group: "Other", Value: valid(20)},
		{Name: "model-a", Group: " UNKNOWN ", Value: valid(10)},
		{Name: "model-a", Group: "unkown", Value: valid(10)},
		{Name: "model-a", Group: "Others", Value: valid(30)},
		{Name: "Others", Group: "go", Value: valid(5)},
		{Name: "unknown", Group: "go", Value: valid(15)},
	}).Chart)
	assert.Equal(t, 3, strings.Count(chart, `<rect class="stacked-segment"`))
	assert.NotContains(t, chart, "Others")
	assert.NotContains(t, chart, "unknown")
	assert.NotContains(t, chart, "unkown")
	assert.Contains(t, chart, "Language: Other\nmodel-a: 70 (70.0%)\nOther: n/a\nLanguage total: 70 (70.0%)")
	assert.Contains(t, chart, "Language: go\nmodel-a: 10 (10.0%)\nOther: 20 (20.0%)")
	goAxis := strings.Index(chart, `>go</div>`)
	otherAxis := strings.Index(chart, `>Other</div>`)
	require.NotEqual(t, -1, goAxis)
	require.NotEqual(t, -1, otherAxis)
	assert.Greater(t, otherAxis, goAxis, "Other stays rightmost even when it has the largest total")
}

func TestCopilotMetricsDailyOtherAliases(t *testing.T) {
	for _, limit := range []int{1, 5} {
		series := copilotMetricsLimitPlotSeries([]copilotMetricsPlotSeries{
			{Name: "go", Values: []float64{10, 0}, Valid: []bool{true, true}},
			{Name: "Other", Values: []float64{20, 0}, Valid: []bool{true, false}},
			{Name: "Others", Values: []float64{30, 0}, Valid: []bool{true, false}},
			{Name: " UNKNOWN ", Values: []float64{40, 5}, Valid: []bool{true, true}},
			{Name: "unkown", Values: []float64{50, 100}, Valid: []bool{true, false}},
			{Name: "rust", Values: []float64{5, 0}, Valid: []bool{true, true}},
		}, limit)
		other := series[len(series)-1]
		assert.Equal(t, "Other", other.Name)
		expected := float64(140)
		if limit == 1 {
			expected += 15
		}
		assert.Equal(t, []float64{expected, 5}, other.Values)
		assert.Equal(t, []bool{true, true}, other.Valid)
		assert.Len(t, series, min(limit, 3))
	}
}

func TestCopilotMetricsTopLanguages(t *testing.T) {
	values := []copilotmetrics.RichValue{
		{Name: "model-a", Group: "unknown", Value: copilotmetrics.Counter{Value: 50, Valid: true}},
		{Name: "model-b", Group: "Other", Value: copilotmetrics.Counter{Value: 10, Valid: true}},
		{Name: "missing", Group: "missing-language", Value: copilotmetrics.Counter{Value: 999}},
	}
	for i := 1; i <= 8; i++ {
		values = append(values, copilotmetrics.RichValue{Name: "model-a", Group: fmt.Sprintf("lang-%d", i), Value: copilotmetrics.Counter{Value: int64(i), Valid: true}})
	}
	grouped := copilotMetricsTopLanguages(values)
	assert.Equal(t, "lang-1", values[3].Group, "source dimensions stay unchanged")
	chart := string(copilotMetricsHTMLRichStackedBarPanel("Languages", grouped).Chart)
	for i := 4; i <= 8; i++ {
		assert.Contains(t, chart, fmt.Sprintf(`data-tooltip="Language: lang-%d`+"\n", i))
		assert.Contains(t, chart, fmt.Sprintf(`>lang-%d</div>`, i))
	}
	for i := 1; i <= 3; i++ {
		assert.NotContains(t, chart, fmt.Sprintf("lang-%d", i))
	}
	assert.Contains(t, chart, "Model: model-a\nLanguage: Other")
	assert.Contains(t, chart, "Language total: 66 (68.8%)")
	assert.NotContains(t, chart, "missing-language")
	assert.Empty(t, copilotMetricsTopLanguages(nil))
	assert.Equal(t, values[3:5], copilotMetricsTopLanguages(values[3:5]))
}

func TestCopilotMetricsModelUsagePerChatMode(t *testing.T) {
	valid := func(n int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: n, Valid: true}
	}
	values := []copilotmetrics.RichValue{
		{Name: "model-a", Group: "chat_panel_agent_mode", Value: valid(30)},
		{Name: "model-b", Group: "chat_panel_agent_mode", Value: valid(10)},
		{Name: "model-a", Group: "chat_panel_ask_mode", Value: valid(20)},
		{Name: "model-a", Group: "chat_panel_edit_mode", Value: valid(10)},
		{Name: "model-a", Group: "chat_panel_plan_mode", Value: valid(10)},
		{Name: "model-a", Group: "chat_panel_custom_mode", Value: valid(10)},
		{Name: "unknown", Group: "chat_panel_unknown_mode", Value: valid(10)},
		{Name: "inline-only", Group: "chat_inline", Value: valid(900)},
		{Name: "edits-only", Group: "agent_edit", Value: valid(900)},
		{Name: "future", Group: "new_feature", Value: valid(900)},
		{Name: "missing", Group: "chat_panel_ask_mode"},
	}
	chart := string(copilotMetricsHTMLRichStackedPanel("Model usage per chat mode", copilotMetricsChatModeModelValues(values), "Model").Chart)
	assert.Contains(t, chart, "Model: model-a\nAgent: 30 (30.0%)\nAsk: 20 (20.0%)")
	assert.Contains(t, chart, "Model total: 80 (80.0%)")
	assert.Contains(t, chart, "Percentages are shares of all chat mode interactions.")
	assert.Contains(t, chart, "Chat mode: Plan")
	assert.Contains(t, chart, "Chat mode: Edit")
	assert.Contains(t, chart, "Chat mode: Custom")
	assert.Contains(t, chart, "Chat mode: Other\nModel: Other")
	assert.Contains(t, chart, ">Model</text>")
	assert.Contains(t, chart, `data-tooltip="Model: model-a`+"\n")
	assert.Contains(t, chart, `>model-a</div>`)
	assert.Contains(t, chart, `<ul class="stacked-legend"`)
	assert.Contains(t, chart, `>Agent</span>`)
	assert.Contains(t, chart, ">80%</text>")
	assert.NotContains(t, chart, "inline-only")
	assert.NotContains(t, chart, "edits-only")
	assert.NotContains(t, chart, "future")
	assert.NotContains(t, chart, "missing")
	assert.NotContains(t, chart, "Language")
	assert.Empty(t, copilotMetricsHTMLRichStackedPanel("Model usage per chat mode", copilotMetricsChatModeModelValues(values[7:10]), "Model").Chart)
}

func TestCopilotMetricsTopChatModeModels(t *testing.T) {
	values := []copilotmetrics.RichValue{
		{Name: "unknown", Group: "chat_panel_ask_mode", Value: copilotmetrics.Counter{Value: 10, Valid: true}},
		{Name: "inline-only", Group: "chat_inline", Value: copilotmetrics.Counter{Value: 999, Valid: true}},
	}
	for i := 1; i <= 8; i++ {
		values = append(values, copilotmetrics.RichValue{
			Name: fmt.Sprintf("model-%d", i), Group: "chat_panel_agent_mode",
			Value: copilotmetrics.Counter{Value: int64(i), Valid: true},
		})
	}
	grouped := copilotMetricsTopGroups(copilotMetricsChatModeModelValues(values))
	chart := string(copilotMetricsHTMLRichStackedPanel("Model usage per chat mode", grouped, "Model").Chart)
	for i := 4; i <= 8; i++ {
		assert.Contains(t, chart, fmt.Sprintf(`data-tooltip="Model: model-%d`+"\n", i))
		assert.Contains(t, chart, fmt.Sprintf(`>model-%d</div>`, i))
	}
	for i := 1; i <= 3; i++ {
		assert.NotContains(t, chart, fmt.Sprintf("model-%d", i))
	}
	assert.Contains(t, chart, "Chat mode: Agent\nModel: Other")
	assert.Contains(t, chart, "Chat mode: Ask\nModel: Other")
	assert.Contains(t, chart, "Model total: 16 (34.8%)")
	assert.NotContains(t, chart, "inline-only")
}

func TestCopilotMetricsChartCategoryLimit(t *testing.T) {
	for _, size := range []int{7, 8, 12} {
		values := make([]copilotmetrics.RichValue, 0, size)
		series := make([]copilotMetricsPlotSeries, 0, size)
		for i := 1; i <= size; i++ {
			name := fmt.Sprintf("model-%02d", i)
			values = append(values, copilotmetrics.RichValue{Name: name, Value: copilotmetrics.Counter{Value: int64(i), Valid: true}})
			series = append(series, copilotMetricsPlotSeries{Name: name, Values: []float64{float64(i), 999}, Valid: []bool{true, false}})
		}
		limited := copilotMetricsLimitPlotSeries(series, len(copilotMetricsChartColors))
		require.Len(t, limited, 7)
		assert.Equal(t, fmt.Sprintf("model-%02d", size), limited[0].Name)
		var total float64
		for _, item := range limited {
			total += item.Values[0]
			assert.False(t, item.Valid[1], "missing observations stay missing")
		}
		assert.Equal(t, float64(size*(size+1)/2), total)
		if size > 7 {
			assert.Equal(t, "Other", limited[6].Name)
			assert.Equal(t, float64((size-6)*(size-5)/2), limited[6].Values[0])
		}
		var valueTotal int64
		for _, row := range copilotMetricsLimitRichValues(values) {
			valueTotal += row.Value.Value
		}
		assert.Equal(t, int64(size*(size+1)/2), valueTotal)
		pie := string(copilotMetricsHTMLRichPiePanel("Models", values).Chart)
		assert.Equal(t, 7, strings.Count(pie, `<path class="pie-slice"`))
		bars := string(copilotMetricsHTMLRichBarPanel("Models", values).Chart)
		assert.Equal(t, 7, strings.Count(bars, `<div class="bar-row"`))
		for i := range values {
			values[i].Group = "go"
		}
		stacked := string(copilotMetricsHTMLRichStackedBarPanel("Models", values).Chart)
		assert.Equal(t, 7, strings.Count(stacked, `<rect class="stacked-segment"`))
		assert.Equal(t, 7, strings.Count(stacked, `<span class="stacked-legend-label">`))
		if size > 7 {
			assert.NotContains(t, stacked, "model-01")
			assert.Contains(t, stacked, "Model: Other")
		}
		assert.Contains(t, stacked, fmt.Sprintf("Language total: %d (100.0%%)", size*(size+1)/2))
	}
}

func TestCopilotMetricsPieThresholdAndCategoryLimit(t *testing.T) {
	values := []copilotmetrics.RichValue{
		{Name: "Unknown", Value: copilotmetrics.Counter{Value: 2, Valid: true}},
		{Name: "tiny", Value: copilotmetrics.Counter{Value: 1, Valid: true}},
	}
	for i := 0; i < 8; i++ {
		values = append(values, copilotmetrics.RichValue{Name: fmt.Sprintf("model-%d", i), Value: copilotmetrics.Counter{Value: 100, Valid: true}})
	}
	chart := string(copilotMetricsHTMLRichPiePanel("Models", values).Chart)
	assert.Equal(t, 7, strings.Count(chart, `<path class="pie-slice"`))
	assert.Equal(t, 1, strings.Count(chart, `<span>Other</span>`))
	assert.Contains(t, chart, `Other: 203 (25.3%)`)
	assert.NotContains(t, chart, "tiny")
	assert.NotContains(t, chart, "Unknown")
}

func TestCopilotMetricsRichPiePanel(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}

	panel := copilotMetricsHTMLRichPiePanel("Languages", []copilotmetrics.RichValue{
		{Name: `a"<script>`, Value: valid(3)},
		{Name: "b", Value: valid(1)},
		{Name: "zero", Value: valid(0)},
		{Name: "missing", Value: copilotmetrics.Counter{Value: 100}},
	})
	chart := string(panel.Chart)
	assert.Equal(t, "Languages", panel.Title)
	assert.Equal(t, 2, strings.Count(chart, `<path class="pie-slice"`))
	assert.Contains(t, chart, `3 (75.0%)`)
	assert.Contains(t, chart, `1 (25.0%)`)
	assert.Contains(t, chart, `A145 145 0 1 1`, "slices over half the circle use the large arc")
	assert.Contains(t, chart, `a&#34;&lt;script&gt;`)
	assert.NotContains(t, chart, `<script>`)
	assert.NotContains(t, chart, `>zero</span>`)
	assert.NotContains(t, chart, `>missing</span>`)
	assert.Contains(t, chart, `tabindex="0" aria-label=`)

	single := copilotMetricsHTMLRichPiePanel("Languages", []copilotmetrics.RichValue{{Name: "a", Value: valid(5)}})
	assert.Contains(t, string(single.Chart), `<circle class="pie-slice"`)
	assert.Contains(t, string(single.Chart), `5 (100.0%)`)
	for _, values := range [][]copilotmetrics.RichValue{
		nil, {{Name: "zero", Value: valid(0)}}, {{Name: "missing"}},
	} {
		assert.Empty(t, copilotMetricsHTMLRichPiePanel("Languages", values).Chart)
	}
}

func TestCopilotMetricsPieUsesDescendingThemeGradient(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	values := []copilotmetrics.RichValue{
		{Name: "small", Value: valid(1)},
		{Name: "largest", Value: valid(9)},
		{Name: "middle", Value: valid(4)},
		{Name: "Other", Value: valid(2)},
	}
	scheme := copilotMetricsNewColorScheme(copilotMetricsThemeOrange, map[string]copilotmetrics.Counter{
		"small": valid(1), "largest": valid(9), "middle": valid(4),
	}, nil)
	chart := string(copilotMetricsHTMLRichPiePanelWithColors("Models", values, scheme).Chart)

	for rank, name := range []string{"largest", "middle", "small"} {
		color := copilotMetricsThemeGradientColor(copilotMetricsThemeOrange, rank, 3).hex()
		assert.Contains(t, chart, `style="background:`+color+`"></span><span>`+name+`</span>`)
	}
	assert.Contains(t, chart, `style="background:#6e7781"></span><span>Other</span>`)
}

func TestCopilotMetricsLanguagePieGroupsBelowOnePercent(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter {
		return copilotmetrics.Counter{Value: value, Valid: true}
	}
	chart := string(copilotMetricsHTMLRichPiePanel("Languages", []copilotmetrics.RichValue{
		{Name: "major", Value: valid(970)},
		{Name: "boundary", Value: valid(10)},
		{Name: "small-a", Value: valid(4)},
		{Name: "small-b", Value: valid(6)},
		{Name: "Other", Value: valid(5)},
		{Name: "Others", Value: valid(2)},
		{Name: "Unknown", Value: valid(2)},
		{Name: "unkown", Value: valid(1)},
	}).Chart)
	assert.Equal(t, 3, strings.Count(chart, `<path class="pie-slice"`))
	assert.Contains(t, chart, `<span>major</span><span>97.0%</span>`)
	assert.Contains(t, chart, `<span>boundary</span><span>1.0%</span>`, "exactly 1% remains separate")
	assert.Contains(t, chart, `<span>Other</span><span>2.0%</span>`)
	assert.Contains(t, chart, `Other: 20 (2.0%)`, "tooltips preserve aggregated counts")
	assert.NotContains(t, chart, "small-a")
	assert.NotContains(t, chart, "small-b")
	assert.NotContains(t, chart, "Others")
	assert.NotContains(t, chart, "Unknown")
	assert.NotContains(t, chart, "unkown")
	assert.Equal(t, 1, strings.Count(chart, `<span>Other</span>`))
}

func mustRenderCopilotMetricsHTML(t *testing.T, s *copilotmetrics.Summary, opts CopilotMetricsReportOptions) string {
	t.Helper()
	out, err := RenderCopilotMetricsHTMLWithOptions(s, opts)
	require.NoError(t, err)
	return out
}

func TestRenderCopilotMetricsEmptySummary(t *testing.T) {
	s := &copilotmetrics.Summary{Source: "acme (enterprise)", Sections: copilotmetrics.SectionSet{copilotmetrics.SectionTrend: true}}
	md := RenderCopilotMetricsMarkdown(s)
	assert.NotContains(t, md, "mermaid")
	assert.Contains(t, md, "No data.")
	html, err := RenderCopilotMetricsHTML(s)
	require.NoError(t, err)
	assert.NotContains(t, html, "<svg")
}

func TestFormatCopilotMetricsInt(t *testing.T) {
	assert.Equal(t, "0", formatCopilotMetricsInt(0))
	assert.Equal(t, "999", formatCopilotMetricsInt(999))
	assert.Equal(t, "1,000", formatCopilotMetricsInt(1000))
	assert.Equal(t, "-12,345", formatCopilotMetricsInt(-12345))
}

func TestRenderCopilotMetricsReport(t *testing.T) {
	s := testCopilotMetricsSummary()
	md, err := RenderCopilotMetricsReport(s, copilotmetrics.FormatMarkdown)
	require.NoError(t, err)
	assert.Equal(t, RenderCopilotMetricsMarkdown(s), md)
	richMarkdown, err := RenderCopilotMetricsReportWithOptions(s, copilotmetrics.FormatMarkdown, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, richMarkdown, "private-login", "rich rendering honors an explicitly selected user section")
	assert.Contains(t, richMarkdown, "## By user")
	assert.Contains(t, richMarkdown, "## Key metrics")
	html, err := RenderCopilotMetricsReport(s, copilotmetrics.FormatHTML)
	require.NoError(t, err)
	assert.Contains(t, html, "<!DOCTYPE html>")
	richHTML, err := RenderCopilotMetricsReportWithOptions(s, copilotmetrics.FormatHTML, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	assert.Contains(t, richHTML, `<body class="rich">`)
	_, err = RenderCopilotMetricsReport(s, copilotmetrics.Format("pdf"))
	assert.Error(t, err)
}

func TestRenderCopilotMetricsUserSectionSelection(t *testing.T) {
	for _, selection := range []struct {
		name        string
		input       []string
		includeUser bool
	}{
		{name: "default"},
		{name: "all", input: []string{"all"}, includeUser: true},
		{name: "user", input: []string{"user"}, includeUser: true},
	} {
		for _, rich := range []bool{false, true} {
			for _, format := range []copilotmetrics.Format{copilotmetrics.FormatHTML, copilotmetrics.FormatMarkdown} {
				t.Run(selection.name+"/"+string(format)+"/rich="+strconv.FormatBool(rich), func(t *testing.T) {
					s := testCopilotMetricsSummary()
					var err error
					s.Sections, err = copilotmetrics.ParseSections(selection.input)
					require.NoError(t, err)
					out, err := RenderCopilotMetricsReportWithOptions(s, format, CopilotMetricsReportOptions{Rich: rich})
					require.NoError(t, err)
					if selection.includeUser {
						assert.Contains(t, out, "private-login")
						assert.Contains(t, out, "By user")
					} else {
						assert.NotContains(t, out, "private-login")
						assert.NotContains(t, out, "By user")
					}
				})
			}
		}
	}
}

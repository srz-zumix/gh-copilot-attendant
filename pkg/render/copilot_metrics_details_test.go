package render

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCopilotMetricsDetailSummary() *copilotmetrics.Summary {
	count := func(n int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: n, Valid: true} }
	s := &copilotmetrics.Summary{
		Source:   "example (organization)",
		Top:      1,
		Sections: copilotmetrics.SectionSet{copilotmetrics.SectionModel: true, copilotmetrics.SectionLanguage: true},
		ModelRequestsByFeature: []copilotmetrics.RichValue{
			{Name: "model-a", Group: "chat_panel_agent_mode", Value: count(60)},
			{Name: "model-a", Group: "chat_panel_ask_mode", Value: count(30)},
			{Name: "model-b", Group: "chat_panel_unknown_mode", Value: count(10)},
			{Name: "", Group: "chat_panel_plan_mode"},
			{Name: "model-zero", Group: "chat_panel_edit_mode", Value: count(0)},
			{Name: "model|<example>", Group: "chat_panel_agent_mode", Value: count(0)},
			{Name: "model-inline", Group: "chat_inline", Value: count(900)},
		},
		ModelCodeGenerationByLanguage: []copilotmetrics.RichValue{
			{Name: "model-a", Group: "language-a", Value: count(60)},
			{Name: "model-b", Group: "language-b", Value: count(40)},
			{Name: "", Group: ""},
			{Name: "model-zero", Group: "language-zero", Value: count(0)},
			{Name: "model|<example>", Group: "language|<example>", Value: count(0)},
		},
	}
	for _, alias := range []string{"unknown", "unkown", "Other", "Others"} {
		s.ModelRequestsByFeature = append(s.ModelRequestsByFeature, copilotmetrics.RichValue{
			Name: alias, Group: "chat_panel_agent_mode", Value: count(0),
		})
		s.ModelCodeGenerationByLanguage = append(s.ModelCodeGenerationByLanguage, copilotmetrics.RichValue{
			Name: alias, Group: alias, Value: count(0),
		})
	}
	for i := 1; i <= 25; i++ {
		name := fmt.Sprintf("item-%02d", i)
		s.ModelRequestsByFeature = append(s.ModelRequestsByFeature, copilotmetrics.RichValue{
			Name: name, Group: "chat_panel_agent_mode", Value: count(0),
		})
		s.ModelCodeGenerationByLanguage = append(s.ModelCodeGenerationByLanguage, copilotmetrics.RichValue{
			Name: name, Group: name, Value: count(0),
		})
	}
	return s
}

func addCopilotMetricsDetailRichData(s *copilotmetrics.Summary) {
	count := func(n int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: n, Valid: true} }
	s.Rich = &copilotmetrics.RichMetrics{
		Days: []string{"2030-01-01", "2030-01-02"},
		ModelRequests: []copilotmetrics.RichValue{
			{Name: "model-a", Value: count(90)}, {Name: "model-b", Value: count(10)},
		},
		ModelRequestsByDay: []copilotmetrics.RichSeries{
			{Name: "model-a", Values: []copilotmetrics.Counter{count(40), count(50)}},
			{Name: "model-b", Values: []copilotmetrics.Counter{count(4), count(6)}},
		},
		ModelRequestsByFeature: s.ModelRequestsByFeature,
		LanguageCodeGeneration: []copilotmetrics.RichValue{
			{Name: "language-a", Value: count(60)}, {Name: "language-b", Value: count(40)},
		},
		LanguageCodeGenerationByDay: []copilotmetrics.RichSeries{
			{Name: "language-a", Values: []copilotmetrics.Counter{count(20), count(40)}},
			{Name: "language-b", Values: []copilotmetrics.Counter{count(15), count(25)}},
		},
		ModelCodeGenerationByLanguage: s.ModelCodeGenerationByLanguage,
	}
}

func TestCopilotMetricsDetailTablesAllFormats(t *testing.T) {
	for _, rich := range []bool{false, true} {
		t.Run(fmt.Sprintf("rich=%t", rich), func(t *testing.T) {
			s := testCopilotMetricsDetailSummary()
			if rich {
				addCopilotMetricsDetailRichData(s)
			}
			markdown := RenderCopilotMetricsMarkdown(s)
			html, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: rich})
			require.NoError(t, err)
			assert.NotContains(t, html, "ZgotmplZ")
			for _, output := range []string{html, markdown} {
				assert.Contains(t, output, copilotMetricsModelChatModeTitle)
				assert.Contains(t, output, copilotMetricsModelLanguageTitle)
				assert.Contains(t, output, "item-25", "detail rows are not truncated by --top")
				assert.Contains(t, output, "(not reported)")
			}
			assert.Contains(t, markdown, "| model-a | Agent | 60 | 60.0% |")
			assert.Contains(t, markdown, "| model-a | Ask | 30 | 30.0% |")
			assert.Contains(t, markdown, "| model-b | Unknown | 10 | 10.0% |")
			assert.Contains(t, markdown, "| (not reported) | Plan | n/a | n/a |")
			assert.Contains(t, markdown, "| model-zero | Edit | 0 | 0.0% |")
			assert.Contains(t, markdown, "| language-a | model-a | 60 | 60.0% |")
			assert.Contains(t, markdown, "| language-b | model-b | 40 | 40.0% |")
			assert.Contains(t, markdown, "| language\\|&lt;example&gt; | model\\|&lt;example&gt; |")
			assert.Contains(t, html, "language|&lt;example&gt;")
			assert.NotContains(t, html, "language|<example>")
			assert.Contains(t, html, `class="num heatmap">60.0%</td>`)
			assert.Contains(t, html, `aria-sort="none"`)
			assert.Contains(t, html, `class="sort-button"`)
			assert.Contains(t, html, `style="background-color:#`)
			assert.Contains(t, markdown, "No data.")
			assert.Contains(t, html, "No data.")

			modelTable := copilotMetricsDetailForSection(s, copilotmetrics.SectionModel)
			require.NotNil(t, modelTable)
			for _, row := range modelTable.Rows {
				assert.NotEqual(t, "model-inline", row[0], "non-chat features are excluded")
			}
			for _, alias := range []string{"unknown", "unkown", "Other", "Others"} {
				assert.Contains(t, markdown, "| "+alias+" | Agent | 0 | 0.0% |")
				assert.Contains(t, markdown, "| "+alias+" | "+alias+" | 0 | 0.0% |")
			}
		})
	}
}

func TestCopilotMetricsDetailTablesZeroAndInvalidCounts(t *testing.T) {
	s := &copilotmetrics.Summary{
		ModelRequestsByFeature: []copilotmetrics.RichValue{
			{Name: "model-zero", Group: "chat_panel_agent_mode", Value: copilotmetrics.Counter{Valid: true}},
			{Name: "model-missing", Group: "chat_panel_agent_mode"},
			{Name: "model-negative", Group: "chat_panel_agent_mode", Value: copilotmetrics.Counter{Value: -1, Valid: true}},
		},
	}
	table := copilotMetricsDetailForSection(s, copilotmetrics.SectionModel)
	require.NotNil(t, table)
	assert.Equal(t, [][]string{
		{"model-zero", "Agent", "0", "n/a"},
		{"model-missing", "Agent", "n/a", "n/a"},
		{"model-negative", "Agent", "-1", "n/a"},
	}, table.Rows)
}

func TestCopilotMetricsDetailTablesRespectSections(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sections     copilotmetrics.SectionSet
		wantModel    bool
		wantLanguage bool
	}{
		{name: "model", sections: copilotmetrics.SectionSet{copilotmetrics.SectionModel: true}, wantModel: true},
		{name: "language", sections: copilotmetrics.SectionSet{copilotmetrics.SectionLanguage: true}, wantLanguage: true},
		{name: "neither", sections: copilotmetrics.SectionSet{copilotmetrics.SectionFeature: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testCopilotMetricsDetailSummary()
			s.Sections = tc.sections
			html, err := RenderCopilotMetricsHTML(s)
			require.NoError(t, err)
			markdown := RenderCopilotMetricsMarkdown(s)
			for _, output := range []string{html, markdown} {
				assert.Equal(t, tc.wantModel, strings.Contains(output, copilotMetricsModelChatModeTitle))
				assert.Equal(t, tc.wantLanguage, strings.Contains(output, copilotMetricsModelLanguageTitle))
			}
		})
	}
}

func TestCopilotMetricsCategoryColorsAcrossChartsAndTables(t *testing.T) {
	s := testCopilotMetricsDetailSummary()
	addCopilotMetricsDetailRichData(s)
	s.Models = []copilotmetrics.BreakdownRow{
		{Key: "model-a"}, {Key: "model-b"},
	}
	s.Languages = []copilotmetrics.BreakdownRow{
		{Key: "language-a"}, {Key: "language-b"},
	}
	s.AllLanguages = s.Languages
	sections := copilotMetricsHTMLSections(s, true)
	require.Len(t, sections, 2)
	for _, section := range sections {
		t.Run(string(section.ID), func(t *testing.T) {
			scheme := copilotMetricsColorsForSection(s, section.ID)
			require.NotNil(t, scheme)
			require.Len(t, section.RichPanels, 3)
			require.Len(t, section.Details, 1)
			group, series, rowIndex := "model-a", "Agent", 0
			if section.ID == copilotmetrics.SectionLanguage {
				group, series = "language-a", "model-a"
			}
			base, shade := scheme.group(group), scheme.seriesColor(series)
			assert.Equal(t, base, section.Table.RowColors[rowIndex].Group)
			assert.Equal(t, base, section.Details[0].Table.RowColors[rowIndex].Group)
			assert.Equal(t, shade, section.Details[0].Table.RowColors[rowIndex].Series)
			assert.Contains(t, string(section.RichPanels[0].Chart), `class="metric-area" fill="`+copilotMetricsHueGradientColor(0, 1).hex()+`"`)
			assert.Contains(t, string(section.RichPanels[1].Chart), `fill="`+copilotMetricsThemeGradientColor(scheme.theme, 0, 2).hex()+`"`)
			stacked := string(section.RichPanels[2].Chart)
			assert.Contains(t, stacked, `fill="`+shade+`"`)
			assert.NotContains(t, stacked, `class="group-key"`)
			assert.Contains(t, stacked, `class="stacked-legend"`)
			keyLabel := "Chat mode: " + series
			if section.ID == copilotmetrics.SectionLanguage {
				keyLabel = "Model: " + series
			}
			keyTooltip := copilotMetricsTooltip{Heading: keyLabel}
			assert.Contains(t, stacked, `class="series-key" style="background-color:`+shade+`" `+keyTooltip.attributes()+` tabindex="0" role="img" aria-label="`+keyLabel+`"`)
			assert.Equal(t, 1, strings.Count(stacked, `class="series-key" style="background-color:`+shade+`"`), "one legend swatch per series")
			assert.Contains(t, stacked, "keeps the same color in every bar.")
			for _, color := range []copilotMetricsColor{scheme.groups[group], scheme.series[series]} {
				if section.ID == copilotmetrics.SectionLanguage {
					assert.Greater(t, color.red, color.blue, "language section uses the orange theme")
				} else {
					assert.Greater(t, color.blue, color.red, "model section uses the blue theme")
				}
			}
			html, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: true})
			require.NoError(t, err)
			assert.Contains(t, html, `style="background-color:`+base+`"`)
			assert.Contains(t, html, `style="background-color:`+shade+`"`)
			assert.NotContains(t, html, "ZgotmplZ")
			for _, alias := range []string{"", "Other", "Others", "unknown", "unkown"} {
				assert.Equal(t, "#6e7781", scheme.group(alias))
			}
		})
	}
	model := copilotMetricsColorsForSection(s, copilotmetrics.SectionModel)
	language := copilotMetricsColorsForSection(s, copilotmetrics.SectionLanguage)
	assert.NotEqual(t, model.seriesColor("Agent"), model.seriesColor("Ask"))
	assert.NotEqual(t, model.group("model-a"), language.group("language-a"), "sections use different themes")
	assert.Equal(t, "#6e7781", model.seriesColor("Other"))
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("item-%02d", i)
	}
	seen := map[copilotMetricsColor]bool{}
	for _, color := range copilotMetricsThemeColors(copilotMetricsThemeOrange, names) {
		assert.False(t, seen[color], "overflow categories receive separate colors")
		seen[color] = true
		assert.Greater(t, color.red, color.blue, "overflow colors stay in the orange theme")
	}
}

func TestCopilotMetricsStackedSeriesColorIsConsistentAcrossBars(t *testing.T) {
	valid := func(value int64) copilotmetrics.Counter { return copilotmetrics.Counter{Value: value, Valid: true} }
	values := []copilotmetrics.RichValue{
		{Name: "model-a", Group: "language-a", Value: valid(30)},
		{Name: "model-b", Group: "language-a", Value: valid(20)},
		{Name: "model-a", Group: "language-b", Value: valid(10)},
		{Name: "model-b", Group: "language-b", Value: valid(5)},
	}
	chart := string(copilotMetricsHTMLRichStackedBarPanel("Languages", values).Chart)
	scheme := copilotMetricsColorsForDimensions(values, copilotMetricsThemeOrange)
	for _, model := range []string{"model-a", "model-b"} {
		color := scheme.seriesColor(model)
		assert.Equal(t, 2, strings.Count(chart, `fill="`+color+`" data-tooltip="Language: `), model)
		assert.Equal(t, 2, strings.Count(chart, `role="img" aria-label="Model: `+model+"\nLanguage: "), model)
	}
	assert.NotEqual(t, scheme.seriesColor("model-a"), scheme.seriesColor("model-b"))
}

func TestCopilotMetricsStackedLegendEscapesLabels(t *testing.T) {
	panel := copilotMetricsHTMLRichStackedBarPanel("Languages", []copilotmetrics.RichValue{
		{Name: `model"><example>`, Group: `language"><example>`, Value: copilotmetrics.Counter{Value: 1, Valid: true}},
	})
	chart := string(panel.Chart)
	assert.Contains(t, chart, `Language: language&#34;&gt;&lt;example&gt;`)
	assert.Contains(t, chart, `Model: model&#34;&gt;&lt;example&gt;`)
	assert.Contains(t, chart, `class="stacked-legend-label">model&#34;&gt;&lt;example&gt;</span>`)
	assert.NotContains(t, chart, `<example>`)
}

func TestCopilotMetricsStackedAxisLongLabels(t *testing.T) {
	values := make([]copilotmetrics.RichValue, 6)
	for i := range values {
		values[i] = copilotmetrics.RichValue{
			Name: "model-a", Group: fmt.Sprintf("language-%d", i),
			Value: copilotmetrics.Counter{Value: 1, Valid: true},
		}
	}
	short := string(copilotMetricsHTMLRichStackedBarPanel("Languages", values).Chart)
	longLabel := strings.Repeat("synthetic-language-", 5)
	values[0].Group = longLabel
	long := string(copilotMetricsHTMLRichStackedBarPanel("Languages", values).Chart)
	height := func(chart string) float64 {
		t.Helper()
		_, after, ok := strings.Cut(chart, `viewBox="`)
		require.True(t, ok)
		viewBox, _, ok := strings.Cut(after, `"`)
		require.True(t, ok)
		fields := strings.Fields(viewBox)
		require.Len(t, fields, 4)
		value, err := strconv.ParseFloat(fields[3], 64)
		require.NoError(t, err)
		return value
	}
	assert.Greater(t, height(long), height(short))
	assert.Equal(t, 6, strings.Count(long, `<foreignObject `))
	assert.Contains(t, long, `data-tooltip="Language: `+longLabel+"\n")
	assert.Contains(t, long, `>`+longLabel+`</div>`)
	assert.Contains(t, long, `class="stacked-axis-label"`)
}

func TestCopilotMetricsCategoryColorsAreStableAcrossInputOrder(t *testing.T) {
	s := testCopilotMetricsDetailSummary()
	modelBefore := copilotMetricsColorsForSection(s, copilotmetrics.SectionModel)
	languageBefore := copilotMetricsColorsForSection(s, copilotmetrics.SectionLanguage)
	for i, j := 0, len(s.ModelRequestsByFeature)-1; i < j; i, j = i+1, j-1 {
		s.ModelRequestsByFeature[i], s.ModelRequestsByFeature[j] = s.ModelRequestsByFeature[j], s.ModelRequestsByFeature[i]
	}
	for i, j := 0, len(s.ModelCodeGenerationByLanguage)-1; i < j; i, j = i+1, j-1 {
		s.ModelCodeGenerationByLanguage[i], s.ModelCodeGenerationByLanguage[j] = s.ModelCodeGenerationByLanguage[j], s.ModelCodeGenerationByLanguage[i]
	}
	modelAfter := copilotMetricsColorsForSection(s, copilotmetrics.SectionModel)
	languageAfter := copilotMetricsColorsForSection(s, copilotmetrics.SectionLanguage)
	assert.Equal(t, modelBefore.groups, modelAfter.groups)
	assert.Equal(t, modelBefore.series, modelAfter.series)
	assert.Equal(t, languageBefore.groups, languageAfter.groups)
	assert.Equal(t, languageBefore.series, languageAfter.series)
}

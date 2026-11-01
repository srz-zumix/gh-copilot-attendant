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

type copilotMetricsHTMLNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr               `xml:",any,attr"`
	Children []copilotMetricsHTMLNode `xml:",any"`
}

func (n copilotMetricsHTMLNode) attr(name string) (string, bool) {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (n copilotMetricsHTMLNode) descendants(name string) []copilotMetricsHTMLNode {
	var matches []copilotMetricsHTMLNode
	for _, child := range n.Children {
		if child.XMLName.Local == name {
			matches = append(matches, child)
		}
		matches = append(matches, child.descendants(name)...)
	}
	return matches
}

func copilotMetricsTestSection(t *testing.T, report, id string) (copilotMetricsHTMLNode, string) {
	t.Helper()
	start := strings.Index(report, `<section id="`+id+`"`)
	require.NotEqual(t, -1, start)
	end := strings.Index(report[start:], "</section>")
	require.NotEqual(t, -1, end)
	fragment := report[start : start+end+len("</section>")]
	xmlFragment := strings.ReplaceAll(fragment, ` open>`, ` open="open">`)
	var section copilotMetricsHTMLNode
	require.NoError(t, xml.Unmarshal([]byte(xmlFragment), &section))
	return section, fragment
}

func TestCopilotMetricsHTMLCollapsibleSectionsAndTables(t *testing.T) {
	for _, rich := range []bool{false, true} {
		t.Run(fmt.Sprintf("rich=%t", rich), func(t *testing.T) {
			s := testCopilotMetricsDetailSummary()
			s.Sections[copilotmetrics.SectionKPI] = true
			s.Sections[copilotmetrics.SectionTrend] = true
			s.Sections[copilotmetrics.SectionFeature] = true
			s.Sections[copilotmetrics.SectionIDE] = true
			s.KPIs = []copilotmetrics.KPI{{Label: "Example", Value: 1, Valid: true}}
			s.Trend = []copilotmetrics.TrendPoint{{
				Day: "2030-01-01", DailyActiveUsers: copilotmetrics.Counter{Value: 1, Valid: true},
			}}
			s.Features = []copilotmetrics.BreakdownRow{{Key: "feature-a"}}
			s.Models = []copilotmetrics.BreakdownRow{{Key: "model-a"}}
			s.Languages = []copilotmetrics.BreakdownRow{{Key: "language-a"}}
			s.Omitted = map[copilotmetrics.Section]int{copilotmetrics.SectionLanguage: 1}
			if rich {
				addCopilotMetricsDetailRichData(s)
			}
			report, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: rich})
			require.NoError(t, err)

			for id, tableCount := range map[string]int{
				"kpi": 1, "trend": 1, "feature": 1, "language": 2, "model": 2, "ide": 0,
			} {
				t.Run(id, func(t *testing.T) {
					section, fragment := copilotMetricsTestSection(t, report, id)
					require.Len(t, section.Children, 2)
					heading, group := section.Children[0], section.Children[1]
					assert.Equal(t, "h2", heading.XMLName.Local)
					assert.Equal(t, "details", group.XMLName.Local)
					assert.Equal(t, "section-details", mustCopilotMetricsHTMLAttr(t, group, "class"))
					_, expanded := group.attr("open")
					assert.True(t, expanded, "sections are initially expanded in both modes")
					require.NotEmpty(t, group.Children)
					summary := group.Children[0]
					assert.Equal(t, "summary", summary.XMLName.Local)
					assert.Equal(t, mustCopilotMetricsHTMLAttr(t, heading, "id"), mustCopilotMetricsHTMLAttr(t, summary, "aria-labelledby"))
					assert.Equal(t, mustCopilotMetricsHTMLAttr(t, heading, "id"), mustCopilotMetricsHTMLAttr(t, section, "aria-labelledby"))

					var tables []copilotMetricsHTMLNode
					for _, detail := range group.descendants("details") {
						if class, _ := detail.attr("class"); class == "table-details" {
							tables = append(tables, detail)
						}
					}
					require.Len(t, tables, tableCount)
					assert.Len(t, section.descendants("table"), tableCount)
					for _, detail := range tables {
						_, expanded := detail.attr("open")
						assert.Equal(t, !rich, expanded, "normal tables are open and rich tables are closed")
						require.NotEmpty(t, detail.Children)
						summary := detail.Children[0]
						assert.Equal(t, "summary", summary.XMLName.Local)
						headingID := mustCopilotMetricsHTMLAttr(t, summary, "aria-labelledby")
						var matchingHeadings int
						for _, heading := range group.descendants("h3") {
							if headingID == mustCopilotMetricsHTMLAttr(t, heading, "id") {
								matchingHeadings++
							}
						}
						assert.Equal(t, 1, matchingHeadings)
						assert.Len(t, detail.descendants("table"), 1)
					}
					if id == "language" {
						assert.Contains(t, fragment, "1 more row(s) omitted by --top.")
						assert.NotEmpty(t, tables[0].descendants("p"), "the omission note belongs to its table")
					}
					if id == "ide" {
						assert.Contains(t, fragment, "<p>No data.</p>")
					}
					if rich && (id == "language" || id == "model") {
						assert.NotEmpty(t, group.descendants("figure"), "section controls include the charts")
					}
				})
			}
			assert.Contains(t, report, `window.addEventListener("beforeprint"`)
			assert.Contains(t, report, `window.addEventListener("afterprint"`)
		})
	}
}

func mustCopilotMetricsHTMLAttr(t *testing.T, node copilotMetricsHTMLNode, name string) string {
	t.Helper()
	value, ok := node.attr(name)
	require.True(t, ok, "missing %s on %s", name, node.XMLName.Local)
	return value
}

func TestCopilotMetricsHTMLEmptyDetailRemainsVisible(t *testing.T) {
	s := testCopilotMetricsDetailSummary()
	s.ModelCodeGenerationByLanguage = nil
	s.Languages = []copilotmetrics.BreakdownRow{{Key: "language-a"}}
	report, err := RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{Rich: true})
	require.NoError(t, err)
	section, fragment := copilotMetricsTestSection(t, report, "language")
	assert.Len(t, section.descendants("table"), 1)
	assert.Contains(t, fragment, "<h3>"+copilotMetricsModelLanguageTitle+"</h3>")
	assert.Contains(t, fragment, "<p>No data.</p>")
}

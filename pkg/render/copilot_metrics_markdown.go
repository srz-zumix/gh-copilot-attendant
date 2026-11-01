package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

type copilotMetricsMarkdownTable struct {
	Headers []string
	Numeric []bool
	Rows    [][]string
}

type copilotMetricsMarkdownSection struct {
	ID          copilotmetrics.Section
	Title       string
	Description string
	Table       copilotMetricsMarkdownTable
	Omitted     int
	Empty       bool
}

func escapeCopilotMetricsMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return " "
	}
	return s
}

func writeCopilotMetricsMarkdownTable(b *strings.Builder, t copilotMetricsMarkdownTable) {
	cells := make([]string, len(t.Headers))
	for i, h := range t.Headers {
		cells[i] = escapeCopilotMetricsMarkdownCell(h)
	}
	fmt.Fprintf(b, "| %s |\n", strings.Join(cells, " | "))
	for i := range t.Headers {
		if i < len(t.Numeric) && t.Numeric[i] {
			cells[i] = "---:"
		} else {
			cells[i] = "---"
		}
	}
	fmt.Fprintf(b, "| %s |\n", strings.Join(cells, " | "))
	for _, row := range t.Rows {
		rc := make([]string, len(row))
		for i, c := range row {
			rc[i] = escapeCopilotMetricsMarkdownCell(c)
		}
		fmt.Fprintf(b, "| %s |\n", strings.Join(rc, " | "))
	}
}

func copilotMetricsMarkdownBreakdownTable(keyHeader string, rows []copilotmetrics.BreakdownRow) copilotMetricsMarkdownTable {
	withInteractions := false
	for _, row := range rows {
		if row.UserInitiatedInteraction.Valid {
			withInteractions = true
			break
		}
	}
	table := copilotMetricsMarkdownTable{Headers: []string{keyHeader}, Numeric: []bool{false}}
	if withInteractions {
		table.Headers = append(table.Headers, "Interactions")
		table.Numeric = append(table.Numeric, true)
	}
	table.Headers = append(table.Headers, "Code generation", "Code acceptance", "Acceptance rate", "LOC added", "LOC deleted")
	table.Numeric = append(table.Numeric, true, true, true, true, true)
	for _, row := range rows {
		cells := []string{row.Key}
		if withInteractions {
			cells = append(cells, formatCopilotMetricsCounter(row.UserInitiatedInteraction))
		}
		rate, ok := row.AcceptanceRate()
		cells = append(cells,
			formatCopilotMetricsCounter(row.CodeGeneration),
			formatCopilotMetricsCounter(row.CodeAcceptance),
			formatCopilotMetricsRate(rate, ok),
			formatCopilotMetricsCounter(row.LOCAdded),
			formatCopilotMetricsCounter(row.LOCDeleted),
		)
		table.Rows = append(table.Rows, cells)
	}
	return table
}

func copilotMetricsMarkdownSections(s *copilotmetrics.Summary) []copilotMetricsMarkdownSection {
	var sections []copilotMetricsMarkdownSection
	for _, id := range s.Sections.Ordered() {
		section := copilotMetricsMarkdownSection{ID: id, Omitted: s.Omitted[id]}
		switch id {
		case copilotmetrics.SectionKPI:
			section.Title = "Key metrics"
			section.Description = "Active-user metrics are snapshots and are never summed across days: daily active users are averaged and peaked, and weekly/monthly active users are taken from the last day with data. Counters are summed over the window and rates are computed from the summed values."
			section.Table = copilotMetricsMarkdownTable{Headers: []string{"Metric", "Value", "Window"}, Numeric: []bool{false, true, false}}
			for _, kpi := range s.KPIs {
				section.Table.Rows = append(section.Table.Rows, []string{kpi.Label, formatCopilotMetricsKPI(kpi), kpi.Window})
			}
			section.Empty = len(s.KPIs) == 0
		case copilotmetrics.SectionTrend:
			section.Title = "Daily trend"
			section.Description = "Per-day values from the organization-level report. Days without a report are shown as n/a."
			section.Table = copilotMetricsMarkdownTable{
				Headers: []string{"Day", "Daily active users", "Interactions", "Code generation", "Code acceptance", "LOC added"},
				Numeric: []bool{false, true, true, true, true, true},
			}
			for _, point := range s.Trend {
				section.Table.Rows = append(section.Table.Rows, []string{
					point.Day,
					formatCopilotMetricsCounter(point.DailyActiveUsers),
					formatCopilotMetricsCounter(point.UserInitiatedInteraction),
					formatCopilotMetricsCounter(point.CodeGeneration),
					formatCopilotMetricsCounter(point.CodeAcceptance),
					formatCopilotMetricsCounter(point.LOCAdded),
				})
			}
			section.Empty = len(s.Trend) == 0
		case copilotmetrics.SectionFeature:
			section.Title = "By feature"
			section.Table = copilotMetricsMarkdownBreakdownTable("Feature", s.Features)
			section.Empty = len(s.Features) == 0
		case copilotmetrics.SectionLanguage:
			section.Title = "By language"
			section.Table = copilotMetricsMarkdownBreakdownTable("Language", s.Languages)
			section.Empty = len(s.Languages) == 0
		case copilotmetrics.SectionModel:
			section.Title = "By model"
			section.Table = copilotMetricsMarkdownBreakdownTable("Model", s.Models)
			section.Empty = len(s.Models) == 0
		case copilotmetrics.SectionIDE:
			section.Title = "By IDE"
			section.Table = copilotMetricsMarkdownBreakdownTable("IDE", s.IDEs)
			section.Empty = len(s.IDEs) == 0
		case copilotmetrics.SectionRepository:
			section.Title = "By repository"
			section.Description = "Pull request activity summed over the 1-day repository reports in the window."
			section.Table = copilotMetricsMarkdownTable{
				Headers: []string{"Repository", "Visibility", "Days", "PRs created", "Created by Copilot", "PRs reviewed", "Reviewed by Copilot", "PRs merged", "Merged (Copilot-created)", "Copilot suggestions", "Applied suggestions"},
				Numeric: []bool{false, false, true, true, true, true, true, true, true, true, true},
			}
			for _, row := range s.Repositories {
				section.Table.Rows = append(section.Table.Rows, []string{
					row.Name, row.Visibility, strconv.Itoa(row.ActiveDays),
					formatCopilotMetricsCounter(row.Created),
					formatCopilotMetricsCounter(row.CreatedByCopilot),
					formatCopilotMetricsCounter(row.Reviewed),
					formatCopilotMetricsCounter(row.ReviewedByCopilot),
					formatCopilotMetricsCounter(row.Merged),
					formatCopilotMetricsCounter(row.MergedCreatedByCopilot),
					formatCopilotMetricsCounter(row.CopilotSuggestions),
					formatCopilotMetricsCounter(row.CopilotAppliedSuggestions),
				})
			}
			section.Empty = len(s.Repositories) == 0
		case copilotmetrics.SectionTeam:
			section.Title = "By team"
			section.Description = "User activity joined with same-day team membership. Users in several teams are counted in each team, so team totals do not add up to the organization total. GitHub may omit small teams from team reports."
			section.Table = copilotMetricsMarkdownTable{
				Headers: []string{"Team", "Members seen", "Active users", "Interactions", "Code generation", "Code acceptance", "Acceptance rate", "LOC added", "AI credits"},
				Numeric: []bool{false, true, true, true, true, true, true, true, true},
			}
			for _, row := range s.Teams {
				rate, ok := row.AcceptanceRate()
				section.Table.Rows = append(section.Table.Rows, []string{
					row.Slug, strconv.Itoa(row.Members), strconv.Itoa(row.ActiveUsers),
					formatCopilotMetricsCounter(row.UserInitiatedInteraction),
					formatCopilotMetricsCounter(row.CodeGeneration),
					formatCopilotMetricsCounter(row.CodeAcceptance),
					formatCopilotMetricsRate(rate, ok),
					formatCopilotMetricsCounter(row.LOCAdded),
					formatCopilotMetricsCredits(row.AICredits),
				})
			}
			section.Empty = len(s.Teams) == 0
		case copilotmetrics.SectionUser:
			section.Title = "By user"
			section.Description = "Users ranked by user-initiated interactions."
			section.Table = copilotMetricsMarkdownTable{
				Headers: []string{"#", "User", "Active days", "Interactions", "Code generation", "Code acceptance", "Acceptance rate", "LOC added", "AI credits", "Surfaces"},
				Numeric: []bool{true, false, true, true, true, true, true, true, true, false},
			}
			for i, row := range s.Users {
				rate, ok := row.AcceptanceRate()
				section.Table.Rows = append(section.Table.Rows, []string{
					strconv.Itoa(i + 1), row.Login, strconv.Itoa(row.ActiveDays),
					formatCopilotMetricsCounter(row.UserInitiatedInteraction),
					formatCopilotMetricsCounter(row.CodeGeneration),
					formatCopilotMetricsCounter(row.CodeAcceptance),
					formatCopilotMetricsRate(rate, ok),
					formatCopilotMetricsCounter(row.LOCAdded),
					formatCopilotMetricsCredits(row.AICredits),
					strings.Join(row.Surfaces, ", "),
				})
			}
			section.Empty = len(s.Users) == 0
		default:
			continue
		}
		sections = append(sections, section)
	}
	return sections
}

func copilotMetricsMarkdownOmittedText(n int) string {
	return fmt.Sprintf("%d more row(s) omitted by --top.", n)
}

// escapeCopilotMetricsMermaidLabel keeps chart labels inside a quoted mermaid string.
func escapeCopilotMetricsMermaidLabel(s string) string {
	return strings.NewReplacer(`"`, "'", "\n", " ", "\r", " ").Replace(s)
}

func writeCopilotMetricsMermaidChart(b *strings.Builder, title, yLabel, kind string, labels []string, values []int64) {
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = fmt.Sprintf("%q", escapeCopilotMetricsMermaidLabel(l))
	}
	nums := make([]string, len(values))
	for i, v := range values {
		nums[i] = fmt.Sprintf("%d", v)
	}
	b.WriteString("```mermaid\nxychart-beta\n")
	fmt.Fprintf(b, "    title %q\n", escapeCopilotMetricsMermaidLabel(title))
	fmt.Fprintf(b, "    x-axis [%s]\n", strings.Join(quoted, ", "))
	fmt.Fprintf(b, "    y-axis %q\n", escapeCopilotMetricsMermaidLabel(yLabel))
	fmt.Fprintf(b, "    %s [%s]\n", kind, strings.Join(nums, ", "))
	b.WriteString("```\n\n")
}

// copilotMetricsMarkdownTrendSeries returns short day labels and per-day values. Missing values
// are 0 and reported through hasMissing so the renderer can disclose it.
func copilotMetricsMarkdownTrendSeries(trend []copilotmetrics.TrendPoint, get func(copilotmetrics.TrendPoint) copilotmetrics.Counter) (labels []string, values []int64, hasMissing bool) {
	for _, p := range trend {
		label := p.Day
		if len(label) == len("2006-01-02") {
			label = label[5:]
		}
		labels = append(labels, label)
		c := get(p)
		if !c.Valid {
			hasMissing = true
		}
		values = append(values, c.Value)
	}
	return labels, values, hasMissing
}

// RenderCopilotMetricsMarkdown renders a usage metrics summary as a Markdown document.
func RenderCopilotMetricsMarkdown(s *copilotmetrics.Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Copilot usage metrics: %s\n\n", s.Source)
	fmt.Fprintf(&b, "- Period: %s\n", copilotMetricsPeriodText(s))
	if g := copilotMetricsGeneratedText(s); g != "" {
		fmt.Fprintf(&b, "- Generated: %s\n", g)
	}
	b.WriteString("\n")
	if len(s.Partial) > 0 {
		b.WriteString("> [!WARNING]\n> Partial data:\n>\n")
		for _, p := range s.Partial {
			fmt.Fprintf(&b, "> - %s\n", copilotMetricsPartialText(p))
		}
		b.WriteString("\n")
	}
	if len(s.Warnings) > 0 {
		b.WriteString("> [!NOTE]\n")
		for i, w := range s.Warnings {
			if i > 0 {
				b.WriteString(">\n")
			}
			fmt.Fprintf(&b, "> %s\n", w)
		}
		b.WriteString("\n")
	}
	for _, sec := range copilotMetricsMarkdownSections(s) {
		fmt.Fprintf(&b, "## %s\n\n", sec.Title)
		if sec.Description != "" {
			fmt.Fprintf(&b, "%s\n\n", sec.Description)
		}
		if sec.Empty {
			b.WriteString("No data.\n\n")
		} else {
			if sec.ID == copilotmetrics.SectionTrend {
				labels, dau, missingDAU := copilotMetricsMarkdownTrendSeries(s.Trend, func(p copilotmetrics.TrendPoint) copilotmetrics.Counter { return p.DailyActiveUsers })
				writeCopilotMetricsMermaidChart(&b, "Daily active users", "Users", "line", labels, dau)
				_, interactions, missingInteractions := copilotMetricsMarkdownTrendSeries(s.Trend, func(p copilotmetrics.TrendPoint) copilotmetrics.Counter { return p.UserInitiatedInteraction })
				writeCopilotMetricsMermaidChart(&b, "User-initiated interactions", "Interactions", "bar", labels, interactions)
				if missingDAU || missingInteractions {
					b.WriteString("Days without data are plotted as 0 in the charts.\n\n")
				}
			}
			writeCopilotMetricsMarkdownTable(&b, sec.Table)
			b.WriteString("\n")
			if sec.Omitted > 0 {
				fmt.Fprintf(&b, "_%s_\n\n", copilotMetricsMarkdownOmittedText(sec.Omitted))
			}
		}
		if detail := copilotMetricsDetailForSection(s, sec.ID); detail != nil {
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", detail.Title, detail.Description)
			if len(detail.Rows) == 0 {
				b.WriteString("No data.\n\n")
			} else {
				writeCopilotMetricsMarkdownTable(&b, copilotMetricsMarkdownTable{
					Headers: detail.Headers, Rows: detail.Rows, Numeric: []bool{false, false, true, true},
				})
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// RenderCopilotMetricsIssueBody renders the short issue body that points to the attached artifacts.
func RenderCopilotMetricsIssueBody(s *copilotmetrics.Summary, artifactNames []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Copilot usage metrics: %s\n\n", s.Source)
	fmt.Fprintf(&b, "- Period: %s\n", copilotMetricsPeriodText(s))
	if g := copilotMetricsGeneratedText(s); g != "" {
		fmt.Fprintf(&b, "- Generated: %s\n", g)
	}
	highlights := map[string]bool{
		"Daily active users (average)": true,
		"Monthly active users":         true,
		"User-initiated interactions":  true,
		"Code acceptance rate":         true,
	}
	var lines []string
	for _, k := range s.KPIs {
		if highlights[k.Label] {
			lines = append(lines, fmt.Sprintf("| %s | %s | %s |", escapeCopilotMetricsMarkdownCell(k.Label), formatCopilotMetricsKPI(k), escapeCopilotMetricsMarkdownCell(k.Window)))
		}
	}
	if len(lines) > 0 {
		b.WriteString("\n| Metric | Value | Window |\n| --- | ---: | --- |\n")
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n")
	}
	if len(s.Partial) > 0 {
		b.WriteString("\n> [!WARNING]\n> Some reports were missing; see the attached report for details.\n")
	}
	if len(artifactNames) > 0 {
		b.WriteString("\nThe full report is attached to this issue as artifacts:\n\n")
		for _, n := range artifactNames {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}

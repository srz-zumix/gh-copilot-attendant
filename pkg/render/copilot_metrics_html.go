package render

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html"
	"html/template"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const (
	copilotMetricsChartWidth   = 720
	copilotMetricsChartHeight  = 220
	copilotMetricsChartPadLeft = 48
	copilotMetricsChartPadTop  = 16
	copilotMetricsChartPadBot  = 36
	copilotMetricsChartPadRgt  = 12
)

type copilotMetricsDayDot struct {
	X, Y  float64
	Color string
}

type copilotMetricsTooltipRow struct {
	Label string
	Value string
	Share string
}

type copilotMetricsTooltip struct {
	Heading string
	Rows    []copilotMetricsTooltipRow
	Note    string
}

func (tooltip copilotMetricsTooltip) text() string {
	var lines []string
	if tooltip.Heading != "" {
		lines = append(lines, tooltip.Heading)
	}
	for _, row := range tooltip.Rows {
		line := row.Label + ": " + row.Value
		if row.Share != "" {
			line += " (" + row.Share + ")"
		}
		lines = append(lines, line)
	}
	if tooltip.Note != "" {
		lines = append(lines, tooltip.Note)
	}
	return strings.Join(lines, "\n")
}

func (tooltip copilotMetricsTooltip) attributes() string {
	fields := url.Values{}
	if tooltip.Heading != "" {
		fields.Set("heading", tooltip.Heading)
	}
	for _, row := range tooltip.Rows {
		fields.Add("label", row.Label)
		fields.Add("value", row.Value)
		fields.Add("share", row.Share)
	}
	if tooltip.Note != "" {
		fields.Set("note", tooltip.Note)
	}
	text := tooltip.text()
	return fmt.Sprintf(`data-tooltip="%s" data-tooltip-fields="%s" data-tooltip-key="%s"`,
		template.HTMLEscapeString(text), template.HTMLEscapeString(fields.Encode()), copilotMetricsTooltipKey(text))
}

// svgAttributes returns the tooltip attributes plus the target's box in SVG
// viewBox units, which positions the CSS-only fallback next to the target.
func (tooltip copilotMetricsTooltip) svgAttributes(x, y, width, height float64) string {
	return fmt.Sprintf(`%s data-tooltip-box="%.2f %.2f %.2f %.2f"`, tooltip.attributes(), x, y, width, height)
}

func copilotMetricsTooltipKey(text string) string {
	digest := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", digest[:12])
}

var copilotMetricsTooltipAttributes = regexp.MustCompile(`data-tooltip="([^"]*)" data-tooltip-fields="[^"]*" data-tooltip-key="([0-9a-f]{24})"`)

type copilotMetricsHTMLTooltip struct {
	Key  string
	Text string
}

// copilotMetricsHTMLTooltipFallback is a CSS-only popup bound to one tooltip
// target. SVG targets are not valid CSS anchors, so they are anchored through
// an HTML proxy box positioned over the enclosing SVG; SVG names that SVG and
// Box holds the target's left, top, width, and height as viewBox fractions.
type copilotMetricsHTMLTooltipFallback struct {
	Target string
	Text   string
	SVG    string
	Box    [4]float64
}

var (
	copilotMetricsTooltipBoxAttribute = regexp.MustCompile(` data-tooltip-box="([0-9.]+) ([0-9.]+) ([0-9.]+) ([0-9.]+)"`)
	copilotMetricsSVGViewBox          = regexp.MustCompile(`^<svg [^>]*viewBox="0 0 ([0-9.]+) ([0-9.]+)"`)
)

type copilotMetricsHTMLInsertion struct {
	position int
	text     string
}

// copilotMetricsChartTooltips assigns a report-unique target ID to every tooltip
// target in chart and returns the rewritten chart, the distinct tooltip texts,
// and one fallback popup per target. nextTarget is advanced for each target.
func copilotMetricsChartTooltips(chart template.HTML, nextTarget *int) (template.HTML, []copilotMetricsHTMLTooltip, []copilotMetricsHTMLTooltipFallback, error) {
	source := string(chart)
	matches := copilotMetricsTooltipAttributes.FindAllStringSubmatchIndex(source, -1)
	if len(matches) != strings.Count(source, ` data-tooltip="`) {
		return "", nil, nil, fmt.Errorf("chart tooltip attributes are missing or malformed")
	}
	seen := make(map[string]string, len(matches))
	svgIDs := make(map[int]string)
	tooltips := make([]copilotMetricsHTMLTooltip, 0, len(matches))
	fallbacks := make([]copilotMetricsHTMLTooltipFallback, 0, len(matches))
	insertions := make([]copilotMetricsHTMLInsertion, 0, len(matches))
	for _, match := range matches {
		text, key := html.UnescapeString(source[match[2]:match[3]]), source[match[4]:match[5]]
		if copilotMetricsTooltipKey(text) != key {
			return "", nil, nil, fmt.Errorf("chart tooltip key does not match its text")
		}
		target := fmt.Sprintf("t%d", *nextTarget)
		*nextTarget++
		fallback := copilotMetricsHTMLTooltipFallback{Target: target, Text: text}
		tagEnd := strings.IndexByte(source[match[1]:], '>')
		if tagEnd < 0 {
			return "", nil, nil, fmt.Errorf("chart tooltip target is not closed")
		}
		if box := copilotMetricsTooltipBoxAttribute.FindStringSubmatch(source[match[1] : match[1]+tagEnd]); box != nil {
			svgStart := strings.LastIndex(source[:match[0]], "<svg ")
			if svgStart < 0 || strings.Contains(source[svgStart:match[0]], "</svg>") {
				return "", nil, nil, fmt.Errorf("chart tooltip box is outside an SVG")
			}
			viewBox := copilotMetricsSVGViewBox.FindStringSubmatch(source[svgStart:])
			if viewBox == nil {
				return "", nil, nil, fmt.Errorf("chart SVG has no viewBox")
			}
			var values [6]float64
			for i, value := range append(box[1:], viewBox[1:]...) {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return "", nil, nil, fmt.Errorf("parse chart tooltip box: %w", err)
				}
				values[i] = parsed
			}
			if values[4] <= 0 || values[5] <= 0 {
				return "", nil, nil, fmt.Errorf("chart SVG has an empty viewBox")
			}
			id, ok := svgIDs[svgStart]
			if !ok {
				id = target
				svgIDs[svgStart] = id
				svgTagEnd := strings.IndexByte(source[svgStart:], '>')
				if source[svgStart+svgTagEnd-1] == '/' {
					svgTagEnd--
				}
				insertions = append(insertions, copilotMetricsHTMLInsertion{svgStart + svgTagEnd, fmt.Sprintf(` data-tooltip-svg="%s"`, id)})
			}
			fallback.SVG = id
			fallback.Box = [4]float64{values[0] / values[4], values[1] / values[5], values[2] / values[4], values[3] / values[5]}
		}
		insertions = append(insertions, copilotMetricsHTMLInsertion{match[1], fmt.Sprintf(` data-tooltip-target="%s"`, target)})
		fallbacks = append(fallbacks, fallback)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = text
		tooltips = append(tooltips, copilotMetricsHTMLTooltip{Key: key, Text: text})
	}
	sort.SliceStable(insertions, func(i, j int) bool { return insertions[i].position < insertions[j].position })
	var b strings.Builder
	last := 0
	for _, insertion := range insertions {
		b.WriteString(source[last:insertion.position])
		b.WriteString(insertion.text)
		last = insertion.position
	}
	b.WriteString(source[last:])
	// #nosec G203 -- the chart is already-rendered HTML; only generated IDs are inserted.
	return template.HTML(b.String()), tooltips, fallbacks, nil
}

// copilotMetricsTooltipFallbackCSS shows each target's popup on focus-visible,
// hover (of the target or the popup), then plain focus, and anchors the popup
// next to its own target where CSS anchor positioning is supported.
func copilotMetricsTooltipFallbackCSS(fallbacks []copilotMetricsHTMLTooltipFallback) template.CSS {
	var b strings.Builder
	for _, fallback := range fallbacks {
		target := fallback.Target
		fmt.Fprintf(&b, `html:not(.chart-tooltip-active):not(.chart-tooltip-dismissed) figure:is(`+
			`:has([data-tooltip-target="%s"]:not([data-tooltip-dismissed]):focus-visible), `+
			`:not(:has([data-tooltip]:focus-visible)):has([data-tooltip-target="%s"]:not([data-tooltip-dismissed]):hover, > .chart-tooltip-fallback[data-tooltip-target="%s"]:hover), `+
			`:not(:has([data-tooltip]:focus-visible, [data-tooltip]:hover, > .chart-tooltip-fallback:hover)):has([data-tooltip-target="%s"]:not([data-tooltip-dismissed]):focus)`+
			`) > .chart-tooltip-fallback[data-tooltip-target="%s"] { display: block; }`+"\n", target, target, target, target, target)
	}
	b.WriteString(copilotMetricsTooltipAnchorSupports + " {\n")
	for _, fallback := range fallbacks {
		target := fallback.Target
		if fallback.SVG == "" {
			fmt.Fprintf(&b, `[data-tooltip-target="%s"] { anchor-name: --chart-tip-%s; }`+"\n", target, target)
		} else {
			if fallback.SVG == target {
				fmt.Fprintf(&b, `[data-tooltip-svg="%s"] { anchor-name: --chart-svg-%s; }`+"\n", target, target)
			}
			fmt.Fprintf(&b, `.chart-tooltip-anchor[data-tooltip-anchor="%s"] { position-anchor: --chart-svg-%s; left: calc(anchor(left) + anchor-size(width) * %.5f); top: calc(anchor(top) + anchor-size(height) * %.5f); width: calc(anchor-size(width) * %.5f); height: calc(anchor-size(height) * %.5f); anchor-name: --chart-tip-%s; }`+"\n",
				target, fallback.SVG, fallback.Box[0], fallback.Box[1], fallback.Box[2], fallback.Box[3], target)
		}
		fmt.Fprintf(&b, `.chart-tooltip-fallback[data-tooltip-target="%s"] { position-anchor: --chart-tip-%s; }`+"\n", target, target)
	}
	b.WriteString("}\n")
	// #nosec G203 -- target IDs and geometry are generated internally, not report text.
	return template.CSS(b.String())
}

const copilotMetricsTooltipAnchorSupports = "@supports (anchor-name: --a) and (width: anchor-size(width))"

func copilotMetricsWriteDayTarget(b *strings.Builder, hitClass, dotClass string, x, y, width, height, radius float64, tooltip copilotMetricsTooltip, dots []copilotMetricsDayDot) {
	label := template.HTMLEscapeString(tooltip.text())
	fmt.Fprintf(b, `<g class="chart-day"><rect class="%s" x="%.3f" y="%.3f" width="%.3f" height="%.3f" %s tabindex="0" role="img" aria-label="%s"></rect>`,
		hitClass, x, y, width, height, tooltip.svgAttributes(x, y, width, height), label)
	for _, dot := range dots {
		fmt.Fprintf(b, `<circle class="%s"`, dotClass)
		if dot.Color != "" {
			fmt.Fprintf(b, ` fill="%s"`, dot.Color)
		}
		fmt.Fprintf(b, ` cx="%.1f" cy="%.1f" r="%g" aria-hidden="true"/>`, dot.X, dot.Y, radius)
	}
	b.WriteString(`</g>`)
}

// copilotMetricsSVGChart renders a self-contained SVG line, area, or bar chart.
// Only numbers and HTML-escaped labels are interpolated, so the result is safe to embed.
func copilotMetricsSVGChart(title, kind string, labels []string, values []int64, valid []bool) template.HTML {
	n := len(values)
	if n == 0 {
		return ""
	}
	var max int64
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		max = 1
	}
	plotW := float64(copilotMetricsChartWidth - copilotMetricsChartPadLeft - copilotMetricsChartPadRgt)
	height := copilotMetricsChartHeight
	if kind != "bar" {
		height = 170
	}
	plotH := float64(height - copilotMetricsChartPadTop - copilotMetricsChartPadBot)
	baseY := float64(copilotMetricsChartPadTop) + plotH
	step := plotW / float64(n)
	x := func(i int) float64 { return float64(copilotMetricsChartPadLeft) + step*(float64(i)+0.5) }
	y := func(v int64) float64 { return baseY - plotH*float64(v)/float64(max) }

	var b strings.Builder
	chartClass, role := "chart", "img"
	if kind != "bar" {
		chartClass += " chart-line"
		role = "group"
	}
	fmt.Fprintf(&b, `<svg class="%s" viewBox="0 0 %d %d" role="%s" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		chartClass, copilotMetricsChartWidth, height, role, template.HTMLEscapeString(title))
	for _, frac := range []float64{0, 0.5, 1} {
		gy := baseY - plotH*frac
		fmt.Fprintf(&b, `<line class="grid" x1="%d" y1="%.1f" x2="%d" y2="%.1f"/>`,
			copilotMetricsChartPadLeft, gy, copilotMetricsChartWidth-copilotMetricsChartPadRgt, gy)
		fmt.Fprintf(&b, `<text class="axis" x="%d" y="%.1f" text-anchor="end">%s</text>`,
			copilotMetricsChartPadLeft-6, gy+4, formatCopilotMetricsInt(int64(float64(max)*frac)))
	}
	labelEvery := 1
	if n > 14 {
		labelEvery = (n + 13) / 14
	}
	for i, l := range labels {
		if i%labelEvery != 0 && i != n-1 {
			continue
		}
		fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%d" text-anchor="middle">%s</text>`,
			x(i), height-copilotMetricsChartPadBot+16, template.HTMLEscapeString(l))
	}
	switch kind {
	case "bar":
		bw := step * 0.7
		for i, v := range values {
			if !valid[i] {
				continue
			}
			tooltip := copilotMetricsTooltip{Rows: []copilotMetricsTooltipRow{{Label: labels[i], Value: formatCopilotMetricsInt(v)}}}
			label := template.HTMLEscapeString(tooltip.text())
			fmt.Fprintf(&b, `<g class="chart-day"><rect class="bar" x="%.1f" y="%.1f" width="%.1f" height="%.1f" aria-hidden="true"/>`,
				x(i)-bw/2, y(v), bw, baseY-y(v))
			fmt.Fprintf(&b, `<rect class="chart-day-hit" x="%.3f" y="%.3f" width="%.3f" height="%.3f" %s tabindex="0" role="img" aria-label="%s"></rect></g>`,
				x(i)-step/2, float64(copilotMetricsChartPadTop)-3, step, plotH+6, tooltip.svgAttributes(x(i)-step/2, float64(copilotMetricsChartPadTop)-3, step, plotH+6), label)
		}
	default:
		var path, fill strings.Builder
		pen := false
		runStart := 0
		closeRun := func(end int) {
			if kind == "area" && pen {
				fmt.Fprintf(&fill, "L%.1f %.1f L%.1f %.1f Z ", x(end), baseY, x(runStart), baseY)
			}
		}
		for i, v := range values {
			if !valid[i] {
				closeRun(i - 1)
				pen = false
				continue
			}
			cmd := "L"
			if !pen {
				cmd = "M"
				runStart = i
			}
			fmt.Fprintf(&path, "%s%.1f %.1f ", cmd, x(i), y(v))
			fmt.Fprintf(&fill, "%s%.1f %.1f ", cmd, x(i), y(v))
			pen = true
		}
		closeRun(n - 1)
		if kind == "area" {
			// Filled region under each contiguous run of valid points; gaps stay blank.
			fmt.Fprintf(&b, `<path class="line-area" d="%s"/>`, strings.TrimSpace(fill.String()))
		}
		fmt.Fprintf(&b, `<path class="line" d="%s"/>`, strings.TrimSpace(path.String()))
		for i, v := range values {
			if !valid[i] {
				continue
			}
			startX := float64(copilotMetricsChartPadLeft) + step*float64(i)
			endX := float64(copilotMetricsChartPadLeft) + step*float64(i+1)
			if i == 0 {
				startX -= 3
			}
			if i == n-1 {
				endX += 3
			}
			tooltip := copilotMetricsTooltip{Rows: []copilotMetricsTooltipRow{{Label: labels[i], Value: formatCopilotMetricsInt(v)}}}
			copilotMetricsWriteDayTarget(&b, "chart-day-hit", "dot", startX, float64(copilotMetricsChartPadTop)-3, endX-startX, plotH+6, 3,
				tooltip, []copilotMetricsDayDot{{X: x(i), Y: y(v)}})
		}
	}
	b.WriteString(`</svg>`)
	// #nosec G203 -- only numbers and escaped labels are interpolated above
	return template.HTML(b.String())
}

func copilotMetricsTrendChart(trend []copilotmetrics.TrendPoint, title, kind string, get func(copilotmetrics.TrendPoint) copilotmetrics.Counter) template.HTML {
	labels := make([]string, len(trend))
	values := make([]int64, len(trend))
	valid := make([]bool, len(trend))
	for i, p := range trend {
		labels[i] = p.Day
		if len(labels[i]) == len("2006-01-02") {
			labels[i] = labels[i][5:]
		}
		counter := get(p)
		values[i] = counter.Value
		valid[i] = counter.Valid
	}
	return copilotMetricsSVGChart(title, kind, labels, values, valid)
}

type copilotMetricsHTMLTable struct {
	Headers      []string
	Rows         [][]string
	NumericClass []string
	RowColors    []copilotMetricsHTMLRowColors
}

type copilotMetricsHTMLRowColors struct {
	Group  string
	Series string
}

type copilotMetricsHTMLDetail struct {
	Title       string
	Description string
	Table       copilotMetricsHTMLTable
}

type copilotMetricsHTMLSection struct {
	ID          copilotmetrics.Section
	Title       string
	Description string
	Table       copilotMetricsHTMLTable
	Empty       bool
	OmittedText string
	Charts      []copilotMetricsHTMLPanel
	RichPanels  []copilotMetricsHTMLPanel
	Details     []copilotMetricsHTMLDetail
}

type copilotMetricsHTMLPanel struct {
	Title     string
	Chart     template.HTML
	HalfWidth bool
	Tooltips  []copilotMetricsHTMLTooltip
	Fallbacks []copilotMetricsHTMLTooltipFallback
}

type copilotMetricsHTMLKPI struct {
	Label  string
	Value  string
	Window string
}

type copilotMetricsHTMLData struct {
	Title      string
	Period     string
	Generated  string
	Rich       bool
	Highlights []copilotMetricsHTMLKPI
	Partial    []string
	Warnings   []string
	Sections   []copilotMetricsHTMLSection
	TooltipCSS template.CSS
}

func copilotMetricsHTMLBreakdownTable(keyHeader string, rows []copilotmetrics.BreakdownRow) copilotMetricsHTMLTable {
	withInteractions := false
	for _, row := range rows {
		if row.UserInitiatedInteraction.Valid {
			withInteractions = true
			break
		}
	}
	table := copilotMetricsHTMLTable{Headers: []string{keyHeader}}
	if withInteractions {
		table.Headers = append(table.Headers, "Interactions")
	}
	table.Headers = append(table.Headers, "Code generation", "Code acceptance", "Acceptance rate", "LOC added", "LOC deleted")
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

func copilotMetricsHTMLSections(s *copilotmetrics.Summary, rich bool) []copilotMetricsHTMLSection {
	var sections []copilotMetricsHTMLSection
	for _, id := range s.Sections.Ordered() {
		section := copilotMetricsHTMLSection{ID: id}
		colors := copilotMetricsColorsForSection(s, id)
		switch id {
		case copilotmetrics.SectionKPI:
			section.Title = "Key metrics"
			section.Description = "Active-user metrics are snapshots and are never summed across days: daily active users are averaged and peaked, and weekly/monthly active users are taken from the last day with data. Counters are summed over the window and rates are computed from the summed values."
			section.Table.Headers = []string{"Metric", "Value", "Window"}
			for _, kpi := range s.KPIs {
				section.Table.Rows = append(section.Table.Rows, []string{kpi.Label, formatCopilotMetricsKPI(kpi), kpi.Window})
			}
			section.Empty = len(s.KPIs) == 0
		case copilotmetrics.SectionTrend:
			section.Title = "Daily trend"
			section.Description = "Per-day values from the organization-level report. Days without a report are shown as n/a."
			section.Table.Headers = []string{"Day", "Daily active users", "Interactions", "Code generation", "Code acceptance", "LOC added"}
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
			section.Table = copilotMetricsHTMLBreakdownTable("Feature", s.Features)
			section.Empty = len(s.Features) == 0
		case copilotmetrics.SectionLanguage:
			section.Title = "By language"
			section.Description = "Color keys identify languages. Blue numeric shading indicates relative values within each column."
			section.Table = copilotMetricsHTMLBreakdownTable("Language", s.Languages)
			section.Empty = len(s.Languages) == 0
		case copilotmetrics.SectionModel:
			section.Title = "By model"
			section.Description = "Color keys identify models. Blue numeric shading indicates relative values within each column."
			section.Table = copilotMetricsHTMLBreakdownTable("Model", s.Models)
			section.Empty = len(s.Models) == 0
		case copilotmetrics.SectionIDE:
			section.Title = "By IDE"
			section.Table = copilotMetricsHTMLBreakdownTable("IDE", s.IDEs)
			section.Empty = len(s.IDEs) == 0
		case copilotmetrics.SectionRepository:
			section.Title = "By repository"
			section.Description = "Pull request activity summed over the 1-day repository reports in the window. Review counts can include the same pull request on multiple days. Charts show the top repositories for each metric plus the remaining repositories."
			section.Table.Headers = []string{"Repository", "Visibility", "Days", "PRs created", "Created by Copilot", "PRs reviewed", "Reviewed by Copilot", "PRs merged", "Merged (Copilot-created)", "Copilot suggestions", "Applied suggestions"}
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
			section.Table.Headers = []string{"Team", "Members seen", "Active users", "Interactions", "Code generation", "Code acceptance", "Acceptance rate", "LOC added", "AI credits"}
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
			section.Table.Headers = []string{"#", "User", "Active days", "Interactions", "Code generation", "Code acceptance", "Acceptance rate", "LOC added", "AI credits", "Surfaces"}
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
		if omitted := s.Omitted[id]; omitted > 0 {
			section.OmittedText = fmt.Sprintf("%d more row(s) omitted by --top.", omitted)
		}
		if colors != nil {
			for _, row := range section.Table.Rows {
				section.Table.RowColors = append(section.Table.RowColors, copilotMetricsHTMLRowColors{Group: colors.group(row[0])})
			}
		}
		if detail := copilotMetricsDetailForSection(s, id); detail != nil {
			table := copilotMetricsHTMLTable{
				Headers: detail.Headers, Rows: detail.Rows,
				NumericClass: []string{"", "", "num heatmap", "num heatmap"},
			}
			for _, dimension := range detail.Dimensions {
				table.RowColors = append(table.RowColors, copilotMetricsHTMLRowColors{
					Group: colors.group(dimension.Group), Series: colors.seriesColor(dimension.Series),
				})
			}
			section.Details = append(section.Details, copilotMetricsHTMLDetail{
				Title:       detail.Title,
				Description: detail.Description + " Color keys match the section chart colors; chart Other can combine several rows. Blue numeric shading is relative within each column.",
				Table:       table,
			})
		}
		if id == copilotmetrics.SectionTrend && !section.Empty {
			if !rich || s.Rich == nil {
				section.Charts = append(section.Charts, copilotMetricsHTMLPanel{
					Title: "Daily active users",
					Chart: copilotMetricsTrendChart(s.Trend, "Daily active users", "line", func(p copilotmetrics.TrendPoint) copilotmetrics.Counter { return p.DailyActiveUsers }),
				})
			}
			section.Charts = append(section.Charts,
				copilotMetricsHTMLPanel{
					Title: "User-initiated interactions",
					Chart: copilotMetricsTrendChart(s.Trend, "User-initiated interactions", "area", func(p copilotmetrics.TrendPoint) copilotmetrics.Counter { return p.UserInitiatedInteraction }),
				},
			)
		}
		if rich && s.Rich != nil {
			section.RichPanels = copilotMetricsRichPanels(s.Rich, id, colors)
		}
		if rich && id == copilotmetrics.SectionRepository {
			section.RichPanels = append(section.RichPanels, copilotMetricsRepositoryPanels(s.AllRepositories, s.Rich)...)
			if panel := copilotMetricsRepositoryDailyPanel(s.Rich); panel.Chart != "" {
				section.RichPanels = append(section.RichPanels, panel)
			}
		}
		if rich && id == copilotmetrics.SectionTeam {
			teams := s.AllTeams
			if teams == nil {
				teams = s.Teams
			}
			section.RichPanels = append(section.RichPanels, copilotMetricsTeamPanels(teams)...)
		}
		if rich && id == copilotmetrics.SectionUser {
			users := s.AllUsers
			if users == nil {
				users = s.Users
			}
			section.RichPanels = append(section.RichPanels, copilotMetricsUserPanels(users)...)
		}
		sections = append(sections, section)
	}
	return sections
}

func copilotMetricsHTMLHighlights(s *copilotmetrics.Summary) []copilotMetricsHTMLKPI {
	if s.Rich != nil {
		var highlights []copilotMetricsHTMLKPI
		if month := s.Rich.IDEActiveUserMonth; month != nil {
			window := fmt.Sprintf("calendar month: %s .. %s; reported IDEs only", month.StartDay, month.EndDay)
			if len(month.MissingDays) > 0 {
				window += "; partial"
			}
			highlights = append(highlights, copilotMetricsHTMLKPI{
				Label: "IDE active users (estimate)", Value: formatCopilotMetricsCounter(month.Users), Window: window,
			})
		}
		agentUsers, totalUsers, snapshotDay := copilotMetricsLastMatchingSnapshots(s.Rich.MonthlyActiveAgentUsers, s.Rich.MonthlyActiveUsers, s.Rich.Days)
		if agentUsers.Valid && totalUsers.Valid && totalUsers.Value > 0 {
			highlights = append(highlights, copilotMetricsHTMLKPI{
				Label:  "Agent adoption",
				Value:  formatCopilotMetricsRate(float64(agentUsers.Value)/float64(totalUsers.Value), true),
				Window: fmt.Sprintf("%s of %s monthly active users on %s", formatCopilotMetricsInt(agentUsers.Value), formatCopilotMetricsInt(totalUsers.Value), snapshotDay),
			})
		}
		if len(s.Rich.ModelRequests) > 0 && s.Rich.ModelRequests[0].Value.Valid {
			highlights = append(highlights, copilotMetricsHTMLKPI{
				Label: "Most used chat model", Value: s.Rich.ModelRequests[0].Name,
				Window: formatCopilotMetricsCounter(s.Rich.ModelRequests[0].Value) + " interactions",
			})
		}
		var total float64
		count := 0
		for _, value := range s.Rich.AverageChatRequestsPerActiveUser {
			if value.Valid {
				total += value.Value
				count++
			}
		}
		if count > 0 {
			highlights = append(highlights, copilotMetricsHTMLKPI{
				Label: "Average chat requests per active user",
				Value: fmt.Sprintf("%.1f", total/float64(count)), Window: "average of available daily values",
			})
		}
		return highlights
	}
	wanted := []string{
		"Daily active users (average)",
		"Monthly active users",
		"User-initiated interactions",
		"Code acceptance rate",
	}
	byLabel := make(map[string]copilotmetrics.KPI, len(s.KPIs))
	for _, kpi := range s.KPIs {
		byLabel[kpi.Label] = kpi
	}
	var highlights []copilotMetricsHTMLKPI
	for _, label := range wanted {
		if kpi, ok := byLabel[label]; ok {
			highlights = append(highlights, copilotMetricsHTMLKPI{
				Label: label, Value: formatCopilotMetricsKPI(kpi), Window: kpi.Window,
			})
		}
	}
	return highlights
}

func copilotMetricsLastMatchingSnapshots(left, right []copilotmetrics.Counter, days []string) (copilotmetrics.Counter, copilotmetrics.Counter, string) {
	for i := len(days) - 1; i >= 0; i-- {
		if i < len(left) && i < len(right) && left[i].Valid && right[i].Valid {
			return left[i], right[i], days[i]
		}
	}
	return copilotmetrics.Counter{}, copilotmetrics.Counter{}, ""
}

type copilotMetricsPlotSeries struct {
	Name   string
	Values []float64
	Valid  []bool
}

func copilotMetricsCounterPlotSeries(series []copilotmetrics.RichSeries) []copilotMetricsPlotSeries {
	out := make([]copilotMetricsPlotSeries, 0, len(series))
	for _, item := range series {
		plot := copilotMetricsPlotSeries{Name: item.Name, Values: make([]float64, len(item.Values)), Valid: make([]bool, len(item.Values))}
		for i, value := range item.Values {
			plot.Values[i] = float64(value.Value)
			plot.Valid[i] = value.Valid
		}
		out = append(out, plot)
	}
	return out
}

func copilotMetricsFloatPlotSeries(name string, values []copilotmetrics.FloatCounter) []copilotMetricsPlotSeries {
	plot := copilotMetricsPlotSeries{Name: name, Values: make([]float64, len(values)), Valid: make([]bool, len(values))}
	for i, value := range values {
		plot.Values[i] = value.Value
		plot.Valid[i] = value.Valid
	}
	return []copilotMetricsPlotSeries{plot}
}

func copilotMetricsLimitPlotSeries(series []copilotMetricsPlotSeries, limit int) []copilotMetricsPlotSeries {
	var regular []copilotMetricsPlotSeries
	var other copilotMetricsPlotSeries
	for _, item := range series {
		if copilotMetricsNormalizeOther(item.Name) != "Other" {
			regular = append(regular, item)
			continue
		}
		if other.Name == "" {
			other = copilotMetricsPlotSeries{Name: "Other", Values: make([]float64, len(item.Values)), Valid: make([]bool, len(item.Values))}
		}
		for i, value := range item.Values {
			if item.Valid[i] {
				other.Values[i] += value
				other.Valid[i] = true
			}
		}
	}
	series = regular
	sort.SliceStable(series, func(i, j int) bool {
		var left, right float64
		for k, value := range series[i].Values {
			if series[i].Valid[k] {
				left += value
			}
		}
		for k, value := range series[j].Values {
			if series[j].Valid[k] {
				right += value
			}
		}
		return left > right
	})
	if len(series) <= limit && (other.Name == "" || len(series) < limit) {
		if other.Name != "" {
			return append(series, other)
		}
		return series
	}
	limit--
	out := append([]copilotMetricsPlotSeries(nil), series[:limit]...)
	if other.Name == "" {
		other = copilotMetricsPlotSeries{Name: "Other", Values: make([]float64, len(series[0].Values)), Valid: make([]bool, len(series[0].Values))}
	}
	for _, item := range series[limit:] {
		for i, value := range item.Values {
			if item.Valid[i] {
				other.Values[i] += value
				other.Valid[i] = true
			}
		}
	}
	return append(out, other)
}

func copilotMetricsLimitRichValues(values []copilotmetrics.RichValue) []copilotmetrics.RichValue {
	series := make([]copilotMetricsPlotSeries, 0, len(values))
	counts := map[string]copilotmetrics.Counter{}
	for _, row := range values {
		if !row.Value.Valid || row.Value.Value < 0 {
			continue
		}
		name := row.Name
		if row.Group != "" {
			name = row.Group + " / " + name
		}
		name = copilotMetricsNormalizeOther(name)
		total := counts[name]
		total.AddCounter(row.Value)
		counts[name] = total
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		series = append(series, copilotMetricsPlotSeries{Name: name, Values: []float64{float64(counts[name].Value)}, Valid: []bool{true}})
	}
	series = copilotMetricsLimitPlotSeries(series, len(copilotMetricsChartColors))
	kept := map[string]bool{}
	for _, item := range series {
		kept[item.Name] = true
	}
	for name, count := range counts {
		if !kept[name] {
			other := counts["Other"]
			other.AddCounter(count)
			counts["Other"] = other
		}
	}
	out := make([]copilotmetrics.RichValue, 0, len(series))
	for _, item := range series {
		out = append(out, copilotmetrics.RichValue{Name: item.Name, Value: counts[item.Name]})
	}
	return out
}

func copilotMetricsSVGMultiLineChart(title string, days []string, series []copilotMetricsPlotSeries) template.HTML {
	return copilotMetricsSVGMultiLineChartWithPercentage(title, days, series, false)
}

func copilotMetricsDailyPercentageSeries(series []copilotMetricsPlotSeries, days int) []copilotMetricsPlotSeries {
	totals := make([]float64, days)
	for _, item := range series {
		for i, value := range item.Values {
			if i < days && i < len(item.Valid) && item.Valid[i] {
				totals[i] += value
			}
		}
	}
	out := make([]copilotMetricsPlotSeries, 0, len(series))
	for _, item := range series {
		result := copilotMetricsPlotSeries{Name: item.Name, Values: make([]float64, days), Valid: make([]bool, days)}
		for i, value := range item.Values {
			if i < days && i < len(item.Valid) && item.Valid[i] && totals[i] > 0 {
				result.Values[i] = value / totals[i] * 100
				result.Valid[i] = true
			}
		}
		out = append(out, result)
	}
	return out
}

type copilotMetricsAreaPoint struct {
	Lower float64
	Upper float64
	Valid bool
}

func copilotMetricsStackedAreaPoints(series []copilotMetricsPlotSeries, days int) [][]copilotMetricsAreaPoint {
	points := make([][]copilotMetricsAreaPoint, len(series))
	totals := make([]float64, days)
	for i, item := range series {
		points[i] = make([]copilotMetricsAreaPoint, days)
		for day := 0; day < days; day++ {
			point := copilotMetricsAreaPoint{Lower: totals[day], Upper: totals[day]}
			if day < len(item.Values) && day < len(item.Valid) && item.Valid[day] {
				point.Upper += item.Values[day]
				point.Valid = true
				totals[day] += item.Values[day]
			}
			points[i][day] = point
		}
	}
	return points
}

// Shared horizontal-tangent controls keep adjacent layers aligned without overshoot.
func copilotMetricsWriteAreaCurve(path *strings.Builder, x1, y1, x2, y2 float64) {
	fmt.Fprintf(path, "C%.1f %.1f %.1f %.1f %.1f %.1f ",
		(2*x1+x2)/3, y1, (x1+2*x2)/3, y2, x2, y2)
}

func copilotMetricsStackedAreaPath(points [][]copilotMetricsAreaPoint, index, start, end int, x func(int) float64, y func(float64) float64, left, right float64) string {
	series := points[index]
	startX, endX := x(start), x(end)
	if len(series) == 1 {
		startX, endX = left, right
	} else {
		if start > 0 {
			startX = (x(start-1) + x(start)) / 2
		}
		if end < len(series)-1 {
			endX = (x(end) + x(end+1)) / 2
		}
	}
	var path strings.Builder
	fmt.Fprintf(&path, "M%.1f %.1f L%.1f %.1f ", startX, y(series[start].Upper), x(start), y(series[start].Upper))
	for day := start; day < end; day++ {
		copilotMetricsWriteAreaCurve(&path, x(day), y(series[day].Upper), x(day+1), y(series[day+1].Upper))
	}
	fmt.Fprintf(&path, "L%.1f %.1f L%.1f %.1f L%.1f %.1f ", endX, y(series[end].Upper), endX, y(series[end].Lower), x(end), y(series[end].Lower))
	for day := end - 1; day >= start; day-- {
		copilotMetricsWriteAreaCurve(&path, x(day+1), y(series[day+1].Lower), x(day), y(series[day].Lower))
	}
	fmt.Fprintf(&path, "L%.1f %.1f Z", startX, y(series[start].Lower))
	return path.String()
}

func copilotMetricsSVGMultiLineChartWithPercentage(title string, days []string, series []copilotMetricsPlotSeries, percentage bool) template.HTML {
	return copilotMetricsSVGMultiLineChartWithColors(title, days, series, percentage, nil)
}

func copilotMetricsSVGMultiLineChartWithColors(title string, days []string, series []copilotMetricsPlotSeries, percentage bool, scheme *copilotMetricsColorScheme) template.HTML {
	return copilotMetricsSVGTimeSeriesChart(title, days, series, percentage, false, scheme)
}

func copilotMetricsSVGStackedAreaChart(title string, days []string, series []copilotMetricsPlotSeries, scheme *copilotMetricsColorScheme) template.HTML {
	return copilotMetricsSVGTimeSeriesChart(title, days, series, true, true, scheme)
}

func copilotMetricsSVGTimeSeriesChart(title string, days []string, series []copilotMetricsPlotSeries, percentage, stackedArea bool, scheme *copilotMetricsColorScheme) template.HTML {
	if len(days) == 0 || len(series) == 0 {
		return ""
	}
	const width, left, right, bottom = 720.0, 72.0, 35.0, 38.0
	legendRows := (len(series) + 2) / 3
	top := 36.0 + float64(legendRows-1)*14
	height := 205.0 + float64(legendRows-1)*14
	maxValue := 0.0
	hasData := false
	for _, item := range series {
		for i, value := range item.Values {
			if i < len(days) && i < len(item.Valid) && item.Valid[i] {
				hasData = true
				if value > maxValue {
					maxValue = value
				}
			}
		}
	}
	if !hasData {
		return ""
	}
	if maxValue <= 0 {
		maxValue = 1
	}
	suffix := ""
	if percentage {
		maxValue = 100
		suffix = "%"
	}
	plotWidth := width - left - right
	plotHeight := height - top - bottom
	x := func(index int) float64 {
		if len(days) == 1 {
			return left + plotWidth/2
		}
		return left + plotWidth*float64(index)/float64(len(days)-1)
	}
	y := func(value float64) float64 { return top + plotHeight*(1-value/maxValue) }
	colors := copilotMetricsChartColors
	dayTooltips := make([]copilotMetricsTooltip, len(days))
	for i, day := range days {
		tooltip := copilotMetricsTooltip{Heading: day}
		for _, item := range series {
			value := copilotMetricsNA
			if i < len(item.Values) && i < len(item.Valid) && item.Valid[i] {
				value = strconv.FormatFloat(item.Values[i], 'f', 1, 64)
				value += suffix
			}
			tooltip.Rows = append(tooltip.Rows, copilotMetricsTooltipRow{Label: item.Name, Value: value})
		}
		dayTooltips[i] = tooltip
	}
	var b strings.Builder
	chartClass := "metric-chart"
	if stackedArea {
		chartClass = "metric-chart metric-stacked-area"
	}
	fmt.Fprintf(&b, `<svg class="%s" viewBox="0 0 %.0f %.0f" role="group" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		chartClass, width, height, template.HTMLEscapeString(title))
	for _, fraction := range []float64{0, 0.5, 1} {
		gy := top + plotHeight*(1-fraction)
		fmt.Fprintf(&b, `<line class="grid" x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f"/>`, left, gy, width-right, gy)
		fmt.Fprintf(&b, `<text class="axis" x="%.0f" y="%.1f" text-anchor="end">%s</text>`, left-7, gy+4, template.HTMLEscapeString(strconv.FormatFloat(maxValue*fraction, 'f', 1, 64)+suffix))
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
	var areaColors map[string]copilotMetricsColor
	if stackedArea {
		areaColors = copilotMetricsAreaHueColors(series)
	}
	seriesColor := func(seriesIndex int, name string) string {
		if stackedArea {
			return areaColors[copilotMetricsNormalizeOther(name)].hex()
		}
		color := colors[seriesIndex%len(colors)].hex()
		if scheme != nil {
			color = scheme.group(name)
		}
		return color
	}
	var points [][]copilotMetricsAreaPoint
	if stackedArea {
		points = copilotMetricsStackedAreaPoints(series, len(days))
		dayHasValue := make([]bool, len(days))
		for _, item := range points {
			for day, point := range item {
				dayHasValue[day] = dayHasValue[day] || point.Valid
			}
		}
		for seriesIndex, item := range series {
			for start := 0; start < len(days); {
				if !dayHasValue[start] {
					start++
					continue
				}
				end := start
				hasSeries := points[seriesIndex][start].Valid
				for end+1 < len(days) && dayHasValue[end+1] {
					end++
					hasSeries = hasSeries || points[seriesIndex][end].Valid
				}
				if hasSeries {
					path := copilotMetricsStackedAreaPath(points, seriesIndex, start, end, x, y, left, width-right)
					fmt.Fprintf(&b, `<path class="metric-area" fill="%s" d="%s" aria-hidden="true"/>`,
						seriesColor(seriesIndex, item.Name), path)
				}
				start = end + 1
			}
		}
	} else {
		for seriesIndex, item := range series {
			color := seriesColor(seriesIndex, item.Name)
			var path strings.Builder
			pen := false
			for i, value := range item.Values {
				if i >= len(days) || i >= len(item.Valid) || !item.Valid[i] {
					pen = false
					continue
				}
				command := "L"
				if !pen {
					command = "M"
				}
				fmt.Fprintf(&path, "%s%.1f %.1f ", command, x(i), y(value))
				pen = true
			}
			fmt.Fprintf(&b, `<path class="metric-line" stroke="%s" d="%s"/>`, color, strings.TrimSpace(path.String()))
		}
	}
	for day := range days {
		var dots []copilotMetricsDayDot
		for seriesIndex, item := range series {
			if day >= len(item.Values) || day >= len(item.Valid) || !item.Valid[day] {
				continue
			}
			value := item.Values[day]
			if stackedArea {
				value = points[seriesIndex][day].Upper
			}
			dots = append(dots, copilotMetricsDayDot{X: x(day), Y: y(value), Color: seriesColor(seriesIndex, item.Name)})
		}
		if len(dots) == 0 {
			continue
		}
		startX, endX := left-3, width-right+3
		if day > 0 {
			startX = (x(day-1) + x(day)) / 2
		}
		if day < len(days)-1 {
			endX = (x(day) + x(day+1)) / 2
		}
		copilotMetricsWriteDayTarget(&b, "metric-day-hit", "metric-dot", startX, top-3, endX-startX, plotHeight+6, 2.5, dayTooltips[day], dots)
	}
	for seriesIndex, item := range series {
		color := seriesColor(seriesIndex, item.Name)
		legendX := left + float64(seriesIndex%3)*215
		legendY := 15 + float64(seriesIndex/3)*14
		if stackedArea {
			fmt.Fprintf(&b, `<rect class="area-legend-key" x="%.1f" y="%.1f" width="14" height="6" fill="%s" aria-hidden="true"/>`,
				legendX, legendY-7, color)
		} else {
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="2"/>`,
				legendX, legendY-3, legendX+14, legendY-3, color)
		}
		fmt.Fprintf(&b, `<text class="legend" x="%.1f" y="%.1f">%s</text>`,
			legendX+19, legendY, template.HTMLEscapeString(item.Name))
	}
	b.WriteString(`</svg>`)
	// #nosec G203 -- all labels are HTML-escaped; plotted coordinates and colors are generated internally.
	return template.HTML(b.String())
}

func copilotMetricsRichPanels(rich *copilotmetrics.RichMetrics, section copilotmetrics.Section, scheme *copilotMetricsColorScheme) []copilotMetricsHTMLPanel {
	var panels []copilotMetricsHTMLPanel
	add := func(title string, series []copilotMetricsPlotSeries) {
		if chart := copilotMetricsSVGMultiLineChart(title, rich.Days, series); chart != "" {
			panels = append(panels, copilotMetricsHTMLPanel{Title: title, Chart: chart})
		}
	}
	addPair := func(firstTitle string, firstSeries []copilotMetricsPlotSeries, secondTitle string, secondSeries []copilotMetricsPlotSeries) {
		start := len(panels)
		add(firstTitle, firstSeries)
		add(secondTitle, secondSeries)
		if len(panels)-start == 2 {
			panels[start].HalfWidth = true
			panels[start+1].HalfWidth = true
		}
	}
	switch section {
	case copilotmetrics.SectionTrend:
		add("Daily and weekly active users", copilotMetricsCounterPlotSeries([]copilotmetrics.RichSeries{
			{Name: "Daily active users", Values: rich.DailyActiveUsers},
			{Name: "Weekly active users", Values: rich.WeeklyActiveUsers},
		}))
		add("Average chat requests per active user", copilotMetricsFloatPlotSeries("Requests per active user", rich.AverageChatRequestsPerActiveUser))
		chatModes := copilotMetricsLimitPlotSeries(copilotMetricsCounterPlotSeries(rich.ChatModeRequests), len(copilotMetricsChartColors))
		chatChart := copilotMetricsDailyStackedBarChart(copilotMetricsChatModeRequestsTitle, rich.Days, chatModes, copilotMetricsThemeGreen,
			copilotMetricsDailyStackedBarOptions{
				incompleteNote:    "Some chat mode counts are unavailable; the total and shares are not calculated.",
				incompleteCaption: "* marks incomplete days; bars show known counts only.",
				markerBelowAxis:   true,
			})
		if chatChart != "" {
			panels = append(panels, copilotMetricsHTMLPanel{Title: copilotMetricsChatModeRequestsTitle, Chart: chatChart})
		}
		addPair("Code generation and acceptance activities",
			copilotMetricsCounterPlotSeries([]copilotmetrics.RichSeries{
				{Name: "Code generation activities", Values: rich.CodeGeneration},
				{Name: "Code acceptance activities", Values: rich.CodeAcceptance},
			}),
			"Code acceptance rate", copilotMetricsFloatPlotSeries("Acceptance rate", rich.AcceptanceRate))
		completionRates := copilotMetricsFloatPlotSeries("Acceptance rate (%)", rich.CodeCompletionsAcceptanceRate)
		for i := range completionRates[0].Values {
			completionRates[0].Values[i] *= 100
		}
		addPair("Code completions", copilotMetricsCounterPlotSeries([]copilotmetrics.RichSeries{
			{Name: "Suggested", Values: rich.CodeCompletionsSuggested},
			{Name: "Accepted", Values: rich.CodeCompletionsAccepted},
		}), "Code completions acceptance rate", completionRates)
	case copilotmetrics.SectionIDE:
		var daily, weekly []copilotmetrics.RichSeries
		for _, ide := range rich.IDEActiveUsers {
			daily = append(daily, copilotmetrics.RichSeries{Name: ide.IDE, Values: ide.Daily})
			weekly = append(weekly, copilotmetrics.RichSeries{Name: ide.IDE, Values: ide.Weekly})
		}
		add("IDE daily active users", copilotMetricsCounterPlotSeries(daily))
		add("IDE weekly active users", copilotMetricsCounterPlotSeries(weekly))
		add("Total IDE active users", copilotMetricsCounterPlotSeries([]copilotmetrics.RichSeries{
			{Name: "Daily active users", Values: rich.IDEActiveUserTotalDaily},
			{Name: "Weekly active users", Values: rich.IDEActiveUserTotalWeekly},
		}))
	case copilotmetrics.SectionModel:
		title := "Model usage per day (interactions)"
		modelSeries := copilotMetricsLimitPlotSeries(copilotMetricsCounterPlotSeries(rich.ModelRequestsByDay), len(copilotMetricsChartColors))
		modelSeries = copilotMetricsDailyPercentageSeries(modelSeries, len(rich.Days))
		if chart := copilotMetricsSVGStackedAreaChart(title, rich.Days, modelSeries, scheme); chart != "" {
			panels = append(panels, copilotMetricsHTMLPanel{Title: title, Chart: chart})
		}
		pie := copilotMetricsHTMLRichPiePanelWithColors("Chat model usage (interactions)", rich.ModelRequests, scheme)
		stacked := copilotMetricsHTMLRichStackedPanelWithColors(copilotMetricsModelChatModeTitle, copilotMetricsTopGroups(copilotMetricsChatModeModelValues(rich.ModelRequestsByFeature)), "Model", scheme)
		pie.HalfWidth = true
		stacked.HalfWidth = true
		panels = append(panels, pie, stacked)
	case copilotmetrics.SectionLanguage:
		languageSeries := copilotMetricsLimitPlotSeries(copilotMetricsCounterPlotSeries(rich.LanguageCodeGenerationByDay), len(copilotMetricsChartColors))
		languageSeries = copilotMetricsDailyPercentageSeries(languageSeries, len(rich.Days))
		title := "Language usage per day (code-generation activities)"
		if chart := copilotMetricsSVGStackedAreaChart(title, rich.Days, languageSeries, scheme); chart != "" {
			panels = append(panels, copilotMetricsHTMLPanel{Title: title, Chart: chart})
		}
		pie := copilotMetricsHTMLRichPiePanelWithColors("Language usage (code-generation activities)", rich.LanguageCodeGeneration, scheme)
		stacked := copilotMetricsHTMLRichStackedPanelWithColors(copilotMetricsModelLanguageTitle, copilotMetricsTopLanguages(rich.ModelCodeGenerationByLanguage), "Language", scheme)
		pie.HalfWidth = true
		stacked.HalfWidth = true
		panels = append(panels, pie, stacked)
	}
	return panels
}

func copilotMetricsNormalizeOther(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "other", "others", "unknown", "unkown":
		return "Other"
	default:
		return name
	}
}

func copilotMetricsHTMLRichStackedBarPanel(title string, values []copilotmetrics.RichValue) copilotMetricsHTMLPanel {
	return copilotMetricsHTMLRichStackedPanel(title, values, "Language")
}

func copilotMetricsTopLanguages(values []copilotmetrics.RichValue) []copilotmetrics.RichValue {
	return copilotMetricsTopGroups(values)
}

func copilotMetricsTopGroups(values []copilotmetrics.RichValue) []copilotmetrics.RichValue {
	totals := map[string]int64{}
	for _, row := range values {
		if row.Value.Valid && row.Value.Value >= 0 {
			name := copilotMetricsNormalizeOther(row.Group)
			if name != "Other" {
				totals[name] += row.Value.Value
			}
		}
	}
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if totals[names[i]] != totals[names[j]] {
			return totals[names[i]] > totals[names[j]]
		}
		return names[i] < names[j]
	})
	kept := map[string]bool{}
	for _, name := range names[:min(5, len(names))] {
		kept[name] = true
	}
	out := append([]copilotmetrics.RichValue(nil), values...)
	for i := range out {
		if !kept[copilotMetricsNormalizeOther(out[i].Group)] {
			out[i].Group = "Other"
		}
	}
	return out
}

func copilotMetricsStackedSeriesLabel(dimension string) string {
	if dimension == "Model" {
		return "Chat mode"
	}
	return "Model"
}

func copilotMetricsStackedTooltip(dimension, group string, series []string, counts map[string]copilotmetrics.Counter, total int64, denominator float64) copilotMetricsTooltip {
	tooltip := copilotMetricsTooltip{
		Heading: dimension + ": " + group,
		Note:    "Percentages are shares of all activities.",
	}
	if dimension == "Model" {
		tooltip.Note = "Percentages are shares of all chat mode interactions."
	}
	share := func(value int64) string {
		if denominator == 0 {
			return copilotMetricsNA
		}
		return strconv.FormatFloat(float64(value)/denominator*100, 'f', 1, 64) + "%"
	}
	for _, name := range series {
		row := copilotMetricsTooltipRow{Label: name, Value: copilotMetricsNA}
		if counter, present := counts[name]; present && counter.Valid {
			row.Value = formatCopilotMetricsCounter(counter)
			row.Share = share(counter.Value)
		}
		tooltip.Rows = append(tooltip.Rows, row)
	}
	tooltip.Rows = append(tooltip.Rows, copilotMetricsTooltipRow{
		Label: dimension + " total", Value: formatCopilotMetricsInt(total), Share: share(total),
	})
	return tooltip
}

func copilotMetricsHTMLRichStackedPanel(title string, values []copilotmetrics.RichValue, dimension string) copilotMetricsHTMLPanel {
	return copilotMetricsHTMLRichStackedPanelWithColors(title, values, dimension, copilotMetricsColorsForDimensions(values, copilotMetricsThemeForDimension(dimension)))
}

func copilotMetricsHTMLRichStackedPanelWithColors(title string, values []copilotmetrics.RichValue, dimension string, scheme *copilotMetricsColorScheme) copilotMetricsHTMLPanel {
	byLanguage := map[string]map[string]copilotmetrics.Counter{}
	totals := map[string]int64{}
	modelSet := map[string]bool{}
	for _, row := range values {
		if !row.Value.Valid || row.Value.Value < 0 {
			continue
		}
		row.Group = copilotMetricsNormalizeOther(row.Group)
		row.Name = copilotMetricsNormalizeOther(row.Name)
		if byLanguage[row.Group] == nil {
			byLanguage[row.Group] = map[string]copilotmetrics.Counter{}
		}
		counter := byLanguage[row.Group][row.Name]
		counter.AddCounter(row.Value)
		byLanguage[row.Group][row.Name] = counter
		totals[row.Group] += row.Value.Value
		modelSet[row.Name] = true
	}
	if len(byLanguage) == 0 {
		return copilotMetricsHTMLPanel{}
	}
	languages := make([]string, 0, len(byLanguage))
	models := make([]string, 0, len(modelSet))
	var totalActivities float64
	for language, total := range totals {
		languages = append(languages, language)
		totalActivities += float64(total)
	}
	sort.Slice(languages, func(i, j int) bool {
		if (languages[i] == "Other") != (languages[j] == "Other") {
			return languages[j] == "Other"
		}
		if totals[languages[i]] != totals[languages[j]] {
			return totals[languages[i]] > totals[languages[j]]
		}
		return languages[i] < languages[j]
	})
	for model := range modelSet {
		models = append(models, model)
	}
	modelTotals := make([]copilotmetrics.RichValue, 0, len(models))
	for _, model := range models {
		var total copilotmetrics.Counter
		for _, counts := range byLanguage {
			total.AddCounter(counts[model])
		}
		modelTotals = append(modelTotals, copilotmetrics.RichValue{Name: model, Value: total})
	}
	kept := map[string]bool{}
	models = nil
	for _, row := range copilotMetricsLimitRichValues(modelTotals) {
		kept[row.Name] = true
		models = append(models, row.Name)
	}
	for _, counts := range byLanguage {
		for model, count := range counts {
			if !kept[model] {
				other := counts["Other"]
				other.AddCounter(count)
				counts["Other"] = other
				delete(counts, model)
			}
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if (models[i] == "Other") != (models[j] == "Other") {
			return models[j] == "Other"
		}
		return models[i] < models[j]
	})
	reportedTotal := totalActivities
	if totalActivities == 0 {
		totalActivities = 1
	}
	maxShare := 0.0
	for _, total := range totals {
		maxShare = math.Max(maxShare, float64(total)/totalActivities)
	}
	axisMax := math.Max(1, math.Ceil(maxShare*100)) / 100
	width := math.Max(720, float64(len(languages))*110+68)
	top := 28.0
	const left, right, plotHeight = 80.0, 16.0, 180.0
	step := (width - left - right) / float64(len(languages))
	bottom := 95.0
	for _, language := range languages {
		lines := math.Ceil(float64(len([]rune(language))) * 18 / step)
		bottom = math.Max(bottom, 55+lines*22)
	}
	height := top + plotHeight + bottom
	baseY := top + plotHeight
	var b strings.Builder
	fmt.Fprintf(&b, `<ul class="stacked-legend" aria-label="%s">`, template.HTMLEscapeString(title+" legend"))
	for _, model := range models {
		tooltip := copilotMetricsTooltip{Heading: copilotMetricsStackedSeriesLabel(dimension) + ": " + model}
		escaped := template.HTMLEscapeString(tooltip.text())
		fmt.Fprintf(&b, `<li><span class="series-key" style="background-color:%s" %s tabindex="0" role="img" aria-label="%s"></span><span class="stacked-legend-label">%s</span></li>`,
			scheme.seriesColor(model), tooltip.attributes(), escaped, template.HTMLEscapeString(model))
	}
	fmt.Fprintf(&b, `</ul><div class="stacked-chart-wrap"><svg class="stacked-chart" style="min-width:var(--stacked-min-width, %.0fpx)" viewBox="0 0 %.0f %.0f" role="group" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		width, width, height, template.HTMLEscapeString(title))
	for _, fraction := range []float64{0, .5, 1} {
		y := baseY - plotHeight*fraction
		fmt.Fprintf(&b, `<line class="grid" x1="%.0f" y1="%.1f" x2="%.1f" y2="%.1f"/><text class="axis" x="%.0f" y="%.1f" text-anchor="end">%s%%</text>`,
			left, y, width-right, y, left-7, y+4, strings.TrimSuffix(strconv.FormatFloat(fraction*axisMax*100, 'f', 1, 64), ".0"))
	}
	for i, language := range languages {
		x := left + step*(float64(i)+.5)
		var stacked int64
		tooltip := copilotMetricsStackedTooltip(dimension, language, models, byLanguage[language], totals[language], reportedTotal)
		tooltipText := template.HTMLEscapeString(tooltip.text())
		for _, model := range models {
			counter, present := byLanguage[language][model]
			if !present {
				continue
			}
			share := float64(counter.Value) / totalActivities
			barHeight := share / axisMax * plotHeight
			y := baseY - float64(stacked+counter.Value)/totalActivities/axisMax*plotHeight
			segmentLabel := copilotMetricsStackedSeriesLabel(dimension)
			segmentLabel = template.HTMLEscapeString(segmentLabel + ": " + model + "\n" + tooltip.text())
			fmt.Fprintf(&b, `<rect class="stacked-segment" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" %s tabindex="0" role="img" aria-label="%s"></rect>`,
				x-step*.3, y, step*.6, barHeight, scheme.seriesColor(model), tooltip.svgAttributes(x-step*.3, y, step*.6, barHeight), segmentLabel)
			stacked += counter.Value
		}
		fmt.Fprintf(&b, `<foreignObject x="%.1f" y="%.1f" width="%.1f" height="%.1f"><div xmlns="http://www.w3.org/1999/xhtml" class="stacked-axis-label" %s tabindex="0" role="img" aria-label="%s">%s</div></foreignObject>`,
			x-step/2, baseY+15, step, bottom-45, tooltip.svgAttributes(x-step/2, baseY+15, step, bottom-45), tooltipText, template.HTMLEscapeString(language))
	}
	fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.1f" text-anchor="middle">%s</text><text class="axis" transform="translate(20 %.1f) rotate(-90)" text-anchor="middle">%%</text></svg></div><p class="chart-color-key">Each %s keeps the same color in every bar.</p>`,
		width/2, height-5, template.HTMLEscapeString(dimension), top+plotHeight/2, template.HTMLEscapeString(strings.ToLower(copilotMetricsStackedSeriesLabel(dimension))))
	// #nosec G203 -- labels and tooltip attributes are escaped; geometry and colors are generated internally.
	return copilotMetricsHTMLPanel{Title: title, Chart: template.HTML(b.String())}
}

func copilotMetricsHTMLRichPiePanel(title string, values []copilotmetrics.RichValue) copilotMetricsHTMLPanel {
	return copilotMetricsHTMLRichPiePanelWithColors(title, values, nil)
}

func copilotMetricsHTMLRichPiePanelWithColors(title string, values []copilotmetrics.RichValue, scheme *copilotMetricsColorScheme) copilotMetricsHTMLPanel {
	var total float64
	for _, row := range values {
		if row.Value.Valid && row.Value.Value > 0 {
			total += float64(row.Value.Value)
		}
	}
	if total == 0 {
		return copilotMetricsHTMLPanel{}
	}
	rows := make([]copilotmetrics.RichValue, 0, len(values))
	var other copilotmetrics.Counter
	for _, row := range values {
		if !row.Value.Valid || row.Value.Value <= 0 {
			continue
		}
		if float64(row.Value.Value)/total < .01 || copilotMetricsNormalizeOther(row.Name) == "Other" {
			other.AddCounter(row.Value)
		} else {
			rows = append(rows, row)
		}
	}
	if other.Valid {
		rows = append(rows, copilotmetrics.RichValue{Name: "Other", Value: other})
	}
	rows = copilotMetricsLimitRichValues(rows)
	theme := copilotMetricsThemeBlue
	if scheme != nil {
		theme = scheme.theme
	}
	gradientNames := make([]string, 0, len(rows))
	for _, row := range rows {
		if copilotMetricsNormalizeOther(row.Name) != "Other" {
			gradientNames = append(gradientNames, row.Name)
		}
	}
	sort.Slice(gradientNames, func(i, j int) bool {
		left, right := copilotMetricsPieValue(rows, gradientNames[i]), copilotMetricsPieValue(rows, gradientNames[j])
		if left != right {
			return left > right
		}
		return gradientNames[i] < gradientNames[j]
	})
	pieColors := make(map[string]string, len(gradientNames))
	for i, name := range gradientNames {
		pieColors[name] = copilotMetricsThemeGradientColor(theme, i, len(gradientNames)).hex()
	}
	pieColors["Other"] = copilotMetricsOtherColor.hex()
	return copilotMetricsHTMLPanel{Title: title, Chart: copilotMetricsPieChart(title, rows, total, pieColors)}
}

// copilotMetricsPieChart renders positive rows in the given order as pie slices with a legend.
func copilotMetricsPieChart(title string, rows []copilotmetrics.RichValue, total float64, pieColors map[string]string) template.HTML {
	colors := copilotMetricsChartColors
	var b, legend strings.Builder
	fmt.Fprintf(&b, `<div class="pie-chart"><svg class="pie-plot" viewBox="0 0 320 320" role="img" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		template.HTMLEscapeString(title))
	angle := -math.Pi / 2
	for _, row := range rows {
		if !row.Value.Valid || row.Value.Value <= 0 {
			continue
		}
		share := float64(row.Value.Value) / total
		end := angle + share*2*math.Pi
		color := pieColors[copilotMetricsNormalizeOther(row.Name)]
		if color == "" {
			color = colors[0].hex()
		}
		label := template.HTMLEscapeString(row.Name)
		value := formatCopilotMetricsCounter(row.Value)
		tooltip := copilotMetricsTooltip{Rows: []copilotMetricsTooltipRow{{
			Label: row.Name, Value: value, Share: strconv.FormatFloat(share*100, 'f', 1, 64) + "%",
		}}}
		tooltipText := template.HTMLEscapeString(tooltip.text())
		if share == 1 {
			fmt.Fprintf(&b, `<circle class="pie-slice" cx="160" cy="160" r="145" fill="%s" %s tabindex="0" aria-label="%s"></circle>`,
				color, tooltip.svgAttributes(160-4, 160-4, 8, 8), tooltipText)
		} else {
			largeArc := 0
			if share > .5 {
				largeArc = 1
			}
			middle := (angle + end) / 2
			fmt.Fprintf(&b, `<path class="pie-slice" d="M160 160 L%.3f %.3f A145 145 0 %d 1 %.3f %.3f Z" fill="%s" %s tabindex="0" aria-label="%s"></path>`,
				160+145*math.Cos(angle), 160+145*math.Sin(angle), largeArc, 160+145*math.Cos(end), 160+145*math.Sin(end), color, tooltip.svgAttributes(156+87*math.Cos(middle), 156+87*math.Sin(middle), 8, 8), tooltipText)
		}
		fmt.Fprintf(&legend, `<li %s tabindex="0" aria-label="%s"><span class="pie-key" style="background:%s"></span><span>%s</span><span>%.1f%%</span></li>`,
			tooltip.attributes(), tooltipText, color, label, share*100)
		angle = end
	}
	b.WriteString(`</svg><ul class="pie-legend">`)
	b.WriteString(legend.String())
	b.WriteString(`</ul></div>`)
	// #nosec G203 -- labels are escaped; SVG coordinates, percentages, and colors are generated internally.
	return template.HTML(b.String())
}

func copilotMetricsPieValue(rows []copilotmetrics.RichValue, name string) int64 {
	for _, row := range rows {
		if row.Name == name {
			return row.Value.Value
		}
	}
	return 0
}

func copilotMetricsHTMLRichBarPanel(title string, values []copilotmetrics.RichValue) copilotMetricsHTMLPanel {
	if len(values) == 0 {
		return copilotMetricsHTMLPanel{}
	}
	rows := copilotMetricsLimitRichValues(values)
	var max int64
	for _, row := range rows {
		if row.Value.Value > max {
			max = row.Value.Value
		}
	}
	if max <= 0 {
		return copilotMetricsHTMLPanel{}
	}
	var b strings.Builder
	b.WriteString(`<div class="bar-chart" role="img" aria-label="`)
	b.WriteString(template.HTMLEscapeString(title))
	b.WriteString(`">`)
	for _, row := range rows {
		label := row.Name
		if row.Group != "" {
			label = row.Group + " / " + row.Name
		}
		width := float64(row.Value.Value) / float64(max) * 100
		value := formatCopilotMetricsCounter(row.Value)
		tooltip := copilotMetricsTooltip{Rows: []copilotMetricsTooltipRow{{Label: label, Value: value}}}
		tooltipText := template.HTMLEscapeString(tooltip.text())
		fmt.Fprintf(&b, `<div class="bar-row" %s tabindex="0" aria-label="%s"><span class="bar-label">%s</span><span class="bar-track"><span class="bar-fill" style="width:%.1f%%"></span></span><span class="bar-value">%s</span></div>`,
			tooltip.attributes(), tooltipText, template.HTMLEscapeString(label), width, value)
	}
	b.WriteString(`</div>`)
	// #nosec G203 -- label text is escaped and width is derived from numeric report values.
	return copilotMetricsHTMLPanel{Title: title, Chart: template.HTML(b.String())}
}

var copilotMetricsHTMLTemplate = template.Must(template.New("report").Parse(`{{define "chartFallbacks"}}{{range .Fallbacks}}{{if .SVG}}<span class="chart-tooltip-anchor" data-tooltip-anchor="{{.Target}}" aria-hidden="true"></span>{{end}}<div class="chart-tooltip-fallback" data-tooltip-target="{{.Target}}" aria-hidden="true" tabindex="-1">{{.Text}}</div>{{end}}
<details class="chart-values"><summary>Chart values</summary><ul>{{range .Tooltips}}<li>{{.Text}}</li>{{end}}</ul></details>{{end}}{{define "metricsTable"}}<div class="table-wrap"><table>
<thead><tr>{{$classes := .NumericClass}}{{range $i, $h := .Headers}}<th scope="col" class="{{index $classes $i}}" aria-sort="none"><button class="sort-button" type="button">{{$h}}</button></th>{{end}}</tr></thead>
<tbody>
{{- range $rowIndex, $row := .Rows}}
<tr>{{range $i, $c := $row}}<td class="{{index $classes $i}}">{{if $.RowColors}}{{$colors := index $.RowColors $rowIndex}}{{if and (eq $i 0) $colors.Group}}<span class="dimension-key" style="background-color:{{$colors.Group}}" aria-hidden="true"></span>{{end}}{{if and (eq $i 1) $colors.Series}}<span class="dimension-key" style="background-color:{{$colors.Series}}" aria-hidden="true"></span>{{end}}{{end}}{{$c}}</td>{{end}}</tr>
{{- end}}
</tbody>
</table></div>{{end}}<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light dark; --fg: #1f2328; --muted: #59636e; --border: #d1d9e0; --accent: #0969da; --bg-alt: #f6f8fa; --warn: #9a6700; --text-body: 14px; --text-detail: 12px; --text-heading: 18px; --text-title: 24px; --text-kpi: 24px; --text-chart: 8px; }
@media (prefers-color-scheme: dark) { :root { --fg: #e6edf3; --muted: #9198a1; --border: #3d444d; --accent: #4493f8; --bg-alt: #151b23; --warn: #d29922; } }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; font-size: var(--text-body); color: var(--fg); margin: 24px; line-height: 1.5; }
h1 { font-size: var(--text-title); border-bottom: 1px solid var(--border); padding-bottom: .3em; }
h2 { font-size: var(--text-heading); margin-top: 1.8em; border-bottom: 1px solid var(--border); padding-bottom: .3em; }
h3 { font-size: var(--text-body); font-weight: 600; margin: 1.5em 0 .25em; }
.visually-hidden { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
.section-toggle, .table-toggle { cursor: pointer; font-weight: 600; }
.section-toggle { font-size: var(--text-heading); margin-top: 1.8em; border-bottom: 1px solid var(--border); padding-bottom: .3em; }
.table-details { margin: 1em 0; }
.table-toggle { font-size: var(--text-body); }
.section-toggle:focus-visible, .table-toggle:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.meta { color: var(--muted); margin: 0 0 1em; padding: 0; list-style: none; }
.callout { border-left: 4px solid var(--warn); padding: .5em 1em; margin: 1em 0; background: var(--bg-alt); }
.callout.note { border-left-color: var(--accent); }
.desc { color: var(--muted); }
.table-wrap { overflow-x: auto; }
table { border-collapse: collapse; margin: .5em 0; font-size: var(--text-detail); }
th, td { border: 1px solid var(--border); padding: 4px 10px; white-space: nowrap; }
th { background: var(--bg-alt); text-align: left; }
td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
.heatmap { background-color: rgba(9, 105, 218, var(--heat-alpha, 0)); }
.dimension-key { display: inline-block; width: .8em; height: .8em; margin-right: .4em; border: 1px solid var(--border); border-radius: 2px; vertical-align: middle; }
.sort-button { appearance: none; background: transparent; border: 0; color: inherit; cursor: pointer; font: inherit; font-weight: 600; padding: 0; text-align: inherit; }
.sort-button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.js-enabled th[aria-sort="ascending"] .sort-button::after { content: " ▲"; }
.js-enabled th[aria-sort="descending"] .sort-button::after { content: " ▼"; }
.kpi-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 190px), 1fr)); gap: 12px; margin: 1em 0 1.5em; }
.kpi-card { border: 1px solid var(--border); border-radius: 8px; padding: 16px; background: var(--bg-alt); }
.kpi-card p { color: var(--muted); font-size: var(--text-detail); font-weight: 600; margin: 0 0 .4em; }
.kpi-card strong { display: block; font-size: var(--text-kpi); line-height: 1.2; font-variant-numeric: tabular-nums; }
.kpi-card small { color: var(--muted); font-size: var(--text-detail); display: block; margin-top: .5em; }
.bar-chart { display: grid; gap: 8px; font-size: var(--text-detail); }
.bar-row { align-items: center; display: grid; gap: 10px; grid-template-columns: minmax(90px, 190px) minmax(70px, 1fr) minmax(54px, auto); }
.bar-label { overflow: hidden; text-overflow: ellipsis; }
.bar-track { background: var(--border); border-radius: 4px; height: 12px; overflow: hidden; }
.bar-fill { background: var(--accent); border-radius: inherit; display: block; height: 100%; }
.bar-value { font-variant-numeric: tabular-nums; text-align: right; }
.stacked-legend { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 190px), 1fr)); gap: 4px 8px; list-style: none; margin: 0 0 .5em; padding: 0; font-size: var(--text-detail); }
.stacked-legend li { display: flex; align-items: center; gap: 6px; min-width: 0; }
.stacked-legend-label { min-width: 0; overflow-wrap: anywhere; }
.series-key { display: inline-block; flex: none; box-sizing: border-box; width: 12px; height: 12px; border: 1px solid var(--border); border-radius: 2px; }
.stacked-chart-wrap { overflow-x: auto; }
.stacked-chart { display: block; width: 100%; height: auto; }
.stacked-chart .grid { stroke: var(--border); stroke-width: 1; }
.stacked-axis-label { color: var(--muted); font-size: var(--text-chart); line-height: 1.2; overflow-wrap: anywhere; text-align: center; }
.chart-color-key { color: var(--muted); font-size: var(--text-detail); margin: .5em 0 0; }
.pie-chart { display: flex; flex-wrap: wrap; align-items: center; gap: 20px; }
.pie-plot { display: block; width: min(100%, 320px); height: auto; }
.pie-slice { stroke: var(--bg-alt); stroke-width: 1; }
.pie-legend { flex: 1; list-style: none; margin: 0; padding: 0; font-size: var(--text-detail); }
.pie-legend li { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 6px 0; overflow-wrap: anywhere; }
.pie-key { width: 12px; height: 12px; flex-shrink: 0; border-radius: 2px; }
.rich section { border: 1px solid var(--border); border-radius: 8px; margin: 16px 0; padding: 16px; }
.rich .section-toggle { margin-top: 0; }
.rich-panels { display: grid; gap: 12px; grid-template-columns: minmax(0, 1fr); margin: 1em 0; }
.rich-panels-paired { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.rich-panels-paired .rich-panel { grid-column: 1 / -1; }
.rich-panels-paired .rich-panel-half { grid-column: auto; --stacked-min-width: 0px; }
.rich-panel, .chart-panel { border: 1px solid var(--border); border-radius: 8px; margin: 0; min-width: 0; overflow: hidden; padding: 12px; position: relative; }
.chart-panel { margin: 1em 0; }
.rich-panel figcaption, .chart-panel figcaption { font-size: var(--text-body); font-weight: 600; line-height: 1.5; margin-bottom: .5em; }
.metric-chart { display: block; height: auto; width: 100%; --text-chart: 7px; }
.metric-chart .grid { stroke: var(--border); stroke-width: 1; }
.metric-chart .axis, .metric-chart .legend, .stacked-chart .axis, .stacked-chart .legend, .chart .axis { fill: var(--muted); font-size: var(--text-chart); }
.rich-panel-half .metric-chart { --text-chart: 13px; }
.rich-panel-half .stacked-chart { --text-chart: 16px; }
.metric-chart .metric-line { fill: none; stroke-width: 2; }
.metric-chart .metric-area { stroke: var(--bg-alt); stroke-width: .8; fill-opacity: .85; }
.metric-chart .metric-day-hit { fill: transparent; pointer-events: all; }
.metric-chart .daily-stacked-segment { stroke: var(--bg-alt); stroke-width: .6; }
.daily-stacked-bar:hover .daily-stacked-segment, .daily-stacked-bar:focus-within .daily-stacked-segment { filter: brightness(1.15); }
.repository-bar:hover rect, .repository-bar:focus rect { filter: brightness(1.15); }
.repository-chart-hit { fill: transparent; pointer-events: all; }
.repository-incomplete-marker, .daily-stacked-incomplete-marker { fill: var(--warn); font-size: 10px; font-weight: 700; opacity: .75; }
.repository-label-wrap { align-items: center; color: var(--muted); display: flex; font-size: var(--text-chart); height: 100%; line-height: 1.2; width: 100%; }
.repository-label { min-width: 0; overflow: hidden; text-align: right; text-overflow: ellipsis; white-space: nowrap; width: 100%; }
.metric-chart .metric-dot { stroke: var(--bg-alt); stroke-width: 1; }
.chart-day .dot, .chart-day .metric-dot { visibility: hidden; pointer-events: none; }
.chart-day:hover .dot, .chart-day:focus-within .dot,
.chart-day:hover .metric-dot, .chart-day:focus-within .metric-dot { visibility: visible; }
.rich .chart { max-width: none; }
@media (max-width: 1000px) {
  .rich-panels-paired { grid-template-columns: minmax(0, 1fr); }
  .rich-panel-half .metric-chart { --text-chart: 7px; }
}
@media (max-width: 720px) {
  .rich-panel-half .stacked-chart { --text-chart: 18px; }
  .metric-chart, .rich-panel-half .metric-chart { --text-chart: 13px; }
  .pie-plot { width: min(100%, 260px); }
  .pie-legend { min-width: 130px; }
  body { margin: 12px; }
  .rich section { padding: 10px; }
  .bar-row { gap: 6px; grid-template-columns: minmax(70px, 120px) minmax(50px, 1fr) minmax(48px, auto); }
}
.chart { width: 100%; max-width: 720px; height: auto; display: block; margin: .5em 0; }
.chart-line { --text-chart: 7px; }
.chart .grid { stroke: var(--border); stroke-width: 1; }
.chart .line { fill: none; stroke: var(--accent); stroke-width: 2; }
.chart .line-area { fill: var(--accent); fill-opacity: .18; stroke: none; }
.chart .dot { fill: var(--accent); }
.chart .chart-day-hit { fill: transparent; pointer-events: all; }
.chart .bar { fill: var(--accent); opacity: .8; }
.omitted { color: var(--muted); font-style: italic; }
.chart-tooltip { position: fixed; z-index: 1000; display: flex; flex-direction: column; max-width: min(360px, calc(100vw - 24px)); max-height: calc(100vh - 24px); overflow: hidden; box-sizing: border-box; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg-alt); color: var(--fg); font-size: var(--text-detail); overflow-wrap: anywhere; box-shadow: 0 2px 8px rgba(0, 0, 0, .2); }
.chart-tooltip[hidden] { display: none; }
.chart-tooltip-body { min-height: 0; overflow: auto; white-space: pre-wrap; }
.chart-tooltip-heading { margin-bottom: 4px; font-weight: 600; }
.chart-tooltip-grid { display: grid; grid-template-columns: minmax(0, 1fr) max-content; column-gap: 12px; }
.chart-tooltip-grid.has-shares { grid-template-columns: minmax(0, 1fr) max-content max-content; }
.chart-tooltip-row { display: contents; }
.chart-tooltip-label { min-width: 0; overflow-wrap: anywhere; }
.chart-tooltip-value, .chart-tooltip-share { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
.chart-tooltip-note, .chart-tooltip-scroll-hint { margin-top: 4px; color: var(--muted); }
.chart-tooltip-scroll-hint { flex: 0 0 auto; }
.chart-tooltip-fallback { display: none; position: absolute; z-index: 10; top: 3em; right: 12px; max-width: min(360px, calc(100% - 24px)); max-height: min(50vh, max(48px, calc(100% - 84px))); overflow: auto; overscroll-behavior: contain; box-sizing: border-box; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--bg-alt); color: var(--fg); font-size: var(--text-detail); white-space: pre-wrap; overflow-wrap: anywhere; box-shadow: 0 2px 8px rgba(0, 0, 0, .2); }
.chart-tooltip-anchor { position: absolute; pointer-events: none; }
@supports (anchor-name: --a) and (width: anchor-size(width)) {
  .chart-tooltip-fallback { inset: auto; min-width: min(200px, 60vw); min-height: 48px; max-width: min(360px, 100%); max-height: min(50vh, 320px, 100%); position-area: right; position-try-fallbacks: flip-inline, bottom span-all, top span-all; }
}
.chart-values { color: var(--muted); font-size: var(--text-detail); margin-top: .5em; }
.chart-values summary { cursor: pointer; font-weight: 600; }
.chart-values ul { list-style: none; max-height: 24em; overflow: auto; margin: .5em 0 0; padding: 0; }
.chart-values li { border-top: 1px solid var(--border); color: var(--fg); padding: .5em 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.js-enabled figure:not(.tooltip-failed) .chart-values { display: none; }
[data-tooltip]:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
{{.TooltipCSS}}
</style>
</head>
<body class="{{if .Rich}}rich{{end}}">
<h1>{{.Title}}</h1>
<ul class="meta">
<li>Period: {{.Period}}</li>
{{- if .Generated}}
<li>Generated: {{.Generated}}</li>
{{- end}}
</ul>
{{- if and .Rich .Highlights}}
<div class="kpi-grid" aria-label="Key metrics">
{{- range .Highlights}}
<article class="kpi-card"><p>{{.Label}}</p><strong>{{.Value}}</strong><small>{{.Window}}</small></article>
{{- end}}
</div>
{{- end}}
{{- if .Partial}}
<div class="callout"><strong>Partial data</strong><ul>{{range .Partial}}<li>{{.}}</li>{{end}}</ul></div>
{{- end}}
{{- if .Warnings}}
<div class="callout note"><strong>Note</strong><ul>{{range .Warnings}}<li>{{.}}</li>{{end}}</ul></div>
{{- end}}
{{- range .Sections}}
{{- $sectionID := .ID}}
<section id="{{.ID}}" aria-labelledby="section-heading-{{.ID}}">
<h2 class="visually-hidden" id="section-heading-{{.ID}}">{{.Title}}</h2>
<details class="section-details" open>
<summary class="section-toggle" aria-labelledby="section-heading-{{.ID}}">{{.Title}}</summary>
{{- if .Description}}
<p class="desc">{{.Description}}</p>
{{- end}}
{{- if .RichPanels}}
<div class="rich-panels{{if or (eq .ID "trend") (eq .ID "language") (eq .ID "model") (eq .ID "repository") (eq .ID "team") (eq .ID "user")}} rich-panels-paired{{end}}">
{{- range .RichPanels}}
{{- if .Chart}}
<figure class="rich-panel{{if .HalfWidth}} rich-panel-half{{end}}"><figcaption>{{.Title}}</figcaption>{{.Chart}}{{if .Tooltips}}{{template "chartFallbacks" .}}{{end}}</figure>
{{- end}}
{{- end}}
</div>
{{- end}}
{{- if .Empty}}
<p>No data.</p>
{{- else}}
{{- range .Charts}}
<figure class="chart-panel"><figcaption>{{.Title}}</figcaption>{{.Chart}}{{if .Tooltips}}{{template "chartFallbacks" .}}{{end}}</figure>
{{- end}}
<h3 class="visually-hidden" id="table-heading-{{.ID}}">{{.Title}} table</h3>
<details class="table-details"{{if not $.Rich}} open{{end}}>
<summary class="table-toggle" aria-labelledby="table-heading-{{.ID}}">Table</summary>
{{template "metricsTable" .Table}}
{{- if .OmittedText}}
<p class="omitted">{{.OmittedText}}</p>
{{- end}}
</details>
{{- end}}
{{- range $index, $detail := .Details}}
{{- if .Table.Rows}}
<h3 class="visually-hidden" id="detail-heading-{{$sectionID}}-{{$index}}">{{.Title}}</h3>
<details class="table-details"{{if not $.Rich}} open{{end}}>
<summary class="table-toggle" aria-labelledby="detail-heading-{{$sectionID}}-{{$index}}">{{.Title}}</summary>
<p class="desc">{{.Description}}</p>
{{template "metricsTable" .Table}}
</details>
{{- else}}
<h3>{{.Title}}</h3>
<p class="desc">{{.Description}}</p>
<p>No data.</p>
{{- end}}
{{- end}}
</details>
</section>
{{- end}}
<script>
(() => {
    const tooltip = document.createElement("div");
    tooltip.className = "chart-tooltip";
    tooltip.id = "chart-tooltip";
    tooltip.setAttribute("role", "tooltip");
    tooltip.hidden = true;
    const tooltipBody = document.createElement("div");
    tooltipBody.className = "chart-tooltip-body";
    const scrollHint = document.createElement("div");
    scrollHint.className = "chart-tooltip-scroll-hint";
    scrollHint.textContent = "Scroll or swipe, or use arrow keys, for more";
    scrollHint.setAttribute("aria-hidden", "true");
    scrollHint.hidden = true;
    tooltip.append(tooltipBody, scrollHint);
    document.body.appendChild(tooltip);
    let activeItem = null;
    let hoveredItem = null;
    let tooltipHovered = false;
    let dismissedItem = null;
    let dismissedOverTrigger = false;
    let dismissedX = -1;
    let dismissedY = -1;
    let hideTimer = null;
    let hoverX = 0;
    let hoverY = 0;
    let pointerX = -1;
    let pointerY = -1;
    document.addEventListener("pointermove", (event) => {
        pointerX = event.clientX;
        pointerY = event.clientY;
    });
    const cancelHide = () => {
        if (hideTimer !== null) clearTimeout(hideTimer);
        hideTimer = null;
    };
    const clearDismissed = () => {
        dismissedItem?.removeAttribute("data-tooltip-dismissed");
        dismissedItem = null;
        dismissedOverTrigger = false;
        document.documentElement.classList.remove("chart-tooltip-dismissed");
    };
    const hideTooltip = () => {
        cancelHide();
        if (activeItem) {
            activeItem.removeAttribute("aria-describedby");
            activeItem.removeAttribute("data-tooltip-active");
        }
        activeItem = null;
        document.documentElement.classList.remove("chart-tooltip-active");
        tooltipHovered = false;
        tooltip.hidden = true;
    };
    const failTooltip = (error) => {
        const figure = activeItem?.closest("figure");
        hideTooltip();
        figure?.classList.add("tooltip-failed");
        throw error;
    };
    const positionTooltip = (x, y) => {
        tooltip.style.left = "12px";
        tooltip.style.top = "12px";
        const bounds = tooltip.getBoundingClientRect();
        tooltip.style.left = Math.max(12, Math.min(x + 12, window.innerWidth - bounds.width - 12)) + "px";
        tooltip.style.top = Math.max(12, Math.min(y + 12, window.innerHeight - bounds.height - 12)) + "px";
    };
    const renderTooltip = (item) => {
        tooltipBody.replaceChildren();
        if (!item.dataset.tooltipFields) {
            tooltipBody.textContent = item.dataset.tooltip;
            return;
        }
        const fields = new URLSearchParams(item.dataset.tooltipFields);
        const heading = fields.get("heading");
        if (heading) {
            const element = document.createElement("div");
            element.className = "chart-tooltip-heading";
            element.textContent = heading;
            tooltipBody.appendChild(element);
        }
        const labels = fields.getAll("label");
        const values = fields.getAll("value");
        const shares = fields.getAll("share");
        const hasShares = shares.some(Boolean);
        const grid = document.createElement("div");
        grid.className = "chart-tooltip-grid" + (hasShares ? " has-shares" : "");
        labels.forEach((label, index) => {
            const row = document.createElement("div");
            row.className = "chart-tooltip-row";
            const name = document.createElement("span");
            name.className = "chart-tooltip-label";
            name.textContent = label + ":";
            const value = document.createElement("span");
            value.className = "chart-tooltip-value";
            value.textContent = values[index];
            row.append(name, value);
            if (hasShares) {
                const share = document.createElement("span");
                share.className = "chart-tooltip-share";
                share.textContent = shares[index] ? "(" + shares[index] + ")" : "";
                row.appendChild(share);
            }
            grid.appendChild(row);
        });
        tooltipBody.appendChild(grid);
        const note = fields.get("note");
        if (note) {
            const element = document.createElement("div");
            element.className = "chart-tooltip-note";
            element.textContent = note;
            tooltipBody.appendChild(element);
        }
    };
    const showTooltip = (item, x, y) => {
        hideTooltip();
        activeItem = item;
        try {
            renderTooltip(item);
            tooltip.setAttribute("aria-label", item.dataset.tooltip);
            scrollHint.hidden = true;
            tooltip.hidden = false;
            tooltipBody.scrollTop = 0;
            tooltipBody.scrollLeft = 0;
            positionTooltip(x, y);
            scrollHint.hidden = tooltipBody.scrollHeight <= tooltipBody.clientHeight && tooltipBody.scrollWidth <= tooltipBody.clientWidth;
            item.setAttribute("aria-describedby", tooltip.id);
            positionTooltip(x, y);
            item.setAttribute("data-tooltip-active", "");
            document.documentElement.classList.add("chart-tooltip-active");
        } catch (error) {
            failTooltip(error);
        }
    };
    const scrollTooltip = (x, y) => {
        try {
            const previousX = tooltipBody.scrollLeft;
            const previousY = tooltipBody.scrollTop;
            tooltipBody.scrollBy(x, y);
            return tooltipBody.scrollLeft !== previousX || tooltipBody.scrollTop !== previousY;
        } catch (error) {
            failTooltip(error);
        }
    };
    const scheduleHide = () => {
        cancelHide();
        hideTimer = setTimeout(() => {
            hideTimer = null;
            if (tooltipHovered || hoveredItem) return;
            const focused = document.activeElement;
            if (focused?.dataset?.tooltip && focused !== dismissedItem) {
                if (activeItem !== focused) {
                    const bounds = focused.getBoundingClientRect();
                    showTooltip(focused, bounds.left + bounds.width / 2, bounds.bottom);
                }
            } else {
                hideTooltip();
            }
        }, 300);
    };
    const addScrollHandlers = (item, isActive) => {
        let touchX = 0;
        let touchY = 0;
        item.addEventListener("wheel", (event) => {
            if (!isActive() || event.ctrlKey) return;
            const scale = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? tooltipBody.clientHeight : 1;
            const x = event.shiftKey && !event.deltaX ? event.deltaY : event.deltaX;
            const y = event.shiftKey && !event.deltaX ? 0 : event.deltaY;
            if (scrollTooltip(x * scale, y * scale)) event.preventDefault();
        }, { passive: false });
        item.addEventListener("touchstart", (event) => {
            if (event.touches.length !== 1) return;
            touchX = event.touches[0].clientX;
            touchY = event.touches[0].clientY;
        }, { passive: true });
        item.addEventListener("touchmove", (event) => {
            if (!isActive() || event.touches.length !== 1) return;
            const x = event.touches[0].clientX;
            const y = event.touches[0].clientY;
            if (scrollTooltip(touchX - x, touchY - y)) event.preventDefault();
            touchX = x;
            touchY = y;
        }, { passive: false });
    };
    tooltip.addEventListener("pointerenter", () => {
        tooltipHovered = true;
        cancelHide();
    });
    tooltip.addEventListener("pointerleave", () => {
        tooltipHovered = false;
        if (!hoveredItem && !tooltip.hidden) scheduleHide();
    });
    addScrollHandlers(tooltip, () => !tooltip.hidden);
    document.querySelectorAll("[data-tooltip]").forEach((item) => {
        // Hiding a tooltip can reveal a target under the stationary pointer.
        item.addEventListener("pointerenter", (event) => {
            if (dismissedItem) return;
            cancelHide();
            hoveredItem = item;
            hoverX = event.clientX;
            hoverY = event.clientY;
            showTooltip(item, hoverX, hoverY);
        });
        item.addEventListener("pointermove", (event) => {
            hoverX = event.clientX;
            hoverY = event.clientY;
            if (dismissedItem) {
                if ((hoverX === dismissedX && hoverY === dismissedY) || (dismissedOverTrigger && dismissedItem === item)) return;
                clearDismissed();
                hoveredItem = item;
                showTooltip(item, hoverX, hoverY);
                return;
            }
            if (activeItem === item) {
                try {
                    positionTooltip(event.clientX, event.clientY);
                } catch (error) {
                    failTooltip(error);
                }
            }
        });
        item.addEventListener("pointerleave", () => {
            if (dismissedItem === item && dismissedOverTrigger) {
                if (document.activeElement === item) dismissedOverTrigger = false;
                else clearDismissed();
            }
            if (hoveredItem === item) hoveredItem = null;
            if (activeItem === item && !tooltipHovered) scheduleHide();
        });
        item.addEventListener("focus", () => {
            clearDismissed();
            const bounds = item.getBoundingClientRect();
            showTooltip(item, bounds.left + bounds.width / 2, bounds.bottom);
        });
        item.addEventListener("blur", () => {
            if (dismissedItem === item && !dismissedOverTrigger) clearDismissed();
            if (activeItem !== item || tooltipHovered) return;
            if (hoveredItem) showTooltip(hoveredItem, hoverX, hoverY);
            else scheduleHide();
        });
        addScrollHandlers(item, () => activeItem === item);
    });
    document.addEventListener("keydown", (event) => {
        if (event.key === "Escape") {
            if (!activeItem) return;
            clearDismissed();
            dismissedItem = activeItem;
            dismissedItem.setAttribute("data-tooltip-dismissed", "");
            document.documentElement.classList.add("chart-tooltip-dismissed");
            dismissedX = pointerX;
            dismissedY = pointerY;
            hideTooltip();
            dismissedOverTrigger = !!dismissedItem?.contains(document.elementFromPoint(pointerX, pointerY));
            return;
        }
        if (activeItem !== document.activeElement) return;
        const page = tooltipBody.clientHeight * .8;
        const moves = {
            ArrowDown: [0, 32], ArrowUp: [0, -32],
            ArrowRight: [32, 0], ArrowLeft: [-32, 0],
            PageDown: [0, page], PageUp: [0, -page]
        };
        const move = moves[event.key];
        if (move && scrollTooltip(move[0], move[1])) event.preventDefault();
    });
    window.addEventListener("scroll", (event) => {
        if (event.target !== tooltipBody) hideTooltip();
    }, true);
    window.addEventListener("resize", () => hideTooltip());
    let printDetails = null;
    window.addEventListener("beforeprint", () => {
        if (printDetails !== null) return;
        hideTooltip();
        printDetails = Array.from(document.querySelectorAll("details:not(.chart-values)"), (item) => [item, item.open]);
        printDetails.forEach(([item]) => { item.open = true; });
    });
    window.addEventListener("afterprint", () => {
        if (printDetails === null) return;
        printDetails.forEach(([item, open]) => { item.open = open; });
        printDetails = null;
    });
    const parseNumber = (text) => {
        const value = text.trim().replace(/,/g, "").replace(/%$/, "");
        if (!/^-?(?:\d+(?:\.\d*)?|\.\d+)$/.test(value)) return null;
        const number = Number(value);
        return Number.isFinite(number) ? number : null;
    };
    document.querySelectorAll("table").forEach((table) => {
        const body = table.tBodies[0];
        if (!body) return;
        const rows = Array.from(body.rows);
        rows.forEach((row, index) => { row.dataset.originalIndex = String(index); });
        Array.from(table.tHead.rows[0].cells).forEach((header, column) => {
            const button = header.querySelector("button");
            if (!button) return;
            button.addEventListener("click", () => {
                const direction = header.getAttribute("aria-sort") === "descending" ? "ascending" : "descending";
                Array.from(table.tHead.rows[0].cells).forEach((cell) => cell.setAttribute("aria-sort", "none"));
                header.setAttribute("aria-sort", direction);
                const numeric = header.classList.contains("num");
                rows.sort((left, right) => {
                    const aText = left.cells[column]?.textContent ?? "";
                    const bText = right.cells[column]?.textContent ?? "";
                    const a = numeric ? parseNumber(aText) : aText.trim().toLocaleLowerCase();
                    const b = numeric ? parseNumber(bText) : bText.trim().toLocaleLowerCase();
                    if (a === null) return b === null ? Number(left.dataset.originalIndex) - Number(right.dataset.originalIndex) : 1;
                    if (b === null) return -1;
                    const compared = numeric ? a - b : a.localeCompare(b, undefined, { numeric: true });
                    if (compared === 0) return Number(left.dataset.originalIndex) - Number(right.dataset.originalIndex);
                    return compared * (direction === "ascending" ? 1 : -1);
                });
                rows.forEach((row) => body.appendChild(row));
            });
        });
        const heatColumns = new Map();
        Array.from(body.rows).forEach((row) => Array.from(row.cells).forEach((cell, column) => {
            if (!cell.classList.contains("heatmap")) return;
            const value = parseNumber(cell.textContent ?? "");
            if (value === null) return;
            if (!heatColumns.has(column)) heatColumns.set(column, []);
            heatColumns.get(column).push({ cell, value });
        }));
        heatColumns.forEach((cells) => {
            let low = Infinity;
            let high = -Infinity;
            cells.forEach((item) => {
                low = Math.min(low, item.value);
                high = Math.max(high, item.value);
            });
            if (high <= 0) return;
            cells.forEach((item) => {
                const intensity = high === low ? 0.5 : (item.value - low) / (high - low);
                item.cell.style.setProperty("--heat-alpha", String(0.03 + intensity * 0.15));
            });
        });
    });
    document.documentElement.classList.add("js-enabled");
})();
</script>
</body>
</html>
`))

// RenderCopilotMetricsHTML renders a usage metrics summary as a self-contained HTML document.
func RenderCopilotMetricsHTML(s *copilotmetrics.Summary) (string, error) {
	return RenderCopilotMetricsHTMLWithOptions(s, CopilotMetricsReportOptions{})
}

// RenderCopilotMetricsHTMLWithOptions renders a self-contained HTML report with optional rich dashboard visuals.
func RenderCopilotMetricsHTMLWithOptions(s *copilotmetrics.Summary, opts CopilotMetricsReportOptions) (string, error) {
	data := copilotMetricsHTMLData{
		Title:      "Copilot usage metrics: " + s.Source,
		Period:     copilotMetricsPeriodText(s),
		Generated:  copilotMetricsGeneratedText(s),
		Rich:       opts.Rich,
		Highlights: copilotMetricsHTMLHighlights(s),
		Warnings:   append([]string(nil), s.Warnings...),
	}
	var tooltipFallbacks []copilotMetricsHTMLTooltipFallback
	nextTooltipTarget := 0
	if opts.Rich && s.Rich != nil && s.Rich.IDEActiveUserMonth != nil && len(s.Rich.IDEActiveUserMonth.MissingDays) > 0 {
		data.Warnings = append(data.Warnings, fmt.Sprintf("IDE active user estimate is partial; users reports are missing for %s",
			strings.Join(s.Rich.IDEActiveUserMonth.MissingDays, ", ")))
	}
	for _, p := range s.Partial {
		data.Partial = append(data.Partial, copilotMetricsPartialText(p))
	}
	for _, hs := range copilotMetricsHTMLSections(s, opts.Rich) {
		for i := range hs.Table.Headers {
			class := ""
			if copilotMetricsHTMLNumericColumn(hs.ID, i) {
				class = "num"
				if copilotMetricsHTMLHeatmapColumn(hs.ID, i) {
					class += " heatmap"
				}
			}
			hs.Table.NumericClass = append(hs.Table.NumericClass, class)
		}
		for i, row := range hs.Table.Rows {
			if len(row) < len(hs.Table.NumericClass) {
				padded := make([]string, len(hs.Table.NumericClass))
				copy(padded, row)
				hs.Table.Rows[i] = padded
			}
		}
		for _, panels := range [][]copilotMetricsHTMLPanel{hs.Charts, hs.RichPanels} {
			for i := range panels {
				panel := &panels[i]
				chart, tooltips, fallbacks, err := copilotMetricsChartTooltips(panel.Chart, &nextTooltipTarget)
				if err != nil {
					return "", fmt.Errorf("collect tooltips for chart %q: %w", panel.Title, err)
				}
				panel.Chart, panel.Tooltips, panel.Fallbacks = chart, tooltips, fallbacks
				tooltipFallbacks = append(tooltipFallbacks, fallbacks...)
			}
		}
		data.Sections = append(data.Sections, hs)
	}
	data.TooltipCSS = copilotMetricsTooltipFallbackCSS(tooltipFallbacks)
	var buf bytes.Buffer
	if err := copilotMetricsHTMLTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func copilotMetricsHTMLHeatmapColumn(section copilotmetrics.Section, column int) bool {
	if section == copilotmetrics.SectionKPI && column == 1 {
		return false
	}
	return section != copilotmetrics.SectionUser || column != 0
}

func copilotMetricsHTMLNumericColumn(section copilotmetrics.Section, column int) bool {
	switch section {
	case copilotmetrics.SectionKPI:
		return column == 1
	case copilotmetrics.SectionTrend:
		return column > 0
	case copilotmetrics.SectionRepository:
		return column >= 2
	case copilotmetrics.SectionTeam:
		return column > 0
	case copilotmetrics.SectionUser:
		return column == 0 || column >= 2 && column <= 8
	default:
		return column > 0
	}
}

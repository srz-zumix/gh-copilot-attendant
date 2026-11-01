package render

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const (
	copilotMetricsNA                  = "n/a"
	copilotMetricsModelChatModeTitle  = "Model usage per chat mode"
	copilotMetricsModelLanguageTitle  = "Model usage per language (code-generation activities)"
	copilotMetricsUnreportedDimension = "(not reported)"
)

func formatCopilotMetricsInt(v int64) string {
	s := strconv.FormatInt(v, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func formatCopilotMetricsCounter(c copilotmetrics.Counter) string {
	if !c.Valid {
		return copilotMetricsNA
	}
	return formatCopilotMetricsInt(c.Value)
}

func formatCopilotMetricsCredits(c copilotmetrics.FloatCounter) string {
	if !c.Valid {
		return copilotMetricsNA
	}
	return fmt.Sprintf("%.2f", c.Value)
}

func formatCopilotMetricsRate(v float64, ok bool) string {
	if !ok {
		return copilotMetricsNA
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

func formatCopilotMetricsKPI(k copilotmetrics.KPI) string {
	if !k.Valid {
		return copilotMetricsNA
	}
	switch k.Kind {
	case copilotmetrics.KindPercent:
		return formatCopilotMetricsRate(k.Value, true)
	case copilotmetrics.KindDecimal:
		return fmt.Sprintf("%.1f", k.Value)
	default:
		return formatCopilotMetricsInt(int64(k.Value))
	}
}

func copilotMetricsPeriodText(s *copilotmetrics.Summary) string {
	return fmt.Sprintf("%s .. %s (%d day(s), %s)", s.StartDay, s.EndDay, s.Days, s.PeriodLabel)
}

func copilotMetricsGeneratedText(s *copilotmetrics.Summary) string {
	if s.GeneratedAt.IsZero() {
		return ""
	}
	return s.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC")
}

func copilotMetricsPartialText(p copilotmetrics.PartialNote) string {
	names := make([]string, 0, len(p.Sections))
	for _, sec := range p.Sections {
		names = append(names, string(sec))
	}
	return fmt.Sprintf("%s reports are missing for %d day(s) (%s); affected sections: %s",
		p.Family, len(p.MissingDays), strings.Join(p.MissingDays, ", "), strings.Join(names, ", "))
}

type copilotMetricsDetailTable struct {
	Title       string
	Description string
	Headers     []string
	Rows        [][]string
	Dimensions  []copilotMetricsDetailDimension
}

type copilotMetricsDetailDimension struct {
	Group  string
	Series string
}

func copilotMetricsModelFeatureValues(s *copilotmetrics.Summary) []copilotmetrics.RichValue {
	if s.ModelRequestsByFeature == nil && s.Rich != nil {
		return s.Rich.ModelRequestsByFeature
	}
	return s.ModelRequestsByFeature
}

func copilotMetricsModelLanguageValues(s *copilotmetrics.Summary) []copilotmetrics.RichValue {
	if s.ModelCodeGenerationByLanguage == nil && s.Rich != nil {
		return s.Rich.ModelCodeGenerationByLanguage
	}
	return s.ModelCodeGenerationByLanguage
}

func copilotMetricsChatModeModelValues(values []copilotmetrics.RichValue) []copilotmetrics.RichValue {
	var out []copilotmetrics.RichValue
	for _, row := range values {
		mode, ok := copilotmetrics.ChatModeForFeature(row.Group)
		if !ok {
			continue
		}
		row.Group, row.Name = row.Name, mode
		out = append(out, row)
	}
	return out
}

func copilotMetricsDetailForSection(s *copilotmetrics.Summary, section copilotmetrics.Section) *copilotMetricsDetailTable {
	var values []copilotmetrics.RichValue
	table := &copilotMetricsDetailTable{}
	switch section {
	case copilotmetrics.SectionModel:
		table.Title = copilotMetricsModelChatModeTitle
		table.Description = "Counts sum available values; shares are of all reported chat-mode interactions. All model and mode entries are shown without chart grouping or --top truncation."
		table.Headers = []string{"Model", "Chat mode", "Interactions", "Share of chat-mode interactions"}
		values = copilotMetricsChatModeModelValues(copilotMetricsModelFeatureValues(s))
	case copilotmetrics.SectionLanguage:
		table.Title = copilotMetricsModelLanguageTitle
		table.Description = "Counts sum available values; shares are of all reported language-model code-generation activities. All language and model entries are shown without chart grouping or --top truncation."
		table.Headers = []string{"Language", "Model", "Code-generation activities", "Share of language-model activities"}
		values = copilotMetricsModelLanguageValues(s)
	default:
		return nil
	}

	var total copilotmetrics.Counter
	for _, row := range values {
		if row.Value.Valid && row.Value.Value >= 0 {
			total.AddCounter(row.Value)
		}
	}
	for _, row := range values {
		group, name := row.Group, row.Name
		if group == "" {
			group = copilotMetricsUnreportedDimension
		}
		if name == "" {
			name = copilotMetricsUnreportedDimension
		}
		share, valid := copilotmetrics.Rate(row.Value, total)
		table.Rows = append(table.Rows, []string{
			group, name, formatCopilotMetricsCounter(row.Value),
			formatCopilotMetricsRate(share, valid && row.Value.Value >= 0),
		})
		table.Dimensions = append(table.Dimensions, copilotMetricsDetailDimension{Group: row.Group, Series: row.Name})
	}
	return table
}

// RenderCopilotMetricsReport renders a usage metrics summary in the given format.
func RenderCopilotMetricsReport(s *copilotmetrics.Summary, f copilotmetrics.Format) (string, error) {
	return RenderCopilotMetricsReportWithOptions(s, f, CopilotMetricsReportOptions{})
}

// CopilotMetricsReportOptions controls optional report presentation features.
type CopilotMetricsReportOptions struct {
	Rich bool
}

// RenderCopilotMetricsReportWithOptions renders a usage metrics report with presentation options.
func RenderCopilotMetricsReportWithOptions(s *copilotmetrics.Summary, f copilotmetrics.Format, opts CopilotMetricsReportOptions) (string, error) {
	switch f {
	case copilotmetrics.FormatMarkdown:
		return RenderCopilotMetricsMarkdown(s), nil
	case copilotmetrics.FormatHTML:
		return RenderCopilotMetricsHTMLWithOptions(s, opts)
	}
	return "", fmt.Errorf("unsupported report format %q", f)
}

// formatCopilotMetricsAxisValue formats an axis tick, keeping one decimal
// place for fractional ticks so labels match their plotted positions.
func formatCopilotMetricsAxisValue(v float64) string {
	tenths := int64(math.Round(v * 10))
	if tenths%10 == 0 {
		return formatCopilotMetricsInt(tenths / 10)
	}
	sign := ""
	if tenths < 0 {
		sign, tenths = "-", -tenths
	}
	return fmt.Sprintf("%s%s.%d", sign, formatCopilotMetricsInt(tenths/10), tenths%10)
}

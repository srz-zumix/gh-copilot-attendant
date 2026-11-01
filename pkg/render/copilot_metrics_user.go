package render

import (
	"fmt"
	"html/template"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const (
	copilotMetricsUserRatioTitle       = "Interaction per active days"
	copilotMetricsUserSurfaceTitle     = "Users by surface"
	copilotMetricsUserDaysTitle        = "Reported days distribution"
	copilotMetricsUserAcceptanceTitle  = "Acceptance rate distribution"
	copilotMetricsUserRatioChartLimit  = 7
	copilotMetricsUserColumnWidth      = 720.0
	copilotMetricsUserColumnHeight     = 360.0
	copilotMetricsUserColumnLeft       = 48.0
	copilotMetricsUserColumnRight      = 12.0
	copilotMetricsUserColumnTop        = 20.0
	copilotMetricsUserColumnBottom     = 48.0
	copilotMetricsUserColumnBarSpacing = .3
)

// copilotMetricsUserSurfaces lists surfaces in a fixed display order.
var copilotMetricsUserSurfaces = []string{"chat", "agent", "cli", "app", "cloud-agent", "coding-agent", "code-review"}

type copilotMetricsUserBucket struct {
	Name     string
	Contains func(float64) bool
}

var copilotMetricsUserDayBuckets = []copilotMetricsUserBucket{
	{Name: "1 day", Contains: func(v float64) bool { return v <= 1 }},
	{Name: "2–3 days", Contains: func(v float64) bool { return v <= 3 }},
	{Name: "4–7 days", Contains: func(v float64) bool { return v <= 7 }},
	{Name: "8–14 days", Contains: func(v float64) bool { return v <= 14 }},
	{Name: "15+ days", Contains: func(float64) bool { return true }},
}

var copilotMetricsUserAcceptanceBuckets = []copilotMetricsUserBucket{
	{Name: "<20%", Contains: func(v float64) bool { return v < .2 }},
	{Name: "20–<40%", Contains: func(v float64) bool { return v < .4 }},
	{Name: "40–<60%", Contains: func(v float64) bool { return v < .6 }},
	{Name: "60–<80%", Contains: func(v float64) bool { return v < .8 }},
	{Name: "≥80%", Contains: func(float64) bool { return true }},
}

type copilotMetricsUserRatio struct {
	Login        string
	Rate         float64
	ActiveDays   int
	Interactions int64
}

// copilotMetricsUserPanels builds rich By user charts from all reported users.
// Panels are ordered as two pairs: ratio and surface, then days and acceptance.
func copilotMetricsUserPanels(users []copilotmetrics.UserRow) []copilotMetricsHTMLPanel {
	ratio := copilotMetricsUserRatioPanel(users)
	surface := copilotMetricsUserSurfacePanel(users)
	days := copilotMetricsUserBucketPanel(copilotMetricsUserDaysTitle, users, copilotMetricsUserDayBuckets,
		func(user copilotmetrics.UserRow) (float64, bool) {
			return float64(user.ActiveDays), user.ActiveDays > 0
		},
		"Users by days with a reported user record in the period.")
	acceptance := copilotMetricsUserBucketPanel(copilotMetricsUserAcceptanceTitle, users, copilotMetricsUserAcceptanceBuckets,
		func(user copilotmetrics.UserRow) (float64, bool) {
			if user.Acceptance.Denominator.Value <= 0 {
				return 0, false
			}
			return user.Acceptance.Rate()
		},
		"Users with code generation, by accepted ÷ generated on matching days. ≥80% includes rates above 100%.")
	panels := make([]copilotMetricsHTMLPanel, 0, 4)
	for _, pair := range [][2]*copilotMetricsHTMLPanel{{&ratio, &surface}, {&days, &acceptance}} {
		if pair[0].Chart != "" && pair[1].Chart != "" {
			pair[0].HalfWidth = true
			pair[1].HalfWidth = true
		}
		for _, panel := range pair {
			if panel.Chart != "" {
				panels = append(panels, *panel)
			}
		}
	}
	return panels
}

func copilotMetricsUserRatios(users []copilotmetrics.UserRow) []copilotMetricsUserRatio {
	ratios := make([]copilotMetricsUserRatio, 0, len(users))
	for _, user := range users {
		interactions := user.UserInitiatedInteraction
		if user.ActiveDays <= 0 || !interactions.Valid || interactions.Value < 0 {
			continue
		}
		ratios = append(ratios, copilotMetricsUserRatio{
			Login: user.Login, Rate: float64(interactions.Value) / float64(user.ActiveDays),
			ActiveDays: user.ActiveDays, Interactions: interactions.Value,
		})
	}
	sort.SliceStable(ratios, func(i, j int) bool {
		if ratios[i].Rate != ratios[j].Rate {
			return ratios[i].Rate > ratios[j].Rate
		}
		if ratios[i].Interactions != ratios[j].Interactions {
			return ratios[i].Interactions > ratios[j].Interactions
		}
		return ratios[i].Login < ratios[j].Login
	})
	if len(ratios) > copilotMetricsUserRatioChartLimit {
		ratios = ratios[:copilotMetricsUserRatioChartLimit]
	}
	return ratios
}

func copilotMetricsUserRatioPanel(users []copilotmetrics.UserRow) copilotMetricsHTMLPanel {
	ratios := copilotMetricsUserRatios(users)
	if len(ratios) == 0 {
		return copilotMetricsHTMLPanel{}
	}
	const (
		rowHeight = 25.0
		barHeight = 14.0
	)
	maxRate := 0.0
	for _, ratio := range ratios {
		maxRate = math.Max(maxRate, ratio.Rate)
	}
	if maxRate <= 0 {
		maxRate = 1
	}
	color := copilotMetricsThemeGradientColor(copilotMetricsThemeRose, 0, 1).hex()
	plotWidth := copilotMetricsRepositoryChartWidth - copilotMetricsRepositoryChartLeft - copilotMetricsRepositoryChartRight
	plotHeight := rowHeight * float64(len(ratios))
	height := copilotMetricsRepositoryChartTop + plotHeight + copilotMetricsRepositoryChartBottom
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="metric-chart repository-chart" viewBox="0 0 %.0f %.1f" role="group" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		copilotMetricsRepositoryChartWidth, height, template.HTMLEscapeString(copilotMetricsUserRatioTitle))
	for _, fraction := range []float64{0, .5, 1} {
		x := copilotMetricsRepositoryChartLeft + plotWidth*fraction
		fmt.Fprintf(&b, `<line class="grid" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			x, copilotMetricsRepositoryChartTop-3, x, copilotMetricsRepositoryChartTop+plotHeight)
		fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x, height-5, copilotMetricsFormatRatio(maxRate*fraction))
	}
	for index, ratio := range ratios {
		y := copilotMetricsRepositoryChartTop + float64(index)*rowHeight
		tooltip := copilotMetricsTooltip{
			Heading: ratio.Login,
			Rows: []copilotMetricsTooltipRow{
				{Label: "Interactions per active day", Value: copilotMetricsFormatRatio(ratio.Rate)},
				{Label: "Interactions", Value: formatCopilotMetricsInt(ratio.Interactions)},
				{Label: "Active days", Value: formatCopilotMetricsInt(int64(ratio.ActiveDays))},
			},
			Note: "Active days count reported user records, including zero-interaction days; missing interaction values are excluded from totals.",
		}
		aria := template.HTMLEscapeString(tooltip.text())
		copilotMetricsWriteRepositoryLabel(&b, ratio.Login, y+(barHeight-rowHeight)/2, rowHeight)
		fmt.Fprintf(&b, `<g class="repository-bar" %s tabindex="0" role="img" aria-label="%s">`, tooltip.attributes(), aria)
		fmt.Fprintf(&b, `<rect class="repository-chart-hit" x="%.1f" y="%.1f" width="%.1f" height="%.1f" aria-hidden="true"/>`,
			copilotMetricsRepositoryChartLeft, y, plotWidth, barHeight)
		if width := ratio.Rate / maxRate * plotWidth; width > 0 {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" aria-hidden="true"/>`,
				copilotMetricsRepositoryChartLeft, y, width, barHeight, color)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg><p class="chart-color-key">Top users by reported interactions ÷ active days (days with a user record).</p>`)
	// #nosec G203 -- logins are escaped; coordinates and colors are generated internally.
	return copilotMetricsHTMLPanel{Title: copilotMetricsUserRatioTitle, Chart: template.HTML(b.String())}
}

func copilotMetricsFormatRatio(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func copilotMetricsUserSurfacePanel(users []copilotmetrics.UserRow) copilotMetricsHTMLPanel {
	counts := make(map[string]int64, len(copilotMetricsUserSurfaces))
	for _, user := range users {
		for _, surface := range user.Surfaces {
			counts[surface]++
		}
	}
	var maxCount int64
	for _, surface := range copilotMetricsUserSurfaces {
		maxCount = max(maxCount, counts[surface])
	}
	if maxCount == 0 {
		return copilotMetricsHTMLPanel{}
	}
	axisMax := maxCount + maxCount%2
	color := copilotMetricsThemeGradientColor(copilotMetricsThemeRose, 0, 1).hex()
	plotWidth := copilotMetricsUserColumnWidth - copilotMetricsUserColumnLeft - copilotMetricsUserColumnRight
	plotHeight := copilotMetricsUserColumnHeight - copilotMetricsUserColumnTop - copilotMetricsUserColumnBottom
	slot := plotWidth / float64(len(copilotMetricsUserSurfaces))
	barWidth := slot * (1 - copilotMetricsUserColumnBarSpacing)
	baseline := copilotMetricsUserColumnTop + plotHeight
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="metric-chart user-surface-chart" viewBox="0 0 %.0f %.0f" role="group" aria-label="%s" xmlns="http://www.w3.org/2000/svg">`,
		copilotMetricsUserColumnWidth, copilotMetricsUserColumnHeight, template.HTMLEscapeString(copilotMetricsUserSurfaceTitle))
	for _, fraction := range []float64{0, .5, 1} {
		y := baseline - plotHeight*fraction
		fmt.Fprintf(&b, `<line class="grid" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
			copilotMetricsUserColumnLeft, y, copilotMetricsUserColumnWidth-copilotMetricsUserColumnRight, y)
		fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			copilotMetricsUserColumnLeft-6, y+4, formatCopilotMetricsInt(axisMax*int64(fraction*2)/2))
	}
	for index, surface := range copilotMetricsUserSurfaces {
		count := counts[surface]
		x := copilotMetricsUserColumnLeft + slot*float64(index) + (slot-barWidth)/2
		barHeight := float64(count) / float64(axisMax) * plotHeight
		tooltip := copilotMetricsTooltip{
			Heading: surface,
			Rows: []copilotMetricsTooltipRow{{
				Label: "Users", Value: formatCopilotMetricsInt(count),
				Share: strconv.FormatFloat(float64(count)/float64(len(users))*100, 'f', 1, 64) + "%",
			}},
			Note: "Share of reported users.",
		}
		aria := template.HTMLEscapeString(tooltip.text())
		fmt.Fprintf(&b, `<g class="repository-bar" %s tabindex="0" role="img" aria-label="%s">`, tooltip.attributes(), aria)
		fmt.Fprintf(&b, `<rect class="repository-chart-hit" x="%.1f" y="%.1f" width="%.1f" height="%.1f" aria-hidden="true"/>`,
			x, copilotMetricsUserColumnTop, barWidth, plotHeight)
		if barHeight > 0 {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" aria-hidden="true"/>`,
				x, baseline-barHeight, barWidth, barHeight, color)
		}
		b.WriteString(`</g>`)
		fmt.Fprintf(&b, `<text class="axis" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			x+barWidth/2, baseline+18, template.HTMLEscapeString(surface))
	}
	b.WriteString(`</svg><p class="chart-color-key">Users can count toward multiple surfaces.</p>`)
	// #nosec G203 -- labels are fixed and escaped; coordinates and colors are generated internally.
	return copilotMetricsHTMLPanel{Title: copilotMetricsUserSurfaceTitle, Chart: template.HTML(b.String())}
}

// copilotMetricsUserBucketPanel renders users grouped into ordered buckets as a pie.
// Bucket colors depend on bucket position so they stay stable when buckets are empty.
func copilotMetricsUserBucketPanel(title string, users []copilotmetrics.UserRow, buckets []copilotMetricsUserBucket,
	value func(copilotmetrics.UserRow) (float64, bool), caption string) copilotMetricsHTMLPanel {
	counts := make([]int64, len(buckets))
	var total int64
	for _, user := range users {
		v, ok := value(user)
		if !ok {
			continue
		}
		for index, bucket := range buckets {
			if bucket.Contains(v) {
				counts[index]++
				total++
				break
			}
		}
	}
	if total == 0 {
		return copilotMetricsHTMLPanel{}
	}
	rows := make([]copilotmetrics.RichValue, 0, len(buckets))
	colors := make(map[string]string, len(buckets))
	for index, bucket := range buckets {
		colors[bucket.Name] = copilotMetricsThemeGradientColor(copilotMetricsThemeRose, index, len(buckets)).hex()
		if counts[index] > 0 {
			rows = append(rows, copilotmetrics.RichValue{Name: bucket.Name, Value: copilotmetrics.Counter{Value: counts[index], Valid: true}})
		}
	}
	chart := copilotMetricsPieChart(title, rows, float64(total), colors)
	// #nosec G203 -- the caption is a fixed internal string.
	chart += template.HTML(`<p class="chart-color-key">` + template.HTMLEscapeString(caption) + `</p>`)
	return copilotMetricsHTMLPanel{Title: title, Chart: chart}
}

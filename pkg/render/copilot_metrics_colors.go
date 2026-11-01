package render

import (
	"fmt"
	"math"
	"sort"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

type copilotMetricsColor struct {
	red   uint8
	green uint8
	blue  uint8
}

func (c copilotMetricsColor) hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.red, c.green, c.blue)
}

var copilotMetricsChartColors = [...]copilotMetricsColor{
	{0x09, 0x69, 0xda}, {0x1a, 0x7f, 0x37}, {0xbf, 0x39, 0x89}, {0xbc, 0x4c, 0x00},
	{0x82, 0x50, 0xdf}, {0x05, 0x50, 0xae}, {0x9a, 0x67, 0x00},
}

type copilotMetricsColorTheme int

const (
	copilotMetricsThemeBlue copilotMetricsColorTheme = iota
	copilotMetricsThemeOrange
	copilotMetricsThemeGreen
	copilotMetricsThemePurple
	copilotMetricsThemeRose
)

// copilotMetricsThemeRamps alternate dark and light steps so that adjacent
// categories stay distinguishable while sharing a single section hue.
var copilotMetricsThemeRamps = map[copilotMetricsColorTheme][]copilotMetricsColor{
	copilotMetricsThemeBlue: {
		{0x09, 0x69, 0xda}, {0x80, 0xcc, 0xff}, {0x0a, 0x30, 0x69}, {0x54, 0xae, 0xff},
		{0x05, 0x50, 0xae}, {0xa8, 0xdc, 0xff}, {0x21, 0x8b, 0xff},
	},
	copilotMetricsThemeOrange: {
		{0xbc, 0x4c, 0x00}, {0xff, 0xb7, 0x7c}, {0x76, 0x2c, 0x00}, {0xfb, 0x8f, 0x44},
		{0x95, 0x38, 0x00}, {0xff, 0xcf, 0xa3}, {0xe1, 0x6f, 0x24},
	},
	copilotMetricsThemeGreen: {
		{0x1a, 0x7f, 0x37}, {0x6f, 0xdd, 0x8b}, {0x04, 0x4f, 0x1e}, {0x4a, 0xc2, 0x6b},
		{0x11, 0x63, 0x29}, {0xac, 0xee, 0xbb}, {0x2d, 0xa4, 0x4e},
	},
	copilotMetricsThemePurple: {
		{0x82, 0x50, 0xdf}, {0xd8, 0xb9, 0xff}, {0x5a, 0x32, 0xa3}, {0xb8, 0x7f, 0xf7},
		{0x6f, 0x42, 0xc1}, {0xe5, 0xd5, 0xff}, {0x9a, 0x67, 0xdf},
	},
	copilotMetricsThemeRose: {
		{0xbe, 0x18, 0x5d}, {0xfb, 0xb6, 0xce}, {0x83, 0x18, 0x43}, {0xf4, 0x72, 0xa0},
		{0x9d, 0x17, 0x4d}, {0xfc, 0xd5, 0xe3}, {0xe4, 0x36, 0x78},
	},
}

var copilotMetricsThemeHues = map[copilotMetricsColorTheme]float64{
	copilotMetricsThemeBlue:   212,
	copilotMetricsThemeOrange: 25,
	copilotMetricsThemeGreen:  137,
	copilotMetricsThemePurple: 265,
	copilotMetricsThemeRose:   338,
}

func copilotMetricsThemeForSection(section copilotmetrics.Section) copilotMetricsColorTheme {
	if section == copilotmetrics.SectionLanguage {
		return copilotMetricsThemeOrange
	}
	return copilotMetricsThemeBlue
}

func copilotMetricsThemeForDimension(dimension string) copilotMetricsColorTheme {
	if dimension == "Language" {
		return copilotMetricsThemeOrange
	}
	return copilotMetricsThemeBlue
}

var copilotMetricsOtherColor = copilotMetricsColor{0x6e, 0x77, 0x81}

// copilotMetricsColorScheme maps the categories of one report section to
// colors from the section theme. Groups are the categories shown by daily
// area charts and table keys; series are the stacked segments, whose color is
// fixed for every bar.
type copilotMetricsColorScheme struct {
	groups map[string]copilotMetricsColor
	series map[string]copilotMetricsColor
	theme  copilotMetricsColorTheme
}

func (s *copilotMetricsColorScheme) group(name string) string {
	return s.groups[copilotMetricsNormalizeOther(name)].hex()
}

func (s *copilotMetricsColorScheme) seriesColor(name string) string {
	return s.series[copilotMetricsNormalizeOther(name)].hex()
}

func copilotMetricsAddColorCount(counts map[string]copilotmetrics.Counter, name string, value copilotmetrics.Counter) {
	name = copilotMetricsNormalizeOther(name)
	counter := counts[name]
	if value.Valid && value.Value >= 0 {
		counter.AddCounter(value)
	}
	counts[name] = counter
}

func copilotMetricsEnsureColorGroup(groups map[string]copilotmetrics.Counter, name string) {
	name = copilotMetricsNormalizeOther(name)
	if _, ok := groups[name]; !ok {
		groups[name] = copilotmetrics.Counter{}
	}
}

func copilotMetricsColorsForSection(s *copilotmetrics.Summary, section copilotmetrics.Section) *copilotMetricsColorScheme {
	groups := map[string]copilotmetrics.Counter{}
	series := map[string]copilotmetrics.Counter{}
	var dimensions []copilotmetrics.RichValue
	var daily []copilotmetrics.RichSeries
	switch section {
	case copilotmetrics.SectionModel:
		features := copilotMetricsModelFeatureValues(s)
		for _, row := range features {
			copilotMetricsAddColorCount(groups, row.Name, row.Value)
		}
		for _, row := range s.Models {
			copilotMetricsEnsureColorGroup(groups, row.Key)
		}
		dimensions = copilotMetricsChatModeModelValues(features)
		if s.Rich != nil {
			daily = s.Rich.ModelRequestsByDay
			for _, row := range s.Rich.ModelRequests {
				copilotMetricsEnsureColorGroup(groups, row.Name)
			}
			for _, row := range s.Rich.ModelRequestsByDay {
				copilotMetricsEnsureColorGroup(groups, row.Name)
			}
		}
	case copilotmetrics.SectionLanguage:
		languages := s.AllLanguages
		if languages == nil {
			languages = s.Languages
		}
		for _, row := range languages {
			copilotMetricsAddColorCount(groups, row.Key, row.CodeGeneration)
		}
		dimensions = copilotMetricsModelLanguageValues(s)
		if s.Rich != nil {
			daily = s.Rich.LanguageCodeGenerationByDay
			for _, row := range s.Rich.LanguageCodeGeneration {
				copilotMetricsEnsureColorGroup(groups, row.Name)
			}
			for _, row := range s.Rich.LanguageCodeGenerationByDay {
				copilotMetricsEnsureColorGroup(groups, row.Name)
			}
		}
	default:
		return nil
	}
	fallback := map[string]copilotmetrics.Counter{}
	for _, row := range dimensions {
		copilotMetricsAddColorCount(fallback, row.Group, row.Value)
		copilotMetricsAddColorCount(series, row.Name, row.Value)
	}
	for name, value := range fallback {
		if _, ok := groups[name]; !ok {
			groups[name] = value
		}
	}
	theme := copilotMetricsThemeForSection(section)
	scheme := copilotMetricsNewColorScheme(theme, groups, series)
	if len(daily) > 0 {
		displayed := copilotMetricsLimitPlotSeries(copilotMetricsCounterPlotSeries(daily), len(copilotMetricsChartColors))
		scheme.setAreaGroupColors(copilotMetricsAreaColors(theme, displayed))
	}
	return scheme
}

func copilotMetricsColorsForDimensions(values []copilotmetrics.RichValue, theme copilotMetricsColorTheme) *copilotMetricsColorScheme {
	groups := map[string]copilotmetrics.Counter{}
	series := map[string]copilotmetrics.Counter{}
	for _, row := range values {
		copilotMetricsAddColorCount(groups, row.Group, row.Value)
		copilotMetricsAddColorCount(series, row.Name, row.Value)
	}
	return copilotMetricsNewColorScheme(theme, groups, series)
}

func copilotMetricsNewColorScheme(theme copilotMetricsColorTheme, groups, series map[string]copilotmetrics.Counter) *copilotMetricsColorScheme {
	scheme := &copilotMetricsColorScheme{
		groups: map[string]copilotMetricsColor{"Other": copilotMetricsOtherColor},
		series: map[string]copilotMetricsColor{"Other": copilotMetricsOtherColor},
		theme:  theme,
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		if name != "Other" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if groups[names[i]].Value != groups[names[j]].Value {
			return groups[names[i]].Value > groups[names[j]].Value
		}
		return names[i] < names[j]
	})
	for name, color := range copilotMetricsThemeColors(theme, names) {
		scheme.groups[name] = color
	}

	values := make([]copilotmetrics.RichValue, 0, len(series))
	for name, value := range series {
		values = append(values, copilotmetrics.RichValue{Name: name, Value: value})
	}
	visible := copilotMetricsLimitRichValues(values)
	order := make([]string, 0, len(series)+1)
	for _, row := range visible {
		order = append(order, row.Name)
	}
	sort.Slice(order, func(i, j int) bool {
		if (order[i] == "Other") != (order[j] == "Other") {
			return order[j] == "Other"
		}
		return order[i] < order[j]
	})
	seen := map[string]bool{}
	for _, name := range order {
		seen[name] = true
	}
	var remaining []string
	for name := range series {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	order = append(order, remaining...)
	named := order[:0]
	for _, name := range order {
		if name != "Other" {
			named = append(named, name)
		}
	}
	for name, color := range copilotMetricsThemeColors(theme, named) {
		scheme.series[name] = color
	}
	return scheme
}

// copilotMetricsThemeColors assigns a distinct theme color to each name in
// order, never reusing the gray reserved for Other.
func copilotMetricsThemeColors(theme copilotMetricsColorTheme, names []string) map[string]copilotMetricsColor {
	colors := make(map[string]copilotMetricsColor, len(names))
	used := map[copilotMetricsColor]bool{copilotMetricsOtherColor: true}
	ramp := copilotMetricsThemeRamps[theme]
	nextGenerated := 0
	for i, name := range names {
		var color copilotMetricsColor
		if i < len(ramp) {
			color = ramp[i]
		} else {
			for {
				color = copilotMetricsGeneratedColor(theme, nextGenerated)
				nextGenerated++
				if !used[color] {
					break
				}
			}
		}
		colors[name] = color
		used[color] = true
	}
	return colors
}

func copilotMetricsShadeLevel(index int) float64 {
	if index == 0 {
		return .06
	}
	if index == 1 {
		return .64
	}
	var fraction float64
	denominator := 2.0
	for n := index - 1; n > 0; n /= 2 {
		fraction += float64(n%2) / denominator
		denominator *= 2
	}
	return .06 + .58*fraction
}

func copilotMetricsGeneratedColor(theme copilotMetricsColorTheme, index int) copilotMetricsColor {
	hue := math.Mod(copilotMetricsThemeHues[theme]+float64(index%5-2)*6+360, 360)
	lightness := .22 + .6*copilotMetricsShadeLevel(index+2)
	return copilotMetricsHSLColor(hue, .8, lightness)
}

func copilotMetricsThemeGradientColor(theme copilotMetricsColorTheme, index, count int) copilotMetricsColor {
	lightness := .34
	if count > 1 {
		lightness += .44 * float64(index) / float64(count-1)
	}
	return copilotMetricsHSLColor(copilotMetricsThemeHues[theme], .8, lightness)
}

// copilotMetricsHueGradientColor walks the hue wheel from blue through green
// and yellow toward red and purple, keeping saturation and lightness fixed so
// that rank is conveyed by hue rather than by shade.
func copilotMetricsHueGradientColor(index, count int) copilotMetricsColor {
	const startHue, hueSpan = 215.0, 280.0
	hue := startHue
	if count > 1 {
		hue -= hueSpan * float64(index) / float64(count-1)
	}
	return copilotMetricsHSLColor(math.Mod(hue+360, 360), .68, .52)
}

// copilotMetricsAreaHueColors assigns stacked-area colors by series rank
// using a hue gradient independent of the section color theme.
func copilotMetricsAreaHueColors(series []copilotMetricsPlotSeries) map[string]copilotMetricsColor {
	return copilotMetricsRankedAreaColors(series, copilotMetricsHueGradientColor)
}

func copilotMetricsAreaColors(theme copilotMetricsColorTheme, series []copilotMetricsPlotSeries) map[string]copilotMetricsColor {
	return copilotMetricsRankedAreaColors(series, func(index, count int) copilotMetricsColor {
		return copilotMetricsThemeGradientColor(theme, index, count)
	})
}

func copilotMetricsRankedAreaColors(series []copilotMetricsPlotSeries, colorAt func(index, count int) copilotMetricsColor) map[string]copilotMetricsColor {
	names := make([]string, 0, len(series))
	seen := map[string]bool{"Other": true}
	for _, item := range series {
		name := copilotMetricsNormalizeOther(item.Name)
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	colors := map[string]copilotMetricsColor{"Other": copilotMetricsOtherColor}
	for rank, name := range names {
		colors[name] = colorAt(rank, len(names))
	}
	return colors
}

func (s *copilotMetricsColorScheme) setAreaGroupColors(areaColors map[string]copilotMetricsColor) {
	used := make(map[copilotMetricsColor]bool, len(s.groups))
	for name, color := range areaColors {
		s.groups[name] = color
		used[color] = true
	}
	var remaining []string
	for name := range s.groups {
		if _, fixed := areaColors[name]; !fixed {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	nextGenerated := 0
	for _, name := range remaining {
		color := s.groups[name]
		if used[color] {
			for {
				color = copilotMetricsGeneratedColor(s.theme, nextGenerated)
				nextGenerated++
				if !used[color] {
					break
				}
			}
			s.groups[name] = color
		}
		used[color] = true
	}
}

func copilotMetricsHSLColor(hue, saturation, lightness float64) copilotMetricsColor {
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	m := lightness - chroma/2
	var red, green, blue float64
	switch {
	case hue < 60:
		red, green = chroma, x
	case hue < 120:
		red, green = x, chroma
	case hue < 180:
		green, blue = chroma, x
	case hue < 240:
		green, blue = x, chroma
	case hue < 300:
		red, blue = x, chroma
	default:
		red, blue = chroma, x
	}
	return copilotMetricsColor{
		red: uint8(math.Round((red + m) * 255)), green: uint8(math.Round((green + m) * 255)), blue: uint8(math.Round((blue + m) * 255)),
	}
}

package render

import (
	"fmt"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
)

const (
	copilotMetricsTeamInteractionsTitle   = "Team usage (interactions)"
	copilotMetricsTeamCodeGenerationTitle = "Team usage (code-generation activities)"
)

func copilotMetricsTeamPanels(teams []copilotmetrics.TeamRow) []copilotMetricsHTMLPanel {
	names := make([]string, len(teams))
	nameCounts := make(map[string]int, len(teams))
	for index, team := range teams {
		name := team.Slug
		if name == "" {
			name = copilotMetricsUnreportedDimension
		}
		names[index] = name
		nameCounts[name]++
	}

	interactions := make([]copilotmetrics.RichValue, len(teams))
	generation := make([]copilotmetrics.RichValue, len(teams))
	for index, team := range teams {
		name := names[index]
		if nameCounts[name] > 1 || copilotMetricsNormalizeOther(name) == "Other" {
			name = fmt.Sprintf("%s (team %d)", name, team.TeamID)
		}
		interactions[index] = copilotmetrics.RichValue{Name: name, Value: team.UserInitiatedInteraction}
		generation[index] = copilotmetrics.RichValue{Name: name, Value: team.CodeGeneration}
	}

	panels := make([]copilotMetricsHTMLPanel, 0, 2)
	purpleScheme := &copilotMetricsColorScheme{theme: copilotMetricsThemePurple}
	for _, panel := range []copilotMetricsHTMLPanel{
		copilotMetricsHTMLRichPiePanelWithColors(copilotMetricsTeamInteractionsTitle, interactions, purpleScheme),
		copilotMetricsHTMLRichPiePanelWithColors(copilotMetricsTeamCodeGenerationTitle, generation, purpleScheme),
	} {
		if panel.Chart != "" {
			panel.Chart += `<p class="chart-color-key">Shares of reported team counts; missing values excluded.</p>`
			panels = append(panels, panel)
		}
	}
	if len(panels) == 2 {
		panels[0].HalfWidth = true
		panels[1].HalfWidth = true
	}
	return panels
}

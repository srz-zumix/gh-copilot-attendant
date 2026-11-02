package render

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/sessionstats"
	sharedrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

// RenderCopilotSessionStats preserves the shared output and adds Fusion statistics.
func RenderCopilotSessionStats(r *sharedrender.Renderer, stats *sessionstats.Stats) error {
	if stats == nil {
		return nil
	}
	if r.HasExporter() {
		return r.RenderExportedData(stats)
	}
	if err := r.RenderCopilotPermissionStats(stats.PermissionStats); err != nil {
		return err
	}

	fusion := stats.Fusion
	r.WriteLine("")
	r.WriteLine("FUSION")
	r.WriteLine(fmt.Sprintf("sessions: %d, resolved_turns: %d, unidentified_resolutions: %d, turns_with_usage: %d, phases_with_usage: %d",
		fusion.Sessions, fusion.ResolvedTurns, fusion.UnidentifiedResolutions, fusion.TurnsWithUsage, fusion.PhasesWithUsage))
	r.WriteLine(fmt.Sprintf("usage_records: %d, recorded_aiu: %s (%d/%d records)",
		fusion.UsageRecords, formatFusionAIU(fusion.AIU, fusion.AIURecords, fusion.UsageRecords), fusion.AIURecords, fusion.UsageRecords))
	r.WriteLine(fmt.Sprintf("records_missing_fusion_id: %d, records_missing_phase_identity: %d",
		fusion.MissingFusionIDRecords, fusion.MissingPhaseIdentityRecords))
	r.WriteLine("counts use recorded Fusion events only; missing metadata and checkpoint-only usage cannot be attributed")
	r.WriteLine("time bounds select individual events; phase and model rows may share turns or phases")
	r.WriteLine("active sessions may change while scanning; totals are not an atomic snapshot")

	if len(fusion.ByPattern) > 0 {
		r.WriteLine("")
		r.WriteLine("FUSION_PATTERN")
		table := r.NewTableWriter([]string{"PATTERN", "SESSIONS", "RESOLVED_TURNS", "UNIDENTIFIED_RESOLUTIONS"})
		for _, entry := range fusion.ByPattern {
			table.Append([]string{entry.Key, strconv.Itoa(entry.Sessions), strconv.Itoa(entry.ResolvedTurns), strconv.Itoa(entry.UnidentifiedResolutions)})
		}
		if err := table.Render(); err != nil {
			return err
		}
	}
	if len(fusion.ByPhase) > 0 {
		r.WriteLine("")
		r.WriteLine("FUSION_PHASE")
		table := r.NewTableWriter([]string{"PATTERN", "PHASE", "ROLE", "MODEL", "SESSIONS", "TURNS", "PHASES", "USAGE_RECORDS", "AIU", "AIU_RECORDS", "MISSING_IDENTITIES"})
		for _, entry := range fusion.ByPhase {
			table.Append([]string{
				entry.Pattern, entry.Phase, entry.Role, entry.Model, strconv.Itoa(entry.Sessions),
				strconv.Itoa(entry.Turns), strconv.Itoa(entry.Phases), strconv.Itoa(entry.UsageRecords),
				formatFusionAIU(entry.AIU, entry.AIURecords, entry.UsageRecords),
				fmt.Sprintf("%d/%d", entry.AIURecords, entry.UsageRecords),
				strconv.Itoa(entry.MissingPhaseIdentityRecords),
			})
		}
		if err := table.Render(); err != nil {
			return err
		}
	}
	return nil
}

func formatFusionAIU(aiu float64, recorded, total int) string {
	if recorded == 0 {
		return "n/a"
	}
	value := strings.TrimRight(strconv.FormatFloat(aiu, 'f', 9, 64), "0")
	if strings.HasSuffix(value, ".") {
		value += "000"
	} else if fraction := len(value) - strings.LastIndexByte(value, '.') - 1; fraction < 3 {
		value += strings.Repeat("0", 3-fraction)
	}
	if recorded < total {
		return ">= " + value
	}
	return value
}

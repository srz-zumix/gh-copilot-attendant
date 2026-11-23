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

	if err := renderHookActivity(r, stats.HookActivity); err != nil {
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

func renderHookActivity(r *sharedrender.Renderer, activity sessionstats.HookActivity) error {
	if activity.Version == 0 {
		return nil
	}
	r.WriteLine("")
	r.WriteLine("HOOK_ACTIVITY")
	r.WriteLine("log: " + activity.State)
	if activity.Data == nil {
		r.WriteLine("recorded invocations are unavailable; this does not establish that no hooks ran")
		return nil
	}
	data := activity.Data
	if activity.State == "partial" {
		r.WriteLine("log read was incomplete; counts cover only the readable part")
	}
	r.WriteLine(fmt.Sprintf("recorded_invocations: %d, permission_request: %d, pre_tool_use: %d",
		data.Invocations, data.PermissionRequests, data.PreToolUses))
	r.WriteLine(fmt.Sprintf("decisions: allow %d, deny %d",
		data.Decisions.Allow, data.Decisions.Deny))
	r.WriteLine(fmt.Sprintf("output: write_succeeded %d, write_failed %d",
		data.Outputs.WriteSucceeded, data.Outputs.WriteFailed))
	r.WriteLine(fmt.Sprintf("processing: success %d, output_error %d",
		data.Processing.Success, data.Processing.OutputError))
	r.WriteLine(fmt.Sprintf("unidentified_in_time_window: missing %d, unmatched %d, ambiguous %d, unscopable %d, attribution_unavailable %d",
		data.Unidentified.Missing, data.Unidentified.Unmatched, data.Unidentified.Ambiguous,
		data.Unidentified.Unscopable, data.Unidentified.Unavailable))
	r.WriteLine(fmt.Sprintf("file_coverage: malformed %d, unsupported_version %d; session_attribution: %s",
		data.MalformedRecords, data.UnsupportedVersions, data.Attribution))
	r.WriteLine("unidentified counts cover the whole time window; unscoped totals already include them, while scoped totals exclude them")
	r.WriteLine("malformed and unsupported records are counted across the file because their time or scope may be unknown")
	r.WriteLine("only recorded allow/deny hook decisions are counted; output does not prove CLI acceptance or cover missed hooks")
	r.WriteLine("session scope and record-time bounds apply; permission filters do not; top limits tool and target rows")
	if len(data.ByTool) > 0 {
		r.WriteLine("")
		r.WriteLine("HOOK_TOOL")
		table := r.NewTableWriter([]string{"TOOL", "INVOCATIONS"})
		for _, entry := range data.ByTool {
			key := entry.Key
			if key == "" {
				key = "<unknown>"
			}
			table.Append([]string{key, strconv.Itoa(entry.Total)})
		}
		if err := table.Render(); err != nil {
			return err
		}
	}
	if len(data.ByTarget) == 0 {
		return nil
	}
	r.WriteLine("")
	r.WriteLine("HOOK_TARGET")
	table := r.NewTableWriter([]string{"DECISION", "TOOL", "TARGET", "INVOCATIONS"})
	for _, entry := range data.ByTarget {
		table.Append([]string{entry.Decision, hookToolLabel(entry.Tool), hookTargetLabel(entry), strconv.Itoa(entry.Total)})
	}
	return table.Render()
}

func hookToolLabel(tool string) string {
	if tool == "" {
		return "<unknown>"
	}
	return tool
}

// hookTargetLabel renders a target on one line and marks truncated or missing targets.
func hookTargetLabel(entry sessionstats.HookTargetCount) string {
	if entry.Target == "" && !entry.Truncated {
		return "<not recorded>"
	}
	label := strings.Join(strings.Fields(entry.Target), " ")
	if entry.Truncated {
		label += " …"
	}
	return label
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

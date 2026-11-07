package render

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/sessionstats"
	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
	sharedrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

func sampleSessionStats() *sessionstats.Stats {
	return &sessionstats.Stats{
		PermissionStats: &copilotext.PermissionStats{Sessions: 2, Requests: 3, UsageSessions: 1, UsageAIU: 6},
		Fusion: sessionstats.FusionStats{
			Sessions: 1, ResolvedTurns: 1, TurnsWithUsage: 1, PhasesWithUsage: 1,
			UsageRecords: 2, AIU: 3, AIURecords: 1,
			ByPattern: []sessionstats.FusionPattern{{Key: "cascade", Sessions: 1, ResolvedTurns: 1}},
			ByPhase: []sessionstats.FusionPhase{{
				Pattern: "cascade", Phase: "judge", Role: "judge", Model: "model-a",
				Sessions: 1, Turns: 1, Phases: 1, UsageRecords: 2, AIU: 3, AIURecords: 1,
			}},
		},
		SessionActivity: sessionstats.SessionActivity{
			Version: 1, TimeZone: "UTC", AsOf: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			StartDate: "2026-01-01", EndDate: "2026-01-01",
			Days: []sessionstats.ActivityDay{{Date: "2026-01-01", Hours: [24]int{0, 2}}},
		},
	}
}

func TestRenderCopilotSessionStatsTable(t *testing.T) {
	sr := sharedrender.NewStringRenderer(nil)
	if err := RenderCopilotSessionStats(&sr.Renderer, sampleSessionStats()); err != nil {
		t.Fatal(err)
	}
	output := sr.Stdout.String()
	for _, part := range []string{
		"SUMMARY", "sessions: 2, requests: 3", "FUSION", "resolved_turns: 1", ">= 3.000",
		"FUSION_PATTERN", "FUSION_PHASE", "cascade", "judge", "model-a", "1/2",
		"checkpoint-only usage cannot be attributed", "not an atomic snapshot",
	} {
		if !strings.Contains(output, part) {
			t.Errorf("table output missing %q:\n%s", part, output)
		}
	}
	for _, test := range []struct {
		value           float64
		recorded, total int
		want            string
	}{
		{0, 0, 1, "n/a"}, {0, 1, 1, "0.000"}, {0, 1, 2, ">= 0.000"},
		{1.5, 2, 2, "1.500"}, {0.000000001, 1, 1, "0.000000001"},
	} {
		if got := formatFusionAIU(test.value, test.recorded, test.total); got != test.want {
			t.Errorf("formatFusionAIU(%v, %d, %d) = %q, want %q", test.value, test.recorded, test.total, got, test.want)
		}
	}
}

func TestRenderCopilotSessionStatsJSON(t *testing.T) {
	sr := sharedrender.NewStringRenderer(cmdutil.NewJSONExporter())
	if err := RenderCopilotSessionStats(&sr.Renderer, sampleSessionStats()); err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(sr.Stdout.Bytes(), &data); err != nil {
		t.Fatalf("output must be one JSON document: %v", err)
	}
	var sessions int
	if err := json.Unmarshal(data["Sessions"], &sessions); err != nil || sessions != 2 {
		t.Fatalf("shared Sessions = %d, error %v", sessions, err)
	}
	var fusion sessionstats.FusionStats
	if err := json.Unmarshal(data["Fusion"], &fusion); err != nil || fusion.UsageRecords != 2 ||
		fusion.AIURecords != 1 || len(fusion.ByPhase) != 1 || fusion.ByPhase[0].Model != "model-a" {
		t.Fatalf("Fusion = %+v, error %v", fusion, err)
	}
	var activity sessionstats.SessionActivity
	if err := json.Unmarshal(data["SessionActivity"], &activity); err != nil || activity.Version != 1 ||
		activity.TimeZone != "UTC" || len(activity.Days) != 1 || activity.Days[0].Hours[1] != 2 {
		t.Fatalf("SessionActivity = %+v, error %v", activity, err)
	}
}

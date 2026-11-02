package sessionstats

import (
	"reflect"
	"testing"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

func assertFusionMetric[T int64 | float64](t *testing.T, name string, got FusionUsageMetric[T], total T, observed, complete int) {
	t.Helper()
	want := FusionUsageMetric[T]{Total: total, ObservedRecords: observed, CompleteRecords: complete}
	if got != want {
		t.Errorf("%s = %+v, want %+v", name, got, want)
	}
}

func TestFusionUsageAggregatesOnlyTaggedRecordsWithCoverage(t *testing.T) {
	root := t.TempDir()
	before := "2026-01-01T00:00:00Z"
	start := "2026-01-02T00:00:00Z"
	end := "2026-01-03T00:00:00Z"
	record := func(id, at string, fields map[string]any) string {
		fields["accounting"] = map[string]any{"sourceSessionId": "source-a", "usageId": id}
		return testEvent(t, "session.usage_record", id, at, map[string]any{"usage": fields})
	}
	tag := func(turn, phase string) map[string]any {
		return map[string]any{"fusionId": turn, "phaseId": phase, "pattern": "cascade", "phaseKind": "primary", "role": "solver"}
	}
	first := record("record-a", start, map[string]any{
		"model": "model-a", "initiator": "user", "inputTokens": 100, "cacheReadTokens": 20,
		"cacheWriteTokens": 30, "outputTokens": 10, "cost": 1.5, "duration": 250,
		"copilotUsage": map[string]any{"totalNanoAiu": int64(2e9)}, "fusion": tag("turn-a", "phase-a"),
	})
	second := record("record-b", start, map[string]any{
		"model": "model-b", "initiator": "sub-agent", "apiCallId": "shared-call", "cost": 999, "duration": 0,
		"copilotUsage": map[string]any{"totalNanoAiu": int64(1e9), "tokenDetails": []map[string]any{
			{"tokenType": "input", "tokenCount": 5}, {"tokenType": "cache_read", "tokenCount": 10},
			{"tokenType": "cache_write", "tokenCount": 0}, {"tokenType": "output", "tokenCount": 3},
		}}, "fusion": tag("turn-a", "phase-b"),
	})
	third := record("record-c", start, map[string]any{
		"model": "model-b", "initiator": "future-agent", "apiCallId": "shared-call",
		"inputTokens": 0, "fusion": map[string]any{},
	})
	untagged := record("ordinary", start, map[string]any{
		"model": "model-a", "inputTokens": 9999, "cost": 9999,
		"copilotUsage": map[string]any{"totalNanoAiu": int64(9999e9)},
	})
	prior := record("replayed", before, map[string]any{"model": "model-a"})
	replay := record("replayed", start, map[string]any{
		"model": "model-a", "copilotUsage": map[string]any{"totalNanoAiu": int64(9999e9)}, "fusion": tag("turn", "phase"),
	})
	outside := record("outside", end, map[string]any{
		"model": "model-a", "copilotUsage": map[string]any{"totalNanoAiu": int64(9999e9)}, "fusion": tag("turn", "phase"),
	})
	snapshot := testEvent(t, "session.usage_checkpoint", "snapshot", start, map[string]any{"totalNanoAiu": int64(9999e9)})
	writeSession(t, root, "session-a", "/work/a",
		first+first+second+third+untagged+prior+replay+outside+snapshot)
	writeSession(t, root, "session-b", "/work/b", record("record-d", start, map[string]any{
		"model": "model-a", "initiator": "user", "inputTokens": 0, "cacheReadTokens": 0,
		"cacheWriteTokens": 0, "outputTokens": 0, "cost": 0, "duration": 0,
		"copilotUsage": map[string]any{"totalNanoAiu": 0}, "fusion": tag("turn-b", "phase-a"),
	}))
	writeSession(t, root, "session-unknown-cwd", "", record("record-e", start, map[string]any{
		"model": "model-c", "initiator": "future-agent", "copilotUsage": map[string]any{
			"tokenDetails": []map[string]any{
				{"tokenType": "input", "tokenCount": 2}, {"tokenType": "input", "tokenCount": nil},
				{"tokenType": "cache_read", "tokenCount": 1}, {"tokenType": "cache_write", "tokenCount": 0},
				{"tokenType": "output", "tokenCount": 4}, {"tokenType": "future", "tokenCount": 9},
			},
		}, "fusion": map[string]any{},
	}))
	writeSession(t, root, "resolution-only", "/work/c", resolution(t, "resolved", start, "turn-c", "single"))

	since, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatal(err)
	}
	until, err := time.Parse(time.RFC3339, end)
	if err != nil {
		t.Fatal(err)
	}
	opts := copilotext.PermissionStatsOptions{Root: root, Since: since, Until: until}
	all, err := CollectFusion(opts)
	if err != nil {
		t.Fatal(err)
	}
	data := all.Usage
	if data.Version != 1 || data.Sessions != 3 || data.UsageRecords != 5 || all.Sessions != 4 ||
		all.UsageRecords != data.UsageRecords || all.AIU != 3 || all.AIURecords != 3 {
		t.Fatalf("record-level Fusion totals = %+v; Fusion = %+v", data, all)
	}
	assertFusionMetric(t, "AIU", data.Metrics.AIU, 3, 3, 3)
	if got, want := data.Metrics.PremiumRequests, (FusionUsageMetric[float64]{
		Total: 1.5, ObservedRecords: 2, CompleteRecords: 2, ExcludedRecords: 3,
	}); got != want {
		t.Errorf("premium multipliers = %+v, want %+v", got, want)
	}
	assertFusionMetric(t, "prompt", data.Metrics.PromptTokens, 118, 5, 4)
	assertFusionMetric(t, "uncached input", data.Metrics.UncachedInputTokens, 57, 4, 3)
	assertFusionMetric(t, "cache read", data.Metrics.CacheReadTokens, 31, 4, 4)
	assertFusionMetric(t, "cache write", data.Metrics.CacheWriteTokens, 30, 4, 4)
	assertFusionMetric(t, "output", data.Metrics.OutputTokens, 17, 4, 3)
	assertFusionMetric(t, "API duration", data.Metrics.APIDurationMs, 250, 3, 3)
	if data.ModelGroups != 3 || data.CWDGroups != 3 || data.RoleGroups != 3 || data.PhaseGroups != 4 {
		t.Fatalf("group counts = %+v", data)
	}
	if len(data.ByModel) != 3 || data.ByModel[0].Key != "model-a" ||
		data.ByModel[0].Sessions != 2 || data.ByModel[0].UsageRecords != 2 ||
		data.ByModel[1].Key != "model-b" || data.ByModel[2].Key != "model-c" {
		t.Fatalf("model groups = %+v", data.ByModel)
	}
	if len(data.ByCWD) != 3 || data.ByCWD[0].Key != "/work/a" || data.ByCWD[2].Key != "" {
		t.Fatalf("workspace groups = %+v", data.ByCWD)
	}
	if len(data.ByRole) != 3 || data.ByRole[0].Key != "main" ||
		data.ByRole[1].Key != "subagent" || data.ByRole[2].Key != "unknown" {
		t.Fatalf("request provenance groups = %+v", data.ByRole)
	}

	opts.Top = 1
	limited, err := CollectFusion(opts)
	if err != nil {
		t.Fatal(err)
	}
	data = limited.Usage
	if len(data.ByModel) != 1 || len(data.ByCWD) != 1 || len(data.ByRole) != 1 ||
		len(limited.ByPhase) != 1 || data.ModelGroups != 3 || data.CWDGroups != 3 ||
		data.RoleGroups != 3 || data.PhaseGroups != 4 || data.Sessions != 3 ||
		data.UsageRecords != 5 || !reflect.DeepEqual(data.Metrics, all.Usage.Metrics) {
		t.Fatalf("Top must independently limit groups without changing totals: %+v", data)
	}
}

func TestFusionUsageRoleUsesRequestProvenance(t *testing.T) {
	tests := []struct {
		name  string
		usage rawUsage
		event rawEvent
		want  string
	}{
		{name: "missing provenance", want: "unknown"},
		{name: "main agent", usage: rawUsage{Initiator: "agent"}, want: "main"},
		{name: "event agent", event: rawEvent{AgentID: "agent-a"}, want: "subagent"},
		{name: "parent tool", usage: rawUsage{ParentToolCallID: "tool-a"}, want: "subagent"},
		{name: "interaction", usage: rawUsage{InteractionType: "conversation-subagent"}, want: "subagent"},
		{name: "initiator", usage: rawUsage{Initiator: "sub-agent"}, want: "subagent"},
		{name: "unknown", usage: rawUsage{Initiator: "future"}, want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.usage.role(tt.event); got != tt.want {
				t.Fatalf("role = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFusionUsageKeepsPartialRecordsWithUnknownModelOrInput(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	partial := testEvent(t, "session.usage_record", "partial", at, map[string]any{"usage": map[string]any{
		"cacheReadTokens": 20, "copilotUsage": map[string]any{"totalNanoAiu": int64(1e9)},
		"fusion": map[string]any{},
	}})
	invalid := testEvent(t, "session.usage_record", "invalid", at, map[string]any{"usage": map[string]any{
		"model": "model-a", "inputTokens": 10, "cacheReadTokens": 20,
		"copilotUsage": map[string]any{"totalNanoAiu": int64(5e9)}, "fusion": map[string]any{},
	}})
	writeSession(t, root, "session-a", "", partial+invalid)
	stats, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	data := stats.Usage
	if data.Sessions != 1 || data.UsageRecords != 1 || len(data.ByModel) != 1 || data.ByModel[0].Key != "" ||
		len(data.ByCWD) != 1 || data.ByCWD[0].Key != "" || len(data.ByRole) != 1 || data.ByRole[0].Key != "unknown" {
		t.Fatalf("partial Fusion record = %+v", data)
	}
	assertFusionMetric(t, "recorded AIU", data.Metrics.AIU, 1, 1, 1)
	assertFusionMetric(t, "recorded cache read", data.Metrics.CacheReadTokens, 20, 1, 1)
	assertFusionMetric(t, "unavailable prompt", data.Metrics.PromptTokens, 0, 0, 0)
	assertFusionMetric(t, "unavailable uncached input", data.Metrics.UncachedInputTokens, 0, 0, 0)
}

func TestFusionUsageEmptyDatasetIsSupported(t *testing.T) {
	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got.Usage.Version != 1 || got.Usage.Sessions != 0 || got.Usage.UsageRecords != 0 ||
		got.Usage.ByModel == nil || got.Usage.ByCWD == nil || got.Usage.ByRole == nil {
		t.Fatalf("empty Fusion usage dataset = %+v", got.Usage)
	}
}

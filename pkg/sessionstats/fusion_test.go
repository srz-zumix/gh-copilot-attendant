package sessionstats

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

func testEvent(t *testing.T, eventType, id, at string, data any) string {
	t.Helper()
	value, err := json.Marshal(map[string]any{"type": eventType, "id": id, "timestamp": at, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return string(value) + "\n"
}

func resolution(t *testing.T, id, at, fusionID, pattern string) string {
	t.Helper()
	return testEvent(t, "session.fusion_resolved", id, at, map[string]any{"fusionId": fusionID, "pattern": pattern})
}

func usage(t *testing.T, id, at, source, usageID, model, pattern, fusionID, phaseID, phase, role string, nano *int64) string {
	t.Helper()
	u := map[string]any{
		"model": model, "initiator": "user", "inputTokens": 10, "cacheReadTokens": 5,
		"cost": 1, "accounting": map[string]any{"sourceSessionId": source, "usageId": usageID},
		"fusion": map[string]any{"pattern": pattern, "fusionId": fusionID, "phaseId": phaseID, "phaseKind": phase, "role": role},
	}
	if nano != nil {
		u["copilotUsage"] = map[string]any{"totalNanoAiu": *nano}
	}
	return testEvent(t, "session.usage_record", id, at, map[string]any{"usage": u})
}

func writeSession(t *testing.T, root, name, cwd, events string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf("id: %q\ncwd: %q\n", name, cwd)
	if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o600); err != nil {
		t.Fatal(err)
	}
}

func int64ptr(value int64) *int64 { return &value }

func TestCollectFusionRecordedActivity(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	first := usage(t, "event-1", at, "source-a", "usage-1", "model-a", "cascade", "turn-a", "phase-a", "primary", "solver", int64ptr(1e9))
	events := resolution(t, "resolution-a", at, "turn-a", "cascade") +
		resolution(t, "resolution-replay", at, "turn-a", "cascade") +
		resolution(t, "resolution-b", at, "turn-b", "single") +
		resolution(t, "resolution-unknown", at, "", "future-pattern") +
		first + first +
		usage(t, "event-2", at, "source-a", "usage-2", "model-b", "cascade", "turn-a", "phase-b", "judge", "judge", int64ptr(2e9)) +
		usage(t, "event-3", at, "source-a", "usage-3", "model-a", "cascade", "turn-a", "phase-b", "judge", "judge", nil) +
		usage(t, "event-4", at, "source-a", "usage-4", "model-a", "single", "", "", "primary", "solver", int64ptr(0)) +
		testEvent(t, "session.usage_checkpoint", "snapshot", at, map[string]any{"totalNanoAiu": int64(900e9)})
	writeSession(t, root, "session-a", "/work/a", events)
	writeSession(t, root, "session-b", "/work/b",
		resolution(t, "resolution-c", at, "turn-a", "cascade")+
			usage(t, "event-5", at, "source-a", "usage-1", "model-a", "cascade", "turn-a", "phase-a", "primary", "solver", int64ptr(1e9)))

	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sessions != 2 || got.ResolvedTurns != 3 || got.UnidentifiedResolutions != 1 ||
		got.TurnsWithUsage != 2 || got.PhasesWithUsage != 3 || got.UsageRecords != 5 ||
		got.AIU != 4 || got.AIURecords != 4 || got.MissingFusionIDRecords != 1 || got.MissingPhaseIdentityRecords != 1 {
		t.Fatalf("Fusion totals = %+v", got)
	}
	wantPatterns := []FusionPattern{
		{Key: "cascade", Sessions: 2, ResolvedTurns: 2},
		{Key: "single", Sessions: 1, ResolvedTurns: 1},
		{Key: "future-pattern", Sessions: 1, UnidentifiedResolutions: 1},
	}
	if !reflect.DeepEqual(got.ByPattern, wantPatterns) {
		t.Fatalf("patterns = %+v, want %+v", got.ByPattern, wantPatterns)
	}
	wantPhases := []FusionPhase{
		{Pattern: "cascade", Phase: "primary", Role: "solver", Model: "model-a", Sessions: 2, Turns: 2, Phases: 2, UsageRecords: 2, AIU: 2, AIURecords: 2},
		{Pattern: "cascade", Phase: "judge", Role: "judge", Model: "model-a", Sessions: 1, Turns: 1, Phases: 1, UsageRecords: 1},
		{Pattern: "cascade", Phase: "judge", Role: "judge", Model: "model-b", Sessions: 1, Turns: 1, Phases: 1, UsageRecords: 1, AIU: 2, AIURecords: 1},
		{Pattern: "single", Phase: "primary", Role: "solver", Model: "model-a", Sessions: 1, UsageRecords: 1, AIURecords: 1, MissingFusionIDRecords: 1, MissingPhaseIdentityRecords: 1},
	}
	if !reflect.DeepEqual(got.ByPhase, wantPhases) {
		t.Fatalf("phases = %+v, want %+v", got.ByPhase, wantPhases)
	}

	limited, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root, Top: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.ByPattern) != 1 || len(limited.ByPhase) != 1 || limited.Sessions != got.Sessions ||
		limited.ResolvedTurns != got.ResolvedTurns || limited.PhasesWithUsage != got.PhasesWithUsage ||
		limited.UsageRecords != got.UsageRecords || limited.AIURecords != got.AIURecords || limited.AIU != got.AIU {
		t.Fatalf("top-limited Fusion totals = %+v", limited)
	}
}

func TestCollectFusionScopeTimeAndFilters(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	child := filepath.Join(project, "child")
	sibling := project + "-other"
	for _, path := range []string{project, child, sibling} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := "2026-01-01T00:00:00Z"
	start := "2026-01-02T00:00:00Z"
	end := "2026-01-03T00:00:00Z"
	events := resolution(t, "resolution-before", before, "turn-before", "cascade") +
		resolution(t, "resolution-start", start, "turn-start", "single") +
		resolution(t, "resolution-end", end, "turn-end", "single") +
		usage(t, "usage-before", before, "source", "replayed", "model-a", "cascade", "turn-before", "phase-before", "primary", "solver", int64ptr(1e9)) +
		usage(t, "usage-replayed", start, "source", "replayed", "model-a", "cascade", "turn-before", "phase-before", "primary", "solver", int64ptr(1e9)) +
		usage(t, "usage-in", start, "source", "inside", "model-a", "cascade", "turn-before", "phase-in", "judge", "judge", int64ptr(2e9)) +
		usage(t, "usage-end", end, "source", "end", "model-a", "single", "turn-end", "phase-end", "primary", "solver", int64ptr(3e9))
	writeSession(t, root, "root", project, events)
	writeSession(t, root, "child", child, resolution(t, "child-event", start, "child-turn", "single"))
	writeSession(t, root, "sibling", sibling, resolution(t, "sibling-event", start, "sibling-turn", "single"))
	writeSession(t, root, "empty-cwd", "", resolution(t, "empty-event", start, "empty-turn", "single"))

	normalized, err := copilotext.ResolveSessionScope(context.Background(), false, project, "")
	if err != nil {
		t.Fatal(err)
	}
	since, _ := time.Parse(time.RFC3339, start)
	until, _ := time.Parse(time.RFC3339, end)
	opts := copilotext.PermissionStatsOptions{
		Root: root, Scope: copilotext.SessionScope{UnderDir: normalized.ExactCWD}, Since: since, Until: until,
		Kind: []string{"write"}, Command: []string{"fictional-command"},
	}
	got, err := CollectFusion(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sessions != 2 || got.ResolvedTurns != 2 || got.TurnsWithUsage != 1 ||
		got.PhasesWithUsage != 1 || got.UsageRecords != 1 || got.AIU != 2 {
		t.Fatalf("scoped/time-bounded Fusion = %+v", got)
	}
	if len(got.ByPattern) != 1 || got.ByPattern[0].Key != "single" ||
		len(got.ByPhase) != 1 || got.ByPhase[0].Pattern != "cascade" {
		t.Fatalf("resolution and usage windows were not selected independently: %+v", got)
	}

	opts.Scope = normalized
	got, err = CollectFusion(opts)
	if err != nil || got.Sessions != 1 || got.ResolvedTurns != 1 {
		t.Fatalf("exact cwd Fusion = %+v, error %v", got, err)
	}
	opts.SessionID = "child"
	got, err = CollectFusion(opts)
	if err != nil || got.Sessions != 0 {
		t.Fatalf("session and scope must intersect: %+v, error %v", got, err)
	}
	opts.Scope = copilotext.SessionScope{}
	opts.SessionID = ""
	got, err = CollectFusion(opts)
	if err != nil || got.Sessions != 4 {
		t.Fatalf("all scope should include empty cwd: %+v, error %v", got, err)
	}

	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(project, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeSession(t, root, "link", link, resolution(t, "link-event", start, "link-turn", "single"))
	opts.Scope = normalized
	got, err = CollectFusion(opts)
	if err != nil || got.Sessions != 2 {
		t.Fatalf("symlinked cwd should match exact scope: %+v, error %v", got, err)
	}
}

func TestCollectFusionTimeBoundsRequireRecordedTimestamps(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	withoutTimestamp := func(eventType, id string, data any) string {
		t.Helper()
		value, err := json.Marshal(map[string]any{"type": eventType, "id": id, "data": data})
		if err != nil {
			t.Fatal(err)
		}
		return string(value) + "\n"
	}
	missingUsage := map[string]any{"model": "model-a", "fusion": map[string]any{}}
	events := withoutTimestamp("session.fusion_resolved", "missing-resolution", map[string]any{"fusionId": "missing-turn"}) +
		withoutTimestamp("session.usage_record", "missing-usage", map[string]any{"usage": missingUsage}) +
		testEvent(t, "session.usage_record", "missing-usage", at, map[string]any{"usage": missingUsage}) +
		resolution(t, "known-resolution", at, "known-turn", "single") +
		usage(t, "known-usage", at, "source-a", "known", "model-a", "single", "known-turn", "phase-a", "primary", "solver", int64ptr(1e9))
	writeSession(t, root, "session-a", "/work/a", events)

	since, err := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	until, err := time.Parse(time.RFC3339, "2026-01-03T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name        string
		since       time.Time
		until       time.Time
		wantRecords int
	}{
		{name: "unbounded", wantRecords: 2},
		{name: "since", since: since, wantRecords: 1},
		{name: "until", until: until, wantRecords: 1},
		{name: "both", since: since, until: until, wantRecords: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root, Since: tt.since, Until: tt.until})
			if err != nil {
				t.Fatal(err)
			}
			if got.ResolvedTurns != tt.wantRecords || got.UsageRecords != tt.wantRecords ||
				got.Usage.UsageRecords != tt.wantRecords {
				t.Fatalf("time-bounded Fusion = %+v, want %d records of each type", got, tt.wantRecords)
			}
		})
	}
}

func TestUsageIdentityPrecedence(t *testing.T) {
	zero := int64(0)
	u := rawUsage{APICallID: "api-a"}
	u.Accounting.SourceSessionID = "source-a"
	u.Accounting.UsageID = "usage-a"
	u.Accounting.Sequence = &zero
	ev := rawEvent{ID: "event-a", AgentID: "agent-a"}
	if got := u.identity(ev, "session-a"); got != "usage:source-a:usage-a" {
		t.Fatalf("usage identity = %q", got)
	}
	u.Accounting.UsageID = ""
	if got := u.identity(ev, "session-a"); got != "sequence:source-a:0" {
		t.Fatalf("sequence identity = %q", got)
	}
	u.Accounting.Sequence = nil
	u.Accounting.SourceSessionID = ""
	if got := u.identity(ev, "session-a"); got != "api:agent-a:api-a" {
		t.Fatalf("API identity = %q", got)
	}
	u.APICallID = ""
	if got := u.identity(ev, "session-a"); got != "event:event-a" {
		t.Fatalf("event identity = %q", got)
	}
	if got := u.identity(rawEvent{}, "session-a"); got != "" {
		t.Fatalf("identity without identifiers = %q", got)
	}
}

func TestCollectFusionDeduplicatesValidRecordsBeforeTimeFiltering(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	invalid := usage(t, "bad", at, "source", "same", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(-1))
	valid := usage(t, "good", at, "source", "same", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9))
	differentSource := usage(t, "different-source", at, "other-source", "same", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9))
	nonFusion := testEvent(t, "session.usage_record", "non-fusion", at, map[string]any{"usage": map[string]any{
		"model": "model-a", "accounting": map[string]any{"sourceSessionId": "source", "usageId": "already-seen"},
	}})
	duplicateWithFusion := usage(t, "fusion-replay", at, "source", "already-seen", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(5e9))
	noID := usage(t, "", at, "", "", "model-a", "single", "turn", "phase", "primary", "solver", nil)
	writeSession(t, root, "session-a", "/work", invalid+valid+differentSource+nonFusion+duplicateWithFusion+noID+noID)

	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got.UsageRecords != 4 || got.AIURecords != 2 || got.AIU != 2 || got.PhasesWithUsage != 1 {
		t.Fatalf("deduplicated Fusion = %+v", got)
	}
}

func TestCollectFusionAttributesReplayedRecordsToFirstCopy(t *testing.T) {
	root := t.TempDir()
	first := usage(t, "first", "2026-01-01T12:00:00Z", "source", "same", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9))
	replay := usage(t, "replay", "2026-01-03T12:00:00Z", "source", "same", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9))
	untimed, err := json.Marshal(map[string]any{"type": "session.usage_record", "id": "untimed", "data": map[string]any{"usage": map[string]any{
		"model": "model-a", "accounting": map[string]any{"sourceSessionId": "source", "usageId": "untimed-first"},
		"fusion": map[string]any{"fusionId": "turn", "phaseId": "phase"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	timedReplay := usage(t, "timed-replay", "2026-01-03T12:00:00Z", "source", "untimed-first", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9))
	writeSession(t, root, "session-a", "/work", first+replay+string(untimed)+"\n"+timedReplay)

	parse := func(value string) time.Time {
		t.Helper()
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	day0, day2, day4 := parse("2026-01-01T00:00:00Z"), parse("2026-01-02T00:00:00Z"), parse("2026-01-04T00:00:00Z")
	for _, tt := range []struct {
		name         string
		since, until time.Time
		want         int
	}{
		{name: "unbounded", want: 2},
		{name: "first copy window", since: day0, until: day2, want: 1},
		{name: "replay window", since: day2, until: day4, want: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root, Since: tt.since, Until: tt.until})
			if err != nil {
				t.Fatal(err)
			}
			if got.UsageRecords != tt.want || got.Usage.UsageRecords != tt.want {
				t.Fatalf("replayed Fusion records = %+v, want %d", got, tt.want)
			}
		})
	}
}

func TestCollectFusionUnknownMetadataAndLargeOrMalformedLines(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	large := testEvent(t, "session.usage_record", "large", at, map[string]any{"usage": map[string]any{
		"model": "model-a", "extra": strings.Repeat("x", 100*1024),
		"fusion": map[string]any{"phaseId": "phase-a"},
	}})
	writeSession(t, root, "session-a", "/work", "{bad json}\n"+large+`{"type":"session.usage_record"`+"\n")
	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sessions != 1 || got.UsageRecords != 1 || got.AIURecords != 0 ||
		got.MissingFusionIDRecords != 1 || got.MissingPhaseIdentityRecords != 1 || got.PhasesWithUsage != 0 {
		t.Fatalf("partial Fusion metadata = %+v", got)
	}
	if len(got.ByPhase) != 1 || got.ByPhase[0].Pattern != "unknown" ||
		got.ByPhase[0].Phase != "unknown" || got.ByPhase[0].Role != "unknown" {
		t.Fatalf("unknown metadata must not be inferred: %+v", got.ByPhase)
	}
}

func TestCollectFusionAcceptsEscapedEventTypes(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	event := usage(t, "event", at, "source", "usage", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9))
	event = strings.Replace(event, `"session.usage_record"`, `"session.\u0075sage_record"`, 1)
	writeSession(t, root, "session-a", "/work", event)
	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil || got.UsageRecords != 1 || got.AIU != 1 {
		t.Fatalf("escaped event type = %+v, error %v", got, err)
	}
}

func TestCollectFusionDoesNotCountPlannedPhases(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	planned := testEvent(t, "session.fusion_resolved", "resolved", at, map[string]any{
		"fusionId": "turn-a", "pattern": "cascade", "primaryModel": "planned-model",
		"phasePlan": []map[string]any{
			{"kind": "primary", "role": "solver"},
			{"kind": "repair", "role": "solver", "conditional": true},
		},
	})
	observed := testEvent(t, "session.usage_record", "usage", at, map[string]any{"usage": map[string]any{
		"model": "actual-model", "accounting": map[string]any{"usageId": "usage-a"},
		"fusion": map[string]any{
			"fusionId": "turn-a", "pattern": "cascade", "phaseId": "phase-a",
			"phaseKind": "primary", "role": "solver", "sourceModel": "other-model",
		},
	}})
	writeSession(t, root, "session-a", "/work", planned+observed)
	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got.ResolvedTurns != 1 || got.PhasesWithUsage != 1 || len(got.ByPhase) != 1 ||
		got.ByPhase[0].Phase != "primary" || got.ByPhase[0].Model != "actual-model" {
		t.Fatalf("planned phase or model must not replace recorded usage: %+v", got)
	}
}

func TestCollectFusionRetainsDistinctAccountingRecordsForOneAPICall(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	event := func(id string) string {
		return testEvent(t, "session.usage_record", id, at, map[string]any{"usage": map[string]any{
			"model": "model-a", "apiCallId": "shared-call",
			"accounting": map[string]any{"sourceSessionId": "source", "usageId": id},
			"fusion":     map[string]any{"fusionId": "turn", "phaseId": "phase", "phaseKind": "primary"},
		}})
	}
	writeSession(t, root, "session-a", "/work", event("usage-a")+event("usage-b"))
	got, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root})
	if err != nil || got.UsageRecords != 2 || got.PhasesWithUsage != 1 {
		t.Fatalf("distinct accounting records sharing one API call = %+v, error %v", got, err)
	}
}

func TestCollectFusionReportsUnreadableEventFile(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "session-a", "/work", "")
	path := filepath.Join(root, "session-a", "events.jsonl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CollectFusion(copilotext.PermissionStatsOptions{Root: root}); err == nil {
		t.Fatal("expected an error reading a directory as events.jsonl")
	}
}

func TestCollectFusionDefaultHomeAndEmptyHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COPILOT_HOME", home)
	empty, err := CollectFusion(copilotext.PermissionStatsOptions{})
	if err != nil || empty.Sessions != 0 || empty.ByPattern == nil || empty.ByPhase == nil {
		t.Fatalf("missing history = %+v, error %v", empty, err)
	}
	at := "2026-01-02T00:00:00Z"
	writeSession(t, filepath.Join(home, "session-state"), "session-a", "/work", resolution(t, "event", at, "turn", "single"))
	got, err := CollectFusion(copilotext.PermissionStatsOptions{})
	if err != nil || got.Sessions != 1 || got.ResolvedTurns != 1 {
		t.Fatalf("COPILOT_HOME history = %+v, error %v", got, err)
	}
}

func TestCollectPreservesSharedStatsAndAddsFusion(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	writeSession(t, root, "session-a", "/work", resolution(t, "resolution", at, "turn", "single")+
		usage(t, "record", at, "source", "usage", "model-a", "single", "turn", "phase", "primary", "solver", int64ptr(1e9)))
	stats, err := Collect(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Sessions", "UsageSessions", "ByModelUsage", "ByRoleUsage", "Fusion"} {
		if _, ok := result[field]; !ok {
			t.Errorf("missing flattened JSON field %q: %s", field, encoded)
		}
	}
	if stats.UsageRequests != 1 || stats.UsageAIU != 1 || stats.Fusion.ResolvedTurns != 1 || stats.Fusion.UsageRecords != 1 {
		t.Fatalf("combined stats = %+v", stats)
	}
}

func TestCollectFiltersUsageByRecordedTime(t *testing.T) {
	root := t.TempDir()
	before := "2026-01-01T00:00:00Z"
	inside := "2026-01-02T00:00:00Z"
	writeSession(t, root, "session-a", "/work",
		usage(t, "before", before, "source", "before", "model-a", "single", "turn-a", "phase-a", "primary", "solver", int64ptr(1e9))+
			usage(t, "inside", inside, "source", "inside", "model-a", "single", "turn-b", "phase-b", "primary", "solver", int64ptr(2e9)))
	since, err := time.Parse(time.RFC3339, inside)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := Collect(copilotext.PermissionStatsOptions{Root: root, Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if stats.UsageRequests != 1 || stats.UsageAIU != 2 || stats.Fusion.UsageRecords != 1 || stats.Fusion.AIU != 2 {
		t.Fatalf("time-bounded stats = %+v", stats)
	}
}

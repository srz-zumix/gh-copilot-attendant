package sessionstats

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

func activityTime(t *testing.T, value string) time.Time {
	t.Helper()
	got, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func activityLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("time zone %s is unavailable: %v", name, err)
	}
	return location
}

func scanActivity(t *testing.T, opts copilotext.PermissionStatsOptions, location *time.Location, asOf string) SessionActivity {
	t.Helper()
	collector := newActivityCollector(opts, location)
	if _, err := collectFusion(opts, collector); err != nil {
		t.Fatal(err)
	}
	result, err := collector.finish(activityTime(t, asOf))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func activityCoverage(t *testing.T, activity SessionActivity, date string) [24]string {
	t.Helper()
	for _, day := range activity.Coverage {
		if day.Date == date {
			return day.Hours
		}
	}
	t.Fatalf("no coverage for %s: %+v", date, activity.Coverage)
	return [24]string{}
}

func TestSessionActivityDistinctSessionsAndScope(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	other := filepath.Join(t.TempDir(), "other")
	events := testEvent(t, "message", "same-id", "2026-01-02T03:20:00Z", nil) +
		testEvent(t, "message", "same-id", "2026-01-02T03:50:00Z", nil) +
		testEvent(t, "message", "same-id", "2026-01-02T04:20:00Z", nil) +
		testEvent(t, "session.usage_record", "invalid-usage", "2026-01-02T04:30:00Z",
			map[string]any{"usage": map[string]any{"model": "model-a", "inputTokens": -1}}) +
		testEvent(t, "message", "after", "2026-01-02T04:45:00Z", nil) +
		"{\"type\":\"message\"}\n" +
		"{\"type\":\"message\",\"timestamp\":null}\n" +
		"{\"type\":\"message\",\"timestamp\":\"invalid\"}\n" +
		"{\"timestamp\":\"2026-01-02T04:00:00Z\"}\n" +
		"{invalid JSON\n"
	writeSession(t, root, "session-a", project, events)
	writeSession(t, root, "session-b", project,
		testEvent(t, "message", "same-id", "2026-01-02T03:30:00Z", nil))
	writeSession(t, root, "session-outside", other,
		testEvent(t, "message", "same-id", "2026-01-02T03:30:00Z", nil))
	scope, err := copilotext.ResolveSessionScope(context.Background(), false, project, "")
	if err != nil {
		t.Fatal(err)
	}
	opts := copilotext.PermissionStatsOptions{
		Root: root, Scope: scope,
		Since: activityTime(t, "2026-01-02T03:30:00Z"), Until: activityTime(t, "2026-01-02T04:45:00Z"),
		Kind: []string{"read"}, Command: []string{"example"}, PermissionMode: []string{"assisted"}, Top: 1,
	}
	result := scanActivity(t, opts, activityLocation(t, "Asia/Kathmandu"), "2026-01-03T00:00:00Z")
	if result.Version != 1 || result.TimeZone != "Asia/Kathmandu" ||
		result.StartDate != "2026-01-02" || result.EndDate != "2026-01-02" ||
		result.ScannedSessions != 2 || result.UntimedSessions != 1 ||
		result.UntimedEvents != 3 || result.MalformedEvents != 2 {
		t.Fatalf("activity metadata = %+v", result)
	}
	if len(result.Days) != 1 || result.Days[0].Hours[9] != 2 || result.Days[0].Hours[10] != 1 {
		t.Fatalf("distinct sessions by local hour = %+v", result.Days)
	}
	hours := activityCoverage(t, result, "2026-01-02")
	if hours[8] != "outside" || hours[9] != "partial" || hours[10] != "partial" || hours[11] != "outside" {
		t.Fatalf("fractional-offset coverage = %v", hours)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"session-a", "same-id", project} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("activity output includes source identifier %q", secret)
		}
	}
	opts.Kind, opts.Command, opts.PermissionMode, opts.Top = nil, nil, nil, 0
	unfiltered := scanActivity(t, opts, activityLocation(t, "Asia/Kathmandu"), "2026-01-03T00:00:00Z")
	if !reflect.DeepEqual(result, unfiltered) {
		t.Fatalf("permission filters and Top changed activity: %+v vs %+v", result, unfiltered)
	}
	opts.SessionID = "session-b"
	onlyOne := scanActivity(t, opts, activityLocation(t, "Asia/Kathmandu"), "2026-01-03T00:00:00Z")
	if onlyOne.ScannedSessions != 1 || len(onlyOne.Days) != 1 || onlyOne.Days[0].Hours[9] != 1 {
		t.Fatalf("session scope = %+v", onlyOne)
	}
}

func TestSessionActivityFiltersByAgentModeAtPermissionRequestTime(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeSession(t, root, "session-plan", project,
		testEvent(t, "message", "unmatched-early", "2026-01-01T10:00:00Z", nil)+
			testEvent(t, "permission.requested", "plan-before-window", "2026-01-02T03:20:00Z",
				map[string]any{"requestId": "plan-before-window", "agentMode": "plan"})+
			testEvent(t, "permission.requested", "plan-a", "2026-01-02T03:35:00Z",
				map[string]any{"requestId": "plan-a", "agentMode": "plan"})+
			testEvent(t, "permission.requested", "plan-b", "2026-01-02T03:50:00Z",
				map[string]any{"requestId": "plan-b", "agentMode": "plan"})+
			testEvent(t, "permission.requested", "interactive-a", "2026-01-02T03:40:00Z",
				map[string]any{"requestId": "interactive-a", "agentMode": "interactive"})+
			testEvent(t, "permission.requested", "plan-after-window", "2026-01-02T04:10:00Z",
				map[string]any{"requestId": "plan-after-window", "agentMode": "plan"}))
	writeSession(t, root, "session-plan-2", project,
		testEvent(t, "permission.requested", "plan-c", "2026-01-02T03:55:00Z",
			map[string]any{"requestId": "plan-c", "agentMode": "plan"}))
	writeSession(t, root, "session-interactive", project,
		testEvent(t, "permission.requested", "interactive-b", "2026-01-02T03:45:00Z",
			map[string]any{"requestId": "interactive-b", "agentMode": "interactive"})+
			"{\"type\":\"permission.requested\",\"data\":{\"requestId\":\"untimed-interactive\",\"agentMode\":\"interactive\"}}\n")
	writeSession(t, root, "session-outside", t.TempDir(),
		testEvent(t, "permission.requested", "plan-outside", "2026-01-02T03:45:00Z",
			map[string]any{"requestId": "plan-outside", "agentMode": "plan"}))
	scope, err := copilotext.ResolveSessionScope(context.Background(), false, project, "")
	if err != nil {
		t.Fatal(err)
	}

	opts := copilotext.PermissionStatsOptions{
		Root: root, Scope: scope, Since: activityTime(t, "2026-01-02T03:30:00Z"),
		Until: activityTime(t, "2026-01-02T04:00:00Z"), AgentMode: []string{"plan"},
	}
	result := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if result.AgentModes == nil || !reflect.DeepEqual(result.AgentModes, []string{"plan"}) {
		t.Fatalf("applied Agent modes = %v", result.AgentModes)
	}
	if result.ScannedSessions != 3 || result.UntimedEvents != 0 || result.UntimedSessions != 0 {
		t.Fatalf("scope and filtered timestamp coverage = %+v", result)
	}
	if len(result.Days) != 1 || result.Days[0].Hours[3] != 2 || result.Days[0].Hours[4] != 0 {
		t.Fatalf("matching permission requests by hour = %+v", result.Days)
	}

	opts.PermissionMode = []string{"assisted"}
	withPermissionMode := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if !reflect.DeepEqual(withPermissionMode, result) {
		t.Fatalf("permission mode changed Agent-mode activity: %+v", withPermissionMode)
	}
	opts.PermissionMode = nil
	opts.AgentMode = []string{"interactive", "plan", "plan"}
	bothModes := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(bothModes.Days) != 1 || bothModes.Days[0].Hours[3] != 3 {
		t.Fatalf("OR of Agent modes = %+v", bothModes.Days)
	}
	if !reflect.DeepEqual(bothModes.AgentModes, []string{"interactive", "plan"}) {
		t.Fatalf("Agent mode metadata should be sorted: %v", bothModes.AgentModes)
	}

	opts.AgentMode = nil
	unfiltered := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(unfiltered.Days) != 1 || unfiltered.Days[0].Hours[3] != 3 {
		t.Fatalf("unfiltered activity should retain all event types: %+v", unfiltered.Days)
	}
	if unfiltered.AgentModes == nil || len(unfiltered.AgentModes) != 0 {
		t.Fatalf("unfiltered Agent mode metadata = %v, want an empty array", unfiltered.AgentModes)
	}

	opts.Since, opts.Until, opts.AgentMode = time.Time{}, time.Time{}, []string{"plan"}
	unbounded := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if unbounded.StartDate != "2026-01-02" || unbounded.EndDate != "2026-01-03" ||
		len(unbounded.Days) != 1 || unbounded.Days[0].Hours[3] != 2 || unbounded.Days[0].Hours[4] != 1 {
		t.Fatalf("unmatched events affected filtered activity range or counts: %+v", unbounded)
	}
}

func TestSessionActivitySandboxFilteringAndAgentIntersection(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "session-a", "/work", `{"type":"message","timestamp":"2026-01-02T01:00:00Z"}
{"type":"permission.requested","timestamp":"2026-01-02T02:00:00Z","data":{"requestId":"req-plan","agentMode":"plan","permissionRequest":{"toolCallId":"call-a"}}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T03:00:00Z","data":{"toolCallId":"call-a","toolTelemetry":{"properties":{"sandboxApplied":true}}}}
{"type":"tool.execution_start","timestamp":"2026-01-02T02:50:00Z","data":{"toolCallId":"call-a","toolName":"bash"}}
{"type":"permission.requested","timestamp":"2026-01-02T03:01:00Z","data":{"requestId":"req-interactive","agentMode":"interactive","permissionRequest":{"toolCallId":"call-a"}}}
{"type":"tool.execution_start","timestamp":"2026-01-02T04:30:00Z","data":{"toolCallId":"unlinked","toolName":"bash"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T04:35:00Z","data":{"toolCallId":"unlinked","toolTelemetry":{"properties":{"sandboxApplied":true}}}}
{"type":"tool.execution_start","timestamp":"2026-01-02T03:45:00Z","data":{"toolCallId":"unlinked-unknown","toolName":"bash"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T03:50:00Z","data":{"toolCallId":"unlinked-unknown"}}
{"type":"permission.requested","timestamp":"invalid","data":{"requestId":"req-unknown","permissionRequest":{"toolCallId":"call-c"}}}
{"type":"tool.execution_start","timestamp":"invalid","data":{"toolCallId":"call-c","toolName":"bash"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T04:10:00Z","data":{"toolCallId":"call-c","toolTelemetry":{"properties":{"sandboxApplied":null}}}}
{"type":"tool.execution_start","timestamp":"2026-01-02T04:20:00Z","data":{"toolCallId":"call-d","toolName":"bash"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T04:25:00Z","data":{"toolCallId":"call-d","toolTelemetry":{"properties":{"sandboxApplied":false}}}}
{"type":"permission.requested","timestamp":"invalid","data":{"requestId":"req-untimed","agentMode":"plan","permissionRequest":{"toolCallId":"untimed"}}}
{"type":"tool.execution_start","timestamp":"invalid","data":{"toolCallId":"untimed","toolName":"powershell"}}
{"type":"tool.execution_complete","timestamp":null,"data":{"toolCallId":"untimed"}}
{"type":"tool.execution_start","timestamp":"2026-01-02T04:30:00Z","data":{"toolCallId":"unrelated","toolName":"view"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T04:31:00Z","data":{"toolCallId":"unrelated"}}
{"type":"tool.execution_start","timestamp":"2026-01-02T04:40:00Z","data":{"toolCallId":"unfinished","toolName":"bash"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T05:00:00Z","data":{"toolCallId":"after","toolTelemetry":{"properties":{"sandboxApplied":true}}}}
`)
	writeSession(t, root, "session-b", "/work", `{"type":"permission.requested","timestamp":"2026-01-02T02:10:00Z","data":{"requestId":"req-b","agentMode":"plan","permissionRequest":{"toolCallId":"call-b"}}}
{"type":"tool.execution_start","timestamp":"2026-01-02T02:55:00Z","data":{"toolCallId":"call-b","toolName":"powershell"}}
{"type":"tool.execution_complete","timestamp":"2026-01-02T03:20:00Z","data":{"toolCallId":"call-b","toolTelemetry":{"properties":{"sandboxApplied":"true"}}}}
`)
	opts := copilotext.PermissionStatsOptions{
		Root: root, Since: activityTime(t, "2026-01-02T03:00:00Z"),
		Until: activityTime(t, "2026-01-02T05:00:00Z"), Sandbox: []string{copilotext.SandboxApplied},
	}
	applied := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if applied.ScannedSessions != 2 || applied.UntimedEvents != 0 ||
		len(applied.Days) != 1 || applied.Days[0].Hours[3] != 2 || applied.Days[0].Hours[4] != 1 ||
		!reflect.DeepEqual(applied.SandboxStates, []string{copilotext.SandboxApplied}) {
		t.Fatalf("applied executions at completion time = %+v", applied)
	}
	opts.Sandbox = []string{copilotext.SandboxUnknown, copilotext.SandboxApplied, copilotext.SandboxNotApplied, copilotext.SandboxApplied}
	all := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(all.Days) != 1 || all.Days[0].Hours[3] != 2 || all.Days[0].Hours[4] != 1 || all.Days[0].Hours[5] != 0 ||
		all.UntimedEvents != 1 || all.UntimedSessions != 1 ||
		!reflect.DeepEqual(all.SandboxStates, []string{"applied", "not-applied", "unknown"}) {
		t.Fatalf("OR of states, distinct sessions, and untimed completion = %+v", all)
	}
	opts.Kind, opts.Results, opts.PermissionMode, opts.Top = []string{"read"}, []string{"denied"}, []string{"assisted"}, 1
	if withOtherFilters := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z"); !reflect.DeepEqual(withOtherFilters, all) {
		t.Fatalf("unrelated permission filters changed sandbox activity: %+v", withOtherFilters)
	}
	opts.Kind, opts.Results, opts.PermissionMode, opts.Top = nil, nil, nil, 0
	opts.Sandbox = []string{copilotext.SandboxApplied}
	opts.AgentMode = []string{"interactive"}
	interactive := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(interactive.Days) != 1 || interactive.Days[0].Hours[3] != 1 || interactive.Days[0].Hours[4] != 0 {
		t.Fatalf("interactive mode should match one of multiple linked requests, not unlinked executions: %+v", interactive)
	}
	opts.AgentMode = []string{"plan", "interactive", "plan"}
	bothModes := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(bothModes.Days) != 1 || bothModes.Days[0].Hours[3] != 2 || bothModes.Days[0].Hours[4] != 0 ||
		!reflect.DeepEqual(bothModes.AgentModes, []string{"interactive", "plan"}) {
		t.Fatalf("OR of linked Agent modes = %+v", bothModes)
	}
	opts.AgentMode = []string{"plan"}
	plan := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(plan.Days) != 1 || plan.Days[0].Hours[3] != 2 || plan.Days[0].Hours[4] != 0 {
		t.Fatalf("request timestamps outside the window should not exclude linked completions: %+v", plan)
	}
	opts.Sandbox = []string{copilotext.SandboxUnknown}
	planUnknown := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(planUnknown.Days) != 0 || planUnknown.UntimedEvents != 1 || planUnknown.UntimedSessions != 1 {
		t.Fatalf("untimed matching completion = %+v", planUnknown)
	}
	opts.AgentMode = []string{"unknown"}
	unknown := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(unknown.Days) != 1 || unknown.Days[0].Hours[3] != 0 || unknown.Days[0].Hours[4] != 1 || unknown.UntimedEvents != 0 {
		t.Fatalf("missing request mode matches unknown without treating unlinked calls as unknown: %+v", unknown)
	}
	opts.AgentMode, opts.Sandbox, opts.SessionID = nil, []string{copilotext.SandboxApplied}, "session-b"
	oneSession := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if oneSession.ScannedSessions != 1 || len(oneSession.Days) != 1 || oneSession.Days[0].Hours[3] != 1 {
		t.Fatalf("session scope on sandbox activity = %+v", oneSession)
	}
}

func TestSessionActivitySandboxDoesNotMatchUnfinishedOrAmbiguousAgent(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "session-a", "/work", `{"type":"tool.execution_start","agentId":"child-a","timestamp":"2026-01-02T02:00:00Z","data":{"toolCallId":"shared","toolName":"bash"}}
{"type":"permission.requested","agentId":"child-b","timestamp":"2026-01-02T02:05:00Z","data":{"requestId":"plan","agentMode":"plan","permissionRequest":{"toolCallId":"shared"}}}
{"type":"permission.requested","agentId":"child-b","timestamp":"2026-01-02T02:06:00Z","data":{"requestId":"unknown","permissionRequest":{"toolCallId":"shared"}}}
{"type":"permission.requested","timestamp":"2026-01-02T02:07:00Z","data":{"requestId":"ambiguous","agentMode":"interactive","permissionRequest":{"toolCallId":"shared"}}}
{"type":"permission.requested","agentId":"child-a","timestamp":"2026-01-02T02:08:00Z","data":{"requestId":"exact","agentMode":"exact","permissionRequest":{"toolCallId":"shared"}}}
{"type":"tool.execution_complete","agentId":"child-a","timestamp":"2026-01-02T03:00:00Z","data":{"toolCallId":"shared","toolTelemetry":{"properties":{"sandboxApplied":true}}}}
{"type":"tool.execution_start","agentId":"child-b","timestamp":"2026-01-02T02:00:00Z","data":{"toolCallId":"shared","toolName":"bash"}}
`)
	opts := copilotext.PermissionStatsOptions{
		Root: root, Since: activityTime(t, "2026-01-02T00:00:00Z"),
		Until: activityTime(t, "2026-01-03T00:00:00Z"), Sandbox: []string{copilotext.SandboxApplied},
	}
	for _, mode := range []string{"plan", "unknown", "interactive"} {
		opts.AgentMode = []string{mode}
		result := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
		if len(result.Days) != 0 {
			t.Errorf("mode %q matched an unfinished or ambiguous execution: %+v", mode, result.Days)
		}
	}
	opts.AgentMode = []string{"exact"}
	exact := scanActivity(t, opts, time.UTC, "2026-01-03T00:00:00Z")
	if len(exact.Days) != 1 || exact.Days[0].Hours[3] != 1 {
		t.Fatalf("exact completed execution did not match: %+v", exact.Days)
	}
}

func TestSessionActivityAgentModeUnknownIncludesMissingAndInvalidValues(t *testing.T) {
	root := t.TempDir()
	missing := `{"type":"permission.requested","timestamp":"2026-01-02T03:10:00Z","data":{"requestId":"missing"}}` + "\n"
	nullMode := testEvent(t, "permission.requested", "null", "2026-01-02T03:20:00Z",
		map[string]any{"requestId": "null", "agentMode": nil})
	invalidMode := testEvent(t, "permission.requested", "invalid", "2026-01-02T03:30:00Z",
		map[string]any{"requestId": "invalid", "agentMode": map[string]any{"future": true}})
	emptyMode := testEvent(t, "permission.requested", "empty", "2026-01-02T03:40:00Z",
		map[string]any{"requestId": "empty", "agentMode": ""})
	namedMode := testEvent(t, "permission.requested", "plan", "2026-01-02T03:50:00Z",
		map[string]any{"requestId": "plan", "agentMode": "plan"})
	writeSession(t, root, "session-a", "/work", missing+nullMode+invalidMode+emptyMode+namedMode)

	result := scanActivity(t, copilotext.PermissionStatsOptions{
		Root: root, AgentMode: []string{"unknown"},
	}, time.UTC, "2026-01-03T00:00:00Z")
	if len(result.Days) != 1 || result.Days[0].Hours[3] != 1 {
		t.Fatalf("unknown mode count = %+v", result.Days)
	}
}

func TestSessionActivityRangeAndUntimedEvents(t *testing.T) {
	now := activityTime(t, "2026-02-01T10:00:00Z")
	timed := `{"type":"message","timestamp":"2026-01-30T12:15:00Z"}`
	untimed := `{"type":"message"}`
	tests := []struct {
		name        string
		since       string
		until       string
		events      []string
		start       string
		end         string
		reason      string
		untimed     int
		expectedDay int
	}{
		{name: "empty unbounded", reason: "no-dated-events"},
		{name: "only untimed unbounded", events: []string{untimed}, reason: "no-dated-events", untimed: 1},
		{name: "since only", since: "2026-01-30T12:30:00Z", start: "2026-01-30", end: "2026-02-01"},
		{name: "until only empty", until: "2026-02-01T00:00:00Z", reason: "no-dated-events"},
		{name: "until only with events", until: "2026-02-01T00:00:00Z", events: []string{timed},
			start: "2026-01-30", end: "2026-01-31", expectedDay: 1},
		{name: "both at midnight", since: "2026-01-30T00:00:00Z", until: "2026-02-01T00:00:00Z",
			start: "2026-01-30", end: "2026-01-31"},
		{name: "equal bounds", since: "2026-01-30T12:15:00Z", until: "2026-01-30T12:15:00Z",
			events: []string{timed}, reason: "empty-window"},
		{name: "reversed bounds", since: "2026-02-01T00:00:00Z", until: "2026-01-30T00:00:00Z",
			events: []string{timed}, reason: "empty-window"},
		{name: "future since empty", since: "2026-02-02T00:00:00Z", reason: "no-dated-events"},
		{name: "future since on scan date", since: "2026-02-01T10:01:00Z", reason: "no-dated-events"},
		{name: "future recorded event extends display", since: "2026-02-02T00:00:00Z",
			events: []string{`{"type":"message","timestamp":"2026-02-03T00:00:00Z"}`},
			start:  "2026-02-02", end: "2026-02-03", expectedDay: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts copilotext.PermissionStatsOptions
			if tt.since != "" {
				opts.Since = activityTime(t, tt.since)
			}
			if tt.until != "" {
				opts.Until = activityTime(t, tt.until)
			}
			collector := newActivityCollector(opts, time.UTC)
			seen := make(map[activityHour]struct{})
			for _, event := range tt.events {
				collector.record([]byte(event), seen, nil)
			}
			result, err := collector.finish(now)
			if err != nil {
				t.Fatal(err)
			}
			if result.StartDate != tt.start || result.EndDate != tt.end || result.EmptyReason != tt.reason ||
				result.UntimedEvents != tt.untimed || len(result.Days) != tt.expectedDay {
				t.Fatalf("range = %+v", result)
			}
		})
	}
}

func TestSessionActivityDoesNotDeduplicateEventIdentities(t *testing.T) {
	opts := copilotext.PermissionStatsOptions{
		Since: activityTime(t, "2026-01-02T00:30:00Z"),
		Until: activityTime(t, "2026-01-02T02:00:00Z"),
	}
	events := []string{
		testEvent(t, "session.usage_record", "replay", "2026-01-02T00:00:00Z", nil),
		testEvent(t, "session.usage_record", "replay", "2026-01-02T01:00:00Z", nil),
		testEvent(t, "session.usage_record", "replay", "2026-01-02T01:30:00Z", nil),
	}
	var previous SessionActivity
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}} {
		c := newActivityCollector(opts, time.UTC)
		seen := make(map[activityHour]struct{})
		for _, index := range order {
			c.record([]byte(events[index]), seen, nil)
		}
		result, err := c.finish(activityTime(t, "2026-01-03T00:00:00Z"))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Days) != 1 || result.Days[0].Hours[1] != 1 || result.Days[0].Hours[0] != 0 {
			t.Fatalf("replayed event timestamps = %+v", result.Days)
		}
		if previous.Version != 0 && !reflect.DeepEqual(previous, result) {
			t.Fatalf("event order changed activity: %+v vs %+v", previous, result)
		}
		previous = result
	}
}

func TestSessionActivityDSTRepeatedAndSkippedHours(t *testing.T) {
	location := activityLocation(t, "America/New_York")
	root := t.TempDir()
	first := testEvent(t, "message", "same", "2026-11-01T01:15:00-04:00", nil)
	second := testEvent(t, "message", "same", "2026-11-01T01:15:00-05:00", nil)
	writeSession(t, root, "session-a", "", first+second)
	writeSession(t, root, "session-b", "", second+first)
	opts := copilotext.PermissionStatsOptions{Root: root}
	all := scanActivity(t, opts, location, "2026-11-02T12:00:00Z")
	if len(all.Days) != 1 || all.Days[0].Hours[1] != 2 {
		t.Fatalf("repeated hour must count each session once: %+v", all.Days)
	}
	opts.Since = activityTime(t, "2026-11-01T01:00:00-05:00")
	opts.Until = activityTime(t, "2026-11-01T02:00:00-05:00")
	selected := scanActivity(t, opts, location, "2026-11-02T12:00:00Z")
	hours := activityCoverage(t, selected, "2026-11-01")
	if selected.Days[0].Hours[1] != 2 || hours[0] != "outside" || hours[1] != "partial" || hours[2] != "outside" {
		t.Fatalf("repeated-hour bounded coverage = %+v, %v", selected.Days, hours)
	}
	for _, tt := range []struct {
		until string
		want  string
	}{
		{"2026-11-01T01:00:00-04:00", "outside"},
		{"2026-11-01T01:00:00-05:00", "partial"},
	} {
		c := newActivityCollector(copilotext.PermissionStatsOptions{Until: activityTime(t, tt.until)}, location)
		day, _ := time.Parse(activityDateFormat, "2026-11-01")
		if got := c.hourCoverage(day.Add(time.Hour)); got != tt.want {
			t.Errorf("until %s: got %s, want %s", tt.until, got, tt.want)
		}
	}

	spring := newActivityCollector(copilotext.PermissionStatsOptions{
		Since: activityTime(t, "2026-03-07T00:00:00-05:00"),
		Until: activityTime(t, "2026-03-10T00:00:00-04:00"),
	}, location)
	result, err := spring.finish(activityTime(t, "2026-03-11T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if got := activityCoverage(t, result, "2026-03-08")[2]; got != "nonexistent" {
		t.Fatalf("interior spring-forward hour = %q", got)
	}
}

func TestSessionActivityNonHourAndDateLineTransitions(t *testing.T) {
	lordHowe := activityLocation(t, "Australia/Lord_Howe")
	opts := copilotext.PermissionStatsOptions{
		Since: activityTime(t, "2026-04-04T15:00:00Z"),
		Until: activityTime(t, "2026-04-04T16:00:00Z"),
	}
	c := newActivityCollector(opts, lordHowe)
	day, _ := time.Parse(activityDateFormat, "2026-04-05")
	if first, second := c.hourCoverage(day.Add(time.Hour)), c.hourCoverage(day.Add(2*time.Hour)); first != "partial" || second != "partial" {
		t.Fatalf("half-hour DST transition coverage = %s, %s", first, second)
	}

	apia := activityLocation(t, "Pacific/Apia")
	skipped := newActivityCollector(copilotext.PermissionStatsOptions{
		Since: activityTime(t, "2011-12-29T00:00:00-10:00"),
		Until: activityTime(t, "2012-01-01T00:00:00+14:00"),
	}, apia)
	result, err := skipped.finish(activityTime(t, "2012-01-02T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	for hour, state := range activityCoverage(t, result, "2011-12-30") {
		if state != "nonexistent" {
			t.Errorf("skipped civil date hour %d = %q", hour, state)
		}
	}
}

func TestSessionActivityRangeIncludesCrossMidnightRollback(t *testing.T) {
	location := activityLocation(t, "Antarctica/Casey")
	start := activityTime(t, "2010-03-04T14:00:00Z")
	end := activityTime(t, "2010-03-04T18:00:00Z")
	tests := []struct {
		name   string
		since  time.Time
		until  time.Time
		events bool
	}{
		{name: "both bounds", since: start, until: end},
		{name: "since only", since: start},
		{name: "until only", until: end, events: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newActivityCollector(copilotext.PermissionStatsOptions{Since: tt.since, Until: tt.until}, location)
			if tt.events {
				c.record([]byte(`{"type":"message","timestamp":"2010-03-04T14:00:00Z"}`), make(map[activityHour]struct{}), nil)
			}
			result, err := c.finish(end)
			if err != nil {
				t.Fatal(err)
			}
			if result.StartDate != "2010-03-04" || result.EndDate != "2010-03-05" {
				t.Fatalf("rollback date range = %s to %s", result.StartDate, result.EndDate)
			}
			hours := activityCoverage(t, result, "2010-03-04")
			want22, want23 := "outside", "partial"
			if tt.since.IsZero() {
				want22, want23 = "full", "full"
			}
			if hours[22] != want22 || hours[23] != want23 {
				t.Fatalf("rollback coverage = %v", hours)
			}
		})
	}
}

func TestCombinedCollectorPreservesStandaloneFusion(t *testing.T) {
	root := t.TempDir()
	at := "2026-01-02T00:00:00Z"
	events := resolution(t, "resolution", at, "turn-a", "cascade") +
		usage(t, "usage", at, "source-a", "usage-a", "model-a", "cascade", "turn-a", "phase-a", "primary", "solver", int64ptr(1e9)) +
		usage(t, "usage-replay", "2026-01-02T01:00:00Z", "source-a", "usage-a", "model-a", "cascade", "turn-a", "phase-a", "primary", "solver", int64ptr(1e9)) +
		`{"type":"session.fusion_resolved","data":{"fusionId":"untimed"}}` + "\n" +
		`{"type":"session.usage_record","data":{"usage":{"model":"model-a","fusion":{}}}}` + "\n" +
		`{"type":"session.usage_record","timestamp":"2026-01-02T01:00:00Z","data":{"usage":{"model":"model-a","inputTokens":-1,"fusion":{}}}}` + "\n" +
		"{invalid JSON\n"
	writeSession(t, root, "session-a", "", events)
	for _, bounded := range []bool{false, true} {
		opts := copilotext.PermissionStatsOptions{Root: root, Top: 1}
		if bounded {
			opts.Since = activityTime(t, "2026-01-02T00:30:00Z")
			opts.Until = activityTime(t, "2026-01-03T00:00:00Z")
		}
		standalone, err := CollectFusion(opts)
		if err != nil {
			t.Fatal(err)
		}
		combined, err := Collect(opts)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(combined.Fusion, standalone) {
			t.Fatalf("bounded=%t: standalone and combined Fusion differ: %+v vs %+v", bounded, standalone, combined.Fusion)
		}
		if combined.SessionActivity.Version != 1 || combined.SessionActivity.UntimedEvents != 2 ||
			combined.SessionActivity.MalformedEvents != 1 {
			t.Fatalf("combined activity = %+v", combined.SessionActivity)
		}
	}
}

func TestSessionActivityCLIJSON(t *testing.T) {
	activityLocation(t, "Asia/Kathmandu")
	home := t.TempDir()
	root := filepath.Join(home, "session-state")
	writeSession(t, root, "session-a", "/work/example",
		testEvent(t, "message", "event-a", "2026-01-02T03:30:00Z", nil)+
			testEvent(t, "message", "event-b", "2026-01-02T03:50:00Z", nil)+
			testEvent(t, "message", "event-c", "2026-01-02T04:20:00Z", nil)+
			testEvent(t, "permission.requested", "request-a", "2026-01-02T03:30:00Z",
				map[string]any{"requestId": "request-a", "agentMode": "plan", "permissionRequest": map[string]any{"toolCallId": "call-a"}})+
			testEvent(t, "permission.requested", "request-b", "2026-01-02T03:50:00Z",
				map[string]any{"requestId": "request-b", "agentMode": "plan"})+
			testEvent(t, "permission.requested", "request-c", "2026-01-02T04:20:00Z",
				map[string]any{"requestId": "request-c", "agentMode": "plan"})+
			testEvent(t, "tool.execution_start", "start-a", "2026-01-02T03:31:00Z",
				map[string]any{"toolCallId": "call-a", "toolName": "bash"})+
			testEvent(t, "tool.execution_complete", "complete-a", "2026-01-02T03:40:00Z",
				map[string]any{"toolCallId": "call-a", "toolTelemetry": map[string]any{"properties": map[string]any{"sandboxApplied": true}}}))
	writeSession(t, root, "session-b", "/work/example",
		testEvent(t, "message", "event-d", "2026-01-02T03:30:00Z", nil)+
			testEvent(t, "permission.requested", "request-d", "2026-01-02T03:30:00Z",
				map[string]any{"requestId": "request-d", "agentMode": "plan", "permissionRequest": map[string]any{"toolCallId": "call-b"}})+
			testEvent(t, "tool.execution_start", "start-b", "2026-01-02T03:31:00Z",
				map[string]any{"toolCallId": "call-b", "toolName": "bash"})+
			testEvent(t, "tool.execution_complete", "complete-b", "2026-01-02T04:20:00Z",
				map[string]any{"toolCallId": "call-b", "toolTelemetry": map[string]any{"properties": map[string]any{"sandboxApplied": true}}}))
	writeSession(t, root, "session-message-only", "/work/example",
		testEvent(t, "message", "event-only", "2026-01-02T04:20:00Z", nil))
	run := func(extra ...string) SessionActivity {
		t.Helper()
		args := []string{"run", ".", "session", "stats", "--all", "--format", "json",
			"--since", "2026-01-02T03:30:00Z", "--until", "2026-01-02T04:45:00Z",
			"--agent-mode", "plan", "--top", "1"}
		cmd := exec.Command("go", append(args, extra...)...)
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = append(os.Environ(), "COPILOT_HOME="+home, "TZ=Asia/Kathmandu")
		output, err := cmd.Output()
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				t.Fatalf("run session stats: %v: %s", err, exit.Stderr)
			}
			t.Fatal(err)
		}
		var result struct {
			SessionActivity SessionActivity
		}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("decode session stats JSON: %v", err)
		}
		return result.SessionActivity
	}
	activity := run()
	if activity.Version != 1 || activity.TimeZone != "Asia/Kathmandu" ||
		!reflect.DeepEqual(activity.AgentModes, []string{"plan"}) ||
		activity.SandboxStates == nil || len(activity.SandboxStates) != 0 ||
		activity.StartDate != "2026-01-02" || activity.EndDate != "2026-01-02" ||
		len(activity.Days) != 1 || activity.Days[0].Hours[9] != 2 || activity.Days[0].Hours[10] != 1 {
		t.Fatalf("CLI activity = %+v", activity)
	}
	hours := activityCoverage(t, activity, "2026-01-02")
	if hours[8] != "outside" || hours[9] != "partial" || hours[10] != "partial" || hours[11] != "outside" {
		t.Fatalf("CLI coverage = %v", hours)
	}
	sandbox := run("--sandbox", "applied")
	if !reflect.DeepEqual(sandbox.AgentModes, []string{"plan"}) ||
		!reflect.DeepEqual(sandbox.SandboxStates, []string{"applied"}) ||
		len(sandbox.Days) != 1 || sandbox.Days[0].Hours[9] != 1 || sandbox.Days[0].Hours[10] != 1 {
		t.Fatalf("CLI sandbox-filtered activity = %+v", sandbox)
	}
}

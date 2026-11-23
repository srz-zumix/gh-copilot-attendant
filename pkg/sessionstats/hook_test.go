package sessionstats

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/permissionhook"
	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

const (
	hookSessionA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	hookSessionB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	hookSessionC = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	hookSessionD = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	hookSessionE = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
)

func hookRecord(at, event, tool, session, decision, status, output string) permissionhook.HookRecord {
	timestamp, err := time.Parse(time.RFC3339, at)
	if err != nil {
		panic(err)
	}
	return permissionhook.HookRecord{
		Version: 1, Timestamp: timestamp, Event: event, Tool: tool, SessionID: session,
		Decision: decision, Status: status, Output: output,
	}
}

func hookLine(t *testing.T, record permissionhook.HookRecord) string {
	t.Helper()
	value, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return "\n" + string(value) + "\n"
}

func hookFixture(t *testing.T) (string, string, string) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, "session-state")
	path, err := permissionhook.HookLogPathForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := permissionhook.OpenHookLog(path)
	if errors.Is(err, permissionhook.ErrLogUnsupported) {
		t.Skip("private hook logging is unavailable in this build")
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return home, root, path
}

func writeHookSession(t *testing.T, root, name, id, cwd string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := fmt.Sprintf("id: %q\ncwd: %q\n", id, cwd)
	if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectHookActivityScopeAndCoverage(t *testing.T) {
	home, root, path := hookFixture(t)
	a := filepath.Join(home, "workspace-a")
	b := filepath.Join(home, "workspace-b")
	for _, dir := range []string{a, b} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeHookSession(t, root, "session-a", hookSessionA, a)
	writeHookSession(t, root, "session-b", hookSessionB, b)
	writeHookSession(t, root, "duplicate-a", hookSessionC, a)
	writeHookSession(t, root, "duplicate-b", hookSessionC, b)
	writeHookSession(t, root, "missing-workspace", hookSessionE, "")
	at := "2026-01-02T00:00:00Z"
	records := []permissionhook.HookRecord{
		hookRecord(at, "preToolUse", "view", hookSessionA, "allow", "success", "write_succeeded"),
		hookRecord(at, "permissionRequest", "bash", hookSessionA, "deny", "success", "write_succeeded"),
		hookRecord(at, "permissionRequest", "", hookSessionB, "none", "input_error", "not_attempted"),
		hookRecord(at, "preToolUse", "", "", "none", "success", "not_attempted"),
		hookRecord(at, "permissionRequest", "bash", hookSessionD, "deny", "success", "write_succeeded"),
		hookRecord(at, "preToolUse", "view", hookSessionC, "allow", "output_error", "write_failed"),
		hookRecord(at, "preToolUse", "view", hookSessionE, "none", "success", "not_attempted"),
		hookRecord("2026-01-03T00:00:00Z", "preToolUse", "view", hookSessionA, "allow", "success", "write_succeeded"),
	}
	var log strings.Builder
	for _, record := range records {
		log.WriteString(hookLine(t, record))
	}
	log.WriteString("{invalid}\n")
	log.WriteString(strings.Repeat("x", maxHookRecordBytes+100) + "\n")
	log.WriteString(`{"version":1,"timestamp":"2026-01-02T00:00:00Z","event":"preToolUse","decision":"allow","status":"success","output":"write_failed"}` + "\n")
	log.WriteString(`{"version":2,"timestamp":{"future":true}}` + "\n")
	log.WriteString(`{"version":1`)
	if err := os.WriteFile(path, []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	since, _ := time.Parse(time.RFC3339, at)
	until, _ := time.Parse(time.RFC3339, "2026-01-03T00:00:00Z")
	opts := copilotext.PermissionStatsOptions{
		Root: root, Since: since, Until: until, Top: 1,
		AgentMode: []string{"plan"}, Results: []string{"denied"}, Path: []string{"fictional-pattern"},
	}
	all := CollectHookActivity(opts)
	if all.State != "available" || all.Data == nil {
		t.Fatalf("all hook activity = %+v", all)
	}
	data := all.Data
	if data.Invocations != 4 || data.PermissionRequests != 2 || data.PreToolUses != 2 ||
		data.Decisions != (HookDecisions{Allow: 2, Deny: 2}) ||
		data.Outputs != (HookOutputs{WriteSucceeded: 3, WriteFailed: 1}) ||
		data.Processing != (HookProcessing{Success: 3, OutputError: 1}) {
		t.Fatalf("hook totals = %+v", data)
	}
	if data.Unidentified != (HookUnidentified{Unmatched: 1, Ambiguous: 1}) ||
		data.MalformedRecords != 4 || data.UnsupportedVersions != 1 || data.Attribution != "available" {
		t.Fatalf("hook coverage = %+v", data)
	}
	if !reflect.DeepEqual(data.ByTool, []HookToolCount{{Key: "bash", Total: 2}}) ||
		!reflect.DeepEqual(data.ByDecisionTool.Allow, []HookToolCount{{Key: "view", Total: 2}}) ||
		!reflect.DeepEqual(data.ByDecisionTool.Deny, []HookToolCount{{Key: "bash", Total: 2}}) {
		t.Fatalf("top tool rows = %+v", data.ByTool)
	}
	if actual, err := os.ReadFile(path); err != nil || string(actual) != log.String() {
		t.Fatalf("collection must not rewrite the log: %v", err)
	}

	scope, err := copilotext.ResolveSessionScope(context.Background(), false, a, "")
	if err != nil {
		t.Fatal(err)
	}
	opts.Scope = copilotext.SessionScope{UnderDir: scope.ExactCWD}
	scoped := CollectHookActivity(opts)
	if scoped.Data == nil || scoped.Data.Invocations != 2 ||
		scoped.Data.Unidentified != (HookUnidentified{Unmatched: 1, Ambiguous: 1}) {
		t.Fatalf("scoped hook activity = %+v", scoped)
	}
	if scoped.Data.Decisions != (HookDecisions{Allow: 1, Deny: 1}) {
		t.Fatalf("scoped decisions = %+v", scoped.Data.Decisions)
	}
	opts.Scope = copilotext.SessionScope{}
	opts.SessionID = hookSessionA
	opts.Top = 0
	session := CollectHookActivity(opts)
	if session.Data == nil || session.Data.Invocations != 2 ||
		session.Data.Unidentified != (HookUnidentified{Unmatched: 1, Ambiguous: 1}) ||
		len(session.Data.ByTool) != 2 {
		t.Fatalf("session hook activity = %+v", session)
	}
	opts.SessionID = hookSessionC
	if got := CollectHookActivity(opts); got.Data == nil || got.Data.Invocations != 0 || got.Data.Unidentified.Ambiguous != 1 {
		t.Fatalf("ambiguous session must not be attributed: %+v", got)
	}
}

func TestCollectHookActivityDecisionToolTop(t *testing.T) {
	_, root, path := hookFixture(t)
	at := "2026-01-02T00:00:00Z"
	var log strings.Builder
	for _, entry := range []struct {
		tool, decision string
		count          int
	}{
		{"view", "allow", 5}, {"Other", "allow", 2}, {"", "allow", 1},
		{"edit", "deny", 2}, {"bash", "deny", 1},
		{"ignored", "none", 3},
	} {
		for i := 0; i < entry.count; i++ {
			output := "write_succeeded"
			if entry.decision == "none" {
				output = "not_attempted"
			}
			log.WriteString(hookLine(t, hookRecord(at, "preToolUse", entry.tool, "", entry.decision, "success", output)))
		}
	}
	if err := os.WriteFile(path, []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := copilotext.PermissionStatsOptions{Root: root, Top: 1}
	got := CollectHookActivity(opts)
	if got.Data == nil || got.Data.Invocations != 11 ||
		got.Data.Decisions != (HookDecisions{Allow: 8, Deny: 3}) ||
		!reflect.DeepEqual(got.Data.ByTool, []HookToolCount{{Key: "view", Total: 5}}) ||
		!reflect.DeepEqual(got.Data.ByDecisionTool.Allow, []HookToolCount{{Key: "view", Total: 5}}) ||
		!reflect.DeepEqual(got.Data.ByDecisionTool.Deny, []HookToolCount{{Key: "edit", Total: 2}}) {
		t.Fatalf("independent decision top = %+v", got.Data)
	}
	opts.Top = 0
	got = CollectHookActivity(opts)
	if got.Data == nil ||
		!reflect.DeepEqual(got.Data.ByDecisionTool.Allow, []HookToolCount{{Key: "view", Total: 5}, {Key: "Other", Total: 2}, {Key: "", Total: 1}}) ||
		!reflect.DeepEqual(got.Data.ByDecisionTool.Deny, []HookToolCount{{Key: "edit", Total: 2}, {Key: "bash", Total: 1}}) {
		t.Fatalf("unlimited decision tools = %+v", got.Data)
	}
}

func TestCollectHookActivityLogAvailability(t *testing.T) {
	home, root, path := hookFixture(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	missing := CollectHookActivity(copilotext.PermissionStatsOptions{Root: root})
	if missing.State != "missing" || missing.Data != nil {
		t.Fatalf("missing log = %+v", missing)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("collect created missing log: %v", err)
	}

	record := hookRecord("2026-01-02T00:00:00Z", "preToolUse", "view", hookSessionA, "allow", "success", "write_succeeded")
	if err := os.WriteFile(path, []byte(hookLine(t, record)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	all := CollectHookActivity(copilotext.PermissionStatsOptions{Root: root})
	if all.State != "available" || all.Data == nil || all.Data.Attribution != "unavailable" ||
		all.Data.Unidentified.Unavailable != 1 || all.Data.Invocations != 1 {
		t.Fatalf("unavailable metadata in all scope = %+v", all)
	}
	scoped := CollectHookActivity(copilotext.PermissionStatsOptions{Root: root, SessionID: hookSessionA})
	if scoped.Data == nil || scoped.Data.Invocations != 0 || scoped.Data.Unidentified.Unavailable != 1 {
		t.Fatalf("unavailable metadata in session scope = %+v", scoped)
	}
	if _, err := os.Stat(filepath.Join(home, "session-state")); err != nil {
		t.Fatal(err)
	}
}

func TestHookScannerRecoversAndHonorsSnapshot(t *testing.T) {
	record := hookRecord("2026-01-02T00:00:00Z", "preToolUse", "view", hookSessionA, "allow", "success", "write_succeeded")
	first := hookLine(t, record)
	second := hookLine(t, record)
	tests := []struct {
		name        string
		input       string
		size        int64
		records     int
		malformed   int
		unsupported int
		complete    bool
	}{
		{name: "snapshot excludes later append", input: first + second, size: int64(len(first)), records: 1, complete: true},
		{name: "unterminated record", input: strings.TrimSuffix(first, "\n"), size: int64(len(first) - 1), malformed: 1, complete: true},
		{name: "truncated while reading", input: strings.TrimSuffix(first, "\n"), size: int64(len(first)), malformed: 1},
		{name: "oversized then valid", input: strings.Repeat("x", 2048) + "\n" + first, size: int64(2049 + len(first)), records: 1, malformed: 1, complete: true},
		{name: "interrupted then valid", input: `{"version":1` + first, size: int64(len(`{"version":1`) + len(first)), records: 1, malformed: 1, complete: true},
		{name: "future schema", input: `{"version":2,"timestamp":{}}` + "\n" + first, size: int64(len(`{"version":2,"timestamp":{}}`) + 1 + len(first)), records: 1, unsupported: 1, complete: true},
		{name: "invalid version", input: `{"version":null}` + "\n" + first, size: int64(len(`{"version":null}`) + 1 + len(first)), records: 1, malformed: 1, complete: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &HookActivityData{}
			count := 0
			complete := scanHookLog(strings.NewReader(tt.input), tt.size, data, func(permissionhook.HookRecord) { count++ })
			if complete != tt.complete || count != tt.records || data.MalformedRecords != tt.malformed ||
				data.UnsupportedVersions != tt.unsupported {
				t.Fatalf("complete=%v records=%d coverage=%+v", complete, count, data)
			}
		})
	}
	data := &HookActivityData{}
	source := io.MultiReader(bytes.NewBufferString(first), errorReader{})
	count := 0
	if scanHookLog(source, int64(len(first)+1), data, func(permissionhook.HookRecord) { count++ }) || count != 1 {
		t.Fatalf("read error must keep complete records and report partial: count=%d data=%+v", count, data)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read error") }

func TestHookWriterRecordsAreCollected(t *testing.T) {
	home, root, _ := hookFixture(t)
	t.Setenv("COPILOT_HOME", home)
	writeHookSession(t, root, "session-a", hookSessionA, "")
	config := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(config, []byte("hooks:\n  permissionRequest:\n    allow:\n      tools: [view]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := permissionhook.Run(strings.NewReader(`{"sessionId":"`+hookSessionA+`","toolName":"view"}`), &output, config); err != nil {
		t.Fatal(err)
	}
	got := CollectHookActivity(copilotext.PermissionStatsOptions{Root: root})
	if got.State != "available" || got.Data == nil || got.Data.Invocations != 1 ||
		got.Data.Decisions.Allow != 1 || got.Data.Outputs.WriteSucceeded != 1 || got.Data.MalformedRecords != 0 {
		t.Fatalf("writer/reader result = %+v", got)
	}
	combined, err := Collect(copilotext.PermissionStatsOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if combined.Requests != 0 || combined.HookActivity.Data == nil || combined.HookActivity.Data.Invocations != 1 {
		t.Fatalf("hook invocation must remain separate from permission requests: %+v", combined)
	}
}

func TestHookAndSessionStatsCLI(t *testing.T) {
	home, root, path := hookFixture(t)
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "inside.txt"), []byte("fictional"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeHookSession(t, root, "session-a", hookSessionA, workspace)
	if err := os.WriteFile(filepath.Join(root, "session-a", "events.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	permissionConfig := filepath.Join(home, "permission.yaml")
	scopedConfig := filepath.Join(home, "scoped.yaml")
	for file, config := range map[string]string{
		permissionConfig: "hooks:\n  permissionRequest:\n    allow:\n      tools: [view]\n    deny:\n      tools: [write]\n",
		scopedConfig: fmt.Sprintf("hooks:\n  permissionRequest:\n    allow:\n      scopedTools:\n"+
			"        - tools: [view]\n          paths: [%q]\n", workspace),
	} {
		if err := os.WriteFile(file, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	bin := filepath.Join(t.TempDir(), "attendant")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	run := func(input string, args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "COPILOT_HOME="+home)
		cmd.Stdin = strings.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("run %v: %v: %s", args, err, stderr.String())
		}
		return string(output)
	}
	permission := run(`{"sessionId":"`+hookSessionA+`","toolName":"view",`+
		`"toolInput":{"path":"fictional-secret"}}`, "hook", "permission-request", "--config", permissionConfig)
	if strings.TrimSpace(permission) != `{"behavior":"allow"}` {
		t.Fatalf("permission output = %q", permission)
	}
	payload, err := json.Marshal(map[string]any{
		"sessionId": hookSessionA, "cwd": workspace, "toolName": "view",
		"toolArgs": map[string]any{"path": "inside.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	scoped := run(string(payload), "hook", "pre-tool-use", "--config", scopedConfig)
	if strings.TrimSpace(scoped) != `{"permissionDecision":"allow"}` {
		t.Fatalf("scoped output = %q", scoped)
	}
	denied := run(`{"toolName":"write"}`, "hook", "permission-request", "--config", permissionConfig)
	if !strings.Contains(denied, `"behavior":"deny"`) {
		t.Fatalf("deny output = %q", denied)
	}
	log, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(log, []byte(`"target":"fictional-secret"`)) || !bytes.Contains(log, []byte(`"target":"inside.txt"`)) ||
		bytes.Contains(log, []byte(workspace)) {
		t.Fatalf("hook log targets = %s", log)
	}

	stats := func(args ...string) Stats {
		t.Helper()
		command := append([]string{"session", "stats", "--all", "--format", "json"}, args...)
		var result Stats
		if err := json.Unmarshal([]byte(run("", command...)), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	all := stats("--agent-mode", "plan", "--top", "1")
	if all.Requests != 0 || all.HookActivity.State != "available" || all.HookActivity.Data == nil {
		t.Fatalf("combined CLI stats = %+v", all)
	}
	data := all.HookActivity.Data
	if data.Invocations != 3 || data.PermissionRequests != 2 || data.PreToolUses != 1 ||
		data.Decisions != (HookDecisions{Allow: 2, Deny: 1}) ||
		data.Outputs.WriteSucceeded != 3 || data.Unidentified.Missing != 1 || len(data.ByTool) != 1 ||
		len(data.ByTarget) != 1 {
		t.Fatalf("all CLI hook activity = %+v", data)
	}
	full := stats("--top", "0").HookActivity.Data
	want := map[HookTargetCount]bool{
		{Tool: "view", Decision: "allow", Target: "fictional-secret", Total: 1}: true,
		{Tool: "view", Decision: "allow", Target: "inside.txt", Total: 1}:       true,
		{Tool: "write", Decision: "deny", Total: 1}:                             true,
	}
	if full == nil || len(full.ByTarget) != len(want) {
		t.Fatalf("CLI hook targets = %+v", full)
	}
	for _, entry := range full.ByTarget {
		if !want[entry] {
			t.Fatalf("unexpected CLI hook target %+v", entry)
		}
	}
	selected := stats("--session", hookSessionA)
	if selected.HookActivity.Data == nil || selected.HookActivity.Data.Invocations != 2 ||
		selected.HookActivity.Data.Unidentified.Missing != 1 {
		t.Fatalf("session-scoped CLI hook activity = %+v", selected.HookActivity)
	}
}

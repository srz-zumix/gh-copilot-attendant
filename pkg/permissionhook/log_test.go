//go:build darwin || linux

package permissionhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic write failure") }

type unexpectedReader struct{}

func (unexpectedReader) Read([]byte) (int, error) { panic("missing config must not read input") }

func readHookRecords(t *testing.T) []HookRecord {
	t.Helper()
	p, err := HookLogPath()
	require.NoError(t, err)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	var records []HookRecord
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var record HookRecord
		require.NoError(t, json.Unmarshal(line, &record))
		records = append(records, record)
	}
	return records
}

func TestHookLogging(t *testing.T) {
	if !loggingSupported() {
		t.Skip("private logging is unavailable")
	}
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(testConfig), 0o600))

	var out bytes.Buffer
	payload := `{"sessionId":"12345678-1234-1234-1234-123456789abc","toolName":"bash","toolInput":{"command":"git status -- synthetic-target"}}`
	require.NoError(t, Run(strings.NewReader(payload), &out, config))
	var warning bytes.Buffer
	require.NoError(t, Run(strings.NewReader(`not json`), &warning, config))
	assertHookWarning(t, warning.String(), "input")
	assert.Error(t, Run(strings.NewReader(payload), failingWriter{}, config))
	require.NoError(t, Run(unexpectedReader{}, &out, filepath.Join(dir, "missing")))
	records := readHookRecords(t)
	require.Len(t, records, 2)
	assert.Equal(t, "allow", records[0].Decision)
	assert.Equal(t, "write_succeeded", records[0].Output)
	assert.Equal(t, "git status -- synthetic-target", records[0].Target)
	assert.False(t, records[0].TargetTruncated)
	assert.Equal(t, "12345678-1234-1234-1234-123456789abc", records[0].SessionID)
	assert.Equal(t, "allow", records[1].Decision)
	assert.Equal(t, "output_error", records[1].Status)
	assert.Equal(t, "write_failed", records[1].Output)

	long := "'" + strings.Repeat("<", 2000) + "'"
	big, err := json.Marshal(map[string]any{"toolName": "bash", "toolInput": map[string]any{"command": "git status " + long}})
	require.NoError(t, err)
	require.NoError(t, Run(bytes.NewReader(big), &out, config))
	undecided := `{"toolName":"bash","toolInput":{"command":"make synthetic-undecided"}}`
	require.NoError(t, Run(strings.NewReader(undecided), &out, config))
	denied := `{"toolName":"bash","toolInput":{"command":"rm synthetic-target"}}`
	require.NoError(t, Run(strings.NewReader(denied), &out, config))
	records = readHookRecords(t)
	require.Len(t, records, 4)
	assert.Equal(t, "allow", records[2].Decision)
	assert.True(t, records[2].Valid())
	assert.True(t, records[2].TargetTruncated)
	assert.True(t, strings.HasPrefix(records[2].Target, "git status '<"))
	assert.Equal(t, "deny", records[3].Decision)
	assert.Equal(t, "rm synthetic-target", records[3].Target)
	path, err := HookLogPath()
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "synthetic-undecided")
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		assert.LessOrEqual(t, len(line), maxRecordBytes)
	}
}

func TestHookLoggingRedactsEnvironmentAssignments(t *testing.T) {
	if !loggingSupported() {
		t.Skip("private logging is unavailable")
	}
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["gh pr view:*", "go build:*"]
    deny:
      commands: ["GH_TOKEN=blocked gh pr view"]
    environmentAssignments:
      mode: denylist
`), 0o600))

	var out bytes.Buffer
	allowed := `{"toolName":"bash","toolInput":{"command":"GH_TOKEN=` +
		strings.Repeat("synthetic-secret", 80) + ` gh pr view"}}`
	require.NoError(t, Run(strings.NewReader(allowed), &out, config))
	assert.Contains(t, out.String(), `"behavior":"allow"`)

	out.Reset()
	denied := `{"toolName":"bash","toolInput":{"command":"GH_TOKEN=blocked gh pr view"}}`
	require.NoError(t, Run(strings.NewReader(denied), &out, config))
	assert.Contains(t, out.String(), `"behavior":"deny"`)
	assert.NotContains(t, out.String(), "blocked")

	out.Reset()
	toolConfig := filepath.Join(dir, "tool-deny.yaml")
	require.NoError(t, os.WriteFile(toolConfig, []byte("hooks:\n  permissionRequest:\n    deny:\n      tools: [bash]\n"), 0o600))
	toolDenied := `{"toolName":"bash","toolInput":{"command":"GH_TOKEN=tool-secret gh pr view"}}`
	require.NoError(t, Run(strings.NewReader(toolDenied), &out, toolConfig))

	out.Reset()
	goAllowed := `{"toolName":"bash","toolInput":{"command":"CUSTOM_VALUE=synthetic-go-secret go build"}}`
	require.NoError(t, RunAllRulesPreToolUse(strings.NewReader(goAllowed), &out, config))
	assert.Contains(t, out.String(), `"permissionDecision":"allow"`)

	data, err := os.ReadFile(filepath.Join(dir, "gh-copilot-attendant", "permission-hook", "events.jsonl"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "synthetic-secret")
	assert.NotContains(t, string(data), "blocked")
	assert.NotContains(t, string(data), "tool-secret")
	assert.NotContains(t, string(data), "synthetic-go-secret")
	assert.Contains(t, string(data), "[REDACTED]")
	assert.Contains(t, string(data), "GH_TOKEN=[REDACTED] gh pr view")
	assert.Contains(t, string(data), "CUSTOM_VALUE=[REDACTED] go build")
	assert.LessOrEqual(t, len(data), 4*maxRecordBytes+4)
}

func TestLoggingFailureDoesNotChangeExit(t *testing.T) {
	if os.Getenv("HOOK_LOG_PIPE_CHILD") == "1" {
		err := Run(strings.NewReader(`{"toolName":"web_fetch"}`), os.Stdout, os.Getenv("HOOK_LOG_TEST_CONFIG"))
		if err != nil {
			os.Exit(2)
		}
		return
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(testConfig), 0o600))
	// A regular file cannot be used as the Copilot home directory.
	home := filepath.Join(dir, "not-a-directory")
	require.NoError(t, os.WriteFile(home, nil, 0o600))
	cmd := exec.Command(os.Args[0], "-test.run=^TestLoggingFailureDoesNotChangeExit$")
	cmd.Env = append(os.Environ(), "HOOK_LOG_PIPE_CHILD=1", "HOOK_LOG_TEST_CONFIG="+config, "COPILOT_HOME="+home)
	read, write, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, read.Close())
	cmd.Stderr = write
	output, err := cmd.Output()
	require.NoError(t, write.Close())
	require.NoError(t, err)
	assert.Contains(t, string(output), `"behavior":"allow"`)
}

func TestHookLoggingDisabledOrInvalid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	path, err := HookLogPath()
	require.NoError(t, err)
	for _, yaml := range []string{
		"hooks:\n  disabled: true\n",
		"hooks:\n  logging:\n    disabled: true\n",
		"hooks: [\n",
		"hooks:\n  permissionRequest:\n    deny:\n      tools: ['[']\n",
	} {
		require.NoError(t, os.WriteFile(config, []byte(yaml), 0o600))
		_ = Run(strings.NewReader(`{}`), &bytes.Buffer{}, config)
		assert.NoFileExists(t, path)
	}
}

func TestPreToolLoggingAndStatus(t *testing.T) {
	if !loggingSupported() {
		t.Skip("private logging is unavailable")
	}
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte("hooks:\n  permissionRequest:\n    allow:\n      tools: [list_repositories]\n"), 0o600))
	var out bytes.Buffer
	require.NoError(t, RunPreToolUse(strings.NewReader(`{"toolName":"view"}`), &out, config))
	require.NoError(t, RunPreToolUse(strings.NewReader(`not json`), &out, config))
	assertHookWarning(t, out.String(), "input")
	out.Reset()
	path, err := HookLogPath()
	require.NoError(t, err)
	assert.NoFileExists(t, path)
	require.NoError(t, RunPreToolUse(strings.NewReader(`{"toolName":"list_repositories"}`), &out, config))
	records := readHookRecords(t)
	require.Len(t, records, 1)
	assert.Equal(t, "preToolUse", records[0].Event)
	assert.Equal(t, "allow", records[0].Decision)
	assert.Equal(t, "write_succeeded", records[0].Output)

	st, err := InspectHook(filepath.Join(dir, "missing-hook"), config)
	require.NoError(t, err)
	out.Reset()
	require.NoError(t, WriteStatus(&out, st))
	assert.Contains(t, out.String(), "Logging: enabled")
	assert.Contains(t, out.String(), "events.jsonl")
	require.NoError(t, os.WriteFile(config, []byte("hooks: [\n"), 0o600))
	st, err = InspectHook(filepath.Join(dir, "missing-hook"), config)
	require.NoError(t, err)
	out.Reset()
	require.NoError(t, WriteStatus(&out, st))
	assert.Contains(t, out.String(), "Logging: stopped (config error)")
	assert.Len(t, readHookRecords(t), 1)
}

func TestAllRulesPreToolLogsOnlyDecisions(t *testing.T) {
	if !loggingSupported() {
		t.Skip("private logging is unavailable")
	}
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(testConfig), 0o600))
	var out bytes.Buffer
	for _, input := range []string{
		`{"toolName":"bash","toolArgs":{"command":"git status"}}`,
		`{"toolName":"bash","toolArgs":{"command":"rm fictional-file"}}`,
		`{"toolName":"bash","toolArgs":{"command":"make fictional-target"}}`,
	} {
		require.NoError(t, RunAllRulesPreToolUse(strings.NewReader(input), &out, config))
	}
	records := readHookRecords(t)
	require.Len(t, records, 2)
	assert.Equal(t, "preToolUse", records[0].Event)
	assert.Equal(t, "allow", records[0].Decision)
	assert.Equal(t, "preToolUse", records[1].Event)
	assert.Equal(t, "deny", records[1].Decision)
}

func TestConcurrentHookRecordsAndUnsafePaths(t *testing.T) {
	if !loggingSupported() {
		t.Skip("private logging is unavailable")
	}
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	const count = 40
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- appendHookRecord(HookRecord{Version: 1})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Len(t, readHookRecords(t), count)
	path, err := HookLogPath()
	require.NoError(t, err)
	require.NoError(t, os.Chmod(path, 0o644))
	assert.Error(t, appendHookRecord(HookRecord{Version: 1}))
}

func TestConcurrentHookProcesses(t *testing.T) {
	if !loggingSupported() {
		t.Skip("private logging is unavailable")
	}
	if os.Getenv("HOOK_LOG_PROCESS_CHILD") == "1" {
		require.NoError(t, appendHookRecord(HookRecord{Version: 1}))
		return
	}
	t.Setenv("COPILOT_HOME", t.TempDir())
	t.Setenv("HOOK_LOG_PROCESS_CHILD", "1")
	const count = 8
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- exec.Command(os.Args[0], "-test.run=^TestConcurrentHookProcesses$").Run()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Len(t, readHookRecords(t), count)
}

func TestHookIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		input, id, tool string
	}{
		{`{"session_id":"12345678-1234-1234-1234-123456789abc","toolName":"view"}`, "12345678-1234-1234-1234-123456789abc", "view"},
		{`{"sessionId":"invalid","toolName":"path/secret"}`, "", ""},
		{`{"sessionId":"12345678-1234-1234-1234-123456789abc","session_id":"12345678-1234-1234-1234-123456789abd"}`, "", ""},
	} {
		req, err := ParseRequest(strings.NewReader(tc.input))
		require.NoError(t, err)
		assert.Equal(t, tc.id, req.SessionID)
		assert.Equal(t, tc.tool, safeToolName(req.ToolName))
	}
}

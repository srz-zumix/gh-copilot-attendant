package permissionhook

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testConfig = `
hooks:
  permissionRequest:
    allow:
      commands:
        - "git status:*"
        - "git log"
        - "ls:*"
        - "tail:*"
        - "go test:*"
      tools:
        - "web_fetch"
        - "example-mcp-read_*"
        - "bash"
    deny:
      commands:
        - "rm:*"
        - "git push:*"
      tools:
        - "example-mcp-delete_*"
`

func mustConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(testConfig))
	require.NoError(t, err)
	return cfg
}

func TestCommandRuleMatch(t *testing.T) {
	prefix, err := ParseCommandRule("git status:*")
	require.NoError(t, err)
	assert.True(t, prefix.Match([]string{"git", "status"}))
	assert.True(t, prefix.Match([]string{"git", "status", "--short"}))
	assert.False(t, prefix.Match([]string{"git", "statusx"}))
	assert.False(t, prefix.Match([]string{"git"}))

	exact, err := ParseCommandRule("git log")
	require.NoError(t, err)
	assert.True(t, exact.Match([]string{"git", "log"}))
	assert.False(t, exact.Match([]string{"git", "log", "-p"}))

	_, err = ParseCommandRule(" :* ")
	assert.Error(t, err)
}

func TestParseConfigErrors(t *testing.T) {
	_, err := ParseConfig([]byte("hooks:\n  permissionRequest:\n    allow:\n      command: [ls]\n"))
	assert.Error(t, err, "unknown field must be rejected")
	_, err = ParseConfig([]byte("hooks:\n  permissionRequest:\n    allow:\n      ghRead: invalid\n"))
	assert.Error(t, err)
	_, err = ParseConfig([]byte("hooks:\n  permissionRequest:\n    allow:\n      commands: [\"\"]\n"))
	assert.Error(t, err)
	_, err = ParseConfig([]byte("hooks:\n  permissionRequest:\n    deny:\n      tools: [\"[\"]\n"))
	assert.Error(t, err)
	cfg, err := ParseConfig(nil)
	require.NoError(t, err)
	assert.Empty(t, cfg.Hooks.PermissionRequest.Allow.Commands)
}

func TestDecide(t *testing.T) {
	cfg := mustConfig(t)
	tests := []struct {
		name string
		req  Request
		want Behavior
	}{
		{"allowed command", Request{ToolName: "bash", Command: "git status --short"}, BehaviorAllow},
		{"all segments allowed", Request{ToolName: "bash", Command: "go test ./... 2>&1 | tail -20"}, BehaviorAllow},
		{"one segment not allowed", Request{ToolName: "bash", Command: "ls && curl example.invalid"}, BehaviorNone},
		{"exact rule rejects extra args", Request{ToolName: "bash", Command: "git log -p"}, BehaviorNone},
		{"deny wins", Request{ToolName: "bash", Command: "ls && rm -rf build"}, BehaviorDeny},
		{"deny exact prefix", Request{ToolName: "bash", Command: "git push origin main"}, BehaviorDeny},
		{"unsupported syntax", Request{ToolName: "bash", Command: "ls $(rm -rf /)"}, BehaviorNone},
		{"glob cannot bypass deny", Request{ToolName: "bash", Command: "git p?sh origin main"}, BehaviorNone},
		{"redirect to file", Request{ToolName: "bash", Command: "ls > out.txt"}, BehaviorNone},
		{"env prefix not matched", Request{ToolName: "bash", Command: "PAGER=evil git log"}, BehaviorNone},
		{"empty command", Request{ToolName: "bash"}, BehaviorNone},
		{"powershell never allowed", Request{ToolName: "powershell", Command: "ls"}, BehaviorNone},
		{"sandbox bypass not allowed", Request{ToolName: "bash", Command: "ls", SandboxBypass: true}, BehaviorNone},
		{"sandbox bypass still denied", Request{ToolName: "bash", Command: "rm x", SandboxBypass: true}, BehaviorDeny},
		{"tool allowed", Request{ToolName: "web_fetch"}, BehaviorAllow},
		{"tool glob allowed", Request{ToolName: "example-mcp-read_file"}, BehaviorAllow},
		{"tool glob denied", Request{ToolName: "example-mcp-delete_file"}, BehaviorDeny},
		{"tool sandbox bypass", Request{ToolName: "web_fetch", SandboxBypass: true}, BehaviorNone},
		{"unknown tool", Request{ToolName: "edit"}, BehaviorNone},
		{"missing tool name", Request{Command: "ls"}, BehaviorNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			got := Decide(cfg, &req)
			assert.Equal(t, tt.want, got.Behavior)
			if got.Behavior == BehaviorDeny {
				assert.NotEmpty(t, got.Message)
			}
		})
	}
}

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Request
	}{
		{"camelCase object", `{"toolName":"bash","toolInput":{"command":"ls"}}`, Request{ToolName: "bash", Command: "ls"}},
		{"snake_case", `{"tool_name":"bash","tool_input":{"command":"ls"}}`, Request{ToolName: "bash", Command: "ls"}},
		{"toolArgs string", `{"toolName":"bash","toolArgs":"{\"command\":\"ls\"}"}`, Request{ToolName: "bash", Command: "ls"}},
		{"consistent duplicates", `{"toolName":"bash","toolInput":{"command":"ls"},"toolArgs":{"command":"ls"}}`, Request{ToolName: "bash", Command: "ls"}},
		{"sandbox bypass", `{"toolName":"bash","toolInput":{"command":"ls","requestSandboxBypass":true}}`, Request{ToolName: "bash", Command: "ls", SandboxBypass: true}},
		{"no input", `{"toolName":"web_fetch","toolInput":null}`, Request{ToolName: "web_fetch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRequest(strings.NewReader(tt.in))
			require.NoError(t, err)
			assert.Equal(t, tt.want.ToolName, got.ToolName)
			assert.Equal(t, tt.want.Command, got.Command)
			assert.Equal(t, tt.want.SandboxBypass, got.SandboxBypass)
		})
	}
}

func TestParseRequestErrors(t *testing.T) {
	inputs := []string{
		``,
		`[]`,
		`{"toolName":"bash","toolName":"view"}`,
		`{"toolName":"bash","tool_name":"view"}`,
		`{"toolName":1}`,
		`{"toolName":"bash","toolInput":{"command":"ls","command":"rm -rf /"}}`,
		`{"toolName":"bash","toolInput":{"command":"ls"},"toolArgs":{"command":"rm x"}}`,
		`{"toolName":"view","toolInput":{"path":"/safe"},"toolArgs":{"path":"/other"}}`,
		`{"toolName":"rg","toolInput":{"pattern":"x","paths":"/safe"},"toolArgs":{"pattern":"x"}}`,
		`{"toolName":"bash","toolInput":{"command":1}}`,
		`{"toolName":"bash","toolInput":{"requestSandboxBypass":"yes"}}`,
		`{"toolName":"view","cwd":null}`,
		`{"toolName":"bash","toolInput":"not json"}`,
		`{"toolName":"list_repositories","toolArgs":[]}`,
		`{"toolName":"apply_patch","toolArgs":"{\"x\":1,\"x\":2}"}`,
		`{"toolName":"apply_patch","toolInput":"patch one","toolArgs":"patch two"}`,
		`{"toolName":"apply_patch","toolInput":{"x":1},"toolArgs":"freeform text"}`,
		`{"toolName":"bash"} {}`,
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			_, err := ParseRequest(strings.NewReader(in))
			assert.Error(t, err)
		})
	}
}

func TestOpaqueToolInputs(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      tools: ["apply_patch", "future_tool"]
`))
	require.NoError(t, err)
	denied, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    deny:
      tools: ["apply_patch"]
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		name  string
		input string
		tool  string
	}{
		{"actual freeform", `{"toolName":"apply_patch","toolArgs":"*** Begin Patch\n*** End Patch\n"}`, "apply_patch"},
		{"snake case", `{"tool_name":"apply_patch","tool_input":"*** Begin Patch\n*** End Patch\n"}`, "apply_patch"},
		{"freeform starting with brace", `{"toolName":"apply_patch","toolArgs":"{not JSON"}`, "apply_patch"},
		{"array", `{"toolName":"future_tool","toolArgs":[1,2]}`, "future_tool"},
		{"number", `{"toolName":"future_tool","toolArgs":42}`, "future_tool"},
		{"consistent opaque aliases", `{"toolName":"apply_patch","toolInput":"patch","toolArgs":"patch"}`, "apply_patch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := ParseRequest(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.tool, req.ToolName)
			assert.Equal(t, BehaviorNone, Decide(cfg, req).Behavior)
			assert.Equal(t, BehaviorNone, DecideAllRulesPreToolUse(cfg, req).Behavior)
			if tt.tool == "apply_patch" {
				assert.Equal(t, BehaviorDeny, Decide(denied, req).Behavior)
				assert.Equal(t, BehaviorDeny, DecideAllRulesPreToolUse(denied, req).Behavior)
			}
		})
	}

	req, err := ParseRequest(strings.NewReader(`{"toolName":"future_tool","toolArgs":{"command":42}}`))
	require.NoError(t, err)
	assert.Equal(t, BehaviorAllow, Decide(cfg, req).Behavior)
}

func TestOpaqueArgumentsCannotSatisfyArgumentFreeAllow(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      tools: ["list_repositories"]
`))
	require.NoError(t, err)
	req := &Request{ToolName: "list_repositories", OpaqueInput: true}
	assert.Equal(t, BehaviorNone, Decide(cfg, req).Behavior)
	assert.Equal(t, BehaviorNone, DecidePreToolUse(cfg, req).Behavior)
	assert.Equal(t, BehaviorNone, DecideAllRulesPreToolUse(cfg, req).Behavior)

	cfg.Hooks.PermissionRequest.Deny.Tools = []string{"list_repositories"}
	assert.Equal(t, BehaviorDeny, Decide(cfg, req).Behavior)
	assert.Equal(t, BehaviorDeny, DecidePreToolUse(cfg, req).Behavior)
	assert.Equal(t, BehaviorDeny, DecideAllRulesPreToolUse(cfg, req).Behavior)
}

func assertHookWarning(t *testing.T, output, category string) {
	t.Helper()
	assert.Equal(t, 1, strings.Count(output, "\n"))
	var warning struct {
		Type               string `json:"type"`
		Message            string `json:"message"`
		Behavior           string `json:"behavior"`
		PermissionDecision string `json:"permissionDecision"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &warning))
	assert.Equal(t, "progress", warning.Type)
	assert.Contains(t, warning.Message, category)
	assert.Contains(t, warning.Message, "no permission decision")
	assert.Empty(t, warning.Behavior)
	assert.Empty(t, warning.PermissionDecision)
	assert.NotContains(t, output, "synthetic-secret")
}

type closedWriter struct{}

func (closedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestHookEvaluationErrorsWarnAndAbstain(t *testing.T) {
	t.Setenv("COPILOT_HOME", t.TempDir())
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(testConfig), 0o600))
	badConfig := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(badConfig, []byte("hooks: [synthetic-secret\n"), 0o600))

	for _, tt := range []struct {
		name string
		run  func(io.Reader, io.Writer, string) error
	}{
		{"permission request", Run},
		{"all rules pre-tool-use", RunAllRulesPreToolUse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, failure := range []struct {
				name     string
				input    string
				config   string
				category string
			}{
				{"invalid config", `{}`, badConfig, "configuration"},
				{"invalid input", `{"toolName":"bash","toolArgs":"synthetic-secret"}`, config, "input"},
				{"duplicate key", `{"toolName":"bash","toolName":"synthetic-secret"}`, config, "input"},
				{"conflicting arguments", `{"toolName":"bash","toolInput":{"command":"ls"},"toolArgs":{"command":"synthetic-secret"}}`, config, "input"},
				{"invalid input for denied tool", `{"toolName":"example-mcp-delete_file","toolArgs":"{\"value\":1,\"value\":2}"}`, config, "input"},
			} {
				t.Run(failure.name, func(t *testing.T) {
					var out bytes.Buffer
					require.NoError(t, tt.run(strings.NewReader(failure.input), &out, failure.config))
					assertHookWarning(t, out.String(), failure.category)
				})
			}
			var out bytes.Buffer
			require.NoError(t, tt.run(brokenReader{}, &out, config))
			assertHookWarning(t, out.String(), "input")
			assert.Error(t, tt.run(strings.NewReader(`not json`), closedWriter{}, config))
		})
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(testConfig), 0o600))

	var out bytes.Buffer
	require.NoError(t, Run(strings.NewReader(`{"toolName":"bash","toolInput":{"command":"git status"}}`), &out, cfgPath))
	assert.Equal(t, "{\"behavior\":\"allow\"}\n", out.String())

	out.Reset()
	require.NoError(t, Run(strings.NewReader(`{"toolName":"bash","toolInput":{"command":"rm a"}}`), &out, cfgPath))
	assert.Contains(t, out.String(), `"behavior":"deny"`)

	out.Reset()
	require.NoError(t, Run(strings.NewReader(`{"toolName":"bash","toolInput":{"command":"curl x"}}`), &out, cfgPath))
	assert.Empty(t, out.String())

	out.Reset()
	require.NoError(t, Run(strings.NewReader("ignored"), &out, filepath.Join(dir, "missing.yaml")))
	assert.Empty(t, out.String(), "missing config gives no decision")

	bad := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("hooks: [\n"), 0o600))
	out.Reset()
	require.NoError(t, Run(strings.NewReader("{}"), &out, bad))
	assertHookWarning(t, out.String(), "configuration")
}

func TestRunEnvironmentAssignments(t *testing.T) {
	t.Setenv("COPILOT_HOME", t.TempDir())
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`
hooks:
  logging:
    disabled: true
  permissionRequest:
    allow:
      commands: ["go build:*"]
    environmentAssignments:
      mode: denylist
`), 0o600))
	payload := `{"toolName":"bash","toolInput":{"command":"GOPROXY=off GOSUMDB=off go build"}}`
	for _, tt := range []struct {
		name string
		run  func(io.Reader, io.Writer, string) error
		want string
	}{
		{"permission request", Run, `{"behavior":"allow"}`},
		{"all rules pre-tool-use", RunAllRulesPreToolUse, `{"permissionDecision":"allow"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, tt.run(strings.NewReader(payload), &out, config))
			assert.JSONEq(t, tt.want, out.String())
		})
	}
}

func TestRunGHReadPreset(t *testing.T) {
	t.Setenv("COPILOT_HOME", t.TempDir())
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`
hooks:
  logging:
    disabled: true
  permissionRequest:
    allow:
      ghRead: true
      commands: ["go build:*"]
    deny:
      commands: ["gh issue view 99"]
`), 0o600))
	for _, mode := range []struct {
		name  string
		run   func(io.Reader, io.Writer, string) error
		field string
	}{
		{"permission request", Run, "behavior"},
		{"all rules pre-tool-use", RunAllRulesPreToolUse, "permissionDecision"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			for _, tt := range []struct {
				payload string
				want    string
			}{
				{`{"toolName":"bash","toolInput":{"command":"gh issue view"}}`, "allow"},
				{`{"toolName":"bash","toolInput":{"command":"go build; GH_TOKEN=synthetic gh issue view"}}`, "allow"},
				{`{"toolName":"bash","toolInput":{"command":"gh issue view 99"}}`, "deny"},
				{`{"toolName":"bash","toolInput":{"command":"gh issue view 99","requestSandboxBypass":true}}`, "deny"},
				{`{"toolName":"bash","toolInput":{"command":"git status; gh issue view 99"}}`, "deny"},
				{`{"toolName":"bash","toolInput":{"command":"PATH=/fictional go build; gh issue view 99"}}`, "deny"},
				{`{"toolName":"bash","toolInput":{"command":"gh issue view; git status"}}`, ""},
				{`{"toolName":"bash","toolInput":{"command":"PATH=/fictional gh issue view"}}`, ""},
				{`{"toolName":"bash","toolInput":{"command":"gh issue view","requestSandboxBypass":true}}`, ""},
			} {
				var out bytes.Buffer
				require.NoError(t, mode.run(strings.NewReader(tt.payload), &out, config))
				if tt.want == "" {
					assert.Empty(t, out.String(), tt.payload)
					continue
				}
				var result map[string]any
				require.NoError(t, json.Unmarshal(out.Bytes(), &result))
				assert.Equal(t, tt.want, result[mode.field], tt.payload)
			}
		})
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Setenv(ConfigEnvVar, "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	p, err := ResolveConfigPath("")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/xdg", "gh-copilot-attendant", "config.yaml"), p)

	t.Setenv(ConfigEnvVar, "/env/config.yaml")
	p, err = ResolveConfigPath("")
	require.NoError(t, err)
	assert.Equal(t, "/env/config.yaml", p)

	p, err = ResolveConfigPath("/flag/config.yaml")
	require.NoError(t, err)
	assert.Equal(t, "/flag/config.yaml", p)
}

func TestInstallHook(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(ConfigEnvVar, "")

	p, err := InstallHook(InstallOptions{TimeoutSec: 5})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "hooks", HookFileName), p)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"hooks":{
		"permissionRequest":[{"type":"command","exec":"gh","args":["copilot-attendant","hook","permission-request"],"timeoutSec":5}]
	}}`, string(data))

	_, err = InstallHook(InstallOptions{TimeoutSec: 5})
	assert.Error(t, err, "existing file requires --force")

	cfgPath := filepath.Join(dir, "config.yaml")
	_, err = InstallHook(InstallOptions{TimeoutSec: 5, Force: true, ConfigPath: cfgPath})
	require.NoError(t, err)
	data, err = os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), cfgPath))

	_, err = InstallHook(InstallOptions{Path: filepath.Join(dir, "x.json"), TimeoutSec: 0})
	assert.Error(t, err)
}

func TestInstallHookAutomaticallySelectsMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COPILOT_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(ConfigEnvVar, "")
	for _, tt := range []struct {
		name        string
		rules       string
		event       string
		allRules    bool
		explicitOff bool
	}{
		{"commands", "allow:\n      commands: ['git status']", "permissionRequest", false, false},
		{"gh read preset", "allow:\n      ghRead: true", "preToolUse", true, false},
		{"git read preset", "allow:\n      gitRead: true", "preToolUse", true, false},
		{"gh read preset disabled", "allow:\n      ghRead: false", "permissionRequest", false, false},
		{"scoped", "allow:\n      scopedTools:\n        - tools: [view]\n          paths: ['" + dir + "']", "preToolUse", true, false},
		{"mixed", "allow:\n      commands: ['git status']\n      scopedTools:\n        - tools: [view]\n          paths: ['" + dir + "']", "preToolUse", true, false},
		{"list repositories", "allow:\n      tools: [list_repositories]", "preToolUse", true, false},
		{"wildcard deny", "deny:\n      tools: ['*']", "preToolUse", true, false},
		{"explicit permission mode", "allow:\n      scopedTools:\n        - tools: [view]\n          paths: ['" + dir + "']", "permissionRequest", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(cfg, []byte("hooks:\n  permissionRequest:\n    "+tt.rules+"\n"), 0o600))
			dest := filepath.Join(t.TempDir(), "hook.json")
			_, err := InstallHook(InstallOptions{Path: dest, ConfigPath: cfg, TimeoutSec: 5, ScopedToolsSet: tt.explicitOff})
			require.NoError(t, err)
			data, err := os.ReadFile(dest)
			require.NoError(t, err)
			var file hookFile
			require.NoError(t, json.Unmarshal(data, &file))
			require.Len(t, file.Hooks, 1)
			entries := file.Hooks[tt.event]
			require.Len(t, entries, 1)
			if tt.allRules {
				assert.Contains(t, entries[0].Args, "--all-rules")
				assert.Empty(t, entries[0].Matcher)
			} else {
				assert.NotContains(t, entries[0].Args, "--all-rules")
			}
			_, err = InstallHook(InstallOptions{Path: dest, ConfigPath: cfg, TimeoutSec: 5})
			assert.ErrorContains(t, err, "already exists")
			unchanged, err := os.ReadFile(dest)
			require.NoError(t, err)
			assert.Equal(t, data, unchanged)
		})
	}
}

func TestInstallHookDoesNotFollowExistingDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(ConfigEnvVar, "")
	dest := filepath.Join(dir, "hook.json")
	target := filepath.Join(dir, "missing.json")
	if err := os.Symlink(target, dest); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := InstallHook(InstallOptions{Path: dest, TimeoutSec: 5})
	assert.ErrorContains(t, err, "already exists")
	assert.NoFileExists(t, target)
	info, err := os.Lstat(dest)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestInstallScopedHook(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
hooks:
  permissionRequest:
    allow:
      tools: [list_repositories]
`), 0o600))
	dest := filepath.Join(dir, "hook.json")
	p, err := InstallHook(InstallOptions{Path: dest, ConfigPath: cfgPath, TimeoutSec: 5, ScopedTools: true})
	require.NoError(t, err)
	assert.Equal(t, dest, p)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	configArg, err := json.Marshal(cfgPath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"hooks":{"preToolUse":[{"type":"command","exec":"gh",`+
		`"args":["copilot-attendant","hook","pre-tool-use","--config",`+
		string(configArg)+`],"matcher":"^(view|rg|grep|glob|web_fetch|list_repositories)$","timeoutSec":5}]}}`, string(data))
}

func TestScopedModeRejectsUnsupportedRules(t *testing.T) {
	for _, tt := range []struct {
		name string
		rule string
	}{
		{"allow command", "allow:\n      commands: ['git status']"},
		{"gh read preset", "allow:\n      ghRead: true"},
		{"git read preset", "allow:\n      gitRead: true"},
		{"deny command", "deny:\n      commands: ['rm:*']"},
		{"unscoped tool", "allow:\n      tools: [view]"},
		{"deny other tool", "deny:\n      tools: [bash]"},
		{"deny wildcard", "deny:\n      tools: ['*']"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(cfgPath, []byte("hooks:\n  permissionRequest:\n    "+tt.rule+"\n"), 0o600))
			dest := filepath.Join(dir, "hook.json")
			_, err := InstallHook(InstallOptions{Path: dest, ConfigPath: cfgPath, TimeoutSec: 5, ScopedTools: true})
			assert.ErrorContains(t, err, "scoped-tools mode")
			assert.NoFileExists(t, dest)
		})
	}
}

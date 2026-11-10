package permissionhook

import (
	"bytes"
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
			assert.Equal(t, tt.want, *got)
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
		`{"toolName":"bash","toolInput":{"command":1}}`,
		`{"toolName":"bash","toolInput":{"requestSandboxBypass":"yes"}}`,
		`{"toolName":"bash","toolInput":"not json"}`,
		`{"toolName":"bash"} {}`,
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			_, err := ParseRequest(strings.NewReader(in))
			assert.Error(t, err)
		})
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
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
	assert.Error(t, Run(strings.NewReader("{}"), &out, bad))
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

	p, err := InstallHook(InstallOptions{TimeoutSec: 5})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "hooks", HookFileName), p)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"hooks":{"permissionRequest":[{"type":"command","exec":"gh","args":["copilot-attendant","hook","permission-request"],"timeoutSec":5}]}}`, string(data))

	_, err = InstallHook(InstallOptions{TimeoutSec: 5})
	assert.Error(t, err, "existing file requires --force")

	cfgPath := filepath.Join(dir, "config.yaml")
	_, err = InstallHook(InstallOptions{TimeoutSec: 5, Force: true, ConfigPath: cfgPath})
	require.NoError(t, err)
	data, err = os.ReadFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(data), cfgPath)

	_, err = InstallHook(InstallOptions{Path: filepath.Join(dir, "x.json"), TimeoutSec: 0})
	assert.Error(t, err)
}

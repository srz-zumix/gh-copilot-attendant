package permissionhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scopedRequest(t *testing.T, tool, cwd string, args map[string]any) *Request {
	t.Helper()
	payload := map[string]any{"toolName": tool, "toolArgs": args}
	if cwd != "" {
		payload["cwd"] = cwd
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := ParseRequest(bytes.NewReader(data))
	require.NoError(t, err)
	return req
}

func TestScopedPathDecisions(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "allowed")
	other := filepath.Join(base, "allowed-other")
	for _, dir := range []string{root, filepath.Join(root, "src"), other} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	for _, p := range []string{filepath.Join(root, "inside.txt"), filepath.Join(root, "src", "inside.go"), filepath.Join(other, "outside.txt")} {
		require.NoError(t, os.WriteFile(p, []byte("synthetic"), 0o600))
	}

	cfg, err := ParseConfig([]byte(fmt.Sprintf(`
hooks:
  permissionRequest:
    allow:
      tools: [list_repositories]
      scopedTools:
        - tools: [view, rg, grep, glob]
          paths: [%q]
`, root)))
	require.NoError(t, err)

	tests := []struct {
		name string
		tool string
		cwd  string
		args map[string]any
		want Behavior
	}{
		{"view file", "view", root, map[string]any{"path": filepath.Join(root, "inside.txt")}, BehaviorAllow},
		{"view relative", "view", root, map[string]any{"path": "src/inside.go"}, BehaviorAllow},
		{"view relative without cwd", "view", "", map[string]any{"path": "src/inside.go"}, BehaviorNone},
		{"view outside", "view", root, map[string]any{"path": filepath.Join(other, "outside.txt")}, BehaviorNone},
		{"view sibling prefix", "view", root, map[string]any{"path": filepath.Join(other, "outside.txt")}, BehaviorNone},
		{"view parent traversal", "view", root, map[string]any{"path": "../allowed-other/outside.txt"}, BehaviorNone},
		{"view nonexistent", "view", root, map[string]any{"path": "missing.txt"}, BehaviorNone},
		{"view unknown option", "view", root, map[string]any{"path": "inside.txt", "follow": true}, BehaviorNone},
		{"rg default cwd", "rg", root, map[string]any{"pattern": "x"}, BehaviorAllow},
		{"rg missing cwd", "rg", "", map[string]any{"pattern": "x"}, BehaviorNone},
		{"rg multiple paths", "rg", root, map[string]any{"pattern": "x", "paths": []string{root, filepath.Join(root, "src")}}, BehaviorAllow},
		{"rg mixed paths", "rg", root, map[string]any{"pattern": "x", "paths": []string{root, other}}, BehaviorNone},
		{"rg empty paths", "rg", root, map[string]any{"pattern": "x", "paths": []string{}}, BehaviorNone},
		{"rg unknown option", "rg", root, map[string]any{"pattern": "x", "follow": true}, BehaviorNone},
		{"grep single path", "grep", root, map[string]any{"pattern": "x", "paths": "src"}, BehaviorAllow},
		{"glob recursive", "glob", root, map[string]any{"pattern": "**/*.go"}, BehaviorAllow},
		{"glob brace extensions", "glob", root, map[string]any{"pattern": "src/*.{go,md}"}, BehaviorAllow},
		{"glob traversal", "glob", root, map[string]any{"pattern": "../allowed-other/*"}, BehaviorNone},
		{"glob absolute", "glob", root, map[string]any{"pattern": filepath.Join(other, "*.txt")}, BehaviorNone},
		{"glob absolute brace alternative", "glob", root, map[string]any{"pattern": "{src,/other}/*.go"}, BehaviorNone},
		{"glob unknown option", "glob", root, map[string]any{"pattern": "*", "follow": true}, BehaviorNone},
		{"sandbox bypass", "view", root, map[string]any{"path": "inside.txt", "requestSandboxBypass": true}, BehaviorNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := scopedRequest(t, tt.tool, tt.cwd, tt.args)
			assert.Equal(t, tt.want, DecidePreToolUse(cfg, req).Behavior)
			assert.Equal(t, tt.want, Decide(cfg, req).Behavior)
		})
	}
	assert.Equal(t, BehaviorAllow, Decide(cfg, scopedRequest(t, "list_repositories", root, nil)).Behavior)
	assert.Equal(t, BehaviorAllow, DecidePreToolUse(cfg, scopedRequest(t, "list_repositories", root, map[string]any{})).Behavior)
	assert.Equal(t, BehaviorNone, DecidePreToolUse(cfg, scopedRequest(t, "list_repositories", root, map[string]any{"unexpected": true})).Behavior)

	cfg.Hooks.PermissionRequest.Deny.Tools = []string{"view"}
	denied := DecidePreToolUse(cfg, scopedRequest(t, "view", root, map[string]any{"path": "inside.txt"}))
	assert.Equal(t, BehaviorDeny, denied.Behavior)
	assert.NotEmpty(t, denied.Message)
}

func TestScopedPathsRejectSymlinkEscapes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "allowed")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "inside.txt"), []byte("outside"), 0o600))
	err := os.Symlink(filepath.Join(outside, "nested"), filepath.Join(root, "link"))
	if err != nil && runtime.GOOS == "windows" {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	require.NoError(t, err)

	cfg, err := ParseConfig([]byte(fmt.Sprintf(`
hooks:
  permissionRequest:
    allow:
      scopedTools:
        - tools: [view, rg, glob]
          paths: [%q]
`, root)))
	require.NoError(t, err)
	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{"view", map[string]any{"path": "link/../inside.txt"}},
		{"rg", map[string]any{"pattern": "x", "paths": "link"}},
		{"glob", map[string]any{"pattern": "link/*"}},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			req := scopedRequest(t, tt.tool, root, tt.args)
			assert.Equal(t, BehaviorNone, DecidePreToolUse(cfg, req).Behavior)
		})
	}
}

func TestScopedURLDecisions(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      scopedTools:
        - tools: [web_fetch]
          urls: ["https://example.invalid/guide?lang=en"]
          allowDespiteNativeDeny: true
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		name string
		args map[string]any
		want Behavior
	}{
		{"exact", map[string]any{"url": "https://example.invalid/guide?lang=en"}, BehaviorAllow},
		{"different query", map[string]any{"url": "https://example.invalid/guide?lang=ja"}, BehaviorNone},
		{"different path", map[string]any{"url": "https://example.invalid/other?lang=en"}, BehaviorNone},
		{"different scheme", map[string]any{"url": "http://example.invalid/guide?lang=en"}, BehaviorNone},
		{"userinfo", map[string]any{"url": "https://user@example.invalid/guide?lang=en"}, BehaviorNone},
		{"unknown option", map[string]any{"url": "https://example.invalid/guide?lang=en", "target": "other"}, BehaviorNone},
		{"sandbox bypass", map[string]any{"url": "https://example.invalid/guide?lang=en", "requestSandboxBypass": true}, BehaviorNone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := scopedRequest(t, "web_fetch", "", tt.args)
			assert.Equal(t, tt.want, Decide(cfg, req).Behavior)
			assert.Equal(t, tt.want, DecidePreToolUse(cfg, req).Behavior)
		})
	}
	cfg.Hooks.PermissionRequest.Deny.Tools = []string{"web_fetch"}
	req := scopedRequest(t, "web_fetch", "", map[string]any{"url": "https://example.invalid/guide?lang=en"})
	assert.Equal(t, BehaviorDeny, DecidePreToolUse(cfg, req).Behavior)
	cfg.Hooks.PermissionRequest.Deny.Tools = nil
	cfg.Hooks.PermissionRequest.Allow.ScopedTools[0].AllowDespiteNativeDeny = false
	assert.Equal(t, BehaviorNone, Decide(cfg, req).Behavior)
	assert.Equal(t, BehaviorNone, DecidePreToolUse(cfg, req).Behavior)
}

func TestScopedRuleValidation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "allowed")
	for _, tt := range []struct {
		name string
		rule string
	}{
		{"no tools", fmt.Sprintf("paths: [%q]", root)},
		{"missing scope", "tools: [view]"},
		{"both scopes", fmt.Sprintf("tools: [view]\n          paths: [%q]\n          urls: ['https://example.invalid/']", root)},
		{"unknown path tool", fmt.Sprintf("tools: [other]\n          paths: [%q]", root)},
		{"wrong URL tool", "tools: [view]\n          urls: ['https://example.invalid/']\n          allowDespiteNativeDeny: true"},
		{"relative path", "tools: [view]\n          paths: [relative]"},
		{"invalid URL", "tools: [web_fetch]\n          urls: ['file:///tmp/test']\n          allowDespiteNativeDeny: true"},
		{"URL credentials", "tools: [web_fetch]\n          urls: ['https://user@example.invalid/']\n          allowDespiteNativeDeny: true"},
		{"URL without native deny acknowledgment", "tools: [web_fetch]\n          urls: ['https://example.invalid/']"},
		{"native deny acknowledgment on path", fmt.Sprintf("tools: [view]\n          paths: [%q]\n          allowDespiteNativeDeny: true", root)},
		{"repository without acknowledgment", "tools: [web_fetch]\n          currentRepository: true"},
		{"wrong repository tool", "tools: [view]\n          currentRepository: true\n          allowDespiteNativeDeny: true"},
		{"repository and URLs", "tools: [web_fetch]\n          currentRepository: true\n          urls: ['https://example.invalid/']\n          allowDespiteNativeDeny: true"},
		{"repository and paths", fmt.Sprintf("tools: [web_fetch]\n          currentRepository: true\n          paths: [%q]\n          allowDespiteNativeDeny: true", root)},
		{"disabled repository without scope", "tools: [web_fetch]\n          currentRepository: false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := "hooks:\n  permissionRequest:\n    allow:\n      scopedTools:\n        - " + tt.rule + "\n"
			_, err := ParseConfig([]byte(input))
			assert.Error(t, err)
		})
	}
	overlap := fmt.Sprintf("hooks:\n  permissionRequest:\n    allow:\n      tools: ['*']\n      scopedTools:\n        - tools: [view]\n          paths: [%q]\n", root)
	_, err := ParseConfig([]byte(overlap))
	assert.Error(t, err)

	withoutAcknowledgment := "hooks:\n  permissionRequest:\n    allow:\n      scopedTools:\n        - tools: [web_fetch]\n          urls: ['https://example.invalid/']\n"
	_, err = ParseConfig([]byte(withoutAcknowledgment))
	assert.ErrorContains(t, err, "allowDespiteNativeDeny: true")
}

func TestRunPreToolUse(t *testing.T) {
	root := t.TempDir()
	t.Setenv("COPILOT_HOME", t.TempDir())
	file := filepath.Join(root, "inside.txt")
	require.NoError(t, os.WriteFile(file, []byte("synthetic"), 0o600))
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(fmt.Sprintf(`
hooks:
  permissionRequest:
    allow:
      scopedTools:
        - tools: [view]
          paths: [%q]
    deny:
      tools: [glob]
`, root)), 0o600))

	payload, err := json.Marshal(map[string]any{"cwd": root, "toolName": "view", "toolArgs": map[string]any{"path": "inside.txt"}})
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, RunPreToolUse(bytes.NewReader(payload), &out, config))
	assert.JSONEq(t, `{"permissionDecision":"allow"}`, out.String())

	out.Reset()
	require.NoError(t, RunPreToolUse(strings.NewReader(`{"toolName":"view","toolArgs":{"path":"/missing"}}`), &out, config))
	assert.Empty(t, out.String())

	require.NoError(t, os.WriteFile(config, []byte(testConfig), 0o600))
	out.Reset()
	require.NoError(t, RunPreToolUse(bytes.NewReader(payload), &out, config))
	assertHookWarning(t, out.String(), "configuration")
}

func TestRunAllRulesPreToolUse(t *testing.T) {
	root := t.TempDir()
	t.Setenv("COPILOT_HOME", t.TempDir())
	file := filepath.Join(root, "inside.txt")
	require.NoError(t, os.WriteFile(file, []byte("fictional"), 0o600))
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(fmt.Sprintf(`
hooks:
  permissionRequest:
    allow:
      commands: ["git status"]
      tools: [list_repositories, edit]
      scopedTools:
        - tools: [view]
          paths: [%q]
    deny:
      commands: ["rm:*"]
      tools: ["gl*", "forbidden_tool"]
`, root)), 0o600))
	for _, tt := range []struct {
		name     string
		payload  string
		decision string
	}{
		{"allowed command", `{"toolName":"bash","toolArgs":{"command":"git status"}}`, "allow"},
		{"denied command", `{"toolName":"bash","toolArgs":{"command":"rm file"}}`, "deny"},
		{"mixed command denied", `{"toolName":"bash","toolArgs":{"command":"git status && rm file"}}`, "deny"},
		{"sandbox bypass", `{"toolName":"bash","toolArgs":{"command":"git status","requestSandboxBypass":true}}`, ""},
		{"scoped read", fmt.Sprintf(`{"toolName":"view","cwd":%q,"toolArgs":{"path":"inside.txt"}}`, root), "allow"},
		{"outside scope", `{"toolName":"view","toolArgs":{"path":"/fictional-outside"}}`, ""},
		{"wildcard deny", `{"toolName":"glob","toolArgs":{"pattern":"*.go"}}`, "deny"},
		{"unscoped tool", `{"toolName":"edit","toolArgs":{}}`, "allow"},
		{"unscoped tool opaque input", `{"toolName":"edit","toolArgs":"freeform text"}`, ""},
		{"unmatched freeform", `{"toolName":"apply_patch","toolArgs":"*** Begin Patch\n*** End Patch\n"}`, ""},
		{"denied freeform", `{"toolName":"forbidden_tool","toolArgs":"freeform text"}`, "deny"},
		{"list without arguments", `{"toolName":"list_repositories"}`, "allow"},
		{"list with arguments", `{"toolName":"list_repositories","toolArgs":{"unexpected":true}}`, ""},
		{"unmatched", `{"toolName":"fictional_tool","toolArgs":{}}`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, RunAllRulesPreToolUse(strings.NewReader(tt.payload), &out, config))
			if tt.decision == "" {
				assert.Empty(t, out.String())
			} else {
				var result struct {
					Decision string `json:"permissionDecision"`
				}
				require.NoError(t, json.Unmarshal(out.Bytes(), &result))
				assert.Equal(t, tt.decision, result.Decision)
			}
		})
	}
}

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

func TestGitReadPreset(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      gitRead: true
      ghRead: true
      commands: ["go build:*"]
    deny:
      commands: ["git status --dangerous"]
    environmentAssignments:
      mode: denylist
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		command string
		want    Behavior
	}{
		{"git status", BehaviorAllow},
		{"git status --short --branch", BehaviorAllow},
		{"git --no-pager -C ./example status --porcelain=v1", BehaviorAllow},
		{"git log --oneline -5", BehaviorAllow},
		{"git show --stat HEAD", BehaviorAllow},
		{"git diff --stat HEAD -- example.go", BehaviorAllow},
		{"git diff --name-only --cached", BehaviorAllow},
		{"git branch", BehaviorAllow},
		{"git branch --show-current", BehaviorAllow},
		{"git branch --list 'feature/*'", BehaviorAllow},
		{"git tag --list 'v*'", BehaviorAllow},
		{"git rev-parse --show-toplevel", BehaviorAllow},
		{"git ls-files --cached", BehaviorAllow},
		{"git ls-tree -r --name-only HEAD", BehaviorAllow},
		{"git show-ref --heads", BehaviorNone},
		{"git show-ref --branches", BehaviorAllow},
		{"git for-each-ref --format='%(refname)'", BehaviorAllow},
		{"git describe --tags", BehaviorAllow},
		{"git cat-file -p HEAD", BehaviorAllow},
		{"git remote -v", BehaviorAllow},
		{"git worktree list", BehaviorAllow},
		{"NO_COLOR=1 git status", BehaviorAllow},
		{"go build; git status; gh issue list", BehaviorAllow},
		{"git status --dangerous", BehaviorDeny},
		{"git status; git status --dangerous", BehaviorDeny},
		{"git push", BehaviorNone},
		{"git reset --hard", BehaviorNone},
		{"git branch new-name", BehaviorNone},
		{"git branch --delete name", BehaviorNone},
		{"git branch --list -D name", BehaviorNone},
		{"git tag new-name", BehaviorNone},
		{"git tag -d name", BehaviorNone},
		{"git remote remove origin", BehaviorNone},
		{"git worktree remove example", BehaviorNone},
		{"git diff --output=out", BehaviorNone},
		{"git log --ext-diff", BehaviorNone},
		{"git show --textconv HEAD", BehaviorNone},
		{"git -c core.pager=example status", BehaviorNone},
		{"git --config-env=core.pager=EXAMPLE status", BehaviorNone},
		{"git -C --no-pager status", BehaviorNone},
		{"git log --format --output=out", BehaviorNone},
		{"git status --unknown", BehaviorNone},
		{"GIT_CONFIG_GLOBAL=example git status", BehaviorNone},
		{"PATH=/fictional git status", BehaviorNone},
		{"/fictional/git status", BehaviorNone},
		{"git status; git push", BehaviorNone},
	} {
		t.Run(tt.command, func(t *testing.T) {
			req := Request{ToolName: "bash", Command: tt.command}
			assert.Equal(t, tt.want, Decide(cfg, &req).Behavior)
		})
	}
	req := Request{ToolName: "bash", Command: "git status", SandboxBypass: true}
	assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior)
	cfg.Hooks.PermissionRequest.Allow.GitRead = false
	req.SandboxBypass = false
	assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior)
}

func TestGitReadPresetHookModes(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`
hooks:
  logging:
    disabled: true
  permissionRequest:
    allow:
      gitRead: true
    deny:
      commands: ["git log --all"]
`), 0o600))
	for _, run := range []func(*bytes.Buffer, string) error{
		func(out *bytes.Buffer, input string) error { return Run(strings.NewReader(input), out, config) },
		func(out *bytes.Buffer, input string) error {
			return RunAllRulesPreToolUse(strings.NewReader(input), out, config)
		},
	} {
		for _, tt := range []struct {
			command string
			want    string
		}{
			{"git status", "allow"},
			{"git branch new-name", ""},
			{"git status; git log --all", "deny"},
			{"git status; git push", ""},
		} {
			var out bytes.Buffer
			input := `{"toolName":"bash","toolInput":{"command":"` + tt.command + `"}}`
			require.NoError(t, run(&out, input))
			if tt.want == "" {
				assert.Empty(t, out.String())
			} else {
				assert.Contains(t, out.String(), `"`+tt.want+`"`)
			}
		}
	}
}

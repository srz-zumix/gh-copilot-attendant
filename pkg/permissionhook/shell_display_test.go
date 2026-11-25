package permissionhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellDisplayPreset(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "work")
	extra := filepath.Join(base, "extra")
	outside := filepath.Join(base, "work-other")
	for _, dir := range []string{cwd, extra, outside} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "input.txt"), []byte("example\n"), 0o600))
	}
	cfg, err := ParseConfig([]byte(fmt.Sprintf(`
hooks:
  permissionRequest:
    allow:
      shellDisplay: true
      gitRead: true
      commands: ["go build:*"]
      scopedTools:
        - tools: [view]
          paths: [%q]
    deny:
      commands: ["head forbidden.txt"]
`, extra)))
	require.NoError(t, err)

	tests := []struct {
		command string
		want    Behavior
	}{
		{"ls", BehaviorAllow},
		{"ls -la", BehaviorAllow},
		{"NO_COLOR=1 ls -la", BehaviorAllow},
		{"PATH=/fictional ls", BehaviorNone},
		{"ls --directory .", BehaviorAllow},
		{"cat -n input.txt", BehaviorAllow},
		{"head -n 5 input.txt", BehaviorAllow},
		{"head -5 input.txt", BehaviorAllow},
		{"tail --lines=2 input.txt", BehaviorAllow},
		{"wc -l input.txt", BehaviorAllow},
		{"pwd -P", BehaviorAllow},
		{"echo hello", BehaviorAllow},
		{"printf '%s\\n' hello", BehaviorAllow},
		{"printf '%%n' hello", BehaviorAllow},
		{"cat " + fmt.Sprintf("%q", filepath.Join(extra, "input.txt")), BehaviorAllow},
		{"cat input.txt | head -n 1", BehaviorAllow},
		{"git status | head -n 1", BehaviorAllow},
		{"echo hello; wc input.txt", BehaviorAllow},
		{"head forbidden.txt", BehaviorDeny},
		{"echo hello; head forbidden.txt", BehaviorDeny},
		{"cat", BehaviorNone},
		{"cat -", BehaviorNone},
		{"echo hello; cat", BehaviorNone},
		{"echo hello | cat 0>&3", BehaviorNone},
		{"head input.txt >/dev/null", BehaviorNone},
		{"tail -f input.txt", BehaviorNone},
		{"tail --follow input.txt", BehaviorNone},
		{"ls -R", BehaviorNone},
		{"ls -L", BehaviorNone},
		{"ls --dereference", BehaviorNone},
		{"wc --files0-from=input.txt", BehaviorNone},
		{"head -n nope input.txt", BehaviorNone},
		{"printf -v name '%s' value", BehaviorNone},
		{"printf '%n' name", BehaviorNone},
		{"printf '%#3.3n' name", BehaviorNone},
		{"cat ../work-other/input.txt", BehaviorNone},
		{"cat " + fmt.Sprintf("%q", filepath.Join(outside, "input.txt")), BehaviorNone},
		{"cat missing.txt", BehaviorNone},
		{"cat .", BehaviorNone},
		{"go build; head input.txt", BehaviorNone},
		{"head input.txt & go build", BehaviorNone},
		{"head input.txt | go build", BehaviorNone},
		{"git status; head input.txt", BehaviorNone},
		{"echo hello && git push", BehaviorNone},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			req := Request{ToolName: "bash", CWD: cwd, Command: tt.command}
			assert.Equal(t, tt.want, Decide(cfg, &req).Behavior)
		})
	}
	req := Request{ToolName: "bash", CWD: cwd, Command: "head input.txt", SandboxBypass: true}
	assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior)
	req.SandboxBypass = false
	req.Arguments = map[string]json.RawMessage{"command": json.RawMessage(`"head input.txt"`), "shellId": json.RawMessage(`"existing"`)}
	assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior)
	req.Arguments = nil
	cfg.Hooks.PermissionRequest.Allow.ShellDisplay = false
	assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior)
}

func TestShellDisplayExtendedExamplesExecute(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input.txt"), []byte("beta:2\nalpha:1\nalpha:1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"items":[{"name":"example"}]}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "+1"), []byte("example-plus-file\n"), 0o600))
	cfg := &Config{}
	cfg.Hooks.PermissionRequest.Allow.ShellDisplay = true

	for _, tt := range []struct {
		command string
		bins    []string
		stdin   string
		want    string
	}{
		{"nl -ba input.txt", []string{"nl", "gnl"}, "", "alpha:1"},
		{"cut -d : -f 1 input.txt", []string{"cut", "gcut"}, "", "alpha"},
		{"uniq -c input.txt", []string{"uniq", "guniq"}, "", "alpha:1"},
		{"uniq -- +1", []string{"uniq", "guniq"}, "stdin", "example-plus-file"},
		{"stat input.txt", []string{"stat", "gstat"}, "", "input.txt"},
		{"sort -k1,1 -t : input.txt", []string{"sort", "gsort"}, "", "alpha:1"},
		{"sort -- +1", []string{"sort", "gsort"}, "stdin", "example-plus-file"},
		{"tail -- +1", []string{"tail", "gtail"}, "stdin", "example-plus-file"},
		{"sed -n '1,2p' input.txt", []string{"sed", "gsed"}, "", "alpha:1"},
		{"find . -maxdepth 1 -name 'input.txt'", []string{"find", "gfind"}, "", "input.txt"},
		{"jq -r '.items[] | .name' data.json", []string{"jq"}, "", "example"},
		{"printf abc | tr '[:lower:]' '[:upper:]'", []string{"tr", "gtr"}, "abc", "ABC"},
		{"basename -s .txt input.txt", []string{"basename", "gbasename"}, "", "input"},
		{"dirname ../outside/input.txt", []string{"dirname", "gdirname"}, "", "../outside"},
	} {
		t.Run(tt.command, func(t *testing.T) {
			req := Request{ToolName: "bash", CWD: dir, Command: tt.command}
			require.Equal(t, BehaviorAllow, Decide(cfg, &req).Behavior)
			segments, err := ParseBashCommand(tt.command)
			require.NoError(t, err)
			args := segments[len(segments)-1][1:]
			for _, bin := range tt.bins {
				t.Run(bin, func(t *testing.T) {
					path, err := exec.LookPath(bin)
					if err != nil {
						t.Skipf("%s is unavailable", bin)
					}
					cmd := exec.CommandContext(t.Context(), path, args...)
					cmd.Dir = dir
					cmd.Stdin = strings.NewReader(tt.stdin)
					output, err := cmd.CombinedOutput()
					require.NoErrorf(t, err, "%s", output)
					assert.Contains(t, string(output), tt.want)
				})
			}
		})
	}
}

func TestShellDisplayRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges")
	}
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "input"), []byte("example"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(outside, "input"), filepath.Join(root, "link")))
	cfg := &Config{}
	cfg.Hooks.PermissionRequest.Allow.ShellDisplay = true
	for _, command := range []string{"cat link", "ls link", "head /dev/null"} {
		req := Request{ToolName: "bash", CWD: root, Command: command}
		assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior, command)
	}
}

func TestShellDisplayExtendedCommands(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "work")
	extra := filepath.Join(base, "extra")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{cwd, extra, outside, filepath.Join(cwd, "~")} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "input.txt"), []byte("{\"items\":[{\"name\":\"example\"}]}\n"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "+1"), []byte("example\n"), 0o600))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(outside, "input.txt"), filepath.Join(cwd, "link")))
		require.NoError(t, os.Symlink(filepath.Join(outside, "input.txt"), filepath.Join(cwd, "-n")))
	}
	cfg := &Config{}
	cfg.Hooks.PermissionRequest.Allow.ShellDisplay = true
	cfg.Hooks.PermissionRequest.Allow.GitRead = true
	cfg.Hooks.PermissionRequest.Allow.ScopedTools = []ScopedToolRule{{Tools: []string{"view"}, Paths: []string{extra}}}
	cfg.Hooks.PermissionRequest.Deny.Commands = []string{"nl forbidden.txt"}
	cfg.Hooks.PermissionRequest.EnvironmentAssignments.Mode = "denylist"

	type testCase struct {
		name    string
		command string
		want    Behavior
	}
	tests := []testCase{
		{"nl file", "nl -ba input.txt", BehaviorAllow},
		{"nl options before operand", "POSIXLY_CORRECT=1 nl -s 1 input.txt", BehaviorAllow},
		{"nl long option", "nl --number-width=3 input.txt", BehaviorAllow},
		{"nl pipe", "cat input.txt | nl -s ': '", BehaviorAllow},
		{"cut fields", "cut -d : -f 1 input.txt", BehaviorAllow},
		{"cut range", "cut --bytes=1-3 input.txt", BehaviorAllow},
		{"cut pipe", "cat input.txt | cut -c -5", BehaviorAllow},
		{"uniq file", "uniq -f 1 input.txt", BehaviorAllow},
		{"uniq pipe", "cat input.txt | uniq -c", BehaviorAllow},
		{"uniq plus filename", "uniq -- +1", BehaviorAllow},
		{"stat file", "stat input.txt", BehaviorAllow},
		{"stat directory", "stat -L .", BehaviorAllow},
		{"sort file", "sort -rk1,1 -t : input.txt", BehaviorAllow},
		{"sort long option", "sort --key=1,1 input.txt", BehaviorAllow},
		{"sort pipe", "cat input.txt | sort -u", BehaviorAllow},
		{"sort plus filename", "sort -- +1", BehaviorAllow},
		{"tail plus filename", "tail -- +1", BehaviorAllow},
		{"sed address range", "sed -n '1,10p' input.txt", BehaviorAllow},
		{"sed expression flag", "sed -e '1p' input.txt", BehaviorAllow},
		{"sed pipe", "cat input.txt | sed -n '2,$p'", BehaviorAllow},
		{"find default root", "find", BehaviorAllow},
		{"find predicates", "find . -maxdepth 1 -name '*.txt' -type f -print", BehaviorAllow},
		{"find explicit scoped root", "find -P " + fmt.Sprintf("%q", extra) + " -type f", BehaviorAllow},
		{"jq file", "jq '.items[0].name' input.txt", BehaviorAllow},
		{"jq pipe", "cat input.txt | jq -r '.items[] | .name'", BehaviorAllow},
		{"jq null input", "jq -cn .", BehaviorAllow},
		{"jq long null input", "jq --null-input '.items'", BehaviorAllow},
		{"tr pipe", "cat input.txt | tr '[:lower:]' '[:upper:]'", BehaviorAllow},
		{"basename text", "basename -s .txt ../outside/input.txt", BehaviorAllow},
		{"dirname text", "dirname ../outside/input.txt", BehaviorAllow},
		{"scoped file", "nl " + fmt.Sprintf("%q", filepath.Join(extra, "input.txt")), BehaviorAllow},
		{"display pipeline", "find . -type f | sort | uniq", BehaviorAllow},
		{"sequential display", "sed -n '1p' input.txt; nl input.txt", BehaviorAllow},
		{"display before git", "nl input.txt; git status", BehaviorAllow},
		{"head options before operand", "POSIXLY_CORRECT=1 head -n 1 input.txt", BehaviorAllow},
		{"denied command", "nl forbidden.txt", BehaviorDeny},
		{"deny wins in pipeline", "echo hello | nl forbidden.txt", BehaviorDeny},
		{"nl unpiped", "nl", BehaviorNone},
		{"cut unpiped", "cut -f 1", BehaviorNone},
		{"uniq unpiped", "uniq", BehaviorNone},
		{"uniq obsolete option", "uniq +1", BehaviorNone},
		{"stat unpiped", "stat", BehaviorNone},
		{"stat does not consume pipe", "echo hello | stat", BehaviorNone},
		{"sort unpiped", "sort", BehaviorNone},
		{"sort obsolete option", "sort +1", BehaviorNone},
		{"tail obsolete option", "tail +1", BehaviorNone},
		{"sed unpiped", "sed -n '1p'", BehaviorNone},
		{"jq unpiped", "jq .", BehaviorNone},
		{"tr unpiped", "tr a b", BehaviorNone},
		{"basename missing operand", "basename", BehaviorNone},
		{"dirname missing operand", "dirname", BehaviorNone},
		{"outside file", "nl ../outside/input.txt", BehaviorNone},
		{"outside directory", "find ../outside -type f", BehaviorNone},
		{"outside jq input", "jq . ../outside/input.txt", BehaviorNone},
		{"unknown nl option", "nl --unknown input.txt", BehaviorNone},
		{"uniq output operand", "uniq input.txt result.txt", BehaviorNone},
		{"stat ambiguous format", "stat -f input.txt", BehaviorNone},
		{"stat ambiguous time", "stat -t input.txt", BehaviorNone},
		{"sort output", "sort -o result.txt input.txt", BehaviorNone},
		{"sort long output", "sort --output=result.txt input.txt", BehaviorNone},
		{"sort temp directory", "sort -T . input.txt", BehaviorNone},
		{"sort compression program", "sort --compress-program=example input.txt", BehaviorNone},
		{"sort random source", "sort --random-source=input.txt input.txt", BehaviorNone},
		{"sort files from", "sort --files0-from=input.txt", BehaviorNone},
		{"sed in place", "sed -i '' '1p' input.txt", BehaviorNone},
		{"sed script file", "sed -f input.txt input.txt", BehaviorNone},
		{"sed write", "sed '1w result.txt' input.txt", BehaviorNone},
		{"sed read", "sed '1r input.txt' input.txt", BehaviorNone},
		{"sed execute", "sed '1e echo example' input.txt", BehaviorNone},
		{"sed unsupported script", "sed -n '/example/p' input.txt", BehaviorNone},
		{"find follows links", "find -L . -type f", BehaviorNone},
		{"find follow predicate", "find . -follow -type f", BehaviorNone},
		{"find executes", "find . -exec echo example ';'", BehaviorNone},
		{"find deletes", "find . -delete", BehaviorNone},
		{"find writes", "find . -fprint result.txt", BehaviorNone},
		{"find reference file", "find . -newer input.txt", BehaviorNone},
		{"jq environment", "jq 'env' input.txt", BehaviorNone},
		{"jq environment variable", "jq '$ENV' input.txt", BehaviorNone},
		{"jq input function", "jq 'input' input.txt", BehaviorNone},
		{"jq script file", "jq -f input.txt input.txt", BehaviorNone},
		{"jq raw file", "jq --rawfile data input.txt .", BehaviorNone},
		{"jq module path", "jq -L . . input.txt", BehaviorNone},
		{"jq unknown expression", "jq 'map(.name)' input.txt", BehaviorNone},
		{"nl flag after operand", "POSIXLY_CORRECT=1 nl input.txt -s 1", BehaviorNone},
		{"head flag after operand", "POSIXLY_CORRECT=1 head input.txt -n 1", BehaviorNone},
		{"ls flag after operand", "POSIXLY_CORRECT=1 ls input.txt -l", BehaviorNone},
		{"cut flag after operand", "cut -f 1 input.txt -d :", BehaviorNone},
		{"sort flag after operand", "sort input.txt -o result.txt", BehaviorNone},
		{"display after git", "git status; nl input.txt", BehaviorNone},
		{"mixed path pipeline", "nl input.txt | git status", BehaviorNone},
		{"mixed pipeline before path", "git status | nl input.txt", BehaviorNone},
		{"redirect", "sort input.txt >/dev/null", BehaviorNone},
		{"unquoted tilde", "nl ~/input.txt", BehaviorNone},
		{"quoted tilde", "nl '~/input.txt'", BehaviorAllow},
	}
	if runtime.GOOS != "windows" {
		tests = append(tests,
			testCase{"file symlink escape", "uniq link", BehaviorNone},
			testCase{"find symlink escape", "find link -type f", BehaviorNone},
			testCase{"operand option symlink", "POSIXLY_CORRECT=1 head input.txt -n 1", BehaviorNone},
		)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{ToolName: "bash", CWD: cwd, Command: tt.command}
			assert.Equal(t, tt.want, Decide(cfg, &req).Behavior, tt.command)
		})
	}
}

func TestShellDisplayHookModes(t *testing.T) {
	cfg, err := ParseConfig([]byte("hooks:\n  permissionRequest:\n    allow:\n      shellDisplay: true\n"))
	require.NoError(t, err)
	assert.True(t, needsPreToolUse(cfg))
	assert.Error(t, validatePreToolConfig(cfg))
	cfg.Hooks.PermissionRequest.Allow.ShellDisplay = false
	assert.False(t, needsPreToolUse(cfg))
	assert.NoError(t, validatePreToolConfig(cfg))
}

func TestShellDisplayHookPayload(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input"), []byte("example"), 0o600))
	config := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`
hooks:
  logging:
    disabled: true
  permissionRequest:
    allow:
      shellDisplay: true
`), 0o600))
	for _, tt := range []struct {
		command string
		want    string
	}{
		{"head input", "allow"},
		{"nl input | sort | uniq", "allow"},
		{"find . -name 'input'", "allow"},
		{"jq -n .", "allow"},
		{"head ../input", ""},
		{"uniq input output", ""},
		{"sort -o output input", ""},
		{"sed -i '' '1p' input", ""},
		{"printf '%n' name", ""},
	} {
		payload, err := json.Marshal(map[string]any{
			"toolName": "bash", "cwd": dir,
			"toolArgs": map[string]any{"command": tt.command, "description": "display output"},
		})
		require.NoError(t, err)
		var out bytes.Buffer
		require.NoError(t, RunAllRulesPreToolUse(strings.NewReader(string(payload)), &out, config))
		if tt.want == "" {
			assert.Empty(t, out.String(), tt.command)
		} else {
			assert.Contains(t, out.String(), `"permissionDecision":"`+tt.want+`"`, tt.command)
		}
	}
}

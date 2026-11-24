package permissionhook

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repositoryURLConfig = `
hooks:
  logging:
    disabled: true
  permissionRequest:
    allow:
      scopedTools:
        - tools: [web_fetch]
          currentRepository: true
          allowDespiteNativeDeny: true
`

func TestRepositoryURLScope(t *testing.T) {
	for _, origin := range []string{
		"https://github.com/fixture-owner/fixture-project.git",
		"git@github.com:fixture-owner/fixture-project.git",
		"ssh://git@github.com:2222/fixture-owner/fixture-project.git",
	} {
		remote, err := git.ParseURL(origin)
		require.NoError(t, err)
		for _, tt := range []struct {
			url  string
			want bool
		}{
			{"https://github.com/fixture-owner/fixture-project", true},
			{"https://github.com/fixture-owner/fixture-project/", true},
			{"https://github.com/fixture-owner/fixture-project/pull/12?tab=files#diff", true},
			{"https://GITHUB.COM:443/Fixture-Owner/Fixture-Project/blob/main/caf%C3%A9.txt", true},
			{"https://api.github.com/repos/fixture-owner/fixture-project/issues?state=open", true},
			{"https://raw.githubusercontent.com/fixture-owner/fixture-project/main/file.txt", true},
			{"https://github.com/fixture-owner/fixture-project/issues?next=../other%2frepo", true},
			{"https://github.com/fixture-owner/fixture-project-other", false},
			{"https://api.github.com/repos/fixture-owner/fixture-project-other/issues", false},
			{"https://raw.githubusercontent.com/fixture-owner/fixture-project-other/main/file.txt", false},
			{"https://github.com/fixture-owner/other-project", false},
			{"https://github.com/other-owner/fixture-project", false},
			{"https://github.com/fixture-owner", false},
			{"https://api.github.com/user", false},
			{"https://github.com.evil.invalid/fixture-owner/fixture-project", false},
			{"https://user@github.com/fixture-owner/fixture-project", false},
			{"https://github.com:8443/fixture-owner/fixture-project", false},
			{"http://github.com/fixture-owner/fixture-project", false},
			{"https:github.com/fixture-owner/fixture-project", false},
			{"https://github.com/fixture-owner/fixture-project/../../other/project", false},
			{"https://github.com/fixture-owner/fixture-project/./issues", false},
			{"https://github.com/fixture-owner/fixture-project/%2e%2e/other-project", false},
			{"https://github.com/fixture-owner/fixture-project/.%2e/other-project", false},
			{"https://github.com/fixture-owner/fixture-project/%252e%252e/other-project", false},
			{"https://github.com/fixture-owner%2ffixture-project/issues", false},
			{"https://github.com/fixture-owner/fixture-project/%2F..%2Fother", false},
			{"https://github.com/fixture-owner/fixture-project/%5c..%5cother", false},
			{"https://github.com/fixture-owner/fixture-project/\\..\\other", false},
			{"https://github.com/fixture-owner/fixture-project//issues", false},
			{"https://github.com/fixture-owner/fixture-project/%00", false},
			{"https://github.com/fixture-owner/fixture-project/%GG", false},
		} {
			t.Run(origin+"/"+tt.url, func(t *testing.T) {
				assert.Equal(t, tt.want, matchesRepositoryURL(remote, tt.url))
			})
		}
	}
	remote, err := git.ParseURL("git@forge.test.invalid:fixture-owner/fixture-project.git")
	require.NoError(t, err)
	for _, tt := range []struct {
		url  string
		want bool
	}{
		{"https://forge.test.invalid/fixture-owner/fixture-project/issues/12", true},
		{"https://forge.test.invalid/api/v3/repos/fixture-owner/fixture-project/contents/file.txt", true},
		{"https://forge.test.invalid/fixture-owner/fixture-project/raw/main/file.txt", true},
		{"https://raw.forge.test.invalid/fixture-owner/fixture-project/main/file.txt", true},
		{"https://forge.test.invalid/api/v3/repos/fixture-owner/fixture-project-other", false},
		{"https://raw.forge.test.invalid/other-owner/fixture-project/main/file.txt", false},
		{"https://api.github.com/repos/fixture-owner/fixture-project", false},
		{"https://raw.githubusercontent.com/fixture-owner/fixture-project/main/file.txt", false},
		{"https://api.forge.test.invalid/repos/fixture-owner/fixture-project", false},
	} {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesRepositoryURL(remote, tt.url))
		})
	}
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return strings.TrimSpace(string(output))
}

func TestRepositoryURLDecisionsUseHookCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := ParseConfig([]byte(repositoryURLConfig))
	require.NoError(t, err)
	require.NoError(t, validatePreToolConfig(cfg))
	root := t.TempDir()
	fixtureGit(t, root, "init", "-q")
	fixtureGit(t, root, "remote", "add", "origin", "git@github.com:fixture-owner/fixture-project.git")
	// Only the first fetch URL is authoritative; push URLs and upstream are unrelated.
	fixtureGit(t, root, "config", "--add", "remote.origin.url", "https://github.com/other-owner/other-project.git")
	fixtureGit(t, root, "remote", "set-url", "--push", "origin", "https://github.com/push-owner/push-project.git")
	fixtureGit(t, root, "remote", "add", "upstream", "https://github.com/upstream-owner/upstream-project.git")
	fixtureGit(t, root, "-c", "user.name=Fixture Author", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-qm", "Fixture commit")
	subdir := filepath.Join(root, "src")
	require.NoError(t, os.Mkdir(subdir, 0o755))
	worktree := filepath.Join(t.TempDir(), "worktree")
	fixtureGit(t, root, "worktree", "add", "--detach", "-q", worktree)

	other := t.TempDir()
	fixtureGit(t, other, "init", "-q")
	fixtureGit(t, other, "remote", "add", "origin", "https://github.com/other-owner/other-project.git")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "remote.origin.url")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://github.com/override-owner/override-project.git")
	t.Setenv("GH_REPO", "github.com/override-owner/override-project")

	for _, dir := range []string{root, subdir, worktree} {
		for _, tt := range []struct {
			url  string
			want Behavior
		}{
			{"https://github.com/fixture-owner/fixture-project/pull/12", BehaviorAllow},
			{"https://api.github.com/repos/fixture-owner/fixture-project/issues", BehaviorAllow},
			{"https://raw.githubusercontent.com/fixture-owner/fixture-project/main/file.txt", BehaviorAllow},
			{"https://github.com/other-owner/other-project", BehaviorNone},
			{"https://github.com/push-owner/push-project", BehaviorNone},
			{"https://github.com/upstream-owner/upstream-project", BehaviorNone},
			{"https://github.com/override-owner/override-project", BehaviorNone},
		} {
			req := scopedRequest(t, "web_fetch", dir, map[string]any{"url": tt.url})
			assert.Equal(t, tt.want, Decide(cfg, req).Behavior)
			assert.Equal(t, tt.want, DecidePreToolUse(cfg, req).Behavior)
			assert.Equal(t, tt.want, DecideAllRulesPreToolUse(cfg, req).Behavior)
		}
	}
	req := scopedRequest(t, "web_fetch", root, map[string]any{"url": "https://github.com/fixture-owner/fixture-project"})
	cfg.Hooks.PermissionRequest.Deny.Tools = []string{"web_fetch"}
	assert.Equal(t, BehaviorDeny, Decide(cfg, req).Behavior)
	assert.Equal(t, BehaviorDeny, DecidePreToolUse(cfg, req).Behavior)
	cfg.Hooks.PermissionRequest.Deny.Tools = nil
	req.SandboxBypass = true
	assert.Equal(t, Decision{}, Decide(cfg, req))
	assert.Equal(t, Decision{}, DecidePreToolUse(cfg, req))
	req.SandboxBypass = false
	req.Arguments["unknown"] = []byte("true")
	assert.Equal(t, Decision{}, Decide(cfg, req))
	cfg.Hooks.PermissionRequest.Allow.ScopedTools[0].AllowDespiteNativeDeny = false
	delete(req.Arguments, "unknown")
	assert.Equal(t, Decision{}, Decide(cfg, req))
}

func TestRepositoryURLOriginResolution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	fixtureGit(t, root, "init", "-q")
	for _, origin := range []string{
		"file:///tmp/fixture-owner/fixture-project.git",
		"https://github.com/fixture-owner/fixture-project/extra.git",
		"https://github.com/fixture-owner/../fixture-project.git",
		"https://github.com/fixture-owner/fixture-project.git?query=value",
		"https://github.com:8443/fixture-owner/fixture-project.git",
		"https://github.com/fixture-owner/%252e%252e.git",
	} {
		fixtureGit(t, root, "config", "remote.origin.url", origin)
		_, err := currentRepositoryRemote(root)
		assert.Error(t, err, origin)
	}
	fixtureGit(t, root, "config", "remote.origin.url", "fixture:fixture-owner/fixture-project.git")
	fixtureGit(t, root, "config", "url.https://github.com/.insteadOf", "fixture:")
	remote, err := currentRepositoryRemote(root)
	require.NoError(t, err)
	assert.True(t, matchesRepositoryURL(remote, "https://github.com/fixture-owner/fixture-project"))
}

func TestRunRepositoryURLRules(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	fixtureGit(t, root, "init", "-q")
	fixtureGit(t, root, "remote", "add", "origin", "https://github.com/fixture-owner/fixture-project.git")
	noOrigin := t.TempDir()
	fixtureGit(t, noOrigin, "init", "-q")
	nonRepository := t.TempDir()
	config := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(repositoryURLConfig), 0o600))
	for _, run := range []struct {
		name string
		fn   func(io.Reader, io.Writer, string) error
		want string
	}{
		{"permissionRequest", Run, `{"behavior":"allow"}`},
		{"scoped preToolUse", RunPreToolUse, `{"permissionDecision":"allow"}`},
		{"all-rules preToolUse", RunAllRulesPreToolUse, `{"permissionDecision":"allow"}`},
	} {
		t.Run(run.name, func(t *testing.T) {
			for _, cwd := range []string{root, "", "relative", noOrigin, nonRepository, filepath.Join(root, "missing")} {
				var out bytes.Buffer
				payload := fmt.Sprintf(`{"toolName":"web_fetch","cwd":%q,"toolArgs":{"url":"https://github.com/fixture-owner/fixture-project/issues"}}`, cwd)
				require.NoError(t, run.fn(strings.NewReader(payload), &out, config))
				if cwd == root {
					assert.JSONEq(t, run.want, out.String())
				} else {
					assertHookWarning(t, out.String(), "origin repository")
					assert.NotContains(t, out.String(), "github.com")
				}
			}
		})
	}

	cfg, err := ParseConfig([]byte(repositoryURLConfig))
	require.NoError(t, err)
	static := ScopedToolRule{Tools: []string{"web_fetch"}, URLs: []string{"https://docs.example.invalid/guide"}, AllowDespiteNativeDeny: true}
	req := scopedRequest(t, "web_fetch", "", map[string]any{"url": static.URLs[0]})
	dynamic := cfg.Hooks.PermissionRequest.Allow.ScopedTools[0]
	for _, rules := range [][]ScopedToolRule{{dynamic, static}, {static, dynamic}} {
		cfg.Hooks.PermissionRequest.Allow.ScopedTools = rules
		assert.Equal(t, Decision{Behavior: BehaviorAllow}, Decide(cfg, req))
		assert.Equal(t, Decision{Behavior: BehaviorAllow}, DecidePreToolUse(cfg, req))
	}
}

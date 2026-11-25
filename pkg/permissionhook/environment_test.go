package permissionhook

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentAssignmentConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  EnvironmentAssignmentConfig
		env  string
		want bool
	}{
		{"default token", EnvironmentAssignmentConfig{}, "GH_TOKEN", true},
		{"default display variable", EnvironmentAssignmentConfig{}, "NO_COLOR", true},
		{"default unknown", EnvironmentAssignmentConfig{}, "CUSTOM_VALUE", false},
		{"denylist unknown", EnvironmentAssignmentConfig{Mode: "denylist"}, "CUSTOM_VALUE", true},
		{"denylist loader", EnvironmentAssignmentConfig{Mode: "denylist"}, "LD_PRELOAD", false},
		{"denylist git family", EnvironmentAssignmentConfig{Mode: "denylist"}, "GIT_CONFIG_GLOBAL", false},
		{"denylist go flags", EnvironmentAssignmentConfig{Mode: "denylist"}, "GOFLAGS", false},
		{"denylist go cache program", EnvironmentAssignmentConfig{Mode: "denylist"}, "GOCACHEPROG", false},
		{"denylist go auth program", EnvironmentAssignmentConfig{Mode: "denylist"}, "GOAUTH", false},
		{"denylist compiler", EnvironmentAssignmentConfig{Mode: "denylist"}, "CC", false},
		{"denylist compiler flags", EnvironmentAssignmentConfig{Mode: "denylist"}, "CGO_CFLAGS_ALLOW", false},
		{"denylist regular go variable", EnvironmentAssignmentConfig{Mode: "denylist"}, "CGO_ENABLED", true},
		{"custom go override", EnvironmentAssignmentConfig{Mode: "denylist", Allow: []string{"GOFLAGS"}}, "GOFLAGS", true},
		{"custom go deny wins", EnvironmentAssignmentConfig{Mode: "denylist", Allow: []string{"GOFLAGS"}, Deny: []string{"GOFLAGS"}}, "GOFLAGS", false},
		{"custom allow", EnvironmentAssignmentConfig{Allow: []string{"CUSTOM_*"}}, "CUSTOM_VALUE", true},
		{"custom deny", EnvironmentAssignmentConfig{Deny: []string{"GH_*"}}, "GH_TOKEN", false},
		{"deny overrides allow", EnvironmentAssignmentConfig{Allow: []string{"*"}, Deny: []string{"GH_*"}}, "GH_TOKEN", false},
		{"override standard deny", EnvironmentAssignmentConfig{Mode: "denylist", Allow: []string{"GH_HOST"}}, "GH_HOST", true},
		{"path remains forbidden", EnvironmentAssignmentConfig{Mode: "denylist", Allow: []string{"*"}}, "PATH", false},
		{"patterns are case sensitive", EnvironmentAssignmentConfig{Mode: "denylist"}, "gh_host", true},
		{"invalid direct mode", EnvironmentAssignmentConfig{Mode: "invalid"}, "GH_TOKEN", false},
		{"invalid direct pattern", EnvironmentAssignmentConfig{Mode: "denylist", Deny: []string{"["}}, "CUSTOM_VALUE", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.allows(tt.env))
		})
	}
}

func TestEnvironmentAssignmentConfigValidation(t *testing.T) {
	for _, input := range []string{
		"hooks:\n  permissionRequest:\n    environmentAssignments:\n      mode: invalid\n",
		"hooks:\n  permissionRequest:\n    environmentAssignments:\n      allow: ['[']\n",
		"hooks:\n  permissionRequest:\n    environmentAssignments:\n      allow: ['PATH']\n",
		"hooks:\n  permissionRequest:\n    environmentAssignments:\n      trustedHosts: ['https://example.invalid/path']\n",
	} {
		_, err := ParseConfig([]byte(input))
		require.Error(t, err)
	}
	cfg, err := ParseConfig([]byte("hooks:\n  permissionRequest:\n    environmentAssignments:\n      mode: denylist\n      allow: ['GH_HOST']\n      deny: ['CUSTOM_*']\n      trustedHosts: ['enterprise.example.invalid:8443']\n"))
	require.NoError(t, err)
	assert.Equal(t, "denylist", cfg.Hooks.PermissionRequest.EnvironmentAssignments.Mode)
}

func TestTrustedHosts(t *testing.T) {
	cfg := EnvironmentAssignmentConfig{TrustedHosts: []string{"enterprise.example.invalid:8443", "other.example.invalid"}}
	for _, tt := range []struct {
		host string
		want bool
	}{
		{"github.com", true},
		{"GITHUB.COM:443", true},
		{"api.github.com", true},
		{"enterprise.example.invalid:8443", true},
		{"enterprise.example.invalid", false},
		{"other.example.invalid:443", true},
		{"github.com.example.invalid", false},
		{"evil.example.invalid", false},
		{"github.com@evil.example.invalid", false},
		{"https://github.com", false},
		{"github.com/path", false},
		{"github.com:invalid", false},
		{"", false},
	} {
		t.Run(tt.host, func(t *testing.T) {
			assert.Equal(t, tt.want, cfg.trustsHost(tt.host))
		})
	}
}

func TestGHReadCommandAllowlist(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want bool
	}{
		{"pr view", []string{"gh", "pr", "view"}, true},
		{"pr view by number", []string{"gh", "pr", "view", "12"}, true},
		{"pr view attached repo", []string{"gh", "pr", "view", "-Rexample-owner/example-repo"}, true},
		{"pr web", []string{"gh", "pr", "view", "--web"}, false},
		{"short web", []string{"gh", "pr", "view", "-w"}, false},
		{"pr list", []string{"gh", "pr", "list", "--state", "open", "--limit", "20"}, true},
		{"issue view", []string{"gh", "issue", "view", "12"}, true},
		{"repo view", []string{"gh", "repo", "view", "example-owner/example-repo"}, true},
		{"run view", []string{"gh", "run", "view", "12", "--log"}, true},
		{"workflow list", []string{"gh", "workflow", "list"}, true},
		{"release list", []string{"gh", "release", "list"}, true},
		{"gist view", []string{"gh", "gist", "view", "0123456789abcdef"}, true},
		{"search repos", []string{"gh", "search", "repos", "language:go"}, true},
		{"status", []string{"gh", "status"}, true},
		{"status org", []string{"gh", "status", "--org", "example-org"}, true},
		{"status exclude", []string{"gh", "status", "-e", "example-owner/example-repo"}, true},
		{"destructive command", []string{"gh", "repo", "delete", "example-owner/example-repo"}, false},
		{"local output command", []string{"gh", "repo", "read-file", "README.md", "--output", "file"}, false},
		{"codespace omitted", []string{"gh", "codespace", "ports", "list"}, false},
		{"skill command omitted", []string{"gh", "skill", "preview", "example"}, false},
		{"unknown flag", []string{"gh", "pr", "view", "--unknown"}, false},
		{"untrusted repo host", []string{"gh", "pr", "view", "--repo", "https://evil.example/owner/repo"}, false},
		{"github URL userinfo", []string{"gh", "pr", "view", "https://github.com@evil.example/owner/repo/pull/1"}, false},
		{"enterprise host not configured", []string{"gh", "pr", "view", "--hostname", "untrusted.example.invalid"}, false},
		{"configured enterprise host", []string{"gh", "pr", "view", "--hostname", "enterprise.example.invalid:8443"}, true},
		{"api method default get", []string{"gh", "api", "/repos/example-owner/example-repo"}, true},
		{"api explicit get combined", []string{"gh", "api", "-XGET", "/repos/example-owner/example-repo"}, true},
		{"api attached repo", []string{"gh", "api", "-Rexample-owner/example-repo", "/repos/example-owner/example-repo"}, true},
		{"api fields implicit post", []string{"gh", "api", "-f", "per_page=20", "/repos/example-owner/example-repo"}, false},
		{"api file field", []string{"gh", "api", "-X", "GET", "-F", "data=@input.json", "/repos/example-owner/example-repo"}, false},
		{"api stdin", []string{"gh", "api", "--method", "GET", "--input", "-", "/repos/example-owner/example-repo"}, false},
		{"api repeated method", []string{"gh", "api", "-X", "GET", "--method", "GET", "/repos/example-owner/example-repo"}, false},
		{"api write method", []string{"gh", "api", "-X", "PATCH", "/repos/example-owner/example-repo"}, false},
		{"api untrusted absolute URL", []string{"gh", "api", "https://evil.example/repos/a/b"}, false},
		{"api github absolute URL", []string{"gh", "api", "https://api.github.com/repos/a/b"}, true},
		{"auth status requires no refresh", []string{"gh", "auth", "status"}, false},
		{"auth status no refresh", []string{"gh", "auth", "status", "--no-refresh"}, true},
		{"auth token excluded", []string{"gh", "auth", "status", "--no-refresh", "--show-token"}, false},
	}
	cfg := EnvironmentAssignmentConfig{TrustedHosts: []string{"enterprise.example.invalid:8443"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ghReadCommandAllowed(tt.argv, cfg, nil))
		})
	}
}

func TestGHReadPreset(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      ghRead: true
      commands: ["go build:*"]
    deny:
      commands: ["gh issue view 99", "gh repo delete:*"]
    environmentAssignments:
      mode: denylist
      trustedHosts: ["enterprise.example.invalid"]
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		name    string
		command string
		bypass  bool
		want    Behavior
	}{
		{"pr view", "gh pr view", false, BehaviorAllow},
		{"pr list", "gh pr list", false, BehaviorAllow},
		{"pr status", "gh pr status", false, BehaviorAllow},
		{"pr checks", "gh pr checks", false, BehaviorAllow},
		{"pr diff", "gh pr diff", false, BehaviorAllow},
		{"issue view", "gh issue view 12", false, BehaviorAllow},
		{"issue list", "gh issue list", false, BehaviorAllow},
		{"issue status", "gh issue status", false, BehaviorAllow},
		{"repo view", "gh repo view", false, BehaviorAllow},
		{"repo list", "gh repo list", false, BehaviorAllow},
		{"run view", "gh run view", false, BehaviorAllow},
		{"run list", "gh run list", false, BehaviorAllow},
		{"workflow view", "gh workflow view example.yml", false, BehaviorAllow},
		{"workflow list", "gh workflow list", false, BehaviorAllow},
		{"release view", "gh release view", false, BehaviorAllow},
		{"release list", "gh release list", false, BehaviorAllow},
		{"gist view", "gh gist view abcdef", false, BehaviorAllow},
		{"gist list", "gh gist list", false, BehaviorAllow},
		{"search code", "gh search code example", false, BehaviorAllow},
		{"search repos", "gh search repos example", false, BehaviorAllow},
		{"search issues", "gh search issues example", false, BehaviorAllow},
		{"search prs", "gh search prs example", false, BehaviorAllow},
		{"search commits", "gh search commits example", false, BehaviorAllow},
		{"status", "gh status", false, BehaviorAllow},
		{"auth status", "gh auth status --no-refresh", false, BehaviorAllow},
		{"api get", "gh api /repos/example-owner/example-repo", false, BehaviorAllow},
		{"api explicit get", "gh api -X GET -f per_page=20 /repos/example-owner/example-repo", false, BehaviorAllow},
		{"trusted host", "gh issue view --hostname enterprise.example.invalid", false, BehaviorAllow},
		{"token assignment", "GH_TOKEN=$TOKEN gh issue view", false, BehaviorAllow},
		{"denylist assignment", "GOPROXY=off gh issue view", false, BehaviorAllow},
		{"compound preset", "gh issue view; gh repo list", false, BehaviorAllow},
		{"compound with command rule", "GOPROXY=off go build && gh issue view", false, BehaviorAllow},
		{"compound unmatched", "gh issue view; git status", false, BehaviorNone},
		{"compound unsafe gh", "GOPROXY=off go build && gh pr view --web", false, BehaviorNone},
		{"explicit deny", "gh issue view 99", false, BehaviorDeny},
		{"deny after assignment", "GH_TOKEN=synthetic gh issue view 99", false, BehaviorDeny},
		{"deny in compound", "gh issue view; gh repo delete example", false, BehaviorDeny},
		{"unmatched write", "gh pr create", false, BehaviorNone},
		{"browser flag", "gh pr view --web", false, BehaviorNone},
		{"unknown flag", "gh issue view --unknown", false, BehaviorNone},
		{"untrusted host", "gh issue view --hostname untrusted.example.invalid", false, BehaviorNone},
		{"api implicit post", "gh api -f per_page=20 /repos/example-owner/example-repo", false, BehaviorNone},
		{"api write", "gh api -X POST /repos/example-owner/example-repo", false, BehaviorNone},
		{"api input", "gh api --input input.json /repos/example-owner/example-repo", false, BehaviorNone},
		{"auth token", "gh auth token", false, BehaviorNone},
		{"auth without no-refresh", "gh auth status", false, BehaviorNone},
		{"path assignment", "PATH=/fictional gh issue view", false, BehaviorNone},
		{"loader assignment", "LD_PRELOAD=fictional gh issue view", false, BehaviorNone},
		{"command substitution", "GH_TOKEN=$(example) gh issue view", false, BehaviorNone},
		{"executable path", "/fictional/gh issue view", false, BehaviorNone},
		{"sandbox bypass", "gh issue view", true, BehaviorNone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{ToolName: "bash", Command: tt.command, SandboxBypass: tt.bypass}
			assert.Equal(t, tt.want, Decide(cfg, &req).Behavior)
			if tt.want == BehaviorAllow && strings.HasPrefix(tt.command, "gh ") {
				req.Command = "GH_TOKEN=synthetic " + tt.command
				assert.Equal(t, BehaviorAllow, Decide(cfg, &req).Behavior, req.Command)
			}
		})
	}
}

func TestGHReadPresetEnvironmentPolicy(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      ghRead: true
      commands: ["go build"]
    environmentAssignments:
      allow: ["GH_TOKEN"]
      deny: ["GH_TOKEN"]
`))
	require.NoError(t, err)
	for _, command := range []string{
		"GH_TOKEN=synthetic gh issue view",
		"CUSTOM_VALUE=1 gh issue view",
		"gh issue view; PATH=/fictional go build",
	} {
		req := Request{ToolName: "bash", Command: command}
		assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior, command)
	}
}

func TestGHReadPresetToolDeny(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      ghRead: true
    deny:
      tools: ["bash"]
`))
	require.NoError(t, err)
	req := Request{ToolName: "bash", Command: "gh issue view"}
	assert.Equal(t, BehaviorDeny, Decide(cfg, &req).Behavior)
}

func TestGHReadPresetPreservesExistingRules(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["gh pr view:*", "git status"]
`))
	require.NoError(t, err)
	presetOnly := Request{ToolName: "bash", Command: "gh issue view"}
	assert.Equal(t, BehaviorNone, Decide(cfg, &presetOnly).Behavior)

	cfg.Hooks.PermissionRequest.Allow.GHRead = true
	for _, tt := range []struct {
		command string
		want    Behavior
	}{
		{"gh issue view", BehaviorAllow},
		{"gh pr view --web", BehaviorAllow},
		{"GH_TOKEN=synthetic gh pr view --web", BehaviorNone},
		{"gh pr view --web; NO_COLOR=1 git status", BehaviorNone},
	} {
		req := Request{ToolName: "bash", Command: tt.command}
		assert.Equal(t, tt.want, Decide(cfg, &req).Behavior, tt.command)
	}
}

func TestGHReadPresetRespectsDynamicAssignmentDeny(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      ghRead: true
    deny:
      commands: ["GH_TOKEN=blocked:*"]
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		command string
		want    Behavior
	}{
		{"GH_TOKEN=$TOKEN gh issue view", BehaviorNone},
		{"GH_TOKEN=blocked gh issue view", BehaviorDeny},
		{"GH_TOKEN=other gh issue view", BehaviorAllow},
	} {
		req := Request{ToolName: "bash", Command: tt.command}
		assert.Equal(t, tt.want, Decide(cfg, &req).Behavior, tt.command)
	}
}

func TestDecideEnvironmentAssignments(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands:
        - "gh pr view:*"
        - "gh api:*"
        - "gh repo delete:*"
    deny:
      commands:
        - "GH_TOKEN=blocked gh pr view"
        - "GH_TOKEN=secret gh pr view"
`))
	require.NoError(t, err)
	tests := []struct {
		name    string
		command string
		want    Behavior
	}{
		{"token literal", "GH_TOKEN=synthetic gh pr view", BehaviorAllow},
		{"explicit deny", "GH_TOKEN=blocked gh pr view", BehaviorDeny},
		{"uncertain dynamic deny", "GH_TOKEN=$TOKEN gh pr view", BehaviorNone},
		{"path is immutable deny", "PATH=/tmp gh pr view", BehaviorNone},
		{"unknown variable in allowlist", "CUSTOM_VALUE=1 gh pr view", BehaviorNone},
		{"side effect command", "GH_TOKEN=x gh repo delete example-owner/example-repo", BehaviorNone},
		{"all segments need read gate", "GH_TOKEN=x gh pr view && gh repo delete example-owner/example-repo", BehaviorNone},
		{"assignment only", "GH_TOKEN=x", BehaviorNone},
		{"sandbox bypass", "GH_TOKEN=x gh pr view", BehaviorNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{ToolName: "bash", Command: tt.command, SandboxBypass: tt.name == "sandbox bypass"}
			assert.Equal(t, tt.want, Decide(cfg, &req).Behavior)
		})
	}
}

func TestDynamicTokenReferenceWithoutPotentialDeny(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["gh pr view:*"]
`))
	require.NoError(t, err)
	req := Request{ToolName: "bash", Command: "GH_TOKEN=$TOKEN gh pr view"}
	assert.Equal(t, BehaviorAllow, Decide(cfg, &req).Behavior)
}

func TestEnvironmentDenylistAndTrustedGHHost(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["gh pr view:*"]
    environmentAssignments:
      mode: denylist
      allow: ["GH_HOST", "GH_REPO"]
      trustedHosts: ["enterprise.example.invalid"]
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		command string
		want    Behavior
	}{
		{"CUSTOM_VALUE=arbitrary gh pr view", BehaviorAllow},
		{"LD_PRELOAD=evil gh pr view", BehaviorNone},
		{"GH_HOST=enterprise.example.invalid gh pr view", BehaviorAllow},
		{"GH_HOST=evil.example.invalid gh pr view", BehaviorNone},
		{"GH_HOST=$HOST gh pr view", BehaviorNone},
		{"GH_HOST=enterprise.example.invalid GH_HOST=evil.example.invalid gh pr view", BehaviorNone},
		{"GH_REPO=https://enterprise.example.invalid/owner/repo gh pr view", BehaviorAllow},
		{"GH_REPO=https://evil.example/owner/repo gh pr view", BehaviorNone},
	} {
		req := Request{ToolName: "bash", Command: tt.command}
		assert.Equal(t, tt.want, Decide(cfg, &req).Behavior, tt.command)
	}
}

func TestDenyMatchesStrippedCommandBeforeAllow(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["gh pr view:*"]
    deny:
      commands: ["gh pr view"]
`))
	require.NoError(t, err)
	req := Request{ToolName: "bash", Command: "GH_TOKEN=synthetic gh pr view"}
	assert.Equal(t, BehaviorDeny, Decide(cfg, &req).Behavior)
}

func TestDynamicAssignmentMayMatchShortPrefixDeny(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["gh pr view:*"]
    deny:
      commands: ["GH_TOKEN=secret:*"]
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		command string
		want    Behavior
	}{
		{"GH_TOKEN=$TOKEN gh pr view", BehaviorNone},
		{"GH_TOKEN=$TOKEN NO_COLOR=1 gh pr view", BehaviorNone},
		{"GH_TOKEN=secret NO_COLOR=1 gh pr view", BehaviorDeny},
		{"GH_TOKEN=other NO_COLOR=1 gh pr view", BehaviorAllow},
	} {
		req := Request{ToolName: "bash", Command: tt.command}
		assert.Equal(t, tt.want, Decide(cfg, &req).Behavior, tt.command)
	}
}

func TestDecideNonGHEnvironmentAssignments(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["go build:*", "git status:*", "gh pr view:*", "gh repo delete:*", "/usr/bin/gh:*"]
    deny:
      commands: ["CUSTOM=blocked:*", "git status --dangerous"]
    environmentAssignments:
      mode: denylist
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		name    string
		command string
		bypass  bool
		want    Behavior
	}{
		{"requested go build", "GOPROXY=off GOSUMDB=off go build", false, BehaviorAllow},
		{"go build arguments", "GOPROXY=off go build ./pkg/example", false, BehaviorAllow},
		{"unknown variable", "CUSTOM=value git status", false, BehaviorAllow},
		{"dynamic variable", "CUSTOM=$VALUE git status", false, BehaviorNone},
		{"multiple dynamic prefix deny", "CUSTOM=$VALUE OTHER=1 git status", false, BehaviorNone},
		{"raw prefix deny", "CUSTOM=blocked OTHER=1 git status", false, BehaviorDeny},
		{"stripped deny", "CUSTOM=value git status --dangerous", false, BehaviorDeny},
		{"deny in other segment", "CUSTOM=$VALUE go build && git status --dangerous", false, BehaviorDeny},
		{"unmatched", "CUSTOM=value go test", false, BehaviorNone},
		{"assignment only", "CUSTOM=value", false, BehaviorNone},
		{"path", "PATH=/fictional go build", false, BehaviorNone},
		{"go flags", "GOFLAGS=-toolexec=example go build", false, BehaviorNone},
		{"go auth", "GOAUTH=example go build", false, BehaviorNone},
		{"compiler", "CC=example go build", false, BehaviorNone},
		{"compound allowed", "GOPROXY=off go build && git status", false, BehaviorAllow},
		{"compound gh read", "GOPROXY=off go build && gh pr view", false, BehaviorAllow},
		{"compound gh write", "GOPROXY=off go build && gh repo delete example", false, BehaviorNone},
		{"compound gh untrusted host", "GOPROXY=off go build && gh pr view --hostname untrusted.example.invalid", false, BehaviorNone},
		{"explicit gh path", "GOPROXY=off /usr/bin/gh repo delete example", false, BehaviorNone},
		{"assignment-free unchanged", "go build && gh repo delete example", false, BehaviorAllow},
		{"command substitution", "GOPROXY=$(example) go build", false, BehaviorNone},
		{"sandbox bypass", "GOPROXY=off go build", true, BehaviorNone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{ToolName: "bash", Command: tt.command, SandboxBypass: tt.bypass}
			assert.Equal(t, tt.want, Decide(cfg, &req).Behavior)
		})
	}
}

func TestDecideNonGHAllowlist(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["go build:*"]
    environmentAssignments:
      allow: ["CUSTOM_*"]
`))
	require.NoError(t, err)
	for _, tt := range []struct {
		command string
		want    Behavior
	}{
		{"GOPROXY=off go build", BehaviorNone},
		{"NO_COLOR=1 go build", BehaviorAllow},
		{"CUSTOM_VALUE=1 go build", BehaviorAllow},
		{"PATH=/fictional go build", BehaviorNone},
	} {
		req := Request{ToolName: "bash", Command: tt.command}
		assert.Equal(t, tt.want, Decide(cfg, &req).Behavior, tt.command)
	}
}

func TestGoExecutionEnvironmentExcluded(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
hooks:
  permissionRequest:
    allow:
      commands: ["go build:*"]
    environmentAssignments:
      mode: denylist
`))
	require.NoError(t, err)
	for _, name := range []string{
		"GOFLAGS", "GOCACHEPROG", "GOAUTH", "GOENV", "GOROOT", "GOTOOLCHAIN",
		"CC", "CXX", "FC", "AR", "PKG_CONFIG", "GCCGO", "GCCGOTOOLDIR",
		"CGO_CFLAGS", "CGO_CPPFLAGS_ALLOW", "CGO_CXXFLAGS", "CGO_FFLAGS", "CGO_LDFLAGS",
	} {
		t.Run(name, func(t *testing.T) {
			req := Request{ToolName: "bash", Command: name + "=example go build"}
			assert.Equal(t, BehaviorNone, Decide(cfg, &req).Behavior)
		})
	}
}

func TestRedactBashAssignments(t *testing.T) {
	for _, tt := range []struct {
		command string
		want    string
	}{
		{"GH_TOKEN=synthetic gh pr view", "GH_TOKEN=[REDACTED] gh pr view"},
		{`GH_TOKEN="$TOKEN" gh pr view`, `GH_TOKEN=[REDACTED] gh pr view`},
		{`A=one B='two words' gh pr view`, `A=[REDACTED] B=[REDACTED] gh pr view`},
		{"gh pr view", "gh pr view"},
		{"GH_TOKEN=$(printenv) gh pr view", ""},
		{"PATH+=/tmp gh pr view", ""},
	} {
		assert.Equal(t, tt.want, redactBashAssignments(tt.command), tt.command)
	}
}

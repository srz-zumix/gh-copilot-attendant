package copilotsettings_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v93/github"
	"github.com/spf13/cobra"
	rootcmd "github.com/srz-zumix/gh-copilot-attendant/cmd"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/copilot/settings/policy"
	"github.com/srz-zumix/gh-copilot-attendant/cmd/copilot/settings/runner"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/gh/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commandRequest struct {
	method string
	path   string
	body   []byte
}

type commandServer struct {
	mu       sync.Mutex
	requests []commandRequest
}

func (s *commandServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.requests = append(s.requests, commandRequest{method: r.Method, path: r.URL.Path, body: body})
	s.mu.Unlock()

	if strings.Contains(r.URL.Path, "/policies/code-review") && r.Method == http.MethodPut {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var response any
	switch {
	case strings.HasPrefix(r.URL.Path, "/enterprises/"):
		response = map[string]any{"policy_state": nil}
	case strings.HasSuffix(r.URL.Path, "/policies/code-review"):
		response = map[string]any{
			"configured_state": nil, "effective_state": "enabled",
			"editable": false, "managed_by": "enterprise",
		}
	case strings.HasSuffix(r.URL.Path, "/code-review/runner"):
		configuredBy := "coding_agent"
		allow := true
		runnerType := "labeled"
		if r.Method == http.MethodPut {
			configuredBy = "code_review"
			var request map[string]any
			if err := json.Unmarshal(body, &request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if value, ok := request["allow_repository_customization"].(bool); ok {
				allow = value
			}
			if value, ok := request["runner_type"].(string); ok {
				runnerType = value
			}
		}
		response = map[string]any{
			"runner_type": runnerType, "runner_labels": []string{"runner-amber"},
			"runner_group_name": nil, "allow_repository_customization": allow,
			"configured_by": configuredBy,
		}
	default:
		http.Error(w, "unexpected fixture path", http.StatusNotFound)
		return
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *commandServer) snapshot() []commandRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]commandRequest(nil), s.requests...)
}

func commandService(t *testing.T) (*copilotsettings.Service, *commandServer, *atomic.Int32) {
	t.Helper()
	recorder := &commandServer{}
	server := httptest.NewServer(recorder)
	t.Cleanup(server.Close)
	base := server.URL + "/"
	api, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&base, nil))
	require.NoError(t, err)
	shared, err := client.NewClient(api)
	require.NoError(t, err)
	calls := &atomic.Int32{}
	service := copilotsettings.NewService(func(target repository.Repository) (*gh.GitHubClient, error) {
		calls.Add(1)
		return shared, nil
	})
	return service, recorder, calls
}

func isolateCommandEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_HOST", "forge.test.invalid")
	t.Setenv("GH_REPO", "forge.test.invalid/fixture-team/fixture-project")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_SERVER_URL", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")
}

func commandRoot(service *copilotsettings.Service) *cobra.Command {
	root := &cobra.Command{Use: "gh-copilot-attendant", SilenceUsage: true, SilenceErrors: true}
	copilot := &cobra.Command{Use: "copilot"}
	settings := &cobra.Command{Use: "settings"}
	settings.AddCommand(policy.NewPolicyCmd(service), runner.NewRunnerCmd(service))
	copilot.AddCommand(settings)
	root.AddCommand(copilot)
	return root
}

func executeCommand(root *cobra.Command, args ...string) (string, string, error) {
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestSettingsCommandsStructuredOutput(t *testing.T) {
	isolateCommandEnvironment(t)
	tests := []struct {
		name        string
		args        []string
		method      string
		path        string
		checkOutput func(*testing.T, string)
		checkBody   func(*testing.T, []byte)
	}{
		{
			name:   "organization policy json",
			args:   []string{"copilot", "settings", "policy", "view", "--org", "forge.test.invalid/fixture-team", "--format", "json"},
			method: http.MethodGet,
			path:   "/orgs/fixture-team/copilot/policies/code-review",
			checkOutput: func(t *testing.T, out string) {
				var value map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &value))
				assert.Contains(t, value, "configured_state")
				assert.Nil(t, value["configured_state"])
				assert.Equal(t, false, value["editable"])
				assert.Equal(t, "enterprise", value["managed_by"])
				assert.NotContains(t, value, "organization")
			},
		},
		{
			name:   "enterprise policy jq",
			args:   []string{"copilot", "settings", "policy", "view", "--enterprise", "forge.test.invalid/fixture-enterprise", "--format", "json", "--jq", `.policy_state // "unset"`},
			method: http.MethodGet,
			path:   "/enterprises/fixture-enterprise/copilot/policies/code-review",
			checkOutput: func(t *testing.T, out string) {
				assert.Equal(t, "unset\n", out)
			},
		},
		{
			name:   "runner jq with repository owner",
			args:   []string{"copilot", "settings", "runner", "view", "-R", "forge.test.invalid/fixture-team/fixture-project", "--format", "json", "-q", ".configured_by"},
			method: http.MethodGet,
			path:   "/orgs/fixture-team/copilot/code-review/runner",
			checkOutput: func(t *testing.T, out string) {
				assert.Equal(t, "coding_agent\n", out)
			},
		},
		{
			name:   "runner view json with default target",
			args:   []string{"copilot", "settings", "runner", "view", "--format", "json"},
			method: http.MethodGet,
			path:   "/orgs/fixture-team/copilot/code-review/runner",
			checkOutput: func(t *testing.T, out string) {
				var value map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &value))
				assert.Equal(t, "coding_agent", value["configured_by"])
				assert.Nil(t, value["runner_group_name"])
			},
		},
		{
			name: "runner set json with inherited customization",
			args: []string{
				"copilot", "settings", "runner", "set", "--org", "forge.test.invalid/fixture-team",
				"--type", "labeled", "--labels", "runner-amber", "--format", "json",
			},
			method: http.MethodPut,
			path:   "/orgs/fixture-team/copilot/code-review/runner",
			checkOutput: func(t *testing.T, out string) {
				var value map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &value))
				assert.Equal(t, map[string]any{
					"runner_type": "labeled", "runner_labels": []any{"runner-amber"},
					"runner_group_name": nil, "allow_repository_customization": true,
					"configured_by": "code_review",
				}, value)
			},
			checkBody: func(t *testing.T, data []byte) {
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				assert.Equal(t, map[string]any{
					"runner_type": "labeled", "runner_labels": []any{"runner-amber"},
				}, body)
			},
		},
		{
			name: "runner set json with explicit customization",
			args: []string{
				"copilot", "settings", "runner", "set", "--org", "forge.test.invalid/fixture-team",
				"--type", "standard", "--allow-repository-customization", "--format", "json",
			},
			method: http.MethodPut,
			path:   "/orgs/fixture-team/copilot/code-review/runner",
			checkOutput: func(t *testing.T, out string) {
				var value map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &value))
				assert.Equal(t, true, value["allow_repository_customization"])
				assert.Equal(t, "code_review", value["configured_by"])
			},
			checkBody: func(t *testing.T, data []byte) {
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				assert.Equal(t, map[string]any{
					"runner_type": "standard", "allow_repository_customization": true,
				}, body)
			},
		},
		{
			name: "runner set template",
			args: []string{
				"copilot", "settings", "runner", "set", "--org", "forge.test.invalid/fixture-team",
				"--type", "labeled", "--labels", "runner-amber,runner-cobalt", "--labels", "runner-copper",
				"--group", "fixture-pool", "--allow-repository-customization=false",
				"--format", "json", "-t", `{{.runner_type}}:{{.allow_repository_customization}}`,
			},
			method: http.MethodPut,
			path:   "/orgs/fixture-team/copilot/code-review/runner",
			checkOutput: func(t *testing.T, out string) {
				assert.Equal(t, "labeled:false", strings.TrimSpace(out))
				assert.NotContains(t, out, "Updated")
			},
			checkBody: func(t *testing.T, data []byte) {
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				assert.Equal(t, map[string]any{
					"runner_type": "labeled", "runner_labels": []any{"runner-amber", "runner-cobalt", "runner-copper"},
					"runner_group_name": "fixture-pool", "allow_repository_customization": false,
				}, body)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, calls := commandService(t)
			out, errOut, err := executeCommand(commandRoot(service), tt.args...)
			require.NoError(t, err)
			assert.Empty(t, errOut)
			tt.checkOutput(t, out)
			require.EqualValues(t, 1, calls.Load())
			requests := recorder.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, tt.method, requests[0].method)
			assert.Equal(t, tt.path, requests[0].path)
			if tt.checkBody != nil {
				tt.checkBody(t, requests[0].body)
			}
		})
	}
}

func TestSettingsPolicySetMessages(t *testing.T) {
	isolateCommandEnvironment(t)
	tests := []struct {
		name string
		args []string
		path string
		want string
	}{
		{
			"organization",
			[]string{"copilot", "settings", "policy", "set", "--org", "fixture-team", "-s", "enabled"},
			"/orgs/fixture-team/copilot/policies/code-review",
			"Updated Copilot code review policy",
		},
		{
			"enterprise",
			[]string{"copilot", "settings", "policy", "set", "--enterprise", "fixture-enterprise", "--state", "disabled_for_all_organizations"},
			"/enterprises/fixture-enterprise/copilot/policies/code-review",
			"Accepted Copilot code review policy update",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, _ := commandService(t)
			out, _, err := executeCommand(commandRoot(service), tt.args...)
			require.NoError(t, err)
			assert.Contains(t, out, tt.want)
			if tt.name == "enterprise" {
				assert.Contains(t, out, "asynchronous")
			}
			requests := recorder.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPut, requests[0].method)
			assert.Equal(t, tt.path, requests[0].path)
		})
	}
}

func TestSettingsInvalidFlagsNeverReachAPI(t *testing.T) {
	isolateCommandEnvironment(t)
	tests := []struct {
		name string
		args []string
	}{
		{"missing policy state", []string{"policy", "set", "--org", "fixture-team"}},
		{"invalid organization state", []string{"policy", "set", "--org", "fixture-team", "--state", "configured_by_organization_admins"}},
		{"invalid enterprise state", []string{"policy", "set", "--enterprise", "fixture-enterprise", "--state", "enabled"}},
		{"empty organization", []string{"policy", "set", "--org=", "--state", "enabled"}},
		{"empty repository", []string{"policy", "set", "--repo=", "--state", "enabled"}},
		{"empty enterprise", []string{"policy", "set", "--enterprise=", "--state", "enabled"}},
		{"multiple targets", []string{"policy", "view", "--org", "fixture-team", "--repo", "fixture-team/fixture-project"}},
		{"missing runner type", []string{"runner", "set", "--org", "fixture-team"}},
		{"invalid runner type", []string{"runner", "set", "--org", "fixture-team", "--type", "unknown"}},
		{"empty labels on standard", []string{"runner", "set", "--org", "fixture-team", "--type", "standard", "--labels="}},
		{"empty repeated label", []string{"runner", "set", "--org", "fixture-team", "--type", "labeled", "--labels=", "--labels", "runner-amber"}},
		{"empty label element", []string{"runner", "set", "--org", "fixture-team", "--type", "labeled", "--labels", "runner-amber,,runner-cobalt"}},
		{"empty group with label", []string{"runner", "set", "--org", "fixture-team", "--type", "labeled", "--group=", "--labels", "runner-amber"}},
		{"enterprise on runner", []string{"runner", "view", "--enterprise", "fixture-enterprise"}},
		{"jq without format", []string{"runner", "view", "--org", "fixture-team", "--jq", "."}},
		{"invalid jq", []string{"runner", "set", "--org", "fixture-team", "--type", "standard", "--format", "json", "--jq", "[."}},
		{"undefined jq function", []string{"runner", "set", "--org", "fixture-team", "--type", "standard", "--format", "json", "--jq", "fixture_unknown_function"}},
		{"invalid template", []string{"runner", "set", "--org", "fixture-team", "--type", "standard", "--format", "json", "--template", "{{"}},
		{"extra argument", []string{"runner", "view", "--org", "fixture-team", "extra"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, calls := commandService(t)
			args := append([]string{"copilot", "settings"}, tt.args...)
			out, _, err := executeCommand(commandRoot(service), args...)
			require.Error(t, err)
			assert.Empty(t, out)
			assert.Zero(t, calls.Load())
			assert.Empty(t, recorder.snapshot())
		})
	}
}

func TestRunnerUpdateReportsOutputFailureAfterSuccess(t *testing.T) {
	isolateCommandEnvironment(t)
	tests := []struct {
		name   string
		args   []string
		detail string
	}{
		{"jq", []string{"--jq", `error("fixture output failure")`}, "fixture output failure"},
		{"template", []string{"--template", "{{index .runner_labels 99}}"}, "index out of range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, _ := commandService(t)
			args := []string{
				"copilot", "settings", "runner", "set", "--org", "fixture-team",
				"--type", "standard", "--format", "json",
			}
			out, _, err := executeCommand(commandRoot(service), append(args, tt.args...)...)
			require.ErrorContains(t, err, "succeeded but rendering its result failed")
			assert.ErrorContains(t, err, tt.detail)
			assert.Empty(t, out)
			requests := recorder.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPut, requests[0].method)
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture output write failure")
}

func TestRunnerUpdateReportsWriteFailureAfterSuccess(t *testing.T) {
	isolateCommandEnvironment(t)
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"human", nil},
		{"json", []string{"--format", "json"}},
		{"jq", []string{"--format", "json", "--jq", ".runner_type"}},
		{"template", []string{"--format", "json", "--template", "{{.runner_type}}"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, _ := commandService(t)
			root := commandRoot(service)
			root.SetOut(failingWriter{})
			args := []string{"copilot", "settings", "runner", "set", "--org", "fixture-team", "--type", "standard"}
			root.SetArgs(append(args, tt.args...))
			err := root.Execute()
			require.ErrorContains(t, err, "succeeded but rendering its result failed")
			assert.ErrorContains(t, err, "fixture output write failure")
			require.Len(t, recorder.snapshot(), 1)
		})
	}
}

func TestPolicyUpdateReportsWriteFailureAfterSuccess(t *testing.T) {
	isolateCommandEnvironment(t)
	service, recorder, _ := commandService(t)
	root := commandRoot(service)
	root.SetOut(failingWriter{})
	root.SetArgs([]string{"copilot", "settings", "policy", "set", "--org", "fixture-team", "--state", "enabled"})
	err := root.Execute()
	require.ErrorContains(t, err, "succeeded but writing confirmation failed")
	assert.ErrorContains(t, err, "fixture output write failure")
	require.Len(t, recorder.snapshot(), 1)
}

func TestRegisteredCommandsHelpCompletionAndExtension(t *testing.T) {
	isolateCommandEnvironment(t)
	tests := []struct {
		name     string
		args     []string
		contains []string
		excludes []string
		wantErr  bool
	}{
		{"copilot help", []string{"copilot", "--help"}, []string{"extension", "settings"}, nil, false},
		{"extension help", []string{"copilot", "extension", "--help"}, []string{"install", "list"}, nil, false},
		{"extension list", []string{"copilot", "extension", "list"}, []string{"attendant-dashboard"}, nil, false},
		{"runner help", []string{"copilot", "settings", "runner", "set", "--help"}, []string{"--type", "-t, --template"}, []string{"-t, --type"}, false},
		{"organization state completion", []string{"__complete", "copilot", "settings", "policy", "set", "--state", ""}, []string{"enabled", "disabled", ":4"}, []string{"enabled_for_all_organizations"}, false},
		{"enterprise state completion", []string{"__complete", "copilot", "settings", "policy", "set", "--enterprise", "fixture-enterprise", "--state", ""}, []string{"configured_by_organization_admins", "enabled_for_all_organizations", ":4"}, nil, false},
		{"runner type completion", []string{"__complete", "copilot", "settings", "runner", "set", "--type", ""}, []string{"standard", "labeled", ":4"}, nil, false},
		{"organization state prefix", []string{"__complete", "copilot", "settings", "policy", "set", "--state", "en"}, []string{"enabled", ":4"}, nil, false},
		{"enterprise state prefix", []string{"__complete", "copilot", "settings", "policy", "set", "--enterprise", "fixture-enterprise", "--state", "dis"}, []string{"disabled_for_all_organizations", ":4"}, nil, false},
		{"runner type prefix", []string{"__complete", "copilot", "settings", "runner", "set", "--type", "lab"}, []string{"labeled", ":4"}, nil, false},
		{"unknown settings command", []string{"copilot", "settings", "unexpected"}, nil, nil, true},
		{"unknown policy command", []string{"copilot", "settings", "policy", "unexpected"}, nil, nil, true},
		{"unknown runner command", []string{"copilot", "settings", "runner", "unexpected"}, nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &cobra.Command{Use: "gh-copilot-attendant", SilenceUsage: true, SilenceErrors: true}
			root.AddCommand(rootcmd.NewCopilotCmd())
			out, _, err := executeCommand(root, tt.args...)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			for _, value := range tt.contains {
				assert.Contains(t, out, value)
			}
			for _, value := range tt.excludes {
				assert.NotContains(t, out, value)
			}
		})
	}
}

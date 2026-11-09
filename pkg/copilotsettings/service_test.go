package copilotsettings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/gh/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedRequest struct {
	method string
	path   string
	body   []byte
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []recordedRequest
	handler  func(http.ResponseWriter, *http.Request)
}

func (r *requestRecorder) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.mu.Lock()
	r.requests = append(r.requests, recordedRequest{method: request.Method, path: request.URL.Path, body: body})
	r.mu.Unlock()
	request.Body = io.NopCloser(bytes.NewReader(body))
	r.handler(w, request)
}

func (r *requestRecorder) snapshot() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.requests...)
}

func (r *requestRecorder) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = nil
}

func newHTTPService(t *testing.T, handler func(http.ResponseWriter, *http.Request)) (*Service, *requestRecorder, *[]repository.Repository) {
	t.Helper()
	recorder := &requestRecorder{handler: handler}
	server := httptest.NewServer(recorder)
	t.Cleanup(server.Close)
	base := server.URL + "/"
	api, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&base, nil))
	require.NoError(t, err)
	shared, err := client.NewClient(api)
	require.NoError(t, err)
	var targets []repository.Repository
	service := NewService(func(target repository.Repository) (*gh.GitHubClient, error) {
		targets = append(targets, target)
		return shared, nil
	})
	return service, recorder, &targets
}

func decodeBody(t *testing.T, request recordedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(request.body, &body))
	return body
}

func TestPolicyRequests(t *testing.T) {
	service, recorder, targets := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/enterprises/") {
			_, err := io.WriteString(w, `{"policy_state":null}`)
			assert.NoError(t, err)
			return
		}
		_, err := io.WriteString(w, `{"configured_state":null,"effective_state":"disabled","editable":true,"managed_by":"organization"}`)
		assert.NoError(t, err)
	})
	org := TargetInput{Repository: "forge.test.invalid/fixture-team/fixture-project", RepositorySet: true}
	enterprise := TargetInput{Enterprise: "forge.test.invalid/fixture-enterprise", EnterpriseSet: true}

	orgPolicy, err := service.ViewPolicy(t.Context(), org)
	require.NoError(t, err)
	require.NotNil(t, orgPolicy.Organization)
	assert.Nil(t, orgPolicy.Organization.ConfiguredState)
	assert.Equal(t, "disabled", orgPolicy.Organization.EffectiveState)
	assert.True(t, orgPolicy.Organization.Editable)
	assert.Equal(t, "organization", orgPolicy.Organization.ManagedBy)
	orgTarget, err := service.SetPolicy(t.Context(), org, "enabled")
	require.NoError(t, err)
	assert.False(t, orgTarget.Enterprise)

	enterprisePolicy, err := service.ViewPolicy(t.Context(), enterprise)
	require.NoError(t, err)
	require.NotNil(t, enterprisePolicy.Enterprise)
	assert.Nil(t, enterprisePolicy.Enterprise.PolicyState)
	enterpriseTarget, err := service.SetPolicy(t.Context(), enterprise, "configured_by_organization_admins")
	require.NoError(t, err)
	assert.True(t, enterpriseTarget.Enterprise)

	requests := recorder.snapshot()
	require.Len(t, requests, 4)
	assert.Equal(t, http.MethodGet, requests[0].method)
	assert.Equal(t, "/orgs/fixture-team/copilot/policies/code-review", requests[0].path)
	assert.Empty(t, requests[0].body)
	assert.Equal(t, http.MethodPut, requests[1].method)
	assert.Equal(t, "/orgs/fixture-team/copilot/policies/code-review", requests[1].path)
	assert.Equal(t, map[string]any{"state": "enabled"}, decodeBody(t, requests[1]))
	assert.Equal(t, http.MethodGet, requests[2].method)
	assert.Equal(t, "/enterprises/fixture-enterprise/copilot/policies/code-review", requests[2].path)
	assert.Equal(t, http.MethodPut, requests[3].method)
	assert.Equal(t, "/enterprises/fixture-enterprise/copilot/policies/code-review", requests[3].path)
	assert.Equal(t, map[string]any{"policy_state": "configured_by_organization_admins"}, decodeBody(t, requests[3]))
	require.Len(t, *targets, 4)
	assert.Equal(t, repository.Repository{Host: "forge.test.invalid", Owner: "fixture-team"}, (*targets)[0])
	assert.Equal(t, repository.Repository{Host: "forge.test.invalid", Owner: "fixture-enterprise"}, (*targets)[2])
}

func TestOrganizationPolicyViewRejectsMissingEffectiveState(t *testing.T) {
	for _, body := range []string{"", `{"configured_state":null}`} {
		t.Run(fmt.Sprintf("body=%q", body), func(t *testing.T) {
			service, recorder, _ := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
				_, err := io.WriteString(w, body)
				assert.NoError(t, err)
			})
			input := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}

			policy, err := service.ViewPolicy(t.Context(), input)
			require.ErrorContains(t, err, "effective_state")
			assert.ErrorContains(t, err, "forge.test.invalid/fixture-team")
			assert.Nil(t, policy)
			requests := recorder.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodGet, requests[0].method)
		})
	}
}

func TestRunnerRequestsPreserveOmittedCustomization(t *testing.T) {
	allow := true
	configuredBy := "coding_agent"
	service, recorder, _ := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if value, ok := body["allow_repository_customization"].(bool); ok {
				allow = value
			}
			configuredBy = "code_review"
		}
		err := json.NewEncoder(w).Encode(map[string]any{
			"runner_type": "labeled", "runner_labels": []string{"runner-amber"},
			"runner_group_name": "fixture-pool", "allow_repository_customization": allow,
			"configured_by": configuredBy,
		})
		assert.NoError(t, err)
	})
	input := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}
	inherited, err := service.ViewRunner(t.Context(), input)
	require.NoError(t, err)
	assert.Equal(t, "coding_agent", inherited.ConfiguredBy)
	assert.True(t, inherited.AllowRepositoryCustomization)

	recorder.clear()
	options := RunnerOptions{Type: "labeled", LabelValues: []string{"runner-amber"}, Group: "fixture-pool"}
	_, updated, err := service.SetRunner(t.Context(), input, options)
	require.NoError(t, err)
	assert.Equal(t, "code_review", updated.ConfiguredBy)
	assert.True(t, updated.AllowRepositoryCustomization)
	requests := recorder.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPut, requests[0].method)
	assert.Equal(t, "/orgs/fixture-team/copilot/code-review/runner", requests[0].path)
	assert.Equal(t, map[string]any{
		"runner_type": "labeled", "runner_labels": []any{"runner-amber"}, "runner_group_name": "fixture-pool",
	}, decodeBody(t, requests[0]))

	no := false
	options.AllowRepositoryCustomization = &no
	_, updated, err = service.SetRunner(t.Context(), input, options)
	require.NoError(t, err)
	assert.False(t, updated.AllowRepositoryCustomization)
	yes := true
	options.AllowRepositoryCustomization = &yes
	_, updated, err = service.SetRunner(t.Context(), input, options)
	require.NoError(t, err)
	assert.True(t, updated.AllowRepositoryCustomization)
	requests = recorder.snapshot()
	require.Len(t, requests, 3)
	assert.Equal(t, false, decodeBody(t, requests[1])["allow_repository_customization"])
	assert.Equal(t, true, decodeBody(t, requests[2])["allow_repository_customization"])
}

func TestRunnerViewRejectsMissingResponse(t *testing.T) {
	service, recorder, _ := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	input := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}

	runner, err := service.ViewRunner(t.Context(), input)
	require.ErrorContains(t, err, "runner_type")
	assert.Nil(t, runner)
	requests := recorder.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodGet, requests[0].method)
}

func TestRunnerUpdateReportsInvalidSuccessResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		detail string
	}{
		{"empty response", http.StatusOK, "", "runner_type"},
		{"no content", http.StatusNoContent, "", "runner_type"},
		{"missing runner type", http.StatusOK, `{"configured_by":"code_review"}`, "runner_type"},
		{"malformed json", http.StatusOK, `{"runner_type":`, "decod"},
		{"incorrect field type", http.StatusOK, `{"runner_type":false}`, "decod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, _ := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, err := io.WriteString(w, tt.body)
				assert.NoError(t, err)
			})
			input := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}
			_, runner, err := service.SetRunner(t.Context(), input, RunnerOptions{Type: "standard"})
			require.ErrorContains(t, err, "succeeded")
			assert.ErrorContains(t, err, tt.detail)
			assert.ErrorContains(t, err, "forge.test.invalid/fixture-team")
			assert.Nil(t, runner)
			if tt.name == "malformed json" {
				var syntaxErr *json.SyntaxError
				assert.ErrorAs(t, err, &syntaxErr)
			}
			if tt.name == "incorrect field type" {
				var typeErr *json.UnmarshalTypeError
				assert.ErrorAs(t, err, &typeErr)
			}
			requests := recorder.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPut, requests[0].method)
		})
	}
}

func TestRunnerUpdatePreservesAPIErrorDetails(t *testing.T) {
	service, recorder, _ := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, err := io.WriteString(w, `{"message":"fixture runner rejected","diagnostic":"fixture restriction"}`)
		assert.NoError(t, err)
	})
	input := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}

	_, runner, err := service.SetRunner(t.Context(), input, RunnerOptions{Type: "standard"})
	require.ErrorContains(t, err, "422")
	assert.ErrorContains(t, err, "fixture restriction")
	assert.ErrorContains(t, err, "forge.test.invalid/fixture-team")
	assert.NotContains(t, err.Error(), "succeeded")
	var apiErr *github.ErrorResponse
	require.ErrorAs(t, err, &apiErr)
	assert.Nil(t, runner)
	requests := recorder.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPut, requests[0].method)
}

func TestInvalidInputDoesNotCreateClient(t *testing.T) {
	calls := 0
	service := NewService(func(repository.Repository) (*gh.GitHubClient, error) {
		calls++
		return nil, errors.New("unexpected client construction")
	})
	valid := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}
	_, err := service.SetPolicy(t.Context(), valid, "configured_by_organization_admins")
	require.ErrorContains(t, err, "invalid --state")
	_, _, err = service.SetRunner(t.Context(), valid, RunnerOptions{Type: "standard", LabelsSet: true})
	require.ErrorContains(t, err, "cannot be used")
	_, _, err = service.SetRunner(t.Context(), valid, RunnerOptions{Type: "labeled", LabelValues: []string{"", "runner-amber"}})
	require.ErrorContains(t, err, "empty label")
	_, err = service.ViewPolicy(t.Context(), TargetInput{OrganizationSet: true})
	require.ErrorContains(t, err, "--org must not be empty")
	assert.Zero(t, calls)
}

func TestAPIErrorsPreserveResponseDetails(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		rateLimit bool
		want      []string
	}{
		{
			name:   "enterprise managed conflict",
			status: http.StatusConflict,
			body:   `{"message":"fixture policy is locked","code":"POLICY_MANAGED_BY_ENTERPRISE","policy":{"configured_state":null,"effective_state":"enabled","editable":false,"managed_by":"enterprise"}}`,
			want:   []string{"POLICY_MANAGED_BY_ENTERPRISE", "fixture policy is locked", "configured_state", "effective_state"},
		},
		{
			name:   "forbidden with details",
			status: http.StatusForbidden,
			body:   `{"message":"fixture access denied","diagnostic":"fixture restriction"}`,
			want:   []string{"fixture access denied", "fixture restriction"},
		},
		{
			name:   "validation error",
			status: http.StatusUnprocessableEntity,
			body:   `{"message":"fixture rejected","errors":[{"field":"state","code":"invalid","message":"fixture detail"}]}`,
			want:   []string{"fixture rejected", "fixture detail"},
		},
		{
			name:      "rate limit",
			status:    http.StatusForbidden,
			body:      `{"message":"fixture rate limit","diagnostic":"fixture rate detail"}`,
			rateLimit: true,
			want:      []string{"fixture rate limit", "fixture rate detail"},
		},
		{
			name:   "non-json response",
			status: http.StatusForbidden,
			body:   "fixture proxy rejection",
			want:   []string{"fixture proxy rejection"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recorder, _ := newHTTPService(t, func(w http.ResponseWriter, r *http.Request) {
				if tt.rateLimit {
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("X-RateLimit-Reset", "1")
				}
				w.WriteHeader(tt.status)
				_, writeErr := io.WriteString(w, tt.body)
				assert.NoError(t, writeErr)
			})
			input := TargetInput{Organization: "forge.test.invalid/fixture-team", OrganizationSet: true}
			_, err := service.SetPolicy(t.Context(), input, "enabled")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "forge.test.invalid/fixture-team")
			assert.Contains(t, err.Error(), fmt.Sprint(tt.status))
			for _, detail := range tt.want {
				assert.Contains(t, err.Error(), detail)
			}
			requests := recorder.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPut, requests[0].method)
			assert.Equal(t, map[string]any{"state": "enabled"}, decodeBody(t, requests[0]))

			var response *http.Response
			if tt.rateLimit {
				var rateErr *github.RateLimitError
				require.ErrorAs(t, err, &rateErr)
				response = rateErr.Response
			} else {
				var apiErr *github.ErrorResponse
				require.ErrorAs(t, err, &apiErr)
				response = apiErr.Response
			}
			body, readErr := io.ReadAll(response.Body)
			require.NoError(t, readErr)
			assert.Equal(t, tt.body, string(body))
		})
	}
}

type failingResponseBody struct{}

func (failingResponseBody) Read([]byte) (int, error) {
	return 0, errors.New("fixture body read failure")
}

func (failingResponseBody) Close() error {
	return nil
}

func TestResponseDetailReadFailurePreservesAPIError(t *testing.T) {
	original := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: failingResponseBody{}},
		Message:  "fixture rejected",
	}
	err := withResponseDetails(original)
	require.ErrorContains(t, err, "failed to read API response details")
	assert.ErrorContains(t, err, "fixture body read failure")
	var apiErr *github.ErrorResponse
	require.ErrorAs(t, err, &apiErr)
	assert.Same(t, original, apiErr)
}

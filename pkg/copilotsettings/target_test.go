package copilotsettings

import (
	"os/exec"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolateTargetEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_HOST", "default.test.invalid")
	t.Setenv("GH_REPO", "")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_SERVER_URL", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")
}

func TestResolveTarget(t *testing.T) {
	isolateTargetEnvironment(t)
	t.Setenv("GH_REPO", "fallback.test.invalid/fixture-owner/fixture-repo")

	tests := []struct {
		name            string
		input           TargetInput
		allowEnterprise bool
		expected        Target
	}{
		{
			name:            "organization on an explicit host",
			input:           TargetInput{Organization: "forge.test.invalid/fixture-org", OrganizationSet: true},
			allowEnterprise: true,
			expected:        Target{Repository: repository.Repository{Host: "forge.test.invalid", Owner: "fixture-org"}},
		},
		{
			name:            "organization on the default host",
			input:           TargetInput{Organization: "fixture-org", OrganizationSet: true},
			allowEnterprise: true,
			expected:        Target{Repository: repository.Repository{Host: "default.test.invalid", Owner: "fixture-org"}},
		},
		{
			name:            "repository owner on an explicit host",
			input:           TargetInput{Repository: "forge.test.invalid/fixture-org/fixture-project", RepositorySet: true},
			allowEnterprise: true,
			expected:        Target{Repository: repository.Repository{Host: "forge.test.invalid", Owner: "fixture-org"}},
		},
		{
			name:            "repository owner on the default host",
			input:           TargetInput{Repository: "fixture-org/fixture-project", RepositorySet: true},
			allowEnterprise: false,
			expected:        Target{Repository: repository.Repository{Host: "default.test.invalid", Owner: "fixture-org"}},
		},
		{
			name:            "enterprise on an explicit host",
			input:           TargetInput{Enterprise: "forge.test.invalid/fixture-enterprise", EnterpriseSet: true},
			allowEnterprise: true,
			expected:        Target{Repository: repository.Repository{Host: "forge.test.invalid", Owner: "fixture-enterprise"}, Enterprise: true},
		},
		{
			name:            "enterprise on the default host",
			input:           TargetInput{Enterprise: "fixture-enterprise", EnterpriseSet: true},
			allowEnterprise: true,
			expected:        Target{Repository: repository.Repository{Host: "default.test.invalid", Owner: "fixture-enterprise"}, Enterprise: true},
		},
		{
			name:            "current repository override",
			allowEnterprise: true,
			expected:        Target{Repository: repository.Repository{Host: "fallback.test.invalid", Owner: "fixture-owner"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ResolveTarget(tt.input, tt.allowEnterprise)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, target)
			assert.NotContains(t, target.String(), "fixture-project")
		})
	}
}

func TestResolveTargetRejectsInvalidSelectors(t *testing.T) {
	isolateTargetEnvironment(t)
	tests := []struct {
		name            string
		input           TargetInput
		allowEnterprise bool
		message         string
	}{
		{"empty organization", TargetInput{OrganizationSet: true}, true, "--org must not be empty"},
		{"blank organization", TargetInput{Organization: " \t", OrganizationSet: true}, true, "--org must not be empty"},
		{"empty repository", TargetInput{RepositorySet: true}, true, "--repo must not be empty"},
		{"empty enterprise", TargetInput{EnterpriseSet: true}, true, "--enterprise must not be empty"},
		{"whitespace in organization", TargetInput{Organization: "fixture org", OrganizationSet: true}, true, "must not contain whitespace"},
		{"malformed repository", TargetInput{Repository: "fixture", RepositorySet: true}, true, "resolve target"},
		{"malformed organization", TargetInput{Organization: "host/owner/extra", OrganizationSet: true}, true, "resolve target"},
		{"multiple selectors", TargetInput{Organization: "fixture-org", Repository: "fixture-org/fixture-repo"}, true, "mutually exclusive"},
		{"enterprise on runner", TargetInput{Enterprise: "fixture-enterprise", EnterpriseSet: true}, false, "not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveTarget(tt.input, tt.allowEnterprise)
			require.ErrorContains(t, err, tt.message)
		})
	}
}

func TestResolveTargetFromCurrentRepository(t *testing.T) {
	isolateTargetEnvironment(t)
	t.Setenv("GH_HOST", "forge.test.invalid")
	dir := t.TempDir()
	for _, args := range [][]string{
		{"-C", dir, "init", "-q"},
		{"-C", dir, "remote", "add", "origin", "https://forge.test.invalid/local-fixture/sample-repo.git"},
	} {
		output, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	t.Chdir(dir)

	target, err := ResolveTarget(TargetInput{}, false)
	require.NoError(t, err)
	assert.Equal(t, repository.Repository{Host: "forge.test.invalid", Owner: "local-fixture"}, target.Repository)
}

func TestResolveTargetOutsideRepository(t *testing.T) {
	isolateTargetEnvironment(t)
	t.Chdir(t.TempDir())

	_, err := ResolveTarget(TargetInput{}, true)
	require.ErrorContains(t, err, "resolve target")
}

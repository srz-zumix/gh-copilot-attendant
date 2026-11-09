// Package copilotsettings manages Copilot code review policies and runner settings.
package copilotsettings

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
)

// TargetInput contains the target values and whether each flag was supplied.
type TargetInput struct {
	Organization    string
	Repository      string
	Enterprise      string
	OrganizationSet bool
	RepositorySet   bool
	EnterpriseSet   bool
}

// Target identifies an organization or enterprise on a GitHub host.
type Target struct {
	Repository repository.Repository
	Enterprise bool
}

func (t Target) String() string {
	kind := "organization"
	if t.Enterprise {
		kind = "enterprise"
	}
	return fmt.Sprintf("%s %q", kind, parser.GetRepositoryFullNameWithHost(t.Repository))
}

// ResolveTarget resolves the selected target, or the current repository's owner.
func ResolveTarget(input TargetInput, allowEnterprise bool) (Target, error) {
	orgSet := input.OrganizationSet || input.Organization != ""
	repoSet := input.RepositorySet || input.Repository != ""
	enterpriseSet := input.EnterpriseSet || input.Enterprise != ""

	if enterpriseSet && !allowEnterprise {
		return Target{}, fmt.Errorf("--enterprise is not supported for runner settings")
	}
	count := 0
	for _, selected := range []bool{orgSet, repoSet, enterpriseSet} {
		if selected {
			count++
		}
	}
	if count > 1 {
		return Target{}, fmt.Errorf("--org, --repo, and --enterprise are mutually exclusive")
	}

	for _, selector := range []struct {
		name  string
		value string
		set   bool
	}{
		{"--org", input.Organization, orgSet},
		{"--repo", input.Repository, repoSet},
		{"--enterprise", input.Enterprise, enterpriseSet},
	} {
		if !selector.set {
			continue
		}
		if strings.TrimSpace(selector.value) == "" {
			return Target{}, fmt.Errorf("%s must not be empty", selector.name)
		}
		if strings.ContainsFunc(selector.value, unicode.IsSpace) {
			return Target{}, fmt.Errorf("%s must not contain whitespace", selector.name)
		}
	}

	var (
		repo repository.Repository
		err  error
	)
	switch {
	case orgSet:
		repo, err = parser.Repository(parser.RepositoryOwnerWithHost(input.Organization))
	case repoSet:
		repo, err = parser.Repository(parser.RepositoryInput(input.Repository))
	case enterpriseSet:
		repo, err = parser.Repository(parser.RepositoryOwnerWithHost(input.Enterprise))
	default:
		repo, err = parser.Repository()
	}
	if err != nil {
		return Target{}, fmt.Errorf("resolve target: %w", err)
	}
	if strings.TrimSpace(repo.Host) == "" || strings.ContainsFunc(repo.Host, unicode.IsSpace) ||
		strings.TrimSpace(repo.Owner) == "" || strings.ContainsFunc(repo.Owner, unicode.IsSpace) {
		return Target{}, fmt.Errorf("resolved target must have a valid host and owner")
	}
	repo.Name = ""
	return Target{Repository: repo, Enterprise: enterpriseSet}, nil
}

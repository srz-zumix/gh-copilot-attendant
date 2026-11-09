package copilotsettings

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// ClientFactory constructs a GitHub client for a resolved target.
type ClientFactory func(repository.Repository) (*gh.GitHubClient, error)

// Service manages code review settings through the shared GitHub wrappers.
type Service struct {
	newClient ClientFactory
}

// NewService creates a settings service with the supplied client factory.
func NewService(factory ClientFactory) *Service {
	return &Service{newClient: factory}
}

// Policy contains exactly one organization or enterprise policy.
type Policy struct {
	Organization *gh.OrganizationCopilotCodeReviewPolicy
	Enterprise   *gh.EnterpriseCopilotCodeReviewPolicy
}

func (s *Service) client(target Target) (*gh.GitHubClient, error) {
	if s == nil || s.newClient == nil {
		return nil, fmt.Errorf("GitHub client factory is not configured")
	}
	client, err := s.newClient(target.Repository)
	if err != nil {
		return nil, fmt.Errorf("create GitHub client for %s: %w", target, err)
	}
	if client == nil {
		return nil, fmt.Errorf("create GitHub client for %s: factory returned nil", target)
	}
	return client, nil
}

// ViewPolicy retrieves an organization or enterprise policy.
func (s *Service) ViewPolicy(ctx context.Context, input TargetInput) (*Policy, error) {
	target, err := ResolveTarget(input, true)
	if err != nil {
		return nil, err
	}
	client, err := s.client(target)
	if err != nil {
		return nil, err
	}
	if target.Enterprise {
		policy, err := gh.GetEnterpriseCopilotCodeReviewPolicy(ctx, client, target.Repository)
		if err != nil {
			return nil, apiError(target, err)
		}
		return &Policy{Enterprise: policy}, nil
	}
	policy, err := gh.GetOrganizationCopilotCodeReviewPolicy(ctx, client, target.Repository)
	if err != nil {
		return nil, apiError(target, err)
	}
	if policy == nil || strings.TrimSpace(policy.EffectiveState) == "" {
		return nil, fmt.Errorf("invalid policy response for %s: missing effective_state", target)
	}
	return &Policy{Organization: policy}, nil
}

// SetPolicy validates and submits an organization or enterprise policy update.
func (s *Service) SetPolicy(ctx context.Context, input TargetInput, state string) (Target, error) {
	target, err := ResolveTarget(input, true)
	if err != nil {
		return Target{}, err
	}
	if err := ValidatePolicyState(target.Enterprise, state); err != nil {
		return Target{}, err
	}
	client, err := s.client(target)
	if err != nil {
		return Target{}, err
	}
	if target.Enterprise {
		err = gh.SetEnterpriseCopilotCodeReviewPolicy(ctx, client, target.Repository, state)
	} else {
		err = gh.SetOrganizationCopilotCodeReviewPolicy(ctx, client, target.Repository, state)
	}
	if err != nil {
		return Target{}, apiError(target, err)
	}
	return target, nil
}

// ViewRunner retrieves an organization's effective runner configuration.
func (s *Service) ViewRunner(ctx context.Context, input TargetInput) (*gh.OrganizationCopilotCodeReviewRunner, error) {
	target, err := ResolveTarget(input, false)
	if err != nil {
		return nil, err
	}
	client, err := s.client(target)
	if err != nil {
		return nil, err
	}
	runner, err := gh.GetOrganizationCopilotCodeReviewRunner(ctx, client, target.Repository)
	if err != nil {
		return nil, apiError(target, err)
	}
	if err := validateRunnerResponse(runner); err != nil {
		return nil, fmt.Errorf("invalid runner response for %s: %w", target, err)
	}
	return runner, nil
}

// SetRunner validates and submits an organization runner update.
func (s *Service) SetRunner(ctx context.Context, input TargetInput, options RunnerOptions) (Target, *gh.OrganizationCopilotCodeReviewRunner, error) {
	target, err := ResolveTarget(input, false)
	if err != nil {
		return Target{}, nil, err
	}
	update, err := options.Update()
	if err != nil {
		return Target{}, nil, err
	}
	client, err := s.client(target)
	if err != nil {
		return Target{}, nil, err
	}
	runner, err := gh.SetOrganizationCopilotCodeReviewRunner(ctx, client, target.Repository, update)
	if err != nil {
		return Target{}, nil, runnerUpdateError(target, err)
	}
	if err := validateRunnerResponse(runner); err != nil {
		return Target{}, nil, fmt.Errorf("runner update for %s succeeded but its response is invalid: %w", target, err)
	}
	return target, runner, nil
}

func validateRunnerResponse(runner *gh.OrganizationCopilotCodeReviewRunner) error {
	if runner == nil || strings.TrimSpace(runner.RunnerType) == "" {
		return fmt.Errorf("API response omitted runner_type")
	}
	return nil
}

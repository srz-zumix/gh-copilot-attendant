package copilotmetrics

import (
	"context"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// GitHubFetcher reads usage metrics reports through the GitHub REST API.
type GitHubFetcher struct {
	client *gh.GitHubClient
	source gh.CopilotMetricsSource
}

// NewGitHubFetcher returns a Fetcher for the organization or enterprise in source.
func NewGitHubFetcher(client *gh.GitHubClient, source gh.CopilotMetricsSource) *GitHubFetcher {
	return &GitHubFetcher{client: client, source: source}
}

func (f *GitHubFetcher) LatestMetrics(ctx context.Context) (*gh.CopilotLatestMetrics, error) {
	return gh.GetCopilotLatestMetrics(ctx, f.client, f.source)
}

func (f *GitHubFetcher) DailyMetrics(ctx context.Context, day string) ([]*github.CopilotDailyMetrics, bool, error) {
	return gh.GetCopilotDailyMetrics(ctx, f.client, f.source, day)
}

func (f *GitHubFetcher) LatestUserMetrics(ctx context.Context) (*gh.CopilotLatestUserMetrics, error) {
	return gh.GetCopilotLatestUserMetrics(ctx, f.client, f.source)
}

func (f *GitHubFetcher) DailyUserMetrics(ctx context.Context, day string) ([]*github.CopilotUserDailyMetrics, bool, error) {
	return gh.GetCopilotDailyUserMetrics(ctx, f.client, f.source, day)
}

func (f *GitHubFetcher) DailyRepositoryMetrics(ctx context.Context, day string) ([]*github.CopilotRepositoryDailyMetrics, bool, error) {
	return gh.GetCopilotDailyRepositoryMetrics(ctx, f.client, f.source, day)
}

func (f *GitHubFetcher) DailyUserTeamsMetrics(ctx context.Context, day string) ([]*github.CopilotUserTeamsDailyMetrics, bool, error) {
	return gh.GetCopilotDailyUserTeamsMetrics(ctx, f.client, f.source, day)
}

// GitHubIssuePublisher publishes reports to issues of a repository through the GitHub REST API.
type GitHubIssuePublisher struct {
	client *gh.GitHubClient
	repo   repository.Repository
}

// NewGitHubIssuePublisher returns an IssuePublisher for repo.
func NewGitHubIssuePublisher(client *gh.GitHubClient, repo repository.Repository) *GitHubIssuePublisher {
	return &GitHubIssuePublisher{client: client, repo: repo}
}

func (p *GitHubIssuePublisher) IsPublic(ctx context.Context) (bool, error) {
	r, err := gh.GetRepository(ctx, p.client, p.repo)
	if err != nil {
		return false, err
	}
	if r.Visibility != nil {
		return r.GetVisibility() == "public", nil
	}
	return !r.GetPrivate(), nil
}

func (p *GitHubIssuePublisher) IssueURL(ctx context.Context, number int) (string, error) {
	issue, err := gh.GetIssue(ctx, p.client, p.repo, number)
	if err != nil {
		return "", err
	}
	return issue.GetHTMLURL(), nil
}

func (p *GitHubIssuePublisher) CreateIssue(ctx context.Context, title, body string, labels []string) (int, string, error) {
	issue, err := gh.CreateIssue(ctx, p.client, p.repo, title, body, labels)
	if err != nil {
		return 0, "", err
	}
	return issue.GetNumber(), issue.GetHTMLURL(), nil
}

func (p *GitHubIssuePublisher) UpsertArtifact(ctx context.Context, number int, a Artifact) (string, bool, error) {
	artifact, created, err := gh.UpsertIssueArtifact(ctx, p.client, p.repo, number, &gh.IssueArtifactRequest{
		Type: a.Type,
		Name: a.Name,
		Body: &a.Body,
	})
	if err != nil {
		return "", false, err
	}
	return artifact.HTMLURL, created, nil
}

package copilotmetrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// Artifact is a rendered report to attach to an issue.
type Artifact struct {
	Format Format
	Name   string
	// Type is the issue artifact type (generic for Markdown, embedded_html for HTML).
	Type string
	Body string
}

// ArtifactType returns the issue artifact type for the format.
func ArtifactType(f Format) string {
	if f == FormatHTML {
		return gh.IssueArtifactTypeEmbeddedHTML
	}
	return gh.IssueArtifactTypeGeneric
}

// ArtifactName returns the artifact name identifying the source, window, and format. Re-running the
// report for the same source and window updates the artifact instead of adding a new one.
func ArtifactName(s *Summary, f Format) string {
	label := "Markdown"
	if f == FormatHTML {
		label = "HTML"
	}
	name := fmt.Sprintf("Copilot usage metrics: %s %s..%s (%s)", s.Source, s.StartDay, s.EndDay, label)
	// Issue artifact names allow only ASCII alphanumerics, spaces, periods, hyphens, and underscores.
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == ' ' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return ' '
	}, name)
	return strings.Join(strings.Fields(name), " ")
}

// NewArtifact builds the artifact for a rendered report body.
func NewArtifact(s *Summary, f Format, body string) Artifact {
	return Artifact{Format: f, Name: ArtifactName(s, f), Type: ArtifactType(f), Body: body}
}

// DefaultIssueTitle returns the default title of a created issue.
func DefaultIssueTitle(s *Summary) string {
	return fmt.Sprintf("Copilot usage metrics: %s %s..%s", s.Source, s.StartDay, s.EndDay)
}

// ReportFileName returns the output file name for the format.
func ReportFileName(f Format) string {
	return "report." + string(f)
}

// WriteReports writes each rendered report to dir as report.<ext> and returns the written paths.
func WriteReports(dir string, reports map[Format]string, formats []Format) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, f := range formats {
		p := filepath.Join(dir, ReportFileName(f))
		if err := os.WriteFile(p, []byte(reports[f]), 0o644); err != nil {
			return paths, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// IssuePublisher creates issues and attaches artifacts in a destination repository.
type IssuePublisher interface {
	// IsPublic reports whether the destination repository is public.
	IsPublic(ctx context.Context) (bool, error)
	// IssueURL returns the URL of an existing issue, failing if it does not exist.
	IssueURL(ctx context.Context, number int) (string, error)
	CreateIssue(ctx context.Context, title, body string, labels []string) (int, string, error)
	// UpsertArtifact updates the artifact with the same name and type, or creates it.
	UpsertArtifact(ctx context.Context, number int, a Artifact) (url string, created bool, err error)
}

// CheckPublishTarget verifies the destination before any data is fetched or written: public
// repositories require AllowPublic and an existing issue must exist.
func CheckPublishTarget(ctx context.Context, p IssuePublisher, opts PublishOptions) error {
	if !opts.IssueEnabled() {
		return nil
	}
	public, err := p.IsPublic(ctx)
	if err != nil {
		return fmt.Errorf("failed to check the destination repository visibility: %w", err)
	}
	if public && !opts.AllowPublic {
		return fmt.Errorf("the destination repository is public; usage metrics would be visible to anyone. Pass --allow-public to publish anyway")
	}
	if opts.Issue > 0 {
		if _, err := p.IssueURL(ctx, opts.Issue); err != nil {
			return fmt.Errorf("failed to get issue #%d: %w", opts.Issue, err)
		}
	}
	return nil
}

// PublishedArtifact is an artifact attached by Publish.
type PublishedArtifact struct {
	Name    string
	URL     string
	Created bool
}

// PublishResult is the outcome of Publish, filled in as far as publication succeeded.
type PublishResult struct {
	IssueNumber int
	IssueURL    string
	Artifacts   []PublishedArtifact
}

// PublishError reports a failure after the issue was resolved, so the caller can retry with --issue.
type PublishError struct {
	IssueNumber int
	IssueURL    string
	Err         error
}

func (e *PublishError) Error() string {
	return fmt.Sprintf("failed to attach report artifacts to issue #%d (%s); retry with --issue %d: %v", e.IssueNumber, e.IssueURL, e.IssueNumber, e.Err)
}

func (e *PublishError) Unwrap() error {
	return e.Err
}

// Publish creates or resolves the issue and attaches the artifacts. All artifacts must already be
// rendered so that a rendering failure never leaves an orphan issue.
func Publish(ctx context.Context, p IssuePublisher, opts PublishOptions, title, body string, artifacts []Artifact) (*PublishResult, error) {
	result := &PublishResult{}
	if opts.CreateIssue {
		number, url, err := p.CreateIssue(ctx, title, body, opts.Labels)
		if err != nil {
			return nil, fmt.Errorf("failed to create issue: %w", err)
		}
		result.IssueNumber, result.IssueURL = number, url
	} else {
		url, err := p.IssueURL(ctx, opts.Issue)
		if err != nil {
			return nil, fmt.Errorf("failed to get issue #%d: %w", opts.Issue, err)
		}
		result.IssueNumber, result.IssueURL = opts.Issue, url
	}
	for _, a := range artifacts {
		url, created, err := p.UpsertArtifact(ctx, result.IssueNumber, a)
		if err != nil {
			return result, &PublishError{IssueNumber: result.IssueNumber, IssueURL: result.IssueURL, Err: fmt.Errorf("%s: %w", a.Name, err)}
		}
		result.Artifacts = append(result.Artifacts, PublishedArtifact{Name: a.Name, URL: url, Created: created})
	}
	return result, nil
}

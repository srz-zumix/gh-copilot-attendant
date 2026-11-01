package copilotmetrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	public      bool
	publicErr   error
	issues      map[int]string
	created     []string
	upserted    []Artifact
	failOn      string
	nextNumber  int
	createErr   error
	createdNums []int
}

func (p *fakePublisher) IsPublic(context.Context) (bool, error) {
	return p.public, p.publicErr
}

func (p *fakePublisher) IssueURL(_ context.Context, number int) (string, error) {
	url, ok := p.issues[number]
	if !ok {
		return "", errors.New("not found")
	}
	return url, nil
}

func (p *fakePublisher) CreateIssue(_ context.Context, title, _ string, _ []string) (int, string, error) {
	if p.createErr != nil {
		return 0, "", p.createErr
	}
	p.created = append(p.created, title)
	p.createdNums = append(p.createdNums, p.nextNumber)
	return p.nextNumber, fmt.Sprintf("https://example.test/octo/reports/issues/%d", p.nextNumber), nil
}

func (p *fakePublisher) UpsertArtifact(_ context.Context, number int, a Artifact) (string, bool, error) {
	if a.Name == p.failOn {
		return "", false, errors.New("rejected")
	}
	p.upserted = append(p.upserted, a)
	return fmt.Sprintf("https://example.test/octo/reports/issues/%d#artifact-%d", number, len(p.upserted)), true, nil
}

func testSummary() *Summary {
	return &Summary{Source: "octo (organization)", StartDay: "2030-01-01", EndDay: "2030-01-28"}
}

func TestArtifactNaming(t *testing.T) {
	s := testSummary()
	md := NewArtifact(s, FormatMarkdown, "body")
	assert.Equal(t, "Copilot usage metrics octo organization 2030-01-01..2030-01-28 Markdown", md.Name)
	assert.Equal(t, gh.IssueArtifactTypeGeneric, md.Type)
	html := NewArtifact(s, FormatHTML, "<p>")
	assert.Equal(t, gh.IssueArtifactTypeEmbeddedHTML, html.Type)
	assert.NotEqual(t, md.Name, html.Name)
	assert.Equal(t, "report.html", ReportFileName(FormatHTML))
	assert.Equal(t, "Copilot usage metrics: octo (organization) 2030-01-01..2030-01-28", DefaultIssueTitle(s))
}

func TestArtifactNameConstraints(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   string
	}{
		{source: "sample-org (organization)", want: "sample-org organization"},
		{source: "sample-org (enterprise)", want: "sample-org enterprise"},
		{source: "github.example.test/sample-org (organization)", want: "github.example.test sample-org organization"},
		{source: "github.example.test/sample_enterprise (enterprise)", want: "github.example.test sample_enterprise enterprise"},
		{source: "  sample\torg\n(test)  ", want: "sample org test"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			s := testSummary()
			s.Source = tt.source
			names := map[Format]string{}
			for _, f := range []Format{FormatMarkdown, FormatHTML} {
				a := NewArtifact(s, f, "report body")
				label := "Markdown"
				if f == FormatHTML {
					label = "HTML"
				}
				assert.Equal(t, "Copilot usage metrics "+tt.want+" 2030-01-01..2030-01-28 "+label, a.Name)
				assert.Regexp(t, `^[A-Za-z0-9](?:[A-Za-z0-9 ._-]*[A-Za-z0-9])?$`, a.Name)
				assert.Equal(t, a.Name, ArtifactName(s, f), "names must be stable for upserts")
				assert.Equal(t, "report body", a.Body)
				names[f] = a.Name
			}
			assert.NotEqual(t, names[FormatMarkdown], names[FormatHTML])
			s.EndDay = "2030-01-29"
			assert.NotEqual(t, names[FormatHTML], ArtifactName(s, FormatHTML))
		})
	}
}

func TestWriteReports(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	paths, err := WriteReports(dir, map[Format]string{FormatMarkdown: "# md", FormatHTML: "<p>"}, []Format{FormatMarkdown, FormatHTML})
	require.NoError(t, err)
	require.Len(t, paths, 2)
	b, err := os.ReadFile(paths[0])
	require.NoError(t, err)
	assert.Equal(t, "# md", string(b))
}

func TestCheckPublishTarget(t *testing.T) {
	ctx := t.Context()
	assert.NoError(t, CheckPublishTarget(ctx, &fakePublisher{public: true}, PublishOptions{}), "nothing is checked when publishing is disabled")

	err := CheckPublishTarget(ctx, &fakePublisher{public: true}, PublishOptions{CreateIssue: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--allow-public")
	assert.NoError(t, CheckPublishTarget(ctx, &fakePublisher{public: true}, PublishOptions{CreateIssue: true, AllowPublic: true}))

	require.Error(t, CheckPublishTarget(ctx, &fakePublisher{publicErr: errors.New("denied")}, PublishOptions{CreateIssue: true}))
	require.Error(t, CheckPublishTarget(ctx, &fakePublisher{}, PublishOptions{Issue: 5}))
	assert.NoError(t, CheckPublishTarget(ctx, &fakePublisher{issues: map[int]string{5: "u"}}, PublishOptions{Issue: 5}))
}

func TestPublishCreateIssue(t *testing.T) {
	p := &fakePublisher{nextNumber: 42}
	s := testSummary()
	artifacts := []Artifact{NewArtifact(s, FormatMarkdown, "md"), NewArtifact(s, FormatHTML, "html")}
	res, err := Publish(t.Context(), p, PublishOptions{CreateIssue: true}, "title", "body", artifacts)
	require.NoError(t, err)
	assert.Equal(t, 42, res.IssueNumber)
	assert.Equal(t, []string{"title"}, p.created)
	require.Len(t, res.Artifacts, 2)
	assert.Equal(t, artifacts[0].Name, res.Artifacts[0].Name)
	assert.True(t, res.Artifacts[1].Created)
}

func TestPublishExistingIssueFailure(t *testing.T) {
	s := testSummary()
	artifacts := []Artifact{NewArtifact(s, FormatMarkdown, "md"), NewArtifact(s, FormatHTML, "html")}
	p := &fakePublisher{issues: map[int]string{7: "https://example.test/octo/reports/issues/7"}, failOn: artifacts[1].Name}
	res, err := Publish(t.Context(), p, PublishOptions{Issue: 7}, "", "", artifacts)
	require.Error(t, err)
	var pe *PublishError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, 7, pe.IssueNumber)
	assert.Contains(t, err.Error(), "--issue 7")
	assert.Contains(t, err.Error(), "https://example.test/octo/reports/issues/7")
	require.NotNil(t, res)
	assert.Len(t, res.Artifacts, 1)
	assert.Empty(t, p.created)
}

func TestPublishCreateIssueFailure(t *testing.T) {
	p := &fakePublisher{createErr: errors.New("forbidden")}
	_, err := Publish(t.Context(), p, PublishOptions{CreateIssue: true}, "t", "b", nil)
	require.Error(t, err)
	var pe *PublishError
	assert.False(t, errors.As(err, &pe))
}

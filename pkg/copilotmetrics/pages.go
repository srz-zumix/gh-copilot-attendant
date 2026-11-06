package copilotmetrics

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

const pagesBranch = "copilot-metrics-pages"

var (
	errPagesSiteNotFound   = errors.New("pages site not found")
	errPagesBranchNotFound = errors.New("pages branch not found")
)

type pagesStore interface {
	Repository(context.Context) (*github.Repository, error)
	Site(context.Context) (*github.Pages, error)
	Branch(context.Context, string) (string, error)
	Snapshot(context.Context, string) (*pagesSnapshot, error)
	CreateCleanBranch(context.Context, string, map[string][]byte) error
	CreateWorkflowSite(context.Context) error
	SetPrivate(context.Context) error
	SetBranchSource(context.Context, string) error
	Commit(context.Context, string, string, map[string][]byte) (string, error)
}

// PagesPublisher publishes reports to a dedicated private GitHub Pages site.
type PagesPublisher struct {
	store pagesStore
}

// PagesTarget identifies the private site verified before collecting metrics.
type PagesTarget struct {
	repositoryID int64
	url          string
	initialized  bool
}

// Initialized reports whether the site was initialized during preparation.
func (t *PagesTarget) Initialized() bool {
	return t.initialized
}

// URL returns the URL of the prepared site.
func (t *PagesTarget) URL() string {
	return t.url
}

// Branch returns the dedicated source branch of the prepared site.
func (t *PagesTarget) Branch() string {
	return pagesBranch
}

// PagesResult describes an update to a Pages source branch.
type PagesResult struct {
	URL       string
	Branch    string
	CommitSHA string
}

func validatePagesRepository(repo *github.Repository) error {
	if repo == nil || repo.ID == nil || repo.GetID() <= 0 {
		return fmt.Errorf("destination repository identity is unavailable")
	}
	if repo.Owner == nil || repo.Owner.Type == nil || repo.Owner.GetType() != "Organization" || repo.Owner.GetLogin() == "" {
		return fmt.Errorf("pages requires an organization-owned repository")
	}
	if repo.Name == nil || repo.GetName() == "" {
		return fmt.Errorf("destination repository name is unavailable")
	}
	if strings.EqualFold(repo.GetName(), repo.Owner.GetLogin()+".github.io") {
		return fmt.Errorf("organization sites do not support private Pages access control")
	}
	if repo.Visibility == nil || (repo.GetVisibility() != "private" && repo.GetVisibility() != "internal") {
		return fmt.Errorf("pages requires a private or internal repository with verified visibility")
	}
	if repo.DefaultBranch == nil || repo.GetDefaultBranch() == "" {
		return fmt.Errorf("destination repository must have an initialized default branch")
	}
	return nil
}

func validatePrivatePages(site *github.Pages) error {
	if site == nil || site.Public == nil {
		return fmt.Errorf("pages visibility could not be verified")
	}
	if *site.Public {
		return fmt.Errorf("pages site is public; reports can only be published to a private site")
	}
	return nil
}

func validateManagedPages(site *github.Pages) (string, error) {
	if err := validatePrivatePages(site); err != nil {
		return "", err
	}
	if site.BuildType == nil || site.GetBuildType() != "legacy" || site.Source == nil ||
		site.Source.GetBranch() != pagesBranch || site.Source.GetPath() != "/" {
		return "", fmt.Errorf("pages site is not configured for the dedicated %s branch at the root", pagesBranch)
	}
	if site.HTMLURL == nil || site.GetHTMLURL() == "" {
		return "", fmt.Errorf("pages site URL is unavailable")
	}
	u, err := url.Parse(site.GetHTMLURL())
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid HTTPS Pages site URL %q", site.GetHTMLURL())
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		u.RawPath = ""
	}
	return u.String(), nil
}

// Prepare checks the destination before metrics are fetched. When the repository has no Pages site,
// it initializes a dedicated private site, creating the managed branch or reusing a valid one.
func (p *PagesPublisher) Prepare(ctx context.Context) (*PagesTarget, error) {
	repo, err := p.store.Repository(ctx)
	if err != nil {
		return nil, fmt.Errorf("get Pages destination repository: %w", err)
	}
	if err := validatePagesRepository(repo); err != nil {
		return nil, err
	}
	if _, err := p.store.Branch(ctx, repo.GetDefaultBranch()); err != nil {
		return nil, fmt.Errorf("verify the initialized default branch %q: %w", repo.GetDefaultBranch(), err)
	}

	site, err := p.store.Site(ctx)
	initialized := false
	if err != nil {
		if !errors.Is(err, errPagesSiteNotFound) {
			return nil, fmt.Errorf("get Pages site: %w", err)
		}
		if repo.HasPages == nil || repo.GetHasPages() {
			return nil, fmt.Errorf("cannot confirm that the repository has no Pages site")
		}
		site, err = p.initialize(ctx, repo.GetID())
		if err != nil {
			return nil, err
		}
		initialized = true
	}
	siteURL, err := validateManagedPages(site)
	if err != nil {
		return nil, err
	}
	head, err := p.store.Branch(ctx, pagesBranch)
	if err != nil {
		return nil, fmt.Errorf("get Pages source branch: %w", err)
	}
	snapshot, err := p.store.Snapshot(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("read Pages source at %s: %w", head, err)
	}
	if _, err := validatePagesSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("validate managed Pages source: %w", err)
	}
	return &PagesTarget{repositoryID: repo.GetID(), url: siteURL, initialized: initialized}, nil
}

func (p *PagesPublisher) initialize(ctx context.Context, repositoryID int64) (*github.Pages, error) {
	_, err := p.store.Branch(ctx, pagesBranch)
	created := false
	switch {
	case err == nil:
		// Reuse the branch only if it is a managed history, for example after an interrupted setup.
	case errors.Is(err, errPagesBranchNotFound):
		files, err := pagesBootstrapFiles()
		if err != nil {
			return nil, fmt.Errorf("prepare Pages bootstrap: %w", err)
		}
		if err := p.store.CreateCleanBranch(ctx, pagesBranch, files); err != nil {
			return nil, fmt.Errorf("create isolated Pages branch; branch %q may remain and should be inspected before retrying: %w", pagesBranch, err)
		}
		created = true
	default:
		return nil, fmt.Errorf("check Pages source branch: %w", err)
	}
	head, err := p.store.Branch(ctx, pagesBranch)
	if err != nil {
		return nil, fmt.Errorf("verify Pages branch %q: %w", pagesBranch, err)
	}
	snapshot, err := p.store.Snapshot(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("read Pages branch %q: %w", pagesBranch, err)
	}
	manifest, err := validatePagesSnapshot(snapshot)
	if err == nil && created && (len(manifest.Reports) != 0 || manifest.IndexSHA != gitBlobSHA([]byte(pagesPlaceholder))) {
		err = fmt.Errorf("the initial Pages tree is not the expected placeholder")
	}
	if err != nil {
		if created {
			return nil, fmt.Errorf("verify the initial Pages tree; branch %q remains: %w", pagesBranch, err)
		}
		return nil, fmt.Errorf("existing branch %q is not a managed Pages history; refusing to use it: %w", pagesBranch, err)
	}
	if err := p.store.CreateWorkflowSite(ctx); err != nil {
		return nil, fmt.Errorf("create Pages site; branch %q remains and the site may have been created: %w", pagesBranch, err)
	}
	if err := p.store.SetPrivate(ctx); err != nil {
		return nil, fmt.Errorf("set Pages private; the new site may be public and branch %q remains, so inspect the site before retrying: %w", pagesBranch, err)
	}
	site, err := p.store.Site(ctx)
	if err != nil {
		return nil, fmt.Errorf("verify private Pages after initialization; the site and branch %q remain: %w", pagesBranch, err)
	}
	if err := validatePrivatePages(site); err != nil {
		return nil, fmt.Errorf("verify private Pages after initialization; the site and branch %q remain: %w", pagesBranch, err)
	}
	// GitHub reports the default branch as the source of workflow sites, so only the build type is checked.
	if site.BuildType == nil || site.GetBuildType() != "workflow" {
		return nil, fmt.Errorf("new Pages site changed unexpectedly; inspect the site and branch %q before retrying", pagesBranch)
	}
	repo, err := p.store.Repository(ctx)
	if err != nil {
		return nil, fmt.Errorf("recheck repository before configuring Pages source: %w", err)
	}
	if err := validatePagesRepository(repo); err != nil {
		return nil, fmt.Errorf("recheck repository before configuring Pages source: %w", err)
	}
	if repo.GetID() != repositoryID {
		return nil, fmt.Errorf("destination repository changed during Pages initialization")
	}
	if err := p.store.SetBranchSource(ctx, pagesBranch); err != nil {
		return nil, fmt.Errorf("configure the private Pages source; the site and branch %q remain: %w", pagesBranch, err)
	}
	site, err = p.store.Site(ctx)
	if err != nil {
		return nil, fmt.Errorf("verify configured Pages source; the site and branch %q remain: %w", pagesBranch, err)
	}
	if _, err := validateManagedPages(site); err != nil {
		return nil, fmt.Errorf("verify configured Pages source; the site and branch %q remain: %w", pagesBranch, err)
	}
	return site, nil
}

func (p *PagesPublisher) recheckTarget(ctx context.Context, target *PagesTarget) error {
	repo, err := p.store.Repository(ctx)
	if err != nil {
		return fmt.Errorf("recheck Pages destination repository: %w", err)
	}
	if err := validatePagesRepository(repo); err != nil {
		return err
	}
	if repo.GetID() != target.repositoryID {
		return fmt.Errorf("pages destination repository changed after preparation")
	}
	site, err := p.store.Site(ctx)
	if err != nil {
		return fmt.Errorf("recheck Pages site: %w", err)
	}
	siteURL, err := validateManagedPages(site)
	if err != nil {
		return err
	}
	if siteURL != target.url {
		return fmt.Errorf("pages site URL changed after preparation")
	}
	return nil
}

// Publish updates the report history and its index in a single source-branch commit.
func (p *PagesPublisher) Publish(ctx context.Context, target *PagesTarget, source gh.CopilotMetricsSource, summary *Summary, report string) (*PagesResult, error) {
	if target == nil || summary == nil || report == "" {
		return nil, fmt.Errorf("pages target, summary, and HTML report are required")
	}
	if err := p.recheckTarget(ctx, target); err != nil {
		return nil, err
	}
	head, err := p.store.Branch(ctx, pagesBranch)
	if err != nil {
		return nil, fmt.Errorf("get Pages source branch: %w", err)
	}
	snapshot, err := p.store.Snapshot(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("read Pages source at %s: %w", head, err)
	}
	manifest, err := validatePagesSnapshot(snapshot)
	if err != nil {
		return nil, fmt.Errorf("validate managed Pages source: %w", err)
	}
	files, err := buildPagesUpdate(manifest, source, summary, report)
	if err != nil {
		return nil, fmt.Errorf("build Pages history: %w", err)
	}
	if err := p.recheckTarget(ctx, target); err != nil {
		return nil, err
	}
	commit, err := p.store.Commit(ctx, pagesBranch, head, files)
	if err != nil {
		return nil, fmt.Errorf("update Pages source; the commit outcome may be unknown, so inspect the branch or retry: %w", err)
	}
	if commit == "" {
		return nil, fmt.Errorf("pages source update returned no commit ID; inspect the branch before retrying")
	}
	return &PagesResult{URL: target.url, Branch: pagesBranch, CommitSHA: commit}, nil
}

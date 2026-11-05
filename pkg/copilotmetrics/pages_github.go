package copilotmetrics

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

type githubPagesStore struct {
	client *gh.GitHubClient
	repo   repository.Repository
}

func stringPointer(value string) *string {
	return &value
}

// NewGitHubPagesPublisher returns a publisher for the destination repository.
func NewGitHubPagesPublisher(client *gh.GitHubClient, repo repository.Repository) *PagesPublisher {
	return &PagesPublisher{store: &githubPagesStore{client: client, repo: repo}}
}

func (s *githubPagesStore) Repository(ctx context.Context) (*github.Repository, error) {
	return gh.GetRepository(ctx, s.client, s.repo)
}

func (s *githubPagesStore) Site(ctx context.Context) (*github.Pages, error) {
	site, err := gh.GetPagesSite(ctx, s.client, s.repo)
	if gh.IsHTTPNotFound(err) {
		return nil, errPagesSiteNotFound
	}
	return site, err
}

func (s *githubPagesStore) Branch(ctx context.Context, branch string) (string, error) {
	result, err := gh.GetBranch(ctx, s.client, s.repo, branch)
	if gh.IsHTTPNotFound(err) {
		return "", errPagesBranchNotFound
	}
	if err != nil {
		return "", err
	}
	if result == nil || result.Commit == nil || result.Commit.GetSHA() == "" {
		return "", fmt.Errorf("branch %q has no head commit", branch)
	}
	return result.Commit.GetSHA(), nil
}

func (s *githubPagesStore) Snapshot(ctx context.Context, head string) (*pagesSnapshot, error) {
	commit, err := gh.GetGitCommit(ctx, s.client, s.repo, head)
	if err != nil {
		return nil, fmt.Errorf("get Git commit: %w", err)
	}
	if commit == nil || commit.Tree == nil || commit.Tree.GetSHA() == "" {
		return nil, fmt.Errorf("git commit has no tree")
	}
	tree, err := gh.GetGitTreeRecursive(ctx, s.client, s.repo, commit.Tree.GetSHA())
	if err != nil {
		return nil, fmt.Errorf("get Git tree: %w", err)
	}
	if tree == nil || tree.Truncated == nil || tree.GetTruncated() {
		return nil, fmt.Errorf("git tree is incomplete")
	}
	snapshot := &pagesSnapshot{entries: make(map[string]pagesTreeEntry, len(tree.Entries))}
	for _, entry := range tree.Entries {
		if entry == nil || entry.Path == nil || entry.GetPath() == "" {
			return nil, fmt.Errorf("git tree contains an entry without a path")
		}
		path := entry.GetPath()
		if _, exists := snapshot.entries[path]; exists {
			return nil, fmt.Errorf("git tree contains duplicate path %q", path)
		}
		snapshot.entries[path] = pagesTreeEntry{Type: entry.GetType(), Mode: entry.GetMode(), SHA: entry.GetSHA()}
	}
	entry, ok := snapshot.entries[pagesManifestFile]
	if !ok || entry.Type != "blob" || !validGitSHA(entry.SHA) {
		return nil, fmt.Errorf("managed Pages manifest is missing")
	}
	blob, err := gh.GetGitBlob(ctx, s.client, s.repo, entry.SHA)
	if err != nil {
		return nil, fmt.Errorf("get Pages manifest blob: %w", err)
	}
	if blob == nil || blob.Encoding == nil || blob.GetEncoding() != "base64" || blob.GetSHA() != entry.SHA {
		return nil, fmt.Errorf("pages manifest blob metadata is invalid")
	}
	snapshot.manifest, err = base64.StdEncoding.DecodeString(blob.GetContent())
	if err != nil {
		return nil, fmt.Errorf("decode Pages manifest blob: %w", err)
	}
	if gitBlobSHA(snapshot.manifest) != entry.SHA {
		return nil, fmt.Errorf("pages manifest blob does not match the git tree")
	}
	return snapshot, nil
}

func (s *githubPagesStore) CreateCleanBranch(ctx context.Context, branch string, files map[string][]byte) error {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]*github.TreeEntry, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, &github.TreeEntry{
			Path: stringPointer(path), Mode: stringPointer("100644"), Type: stringPointer("blob"), Content: stringPointer(string(files[path])),
		})
	}
	tree, err := gh.CreateGitTree(ctx, s.client, s.repo, "", entries)
	if err != nil {
		return fmt.Errorf("create isolated Git tree: %w", err)
	}
	if tree == nil || tree.GetSHA() == "" {
		return fmt.Errorf("created Git tree has no SHA")
	}
	commit, err := gh.CreateGitCommit(ctx, s.client, s.repo, github.Commit{
		Message: stringPointer("Initialize Copilot metrics Pages"),
		Tree:    &github.Tree{SHA: tree.SHA},
	})
	if err != nil {
		return fmt.Errorf("create root commit: %w", err)
	}
	if commit == nil || commit.GetSHA() == "" {
		return fmt.Errorf("created root commit has no SHA")
	}
	if _, err := gh.CreateBranch(ctx, s.client, s.repo, branch, commit.GetSHA()); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}
	return nil
}

func (s *githubPagesStore) CreateWorkflowSite(ctx context.Context) error {
	_, err := gh.CreatePagesSite(ctx, s.client, s.repo, &github.Pages{BuildType: stringPointer("workflow")})
	return err
}

func (s *githubPagesStore) SetPrivate(ctx context.Context) error {
	private := false
	return gh.UpdatePagesSite(ctx, s.client, s.repo, &github.PagesUpdateWithoutCNAME{Public: &private})
}

func (s *githubPagesStore) SetBranchSource(ctx context.Context, branch string) error {
	return gh.UpdatePagesSite(ctx, s.client, s.repo, &github.PagesUpdateWithoutCNAME{
		BuildType: stringPointer("legacy"),
		Source:    &github.PagesSource{Branch: stringPointer(branch), Path: stringPointer("/")},
	})
}

func (s *githubPagesStore) Commit(ctx context.Context, branch, head string, files map[string][]byte) (string, error) {
	return gh.CreateCommitOnBranch(ctx, s.client, s.repo, branch, head, "Publish Copilot metrics report", files)
}

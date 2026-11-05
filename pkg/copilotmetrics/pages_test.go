package copilotmetrics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePagesStore struct {
	repo               *github.Repository
	site               *github.Pages
	branches           map[string]string
	files              map[string][]byte
	events             []string
	fail               string
	siteCalls          int
	publicOnSiteCall   int
	siteErrorOnCall    int
	unknownAfterSet    bool
	addUnexpectedFile  bool
	timeoutAfterCommit bool
	changeHeadOnCommit bool
	commits            int
}

func pagesTestBoolPointer(value bool) *bool {
	return &value
}

func pagesTestInt64Pointer(value int64) *int64 {
	return &value
}

func newFakePagesStore() *fakePagesStore {
	return &fakePagesStore{
		repo: &github.Repository{
			ID:            pagesTestInt64Pointer(1),
			Owner:         &github.User{Login: stringPointer("example-org"), Type: stringPointer("Organization")},
			Name:          stringPointer("example-reports"),
			Visibility:    stringPointer("private"),
			HasPages:      pagesTestBoolPointer(false),
			DefaultBranch: stringPointer("main"),
		},
		branches: map[string]string{"main": strings.Repeat("1", 40)},
		files:    map[string][]byte{},
	}
}

func (s *fakePagesStore) event(name string) error {
	s.events = append(s.events, name)
	if s.fail == name {
		return errors.New("simulated failure")
	}
	return nil
}

func (s *fakePagesStore) Repository(context.Context) (*github.Repository, error) {
	if err := s.event("repository"); err != nil {
		return nil, err
	}
	return s.repo, nil
}

func (s *fakePagesStore) Site(context.Context) (*github.Pages, error) {
	if err := s.event("site"); err != nil {
		return nil, err
	}
	s.siteCalls++
	if s.siteCalls == s.siteErrorOnCall {
		return nil, errors.New("simulated site lookup failure")
	}
	if s.site == nil {
		return nil, errPagesSiteNotFound
	}
	if s.siteCalls == s.publicOnSiteCall {
		s.site.Public = pagesTestBoolPointer(true)
	}
	return s.site, nil
}

func (s *fakePagesStore) Branch(_ context.Context, branch string) (string, error) {
	if err := s.event("branch:" + branch); err != nil {
		return "", err
	}
	head, ok := s.branches[branch]
	if !ok {
		return "", errPagesBranchNotFound
	}
	return head, nil
}

func snapshotFromFiles(files map[string][]byte) *pagesSnapshot {
	snapshot := &pagesSnapshot{entries: map[string]pagesTreeEntry{}}
	for path, body := range files {
		if strings.HasPrefix(path, "reports/") {
			snapshot.entries["reports"] = pagesTreeEntry{Type: "tree", Mode: "040000", SHA: strings.Repeat("a", 40)}
		}
		snapshot.entries[path] = pagesTreeEntry{Type: "blob", Mode: "100644", SHA: gitBlobSHA(body)}
		if path == pagesManifestFile {
			snapshot.manifest = append([]byte(nil), body...)
		}
	}
	return snapshot
}

func (s *fakePagesStore) Snapshot(_ context.Context, head string) (*pagesSnapshot, error) {
	if err := s.event("snapshot"); err != nil {
		return nil, err
	}
	if head != s.branches[pagesBranch] {
		return nil, errors.New("unknown snapshot")
	}
	return snapshotFromFiles(s.files), nil
}

func (s *fakePagesStore) CreateCleanBranch(_ context.Context, branch string, files map[string][]byte) error {
	if err := s.event("create-branch"); err != nil {
		return err
	}
	s.branches[branch] = strings.Repeat("2", 40)
	for path, body := range files {
		s.files[path] = append([]byte(nil), body...)
	}
	if s.addUnexpectedFile {
		s.files["unexpected.txt"] = []byte("fictional private data")
	}
	return nil
}

func (s *fakePagesStore) CreateWorkflowSite(context.Context) error {
	if err := s.event("create-site"); err != nil {
		return err
	}
	s.site = &github.Pages{
		Public:    pagesTestBoolPointer(true),
		BuildType: stringPointer("workflow"),
		HTMLURL:   stringPointer("https://pages.example.test"),
	}
	s.repo.HasPages = pagesTestBoolPointer(true)
	return nil
}

func (s *fakePagesStore) SetPrivate(context.Context) error {
	if err := s.event("set-private"); err != nil {
		return err
	}
	if s.unknownAfterSet {
		s.site.Public = nil
	} else {
		s.site.Public = pagesTestBoolPointer(false)
	}
	return nil
}

func (s *fakePagesStore) SetBranchSource(_ context.Context, branch string) error {
	if err := s.event("set-source"); err != nil {
		return err
	}
	if s.site.Public == nil || *s.site.Public {
		return errors.New("site is not private")
	}
	s.site.BuildType = stringPointer("legacy")
	s.site.Source = &github.PagesSource{Branch: stringPointer(branch), Path: stringPointer("/")}
	return nil
}

func (s *fakePagesStore) Commit(_ context.Context, branch, expected string, files map[string][]byte) (string, error) {
	if err := s.event("commit"); err != nil {
		return "", err
	}
	if s.site.Public == nil || *s.site.Public {
		return "", errors.New("site is not private")
	}
	if s.changeHeadOnCommit {
		s.branches[branch] = strings.Repeat("3", 40)
	}
	if s.branches[branch] != expected {
		return "", errors.New("branch changed")
	}
	for path, body := range files {
		s.files[path] = append([]byte(nil), body...)
	}
	s.commits++
	s.branches[branch] = fmt.Sprintf("%040x", s.commits+10)
	if s.timeoutAfterCommit {
		s.timeoutAfterCommit = false
		return "", errors.New("simulated timeout")
	}
	return s.branches[branch], nil
}

func configuredPagesStore(t *testing.T) *fakePagesStore {
	t.Helper()
	store := newFakePagesStore()
	files, err := pagesBootstrapFiles()
	require.NoError(t, err)
	store.files = files
	store.branches[pagesBranch] = strings.Repeat("2", 40)
	store.repo.HasPages = pagesTestBoolPointer(true)
	store.site = &github.Pages{
		Public:    pagesTestBoolPointer(false),
		BuildType: stringPointer("legacy"),
		Source:    &github.PagesSource{Branch: stringPointer(pagesBranch), Path: stringPointer("/")},
		HTMLURL:   stringPointer("https://pages.example.test"),
	}
	return store
}

func pagesTestSource() gh.CopilotMetricsSource {
	return gh.CopilotMetricsSource{Owner: repository.Repository{Host: "github.com", Owner: "example-org"}}
}

func pagesTestSummary(start, end string) *Summary {
	return &Summary{StartDay: start, EndDay: end, GeneratedAt: time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)}
}

func TestPagesInitializeAndPublishHistory(t *testing.T) {
	store := newFakePagesStore()
	publisher := &PagesPublisher{store: store}
	target, err := publisher.Prepare(t.Context())
	require.NoError(t, err)
	assert.True(t, target.Initialized())
	assert.Equal(t, "https://pages.example.test/", target.URL())
	assert.Less(t, indexEvent(store.events, "create-branch"), indexEvent(store.events, "create-site"))
	assert.Less(t, indexEvent(store.events, "create-site"), indexEvent(store.events, "set-private"))
	assert.Less(t, indexEvent(store.events, "set-private"), indexEvent(store.events, "set-source"))
	assert.NotContains(t, string(store.files[pagesIndexFile]), "fictional metric")

	first := pagesTestSummary("2030-01-01", "2030-01-28")
	result, err := publisher.Publish(t.Context(), target, pagesTestSource(), first, "<html>fictional metric one</html>")
	require.NoError(t, err)
	assert.Equal(t, "https://pages.example.test/", result.URL)
	assert.Equal(t, pagesBranch, result.Branch)
	assert.NotEmpty(t, result.CommitSHA)

	second := pagesTestSummary("2030-01-02", "2030-01-29")
	_, err = publisher.Publish(t.Context(), target, pagesTestSource(), second, "<html>fictional metric two</html>")
	require.NoError(t, err)
	_, err = publisher.Publish(t.Context(), target, pagesTestSource(), first, "<html>updated fictional metric</html>")
	require.NoError(t, err)
	manifest, err := validatePagesSnapshot(snapshotFromFiles(store.files))
	require.NoError(t, err)
	require.Len(t, manifest.Reports, 2)
	assert.Equal(t, "2030-01-29", manifest.Reports[0].EndDay)
	assert.Equal(t, "2030-01-28", manifest.Reports[1].EndDay)
	assert.Contains(t, string(store.files[manifest.Reports[1].path()]), "updated fictional metric")
	assert.Contains(t, string(store.files[pagesIndexFile]), manifest.Reports[0].path())
	assert.Contains(t, string(store.files[pagesIndexFile]), manifest.Reports[1].path())
}

func indexEvent(events []string, name string) int {
	for i, event := range events {
		if event == name {
			return i
		}
	}
	return -1
}

func TestPagesPrepareRejectsUnsafeTargets(t *testing.T) {
	tests := []struct {
		name   string
		change func(*fakePagesStore)
	}{
		{"public repository", func(s *fakePagesStore) { s.repo.Visibility = stringPointer("public") }},
		{"unknown repository visibility", func(s *fakePagesStore) { s.repo.Visibility = nil }},
		{"personal repository", func(s *fakePagesStore) { s.repo.Owner.Type = stringPointer("User") }},
		{"organization site", func(s *fakePagesStore) { s.repo.Name = stringPointer("example-org.github.io") }},
		{"empty repository", func(s *fakePagesStore) { delete(s.branches, "main") }},
		{"public site", func(s *fakePagesStore) { s.site.Public = pagesTestBoolPointer(true) }},
		{"unknown site visibility", func(s *fakePagesStore) { s.site.Public = nil }},
		{"workflow site", func(s *fakePagesStore) { s.site.BuildType = stringPointer("workflow") }},
		{"other branch", func(s *fakePagesStore) { s.site.Source.Branch = stringPointer("other") }},
		{"other source path", func(s *fakePagesStore) { s.site.Source.Path = stringPointer("/docs") }},
		{"insecure URL", func(s *fakePagesStore) { s.site.HTMLURL = stringPointer("http://pages.example.test") }},
		{"unmanaged files", func(s *fakePagesStore) { s.files["unrelated.txt"] = []byte("unrelated") }},
		{"modified index", func(s *fakePagesStore) { s.files[pagesIndexFile] = []byte("unrelated") }},
		{"missing manifest", func(s *fakePagesStore) { delete(s.files, pagesManifestFile) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := configuredPagesStore(t)
			tt.change(store)
			_, err := (&PagesPublisher{store: store}).Prepare(t.Context())
			require.Error(t, err)
			for _, event := range []string{"create-branch", "create-site", "set-private", "set-source", "commit"} {
				assert.NotContains(t, store.events, event)
			}
		})
	}
}

func TestPagesInitializeFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		change func(*fakePagesStore)
	}{
		{"branch creation fails", func(s *fakePagesStore) { s.fail = "create-branch" }},
		{"site creation fails", func(s *fakePagesStore) { s.fail = "create-site" }},
		{"site permission denied", func(s *fakePagesStore) { s.siteErrorOnCall = 1 }},
		{"default branch permission denied", func(s *fakePagesStore) { s.fail = "branch:main" }},
		{"privacy update fails", func(s *fakePagesStore) { s.fail = "set-private" }},
		{"privacy lookup fails", func(s *fakePagesStore) { s.siteErrorOnCall = 2 }},
		{"privacy unknown", func(s *fakePagesStore) { s.unknownAfterSet = true }},
		{"privacy changed after setting source", func(s *fakePagesStore) { s.publicOnSiteCall = 3 }},
		{"source update fails", func(s *fakePagesStore) { s.fail = "set-source" }},
		{"unexpected bootstrap file", func(s *fakePagesStore) { s.addUnexpectedFile = true }},
		{"unmanaged branch already exists", func(s *fakePagesStore) { s.branches[pagesBranch] = strings.Repeat("3", 40) }},
		{"site absence unconfirmed", func(s *fakePagesStore) { s.repo.HasPages = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakePagesStore()
			tt.change(store)
			_, err := (&PagesPublisher{store: store}).Prepare(t.Context())
			require.Error(t, err)
			assert.NotContains(t, store.events, "commit")
			for path, body := range store.files {
				assert.False(t, strings.HasPrefix(path, "reports/"))
				assert.NotContains(t, string(body), "fictional metric")
			}
			if store.unknownAfterSet || store.fail == "set-private" || store.siteErrorOnCall == 2 {
				assert.NotContains(t, store.events, "set-source")
			}
			if store.addUnexpectedFile || tt.name == "unmanaged branch already exists" {
				assert.NotContains(t, store.events, "create-site")
			}
		})
	}
}

func TestPagesInitializeReusesManagedBranch(t *testing.T) {
	store := configuredPagesStore(t)
	publisher := &PagesPublisher{store: store}
	target, err := publisher.Prepare(t.Context())
	require.NoError(t, err)
	_, err = publisher.Publish(t.Context(), target, pagesTestSource(), pagesTestSummary("2030-01-01", "2030-01-28"), "<html>fictional metric</html>")
	require.NoError(t, err)

	// Simulate an interrupted setup: the managed branch remains but no site exists.
	store.site = nil
	store.repo.HasPages = pagesTestBoolPointer(false)
	store.events = nil
	store.siteCalls = 0
	target, err = publisher.Prepare(t.Context())
	require.NoError(t, err)
	assert.True(t, target.Initialized())
	assert.NotContains(t, store.events, "create-branch")
	assert.Less(t, indexEvent(store.events, "set-private"), indexEvent(store.events, "set-source"))
	manifest, err := validatePagesSnapshot(snapshotFromFiles(store.files))
	require.NoError(t, err)
	assert.Len(t, manifest.Reports, 1)
}

func TestPagesPrepareAcceptsPrivateAndInternal(t *testing.T) {
	for _, visibility := range []string{"private", "internal"} {
		t.Run(visibility, func(t *testing.T) {
			store := configuredPagesStore(t)
			store.repo.Visibility = stringPointer(visibility)
			target, err := (&PagesPublisher{store: store}).Prepare(t.Context())
			require.NoError(t, err)
			assert.False(t, target.Initialized())
			assert.NotContains(t, store.events, "create-site")
		})
	}
}

func TestPagesPublishRejectsChangedTarget(t *testing.T) {
	tests := []struct {
		name   string
		change func(*fakePagesStore)
	}{
		{"repository identity", func(s *fakePagesStore) { s.repo.ID = pagesTestInt64Pointer(2) }},
		{"repository visibility", func(s *fakePagesStore) { s.repo.Visibility = stringPointer("public") }},
		{"site privacy", func(s *fakePagesStore) { s.site.Public = nil }},
		{"site URL", func(s *fakePagesStore) { s.site.HTMLURL = stringPointer("https://other.example.test") }},
		{"site source", func(s *fakePagesStore) { s.site.Source.Branch = stringPointer("other") }},
		{"source contents", func(s *fakePagesStore) { s.files["unexpected.txt"] = []byte("fictional") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := configuredPagesStore(t)
			publisher := &PagesPublisher{store: store}
			target, err := publisher.Prepare(t.Context())
			require.NoError(t, err)
			tt.change(store)
			_, err = publisher.Publish(t.Context(), target, pagesTestSource(), pagesTestSummary("2030-01-01", "2030-01-28"), "<html>fictional</html>")
			require.Error(t, err)
			assert.NotContains(t, store.events, "commit")
		})
	}
}

func TestPagesPublishRechecksPrivacyAndHandlesRetry(t *testing.T) {
	t.Run("privacy changes before commit", func(t *testing.T) {
		store := configuredPagesStore(t)
		publisher := &PagesPublisher{store: store}
		target, err := publisher.Prepare(t.Context())
		require.NoError(t, err)
		store.publicOnSiteCall = 3
		_, err = publisher.Publish(t.Context(), target, pagesTestSource(), pagesTestSummary("2030-01-01", "2030-01-28"), "<html>fictional</html>")
		require.ErrorContains(t, err, "public")
		assert.NotContains(t, store.events, "commit")
	})
	t.Run("commit failure", func(t *testing.T) {
		store := configuredPagesStore(t)
		publisher := &PagesPublisher{store: store}
		target, err := publisher.Prepare(t.Context())
		require.NoError(t, err)
		store.fail = "commit"
		_, err = publisher.Publish(t.Context(), target, pagesTestSource(), pagesTestSummary("2030-01-01", "2030-01-28"), "<html>fictional</html>")
		require.ErrorContains(t, err, "outcome may be unknown")
		assert.Equal(t, 0, store.commits)
	})
	t.Run("concurrent branch update", func(t *testing.T) {
		store := configuredPagesStore(t)
		publisher := &PagesPublisher{store: store}
		target, err := publisher.Prepare(t.Context())
		require.NoError(t, err)
		store.changeHeadOnCommit = true
		_, err = publisher.Publish(t.Context(), target, pagesTestSource(), pagesTestSummary("2030-01-01", "2030-01-28"), "<html>fictional</html>")
		require.ErrorContains(t, err, "branch changed")
		assert.Equal(t, 0, store.commits)
		manifest, err := validatePagesSnapshot(snapshotFromFiles(store.files))
		require.NoError(t, err)
		assert.Empty(t, manifest.Reports)
		assert.Equal(t, strings.Repeat("3", 40), store.branches[pagesBranch])
	})
	t.Run("retry after an ambiguous commit", func(t *testing.T) {
		store := configuredPagesStore(t)
		publisher := &PagesPublisher{store: store}
		target, err := publisher.Prepare(t.Context())
		require.NoError(t, err)
		store.timeoutAfterCommit = true
		summary := pagesTestSummary("2030-01-01", "2030-01-28")
		_, err = publisher.Publish(t.Context(), target, pagesTestSource(), summary, "<html>fictional</html>")
		require.Error(t, err)
		_, err = publisher.Publish(t.Context(), target, pagesTestSource(), summary, "<html>fictional</html>")
		require.NoError(t, err)
		manifest, err := validatePagesSnapshot(snapshotFromFiles(store.files))
		require.NoError(t, err)
		assert.Len(t, manifest.Reports, 1)
	})
}

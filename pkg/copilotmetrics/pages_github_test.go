package copilotmetrics

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPagesHTTPStore(t *testing.T, server *httptest.Server) *githubPagesStore {
	t.Helper()
	base := server.URL + "/"
	api, err := github.NewClient(github.WithHTTPClient(server.Client()), github.WithURLs(&base, nil))
	require.NoError(t, err)
	shared, err := client.NewClient(api)
	require.NoError(t, err)
	return &githubPagesStore{client: shared, repo: repository.Repository{Owner: "example-org", Name: "example-reports"}}
}

func TestGitHubPagesSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		change func(*github.Tree, *github.Blob)
		err    string
	}{
		{"valid", func(*github.Tree, *github.Blob) {}, ""},
		{"truncated tree", func(tree *github.Tree, _ *github.Blob) {
			truncated := true
			tree.Truncated = &truncated
		}, "truncated"},
		{"unknown completeness", func(tree *github.Tree, _ *github.Blob) { tree.Truncated = nil }, "incomplete"},
		{"duplicate paths", func(tree *github.Tree, _ *github.Blob) { tree.Entries = append(tree.Entries, tree.Entries[0]) }, "duplicate"},
		{"missing path", func(tree *github.Tree, _ *github.Blob) { tree.Entries[0].Path = nil }, "without a path"},
		{"unsupported encoding", func(_ *github.Tree, blob *github.Blob) { blob.Encoding = stringPointer("utf-8") }, "metadata"},
		{"invalid encoding", func(_ *github.Tree, blob *github.Blob) { blob.Content = "not-base64!" }, "decode"},
		{"changed content", func(_ *github.Tree, blob *github.Blob) {
			blob.Content = base64.StdEncoding.EncodeToString([]byte("fictional"))
		}, "does not match"},
		{"changed blob identity", func(_ *github.Tree, blob *github.Blob) { blob.SHA = stringPointer(strings.Repeat("3", 40)) }, "metadata"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := pagesBootstrapFiles()
			require.NoError(t, err)
			complete := false
			tree := &github.Tree{Truncated: &complete}
			for path, content := range files {
				tree.Entries = append(tree.Entries, &github.TreeEntry{
					Path: stringPointer(path), Mode: stringPointer("100644"), Type: stringPointer("blob"), SHA: stringPointer(gitBlobSHA(content)),
				})
			}
			blob := &github.Blob{
				SHA:      stringPointer(gitBlobSHA(files[pagesManifestFile])),
				Encoding: stringPointer("base64"),
				Content:  base64.StdEncoding.EncodeToString(files[pagesManifestFile]),
			}
			tt.change(tree, blob)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				var response any
				switch {
				case strings.Contains(r.URL.Path, "/git/commits/"):
					response = &github.Commit{Tree: &github.Tree{SHA: stringPointer(strings.Repeat("2", 40))}}
				case strings.Contains(r.URL.Path, "/git/trees/"):
					response = tree
				case strings.Contains(r.URL.Path, "/git/blobs/"):
					response = blob
				default:
					t.Errorf("unexpected request %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				assert.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer server.Close()
			store := newPagesHTTPStore(t, server)
			snapshot, err := store.Snapshot(t.Context(), strings.Repeat("1", 40))
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			_, err = validatePagesSnapshot(snapshot)
			require.NoError(t, err)
		})
	}
}

func TestGitHubPagesCreatesIsolatedBranch(t *testing.T) {
	treeSHA := strings.Repeat("1", 40)
	commitSHA := strings.Repeat("2", 40)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		requests = append(requests, r.URL.Path)
		var body map[string]any
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var sha string
		switch r.URL.Path {
		case "/repos/example-org/example-reports/git/trees":
			assert.NotContains(t, body, "base_tree")
			assert.Equal(t, []any{map[string]any{
				"path": "index.html", "type": "blob", "mode": "100644", "content": "harmless placeholder",
			}}, body["tree"])
			sha = treeSHA
		case "/repos/example-org/example-reports/git/commits":
			assert.NotContains(t, body, "parents")
			assert.Equal(t, treeSHA, body["tree"])
			sha = commitSHA
		case "/repos/example-org/example-reports/git/refs":
			assert.Equal(t, "refs/heads/"+pagesBranch, body["ref"])
			assert.Equal(t, commitSHA, body["sha"])
			sha = commitSHA
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]string{"sha": sha}))
	}))
	defer server.Close()
	err := newPagesHTTPStore(t, server).CreateCleanBranch(t.Context(), pagesBranch, map[string][]byte{"index.html": []byte("harmless placeholder")})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"/repos/example-org/example-reports/git/trees",
		"/repos/example-org/example-reports/git/commits",
		"/repos/example-org/example-reports/git/refs",
	}, requests)
}

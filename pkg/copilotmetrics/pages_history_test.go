package copilotmetrics

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagesHistoryIdentityAndEscaping(t *testing.T) {
	bootstrap, err := pagesBootstrapFiles()
	require.NoError(t, err)
	manifest, err := validatePagesSnapshot(snapshotFromFiles(bootstrap))
	require.NoError(t, err)
	summary := pagesTestSummary("2030-01-01", "2030-01-28")
	source := gh.CopilotMetricsSource{Owner: repository.Repository{Host: "github.example.test", Owner: "example<script>"}}
	files, err := buildPagesUpdate(manifest, source, summary, "<html>fictional</html>")
	require.NoError(t, err)
	assert.Contains(t, string(files[pagesIndexFile]), "example&lt;script&gt;")
	assert.NotContains(t, string(files[pagesIndexFile]), "example<script>")

	var first pagesManifest
	require.NoError(t, json.Unmarshal(files[pagesManifestFile], &first))
	require.Len(t, first.Reports, 1)
	firstPath := first.Reports[0].path()
	source.Owner.Owner = "EXAMPLE<script>"
	same, err := buildPagesUpdate(&first, source, summary, "<html>updated</html>")
	require.NoError(t, err)
	var updated pagesManifest
	require.NoError(t, json.Unmarshal(same[pagesManifestFile], &updated))
	require.Len(t, updated.Reports, 1)
	assert.Equal(t, firstPath, updated.Reports[0].path())
	source.Enterprise = true
	different, err := buildPagesUpdate(&updated, source, summary, "<html>enterprise</html>")
	require.NoError(t, err)
	var withEnterprise pagesManifest
	require.NoError(t, json.Unmarshal(different[pagesManifestFile], &withEnterprise))
	assert.Len(t, withEnterprise.Reports, 2)
	source.Enterprise = false
	source.Owner.Host = "other.example.test"
	different, err = buildPagesUpdate(&updated, source, summary, "<html>other host</html>")
	require.NoError(t, err)
	var withHost pagesManifest
	require.NoError(t, json.Unmarshal(different[pagesManifestFile], &withHost))
	assert.Len(t, withHost.Reports, 2)
}

func TestPagesSnapshotRejectsTampering(t *testing.T) {
	bootstrap, err := pagesBootstrapFiles()
	require.NoError(t, err)
	tests := []struct {
		name   string
		change func(*pagesSnapshot)
	}{
		{"unknown file", func(s *pagesSnapshot) {
			s.entries["other"] = pagesTreeEntry{Type: "blob", Mode: "100644", SHA: strings.Repeat("a", 40)}
		}},
		{"symlink index", func(s *pagesSnapshot) {
			entry := s.entries[pagesIndexFile]
			entry.Mode = "120000"
			s.entries[pagesIndexFile] = entry
		}},
		{"changed manifest", func(s *pagesSnapshot) { s.manifest = append(s.manifest, 'x') }},
		{"changed nojekyll", func(s *pagesSnapshot) {
			entry := s.entries[".nojekyll"]
			entry.SHA = strings.Repeat("b", 40)
			s.entries[".nojekyll"] = entry
		}},
		{"missing index", func(s *pagesSnapshot) { delete(s.entries, pagesIndexFile) }},
		{"extra report", func(s *pagesSnapshot) {
			s.entries["reports"] = pagesTreeEntry{Type: "tree", Mode: "040000", SHA: strings.Repeat("a", 40)}
			s.entries["reports/other.html"] = pagesTreeEntry{Type: "blob", Mode: "100644", SHA: strings.Repeat("b", 40)}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := snapshotFromFiles(bootstrap)
			tt.change(snapshot)
			_, err := validatePagesSnapshot(snapshot)
			require.Error(t, err)
		})
	}
}

func TestPagesManifestRejectsInvalidHistory(t *testing.T) {
	bootstrap, err := pagesBootstrapFiles()
	require.NoError(t, err)
	manifest, err := validatePagesSnapshot(snapshotFromFiles(bootstrap))
	require.NoError(t, err)
	update, err := buildPagesUpdate(manifest, pagesTestSource(), pagesTestSummary("2030-01-01", "2030-01-28"), "<html>fictional</html>")
	require.NoError(t, err)
	for path, content := range update {
		bootstrap[path] = content
	}
	tests := []struct {
		name   string
		change func(*pagesManifest)
		err    string
	}{
		{"unknown manager", func(m *pagesManifest) { m.Manager = "other" }, "not a supported"},
		{"unknown version", func(m *pagesManifest) { m.Version++ }, "not a supported"},
		{"missing records", func(m *pagesManifest) { m.Reports = nil }, "not a supported"},
		{"duplicate identity", func(m *pagesManifest) { m.Reports = append(m.Reports, m.Reports[0]) }, "duplicate"},
		{"invalid host", func(m *pagesManifest) { m.Reports[0].Host = "../host" }, "source host"},
		{"invalid source", func(m *pagesManifest) { m.Reports[0].Source = "../source" }, "report source"},
		{"invalid source kind", func(m *pagesManifest) { m.Reports[0].Kind = "other" }, "source kind"},
		{"invalid start", func(m *pagesManifest) { m.Reports[0].StartDay = "invalid" }, "start day"},
		{"reversed period", func(m *pagesManifest) { m.Reports[0].EndDay = "2029-01-01" }, "end day"},
		{"invalid timestamp", func(m *pagesManifest) { m.Reports[0].GeneratedAt = "invalid" }, "generation time"},
		{"invalid SHA", func(m *pagesManifest) { m.Reports[0].SHA = "../file" }, "blob SHA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := snapshotFromFiles(bootstrap)
			var changed pagesManifest
			require.NoError(t, json.Unmarshal(snapshot.manifest, &changed))
			tt.change(&changed)
			body, err := marshalPagesManifest(changed)
			require.NoError(t, err)
			snapshot.manifest = body
			entry := snapshot.entries[pagesManifestFile]
			entry.SHA = gitBlobSHA(body)
			snapshot.entries[pagesManifestFile] = entry
			_, err = validatePagesSnapshot(snapshot)
			require.ErrorContains(t, err, tt.err)
		})
	}
}

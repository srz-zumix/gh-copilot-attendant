package copilotmetrics

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

const (
	pagesManifestFile = "metrics-manifest.json"
	pagesIndexFile    = "index.html"
	pagesManager      = "gh-copilot-attendant/metrics-pages"
	pagesPlaceholder  = "<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>Site setup</title></head><body><p>Site setup in progress.</p></body></html>\n"
)

type pagesTreeEntry struct {
	Type string
	Mode string
	SHA  string
}

type pagesSnapshot struct {
	entries  map[string]pagesTreeEntry
	manifest []byte
}

type pagesManifest struct {
	Manager  string               `json:"manager"`
	Version  int                  `json:"version"`
	IndexSHA string               `json:"index_sha"`
	Reports  []pagesHistoryRecord `json:"reports"`
}

type pagesHistoryRecord struct {
	Host        string `json:"host"`
	Source      string `json:"source"`
	Kind        string `json:"kind"`
	StartDay    string `json:"start_day"`
	EndDay      string `json:"end_day"`
	GeneratedAt string `json:"generated_at"`
	SHA         string `json:"sha"`
}

func gitBlobSHA(content []byte) string {
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(content))
	_, _ = h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

func validGitSHA(value string) bool {
	if len(value) != 40 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (r pagesHistoryRecord) path() string {
	key := strings.Join([]string{strings.ToLower(r.Host), r.Kind, strings.ToLower(r.Source), r.StartDay, r.EndDay}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return "reports/" + hex.EncodeToString(sum[:]) + ".html"
}

func (r pagesHistoryRecord) validate() error {
	if r.Host == "" || r.Host != strings.ToLower(r.Host) || strings.ContainsAny(r.Host, "/\x00\r\n\t ") {
		return fmt.Errorf("invalid report source host")
	}
	if r.Source == "" || strings.TrimSpace(r.Source) != r.Source || strings.ContainsAny(r.Source, "/\x00\r\n\t") {
		return fmt.Errorf("invalid report source")
	}
	if r.Kind != "organization" && r.Kind != "enterprise" {
		return fmt.Errorf("invalid report source kind %q", r.Kind)
	}
	start, err := time.Parse(dayLayout, r.StartDay)
	if err != nil || start.Format(dayLayout) != r.StartDay {
		return fmt.Errorf("invalid report start day %q", r.StartDay)
	}
	end, err := time.Parse(dayLayout, r.EndDay)
	if err != nil || end.Format(dayLayout) != r.EndDay || start.After(end) {
		return fmt.Errorf("invalid report end day %q", r.EndDay)
	}
	if _, err := time.Parse(time.RFC3339, r.GeneratedAt); err != nil {
		return fmt.Errorf("invalid report generation time %q", r.GeneratedAt)
	}
	if !validGitSHA(r.SHA) {
		return fmt.Errorf("invalid report blob SHA")
	}
	return nil
}

func marshalPagesManifest(manifest pagesManifest) ([]byte, error) {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func pagesBootstrapFiles() (map[string][]byte, error) {
	manifest := pagesManifest{
		Manager:  pagesManager,
		Version:  1,
		IndexSHA: gitBlobSHA([]byte(pagesPlaceholder)),
		Reports:  []pagesHistoryRecord{},
	}
	body, err := marshalPagesManifest(manifest)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		".nojekyll":       {},
		pagesIndexFile:    []byte(pagesPlaceholder),
		pagesManifestFile: body,
	}, nil
}

func validatePagesSnapshot(snapshot *pagesSnapshot) (*pagesManifest, error) {
	if snapshot == nil || snapshot.entries == nil || snapshot.manifest == nil {
		return nil, fmt.Errorf("Pages source snapshot is incomplete")
	}
	var manifest pagesManifest
	decoder := json.NewDecoder(bytes.NewReader(snapshot.manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode Pages manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("Pages manifest contains trailing data")
	}
	if manifest.Manager != pagesManager || manifest.Version != 1 || manifest.Reports == nil || !validGitSHA(manifest.IndexSHA) {
		return nil, fmt.Errorf("Pages manifest is not a supported managed history")
	}
	expected := map[string]string{
		".nojekyll":       gitBlobSHA(nil),
		pagesIndexFile:    manifest.IndexSHA,
		pagesManifestFile: gitBlobSHA(snapshot.manifest),
	}
	for _, report := range manifest.Reports {
		if err := report.validate(); err != nil {
			return nil, fmt.Errorf("invalid Pages history entry: %w", err)
		}
		path := report.path()
		if _, exists := expected[path]; exists {
			return nil, fmt.Errorf("duplicate Pages history entry for %s", path)
		}
		expected[path] = report.SHA
	}
	for path, entry := range snapshot.entries {
		if path == "reports" {
			if entry.Type != "tree" || entry.Mode != "040000" || !validGitSHA(entry.SHA) || len(manifest.Reports) == 0 {
				return nil, fmt.Errorf("unexpected Pages reports directory")
			}
			continue
		}
		sha, exists := expected[path]
		if !exists || entry.Type != "blob" || entry.Mode != "100644" || entry.SHA != sha {
			return nil, fmt.Errorf("unexpected or modified Pages source file %q", path)
		}
		delete(expected, path)
	}
	if len(expected) != 0 {
		return nil, fmt.Errorf("Pages source is missing managed files")
	}
	if len(manifest.Reports) > 0 {
		if _, exists := snapshot.entries["reports"]; !exists {
			return nil, fmt.Errorf("Pages reports directory is missing")
		}
	}
	return &manifest, nil
}

type pagesIndexRow struct {
	pagesHistoryRecord
	Path string
}

var pagesIndexTemplate = template.Must(template.New("pages-index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Copilot usage metrics reports</title>
<style>
body { font-family: system-ui, sans-serif; margin: 2rem auto; max-width: 72rem; padding: 0 1rem; }
table { border-collapse: collapse; width: 100%; }
th, td { border-bottom: 1px solid #d0d7de; padding: .6rem; text-align: left; }
th { background: #f6f8fa; }
</style>
</head>
<body>
<h1>Copilot usage metrics reports</h1>
{{if .}}<table><thead><tr><th>Source</th><th>Period</th><th>Generated (UTC)</th><th>Report</th></tr></thead>
<tbody>{{range .}}<tr><td>{{.Host}}/{{.Source}} ({{.Kind}})</td><td>{{.StartDay}} &ndash; {{.EndDay}}</td><td>{{.GeneratedAt}}</td><td><a href="{{.Path}}">Open report</a></td></tr>
{{end}}</tbody></table>{{else}}<p>No reports have been published.</p>{{end}}
</body>
</html>
`))

func renderPagesIndex(reports []pagesHistoryRecord) ([]byte, error) {
	rows := make([]pagesIndexRow, 0, len(reports))
	for _, report := range reports {
		rows = append(rows, pagesIndexRow{pagesHistoryRecord: report, Path: report.path()})
	}
	var out bytes.Buffer
	if err := pagesIndexTemplate.Execute(&out, rows); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func buildPagesUpdate(manifest *pagesManifest, source gh.CopilotMetricsSource, summary *Summary, report string) (map[string][]byte, error) {
	if manifest == nil || summary == nil || summary.GeneratedAt.IsZero() {
		return nil, fmt.Errorf("Pages history requires a summary with a generation time")
	}
	host := strings.ToLower(source.Owner.Host)
	if host == "" {
		host = "github.com"
	}
	kind := "organization"
	if source.Enterprise {
		kind = "enterprise"
	}
	record := pagesHistoryRecord{
		Host:        host,
		Source:      source.Owner.Owner,
		Kind:        kind,
		StartDay:    summary.StartDay,
		EndDay:      summary.EndDay,
		GeneratedAt: summary.GeneratedAt.UTC().Format(time.RFC3339),
		SHA:         gitBlobSHA([]byte(report)),
	}
	if err := record.validate(); err != nil {
		return nil, err
	}
	reports := append([]pagesHistoryRecord(nil), manifest.Reports...)
	found := false
	for i := range reports {
		if reports[i].path() == record.path() {
			reports[i] = record
			found = true
			break
		}
	}
	if !found {
		reports = append(reports, record)
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].EndDay != reports[j].EndDay {
			return reports[i].EndDay > reports[j].EndDay
		}
		if reports[i].StartDay != reports[j].StartDay {
			return reports[i].StartDay > reports[j].StartDay
		}
		return reports[i].path() < reports[j].path()
	})
	index, err := renderPagesIndex(reports)
	if err != nil {
		return nil, err
	}
	updated := pagesManifest{Manager: pagesManager, Version: 1, IndexSHA: gitBlobSHA(index), Reports: reports}
	manifestBody, err := marshalPagesManifest(updated)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		record.path():     []byte(report),
		pagesIndexFile:    index,
		pagesManifestFile: manifestBody,
	}, nil
}

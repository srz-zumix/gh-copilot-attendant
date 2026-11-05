// Package copilotmetrics collects GitHub Copilot usage metrics reports for an organization or
// enterprise, aggregates them into a shareable summary, and publishes rendered reports.
package copilotmetrics

import (
	"fmt"
	"strings"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/parser"
)

// Section is a part of the summary report that can be selected independently.
type Section string

const (
	SectionKPI        Section = "kpi"
	SectionTrend      Section = "trend"
	SectionFeature    Section = "feature"
	SectionLanguage   Section = "language"
	SectionModel      Section = "model"
	SectionIDE        Section = "ide"
	SectionRepository Section = "repository"
	SectionTeam       Section = "team"
	SectionUser       Section = "user"
)

// AllSections lists every section in report order.
var AllSections = []Section{
	SectionKPI, SectionTrend, SectionFeature, SectionLanguage, SectionModel,
	SectionIDE, SectionRepository, SectionTeam, SectionUser,
}

// SectionNames returns the names of all sections, for flag help and completion.
func SectionNames() []string {
	names := make([]string, 0, len(AllSections))
	for _, s := range AllSections {
		names = append(names, string(s))
	}
	return names
}

// SectionSet is a set of selected sections.
type SectionSet map[Section]bool

// Has reports whether the section is selected.
func (s SectionSet) Has(section Section) bool {
	return s[section]
}

// Ordered returns the selected sections in report order.
func (s SectionSet) Ordered() []Section {
	var out []Section
	for _, sec := range AllSections {
		if s[sec] {
			out = append(out, sec)
		}
	}
	return out
}

// NeedsOrganizationLevel reports whether org/enterprise-level reports must be fetched.
func (s SectionSet) NeedsOrganizationLevel() bool {
	return s.Has(SectionKPI) || s.Has(SectionTrend) || s.Has(SectionFeature) ||
		s.Has(SectionLanguage) || s.Has(SectionModel) || s.Has(SectionIDE)
}

// NeedsUsers reports whether per-user reports must be fetched.
func (s SectionSet) NeedsUsers() bool {
	return s.Has(SectionTeam) || s.Has(SectionUser)
}

// NeedsRepositories reports whether repository reports must be fetched.
func (s SectionSet) NeedsRepositories() bool {
	return s.Has(SectionRepository)
}

// NeedsUserTeams reports whether user-team membership reports must be fetched.
func (s SectionSet) NeedsUserTeams() bool {
	return s.Has(SectionTeam)
}

// DefaultSections returns every section except the per-user ranking, which is opt-in.
func DefaultSections() SectionSet {
	set := SectionSet{}
	for _, s := range AllSections {
		if s != SectionUser {
			set[s] = true
		}
	}
	return set
}

// ParseSections parses section names. Values may be comma separated; "all" selects every section.
// An empty input returns DefaultSections.
func ParseSections(values []string) (SectionSet, error) {
	set := SectionSet{}
	for _, v := range values {
		for _, name := range strings.Split(v, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			if name == "all" {
				for _, s := range AllSections {
					set[s] = true
				}
				continue
			}
			valid := false
			for _, s := range AllSections {
				if string(s) == name {
					set[s] = true
					valid = true
					break
				}
			}
			if !valid {
				return nil, fmt.Errorf("unknown section %q (valid: %s, all)", name, strings.Join(SectionNames(), ", "))
			}
		}
	}
	if len(set) == 0 {
		return DefaultSections(), nil
	}
	return set, nil
}

const dayLayout = "2006-01-02"

// Period selects which reports are read: the latest 28-day report, or 1-day reports for Days.
type Period struct {
	Latest bool
	Days   []string
}

// Label returns a human-readable description of the period mode.
func (p Period) Label() string {
	if p.Latest {
		return "latest 28-day report"
	}
	return "1-day reports"
}

// ResolvePeriod resolves the --day/--since/--until flags into a Period. With no flags the latest
// 28-day report is used. --since without --until ends at the day before now. Days use the local date
// of the parsed time and the range is inclusive.
func ResolvePeriod(day, since, until string, now time.Time) (Period, error) {
	if day != "" && (since != "" || until != "") {
		return Period{}, fmt.Errorf("--day cannot be combined with --since or --until")
	}
	today := truncateDay(now)
	if day != "" {
		d, err := time.ParseInLocation(dayLayout, day, now.Location())
		if err != nil {
			return Period{}, fmt.Errorf("invalid --day %q: expected YYYY-MM-DD", day)
		}
		if !d.Before(today) {
			return Period{}, fmt.Errorf("--day %s must be before today", day)
		}
		return Period{Days: []string{d.Format(dayLayout)}}, nil
	}
	if since == "" && until == "" {
		return Period{Latest: true}, nil
	}
	if since == "" {
		return Period{}, fmt.Errorf("--until requires --since")
	}
	start, err := parser.ParseTime(since)
	if err != nil {
		return Period{}, fmt.Errorf("invalid --since: %w", err)
	}
	yesterday := today.AddDate(0, 0, -1)
	end := yesterday
	if until != "" {
		end, err = parser.ParseTime(until)
		if err != nil {
			return Period{}, fmt.Errorf("invalid --until: %w", err)
		}
	}
	start = truncateDay(start.In(now.Location()))
	end = truncateDay(end.In(now.Location()))
	if end.After(yesterday) {
		end = yesterday
	}
	if start.After(end) {
		return Period{}, fmt.Errorf("--since %s is after the last available day %s", start.Format(dayLayout), end.Format(dayLayout))
	}
	return Period{Days: DayRange(start.Format(dayLayout), end.Format(dayLayout))}, nil
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// DayRange enumerates the inclusive range of YYYY-MM-DD days. It returns nil for invalid input.
func DayRange(start, end string) []string {
	s, err := time.Parse(dayLayout, start)
	if err != nil {
		return nil
	}
	e, err := time.Parse(dayLayout, end)
	if err != nil {
		return nil
	}
	var days []string
	for d := s; !d.After(e); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format(dayLayout))
	}
	return days
}

// Format is an output format of the rendered report.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatHTML     Format = "html"
)

// ParseFormats parses output format names ("md", "markdown", "html"). Values may be comma separated.
// Duplicates are removed and the default is Markdown.
func ParseFormats(values []string) ([]Format, error) {
	var formats []Format
	seen := map[Format]bool{}
	for _, v := range values {
		for _, name := range strings.Split(v, ",") {
			var f Format
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "":
				continue
			case "md", "markdown":
				f = FormatMarkdown
			case "html":
				f = FormatHTML
			default:
				return nil, fmt.Errorf("unknown format %q (valid: md, html)", name)
			}
			if !seen[f] {
				seen[f] = true
				formats = append(formats, f)
			}
		}
	}
	if len(formats) == 0 {
		formats = []Format{FormatMarkdown}
	}
	return formats, nil
}

// PublishOptions controls issue or private Pages publication.
type PublishOptions struct {
	Repo        string
	CreateIssue bool
	Issue       int
	Title       string
	Labels      []string
	AllowPublic bool
	Pages       bool
}

// Enabled reports whether the report is published.
func (o PublishOptions) Enabled() bool {
	return o.IssueEnabled() || o.Pages
}

// IssueEnabled reports whether the report is published to an issue.
func (o PublishOptions) IssueEnabled() bool {
	return o.CreateIssue || o.Issue > 0
}

// Validate checks flag combinations for publication.
func (o PublishOptions) Validate() error {
	if o.Issue < 0 {
		return fmt.Errorf("--issue must be a positive issue number")
	}
	if o.CreateIssue && o.Issue > 0 {
		return fmt.Errorf("--create-issue and --issue cannot be used together")
	}
	if !o.CreateIssue && (o.Title != "" || len(o.Labels) > 0) {
		return fmt.Errorf("--title and --label require --create-issue")
	}
	if o.Pages && o.IssueEnabled() {
		return fmt.Errorf("--pages cannot be combined with --create-issue or --issue")
	}
	if o.AllowPublic && !o.IssueEnabled() {
		return fmt.Errorf("--allow-public requires --create-issue or --issue and cannot be used with --pages")
	}
	if !o.Enabled() && o.Repo != "" {
		return fmt.Errorf("--repo requires --create-issue, --issue, or --pages")
	}
	return nil
}

// OutputOptions controls local output.
type OutputOptions struct {
	Formats   []Format
	OutputDir string
	Rich      bool
}

// Validate checks that the outputs can be written for the given publish options. Multiple formats
// cannot share stdout, so they require an output directory unless the report is only published.
func (o OutputOptions) Validate(publish PublishOptions) error {
	if o.Rich && !containsFormat(o.Formats, FormatHTML) {
		return fmt.Errorf("--rich requires --format html")
	}
	if publish.Pages && !containsFormat(o.Formats, FormatHTML) {
		return fmt.Errorf("--pages requires --format html")
	}
	if len(o.Formats) > 1 && o.OutputDir == "" && (!publish.Enabled() || publish.Pages) {
		return fmt.Errorf("multiple --format values require --output-dir")
	}
	return nil
}

func containsFormat(formats []Format, format Format) bool {
	for _, candidate := range formats {
		if candidate == format {
			return true
		}
	}
	return false
}

// WritesStdout reports whether the single rendered report is written to stdout.
func (o OutputOptions) WritesStdout(publish PublishOptions) bool {
	return o.OutputDir == "" && !publish.Enabled()
}

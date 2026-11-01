package copilotmetrics

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/google/go-github/v93/github"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
)

// Family is a kind of usage metrics report.
type Family string

const (
	FamilyOrganization Family = "organization"
	FamilyUsers        Family = "users"
	FamilyRepositories Family = "repositories"
	FamilyUserTeams    Family = "user-teams"
)

// Sections returns the report sections derived from the family.
func (f Family) Sections() []Section {
	switch f {
	case FamilyOrganization:
		return []Section{SectionKPI, SectionTrend, SectionFeature, SectionLanguage, SectionModel, SectionIDE}
	case FamilyUsers:
		return []Section{SectionTeam, SectionUser}
	case FamilyRepositories:
		return []Section{SectionRepository}
	case FamilyUserTeams:
		return []Section{SectionTeam}
	}
	return nil
}

// Fetcher reads usage metrics reports for a single organization or enterprise.
type Fetcher interface {
	LatestMetrics(ctx context.Context) (*gh.CopilotLatestMetrics, error)
	DailyMetrics(ctx context.Context, day string) ([]*github.CopilotDailyMetrics, bool, error)
	LatestUserMetrics(ctx context.Context) (*gh.CopilotLatestUserMetrics, error)
	DailyUserMetrics(ctx context.Context, day string) ([]*github.CopilotUserDailyMetrics, bool, error)
	DailyRepositoryMetrics(ctx context.Context, day string) ([]*github.CopilotRepositoryDailyMetrics, bool, error)
	DailyUserTeamsMetrics(ctx context.Context, day string) ([]*github.CopilotUserTeamsDailyMetrics, bool, error)
}

// CollectOptions controls which reports are collected.
type CollectOptions struct {
	Sections SectionSet
	Period   Period
	// IncludeIDEActiveUsers fetches user reports only to aggregate IDE-specific active-user counts.
	IncludeIDEActiveUsers bool
	// Strict fails collection when any selected report is missing or has a mismatched window.
	Strict bool
	// Concurrency bounds parallel 1-day report requests. Values below 1 use 4.
	Concurrency int
	// Progress receives progress messages when not nil.
	Progress io.Writer
}

// Dataset is the raw collected data for a single window.
type Dataset struct {
	Period Period
	// Days are the days covered by the report window, in order.
	Days         []string
	OrgDays      []*github.CopilotDailyMetrics
	Users        []UserDay
	Repositories []*github.CopilotRepositoryDailyMetrics
	UserTeams    []*github.CopilotUserTeamsDailyMetrics
	// Missing lists the days without a report per family.
	Missing  map[Family][]string
	Warnings []string

	ideMonthSupplement *ideMonthSupplement
}

// StartDay returns the first day of the window.
func (d *Dataset) StartDay() string {
	if len(d.Days) == 0 {
		return ""
	}
	return d.Days[0]
}

// EndDay returns the last day of the window.
func (d *Dataset) EndDay() string {
	if len(d.Days) == 0 {
		return ""
	}
	return d.Days[len(d.Days)-1]
}

func (d *Dataset) addMissing(f Family, days []string) {
	if len(days) == 0 {
		return
	}
	if d.Missing == nil {
		d.Missing = map[Family][]string{}
	}
	d.Missing[f] = append(d.Missing[f], days...)
	sort.Strings(d.Missing[f])
}

// Collect fetches the reports required by the selected sections.
func Collect(ctx context.Context, f Fetcher, opts CollectOptions) (*Dataset, error) {
	sections := opts.Sections
	if sections == nil {
		sections = DefaultSections()
	}
	ds := &Dataset{Period: opts.Period}
	needsUsers := sections.NeedsUsers() || opts.IncludeIDEActiveUsers
	progress := func(format string, args ...any) {
		if opts.Progress != nil {
			fmt.Fprintf(opts.Progress, format+"\n", args...)
		}
	}

	if opts.Period.Latest {
		progress("Fetching the latest 28-day usage metrics report...")
		latest, err := f.LatestMetrics(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get the latest usage metrics report: %w", err)
		}
		ds.Days = DayRange(latest.Window.StartDay, latest.Window.EndDay)
		if len(ds.Days) == 0 {
			return nil, fmt.Errorf("the latest usage metrics report has no valid window (start %q, end %q)", latest.Window.StartDay, latest.Window.EndDay)
		}
		ds.OrgDays = filterOrgDays(latest.DayTotals, ds.Days, &ds.Warnings)
		ds.addMissing(FamilyOrganization, missingDays(ds.Days, orgDaySet(ds.OrgDays)))

		if needsUsers {
			progress("Fetching the latest 28-day users usage metrics report...")
			users, err := f.LatestUserMetrics(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get the latest users usage metrics report: %w", err)
			}
			if users.Window != latest.Window {
				msg := fmt.Sprintf("users report window %s..%s differs from the organization report window %s..%s; only days inside the organization window are used",
					users.Window.StartDay, users.Window.EndDay, latest.Window.StartDay, latest.Window.EndDay)
				if opts.Strict {
					return nil, fmt.Errorf("%s", msg)
				}
				ds.Warnings = append(ds.Warnings, msg)
				var unavailable []string
				for _, day := range ds.Days {
					if day < users.Window.StartDay || day > users.Window.EndDay {
						unavailable = append(unavailable, day)
					}
				}
				ds.addMissing(FamilyUsers, unavailable)
			}
			for _, r := range users.Records {
				if r != nil {
					ds.Users = append(ds.Users, UserDayFromPeriodic(r))
				}
			}
			ds.Users = filterUserDays(ds.Users, ds.Days, &ds.Warnings)
		}
	} else {
		ds.Days = append([]string(nil), opts.Period.Days...)
		if len(ds.Days) == 0 {
			return nil, fmt.Errorf("no days to report")
		}
		if sections.NeedsOrganizationLevel() {
			progress("Fetching %d daily usage metrics report(s)...", len(ds.Days))
			results, missing, err := fetchDaily(ctx, ds.Days, opts.Concurrency, f.DailyMetrics)
			if err != nil {
				return nil, fmt.Errorf("failed to get daily usage metrics reports: %w", err)
			}
			if len(missing) == len(ds.Days) {
				return nil, fmt.Errorf("no usage metrics reports were found for %s..%s; check that the Copilot usage metrics policy is enabled and the token has access", ds.StartDay(), ds.EndDay())
			}
			ds.OrgDays = filterOrgDays(results, ds.Days, &ds.Warnings)
			ds.addMissing(FamilyOrganization, missingDays(ds.Days, orgDaySet(ds.OrgDays)))
		}
		if needsUsers {
			progress("Fetching %d daily users usage metrics report(s)...", len(ds.Days))
			results, missing, err := fetchDaily(ctx, ds.Days, opts.Concurrency, f.DailyUserMetrics)
			if err != nil {
				return nil, fmt.Errorf("failed to get daily users usage metrics reports: %w", err)
			}
			for _, r := range results {
				if r != nil {
					ds.Users = append(ds.Users, UserDayFromDaily(r))
				}
			}
			ds.Users = filterUserDays(ds.Users, ds.Days, &ds.Warnings)
			ds.addMissing(FamilyUsers, missing)
		}
	}

	if sections.NeedsRepositories() {
		progress("Fetching %d daily repository usage metrics report(s)...", len(ds.Days))
		results, missing, err := fetchDaily(ctx, ds.Days, opts.Concurrency, f.DailyRepositoryMetrics)
		if err != nil {
			return nil, fmt.Errorf("failed to get daily repository usage metrics reports: %w", err)
		}
		ds.Repositories = filterRepositoryDays(results, ds.Days, &ds.Warnings)
		ds.addMissing(FamilyRepositories, missing)
	}
	if sections.NeedsUserTeams() {
		progress("Fetching %d daily user-teams report(s)...", len(ds.Days))
		results, missing, err := fetchDaily(ctx, ds.Days, opts.Concurrency, f.DailyUserTeamsMetrics)
		if err != nil {
			return nil, fmt.Errorf("failed to get daily user-teams reports: %w", err)
		}
		ds.UserTeams = filterUserTeamDays(results, ds.Days, &ds.Warnings)
		ds.addMissing(FamilyUserTeams, missing)
	}

	if opts.IncludeIDEActiveUsers {
		supplement, err := collectIDEMonthSupplement(ctx, f, ds, opts.Concurrency, progress)
		if err != nil {
			return nil, err
		}
		ds.ideMonthSupplement = supplement
	}

	if !sections.NeedsOrganizationLevel() {
		delete(ds.Missing, FamilyOrganization)
	}
	if opts.Strict {
		if ds.ideMonthSupplement != nil {
			ds.addMissing(FamilyUsers, ds.ideMonthSupplement.missingDays)
		}
		if len(ds.Missing) > 0 {
			return nil, fmt.Errorf("reports are missing: %s", describeMissing(ds.Missing))
		}
	}
	return ds, nil
}

func collectIDEMonthSupplement(ctx context.Context, f Fetcher, ds *Dataset, concurrency int, progress func(string, ...any)) (*ideMonthSupplement, error) {
	monthDays := calendarMonthDays(ds.EndDay())
	if len(monthDays) == 0 {
		return nil, fmt.Errorf("invalid report end day %q for the IDE active-user estimate", ds.EndDay())
	}
	inReport := daySet(ds.Days)
	supplement := &ideMonthSupplement{}
	for _, day := range monthDays {
		if !inReport[day] {
			supplement.days = append(supplement.days, day)
		}
	}
	if len(supplement.days) == 0 {
		return supplement, nil
	}

	progress("Fetching %d additional daily users report(s) for the IDE active-user estimate...", len(supplement.days))
	records, missing, err := fetchDaily(ctx, supplement.days, concurrency, f.DailyUserMetrics)
	if err != nil {
		return nil, fmt.Errorf("failed to get additional users reports for the IDE active-user estimate: %w", err)
	}
	supplement.missingDays = missing
	users := make([]UserDay, 0, len(records))
	for _, record := range records {
		if record != nil {
			users = append(users, UserDay{UserID: int64(record.UserID), Day: record.Day, IDEs: userIDEs(record.TotalsByIDE)})
		}
	}
	for _, user := range filterUserDays(users, supplement.days, &ds.Warnings) {
		if hasIDE(user.IDEs) {
			supplement.users = append(supplement.users, ideMonthUserDay{day: user.Day, userID: user.UserID})
		}
	}
	return supplement, nil
}

func describeMissing(missing map[Family][]string) string {
	families := make([]string, 0, len(missing))
	for f := range missing {
		families = append(families, string(f))
	}
	sort.Strings(families)
	parts := make([]string, 0, len(families))
	for _, f := range families {
		parts = append(parts, fmt.Sprintf("%s (%s)", f, strings.Join(missing[Family(f)], ", ")))
	}
	return strings.Join(parts, "; ")
}

// fetchDaily calls fetch for every day with bounded concurrency. It returns all records in day order
// and the days whose report was not found. The first error cancels the remaining requests.
func fetchDaily[T any](ctx context.Context, days []string, concurrency int, fetch func(context.Context, string) ([]T, bool, error)) ([]T, []string, error) {
	if concurrency < 1 {
		concurrency = 4
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		records []T
		found   bool
	}
	results := make([]result, len(days))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	for i, day := range days {
		wg.Add(1)
		go func(i int, day string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			records, found, err := fetch(ctx, day)
			if err != nil {
				once.Do(func() {
					firstErr = fmt.Errorf("%s: %w", day, err)
					cancel()
				})
				return
			}
			results[i] = result{records: records, found: found}
		}(i, day)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	var all []T
	var missing []string
	for i, r := range results {
		if !r.found {
			missing = append(missing, days[i])
			continue
		}
		all = append(all, r.records...)
	}
	return all, missing, nil
}

func daySet(days []string) map[string]bool {
	set := make(map[string]bool, len(days))
	for _, d := range days {
		set[d] = true
	}
	return set
}

func orgDaySet(records []*github.CopilotDailyMetrics) map[string]bool {
	set := map[string]bool{}
	for _, r := range records {
		set[r.Day] = true
	}
	return set
}

func missingDays(days []string, present map[string]bool) []string {
	var missing []string
	for _, d := range days {
		if !present[d] {
			missing = append(missing, d)
		}
	}
	return missing
}

func outsideWarning(warnings *[]string, family Family, n int) {
	if n > 0 {
		*warnings = append(*warnings, fmt.Sprintf("%d %s record(s) outside the report window were ignored", n, family))
	}
}

func duplicateWarning(warnings *[]string, family Family, n int) {
	if n > 0 {
		*warnings = append(*warnings, fmt.Sprintf("%d duplicate %s record(s) were ignored", n, family))
	}
}

// filterOrgDays keeps one record per day inside the window, sorted by day.
func filterOrgDays(records []*github.CopilotDailyMetrics, days []string, warnings *[]string) []*github.CopilotDailyMetrics {
	window := daySet(days)
	seen := map[string]bool{}
	var out []*github.CopilotDailyMetrics
	outside, dup := 0, 0
	for _, r := range records {
		if r == nil {
			continue
		}
		if !window[r.Day] {
			outside++
			continue
		}
		if seen[r.Day] {
			dup++
			continue
		}
		seen[r.Day] = true
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	outsideWarning(warnings, FamilyOrganization, outside)
	duplicateWarning(warnings, FamilyOrganization, dup)
	return out
}

// filterUserDays keeps one record per (user ID, day) inside the window.
func filterUserDays(records []UserDay, days []string, warnings *[]string) []UserDay {
	window := daySet(days)
	type key struct {
		id  int64
		day string
	}
	seen := map[key]bool{}
	var out []UserDay
	outside, dup := 0, 0
	for _, r := range records {
		if !window[r.Day] {
			outside++
			continue
		}
		k := key{r.UserID, r.Day}
		if seen[k] {
			dup++
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	outsideWarning(warnings, FamilyUsers, outside)
	duplicateWarning(warnings, FamilyUsers, dup)
	return out
}

// filterRepositoryDays keeps one record per (repository ID, day) inside the window.
func filterRepositoryDays(records []*github.CopilotRepositoryDailyMetrics, days []string, warnings *[]string) []*github.CopilotRepositoryDailyMetrics {
	window := daySet(days)
	type key struct {
		id  int64
		day string
	}
	seen := map[key]bool{}
	var out []*github.CopilotRepositoryDailyMetrics
	outside, dup := 0, 0
	for _, r := range records {
		if r == nil {
			continue
		}
		if !window[r.Day] {
			outside++
			continue
		}
		k := key{r.RepoID, r.Day}
		if seen[k] {
			dup++
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	outsideWarning(warnings, FamilyRepositories, outside)
	duplicateWarning(warnings, FamilyRepositories, dup)
	return out
}

// filterUserTeamDays keeps one record per (user ID, team ID, day) inside the window.
func filterUserTeamDays(records []*github.CopilotUserTeamsDailyMetrics, days []string, warnings *[]string) []*github.CopilotUserTeamsDailyMetrics {
	window := daySet(days)
	type key struct {
		user, team int64
		day        string
	}
	seen := map[key]bool{}
	var out []*github.CopilotUserTeamsDailyMetrics
	outside, dup := 0, 0
	for _, r := range records {
		if r == nil {
			continue
		}
		if !window[r.Day] {
			outside++
			continue
		}
		k := key{r.UserID, r.TeamID, r.Day}
		if seen[k] {
			dup++
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	outsideWarning(warnings, FamilyUserTeams, outside)
	duplicateWarning(warnings, FamilyUserTeams, dup)
	return out
}

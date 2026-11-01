package copilotmetrics

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/go-github/v93/github"
)

// Counter is a nullable additive integer. Valid is false when no source value was present,
// which is rendered as "n/a" rather than zero.
type Counter struct {
	Value int64
	Valid bool
}

// Add adds a nullable source value.
func (c *Counter) Add(v *int) {
	if v == nil {
		return
	}
	c.Value += int64(*v)
	c.Valid = true
}

// AddCounter adds another counter.
func (c *Counter) AddCounter(o Counter) {
	if !o.Valid {
		return
	}
	c.Value += o.Value
	c.Valid = true
}

// CounterPair aggregates a total and its Copilot subset only from days that
// report both values, so complementary counts are derived from matching days.
type CounterPair struct {
	Total   Counter
	Copilot Counter
	// Partial reports that some days reported only one of the two values.
	Partial bool
}

// Add adds a daily pair when both values are reported.
func (p *CounterPair) Add(total, copilot *int) {
	if total == nil || copilot == nil {
		if total != nil || copilot != nil {
			p.Partial = true
		}
		return
	}
	p.Total.Add(total)
	p.Copilot.Add(copilot)
}

// FloatCounter is a nullable additive float.
type FloatCounter struct {
	Value float64
	Valid bool
}

// Add adds a nullable source value.
func (c *FloatCounter) Add(v *float64) {
	if v == nil {
		return
	}
	c.Value += *v
	c.Valid = true
}

// Rate returns num/den when both are valid and den is positive.
func Rate(num, den Counter) (float64, bool) {
	if !num.Valid || !den.Valid || den.Value <= 0 {
		return 0, false
	}
	return float64(num.Value) / float64(den.Value), true
}

// ValueKind controls how a KPI value is formatted.
type ValueKind int

const (
	KindCount ValueKind = iota
	KindDecimal
	KindPercent
)

// KPI is a single headline metric with its source window.
type KPI struct {
	Label  string
	Value  float64
	Valid  bool
	Kind   ValueKind
	Window string
}

// TrendPoint is one day of the daily trend.
type TrendPoint struct {
	Day                      string
	DailyActiveUsers         Counter
	UserInitiatedInteraction Counter
	CodeGeneration           Counter
	CodeAcceptance           Counter
	LOCAdded                 Counter
}

// ActivityTotals are additive code activity counters.
type ActivityTotals struct {
	UserInitiatedInteraction Counter
	CodeGeneration           Counter
	CodeAcceptance           Counter
	LOCAdded                 Counter
	LOCDeleted               Counter
}

// AcceptanceRate returns Σacceptance / Σgeneration.
func (a ActivityTotals) AcceptanceRate() (float64, bool) {
	return Rate(a.CodeAcceptance, a.CodeGeneration)
}

func (a *ActivityTotals) addCode(c github.CopilotMetricsCodeActivity) {
	a.CodeGeneration.Add(c.CodeGenerationActivityCount)
	a.CodeAcceptance.Add(c.CodeAcceptanceActivityCount)
	a.LOCAdded.Add(c.LOCAddedSum)
	a.LOCDeleted.Add(c.LOCDeletedSum)
}

// BreakdownRow is an aggregated row keyed by feature, language, model, or IDE.
type BreakdownRow struct {
	Key string
	ActivityTotals
}

// RepositoryRow aggregates pull request metrics of a repository.
type RepositoryRow struct {
	Name                      string
	Visibility                string
	ActiveDays                int
	Created                   Counter
	CreatedByCopilot          Counter
	Reviewed                  Counter
	ReviewedByCopilot         Counter
	Merged                    Counter
	MergedCreatedByCopilot    Counter
	CopilotSuggestions        Counter
	CopilotAppliedSuggestions Counter
	// Paired counters aggregate total and Copilot counts from matching days.
	CreatedPair  CounterPair
	ReviewedPair CounterPair
	MergedPair   CounterPair
}

// TeamRow aggregates usage of the members of a team.
type TeamRow struct {
	TeamID      int64
	Slug        string
	Members     int
	ActiveUsers int
	AICredits   FloatCounter
	ActivityTotals
}

// RatioCounts sums a numerator and denominator only over records where both are reported,
// so the ratio compares values covering the same days.
type RatioCounts struct {
	Numerator   Counter
	Denominator Counter
}

// Rate returns Numerator/Denominator when both are valid and the denominator is positive.
func (r RatioCounts) Rate() (float64, bool) {
	return Rate(r.Numerator, r.Denominator)
}

func (r *RatioCounts) add(numerator, denominator *int) {
	if numerator == nil || denominator == nil {
		return
	}
	r.Numerator.Add(numerator)
	r.Denominator.Add(denominator)
}

// UserRow aggregates usage of a single user.
type UserRow struct {
	Login      string
	ActiveDays int
	AICredits  FloatCounter
	Surfaces   []string
	ActivityTotals
	// Acceptance pairs code-acceptance with code-generation activities on matching days.
	Acceptance RatioCounts
}

// PartialNote describes a section with missing daily reports.
type PartialNote struct {
	Family      Family
	Sections    []Section
	MissingDays []string
}

// Summary is the aggregated, render-ready usage metrics report.
type Summary struct {
	Source      string
	PeriodLabel string
	StartDay    string
	EndDay      string
	Days        int
	GeneratedAt time.Time
	Sections    SectionSet
	Top         int

	KPIs                          []KPI
	Trend                         []TrendPoint
	Features                      []BreakdownRow
	Languages                     []BreakdownRow
	AllLanguages                  []BreakdownRow
	Models                        []BreakdownRow
	ModelRequestsByFeature        []RichValue
	ModelCodeGenerationByLanguage []RichValue
	IDEs                          []BreakdownRow
	Repositories                  []RepositoryRow
	AllRepositories               []RepositoryRow
	Teams                         []TeamRow
	AllTeams                      []TeamRow
	Users                         []UserRow
	AllUsers                      []UserRow
	Rich                          *RichMetrics

	// Omitted is the number of rows dropped by Top per section.
	Omitted  map[Section]int
	Partial  []PartialNote
	Warnings []string
}

// SummaryOptions controls aggregation.
type SummaryOptions struct {
	Source             string
	Sections           SectionSet
	Top                int
	GeneratedAt        time.Time
	IncludeRichMetrics bool
}

// BuildSummary aggregates a dataset. Active-user metrics are snapshots and never summed:
// DAU is averaged and peaked across days, and WAU/MAU are taken from the last day with data.
// Additive counters are summed and rates are derived from summed numerators and denominators.
func BuildSummary(ds *Dataset, opts SummaryOptions) *Summary {
	sections := opts.Sections
	if sections == nil {
		sections = DefaultSections()
	}
	s := &Summary{
		Source:      opts.Source,
		PeriodLabel: ds.Period.Label(),
		StartDay:    ds.StartDay(),
		EndDay:      ds.EndDay(),
		Days:        len(ds.Days),
		GeneratedAt: opts.GeneratedAt,
		Sections:    sections,
		Top:         opts.Top,
		Omitted:     map[Section]int{},
		Warnings:    append([]string(nil), ds.Warnings...),
	}
	if sections.Has(SectionKPI) {
		s.KPIs = buildKPIs(ds.OrgDays)
	}
	if sections.Has(SectionTrend) {
		s.Trend = buildTrend(ds.OrgDays, ds.Days)
	}
	if sections.Has(SectionFeature) {
		s.Features = s.limitBreakdown(SectionFeature, buildFeatures(ds.OrgDays))
	}
	if sections.Has(SectionLanguage) {
		s.AllLanguages = buildLanguages(ds.OrgDays)
		s.Languages = s.limitBreakdown(SectionLanguage, s.AllLanguages)
	}
	if sections.Has(SectionModel) {
		s.Models = s.limitBreakdown(SectionModel, buildModels(ds.OrgDays))
	}
	if sections.Has(SectionIDE) {
		s.IDEs = s.limitBreakdown(SectionIDE, buildIDEs(ds.OrgDays))
	}
	if sections.Has(SectionRepository) {
		rows := buildRepositories(ds.Repositories)
		s.AllRepositories = rows
		s.Repositories = limit(s, SectionRepository, rows)
	}
	if sections.Has(SectionTeam) {
		rows := buildTeams(ds.Users, ds.UserTeams)
		s.AllTeams = rows
		s.Teams = limit(s, SectionTeam, rows)
	}
	if sections.Has(SectionUser) {
		rows := buildUsers(ds.Users)
		s.AllUsers = rows
		s.Users = limit(s, SectionUser, rows)
	}
	if sections.Has(SectionModel) || opts.IncludeRichMetrics {
		s.ModelRequestsByFeature = buildModelRequestsByFeature(ds.OrgDays)
	}
	if sections.Has(SectionLanguage) || opts.IncludeRichMetrics {
		s.ModelCodeGenerationByLanguage = buildModelCodeGenerationByLanguage(ds.OrgDays)
	}
	if opts.IncludeRichMetrics {
		s.Rich = buildRichMetrics(ds, sections.Has(SectionIDE), s.ModelRequestsByFeature, s.ModelCodeGenerationByLanguage)
		if sections.Has(SectionRepository) {
			s.Rich.Repositories = buildRichRepositories(ds)
		}
	}
	s.Partial = buildPartial(ds.Missing, sections)
	if opts.IncludeRichMetrics && sections.Has(SectionIDE) && len(ds.Missing[FamilyUsers]) > 0 {
		s.Partial = append(s.Partial, PartialNote{
			Family: FamilyUsers, Sections: []Section{SectionIDE}, MissingDays: append([]string(nil), ds.Missing[FamilyUsers]...),
		})
	}
	return s
}

func limit[T any](s *Summary, section Section, rows []T) []T {
	if s.Top > 0 && len(rows) > s.Top {
		s.Omitted[section] = len(rows) - s.Top
		return rows[:s.Top]
	}
	return rows
}

func (s *Summary) limitBreakdown(section Section, rows []BreakdownRow) []BreakdownRow {
	return limit(s, section, rows)
}

func buildPartial(missing map[Family][]string, sections SectionSet) []PartialNote {
	families := []Family{FamilyOrganization, FamilyUsers, FamilyRepositories, FamilyUserTeams}
	var notes []PartialNote
	for _, f := range families {
		days := missing[f]
		if len(days) == 0 {
			continue
		}
		var affected []Section
		for _, sec := range f.Sections() {
			if sections.Has(sec) {
				affected = append(affected, sec)
			}
		}
		if len(affected) == 0 {
			continue
		}
		notes = append(notes, PartialNote{Family: f, Sections: affected, MissingDays: days})
	}
	return notes
}

func lastValid(days []*github.CopilotDailyMetrics, get func(*github.CopilotDailyMetrics) *int) (int, string, bool) {
	for i := len(days) - 1; i >= 0; i-- {
		if v := get(days[i]); v != nil {
			return *v, days[i].Day, true
		}
	}
	return 0, "", false
}

func avgPeak(days []*github.CopilotDailyMetrics, get func(*github.CopilotDailyMetrics) *int) (avg float64, peak int, n int) {
	sum := 0
	for _, d := range days {
		v := get(d)
		if v == nil {
			continue
		}
		sum += *v
		if n == 0 || *v > peak {
			peak = *v
		}
		n++
	}
	if n > 0 {
		avg = float64(sum) / float64(n)
	}
	return avg, peak, n
}

func sumField(days []*github.CopilotDailyMetrics, get func(*github.CopilotDailyMetrics) *int) Counter {
	var c Counter
	for _, d := range days {
		c.Add(get(d))
	}
	return c
}

func counterKPI(label string, c Counter, window string) KPI {
	return KPI{Label: label, Value: float64(c.Value), Valid: c.Valid, Kind: KindCount, Window: window}
}

func prField(get func(*github.CopilotMetricsPullRequests) *int) func(*github.CopilotDailyMetrics) *int {
	return func(d *github.CopilotDailyMetrics) *int {
		if d.PullRequests == nil {
			return nil
		}
		return get(d.PullRequests)
	}
}

func cliField(get func(*github.CopilotMetricsCLI) *int) func(*github.CopilotDailyMetrics) *int {
	return func(d *github.CopilotDailyMetrics) *int {
		if d.TotalsByCLI == nil {
			return nil
		}
		return get(d.TotalsByCLI)
	}
}

func buildKPIs(days []*github.CopilotDailyMetrics) []KPI {
	var kpis []KPI
	sumWindow := "sum of daily values"
	if len(days) > 0 {
		sumWindow = fmt.Sprintf("sum over %d day(s)", len(days))
	}

	dauGet := func(d *github.CopilotDailyMetrics) *int { return d.DailyActiveUsers }
	avg, peak, n := avgPeak(days, dauGet)
	kpis = append(kpis,
		KPI{Label: "Daily active users (average)", Value: avg, Valid: n > 0, Kind: KindDecimal, Window: fmt.Sprintf("average over %d day(s)", n)},
		KPI{Label: "Daily active users (peak)", Value: float64(peak), Valid: n > 0, Kind: KindCount, Window: fmt.Sprintf("maximum over %d day(s)", n)},
	)

	snapshots := []struct {
		label  string
		window string
		get    func(*github.CopilotDailyMetrics) *int
	}{
		{"Weekly active users", "7 days ending", func(d *github.CopilotDailyMetrics) *int { return d.WeeklyActiveUsers }},
		{"Monthly active users", "28 days ending", func(d *github.CopilotDailyMetrics) *int { return d.MonthlyActiveUsers }},
		{"Monthly active chat users", "28 days ending", func(d *github.CopilotDailyMetrics) *int { return d.MonthlyActiveChatUsers }},
		{"Monthly active agent users", "28 days ending", func(d *github.CopilotDailyMetrics) *int { return d.MonthlyActiveAgentUsers }},
		{"Monthly active Copilot cloud agent users", "28 days ending", func(d *github.CopilotDailyMetrics) *int { return d.MonthlyActiveCopilotCloudAgentUsers }},
		{"Monthly active Copilot code review users", "28 days ending", func(d *github.CopilotDailyMetrics) *int { return d.MonthlyActiveCopilotCodeReviewUsers }},
	}
	for _, snap := range snapshots {
		v, day, ok := lastValid(days, snap.get)
		window := "n/a"
		if ok {
			window = fmt.Sprintf("%s %s", snap.window, day)
		}
		kpis = append(kpis, KPI{Label: snap.label, Value: float64(v), Valid: ok, Kind: KindCount, Window: window})
	}

	cliAvg, _, cliN := avgPeak(days, func(d *github.CopilotDailyMetrics) *int { return d.DailyActiveCLIUsers })
	kpis = append(kpis, KPI{Label: "Daily active CLI users (average)", Value: cliAvg, Valid: cliN > 0, Kind: KindDecimal, Window: fmt.Sprintf("average over %d day(s)", cliN)})

	interactions := sumField(days, func(d *github.CopilotDailyMetrics) *int { return d.UserInitiatedInteractionCount })
	gen := sumField(days, func(d *github.CopilotDailyMetrics) *int { return d.CodeGenerationActivityCount })
	acc := sumField(days, func(d *github.CopilotDailyMetrics) *int { return d.CodeAcceptanceActivityCount })
	rate, rateOK := Rate(acc, gen)
	kpis = append(kpis,
		counterKPI("User-initiated interactions", interactions, sumWindow),
		counterKPI("Code generation activities", gen, sumWindow),
		counterKPI("Code acceptance activities", acc, sumWindow),
		KPI{Label: "Code acceptance rate", Value: rate, Valid: rateOK, Kind: KindPercent, Window: "acceptance / generation"},
		counterKPI("Lines of code added", sumField(days, func(d *github.CopilotDailyMetrics) *int { return d.LOCAddedSum }), sumWindow),
		counterKPI("Lines of code deleted", sumField(days, func(d *github.CopilotDailyMetrics) *int { return d.LOCDeletedSum }), sumWindow),
		counterKPI("CLI sessions", sumField(days, cliField(func(c *github.CopilotMetricsCLI) *int { return c.SessionCount })), sumWindow),
		counterKPI("CLI requests", sumField(days, cliField(func(c *github.CopilotMetricsCLI) *int { return c.RequestCount })), sumWindow),
		counterKPI("Pull requests created", sumField(days, prField(func(p *github.CopilotMetricsPullRequests) *int { return p.TotalCreated })), sumWindow),
		counterKPI("Pull requests created by Copilot", sumField(days, prField(func(p *github.CopilotMetricsPullRequests) *int { return p.TotalCreatedByCopilot })), sumWindow),
		counterKPI("Pull requests reviewed by Copilot", sumField(days, prField(func(p *github.CopilotMetricsPullRequests) *int { return p.TotalReviewedByCopilot })), sumWindow),
		counterKPI("Pull requests merged", sumField(days, prField(func(p *github.CopilotMetricsPullRequests) *int { return p.TotalMerged })), sumWindow),
	)
	return kpis
}

func buildTrend(records []*github.CopilotDailyMetrics, days []string) []TrendPoint {
	byDay := map[string]*github.CopilotDailyMetrics{}
	for _, r := range records {
		byDay[r.Day] = r
	}
	points := make([]TrendPoint, 0, len(days))
	for _, day := range days {
		p := TrendPoint{Day: day}
		if r, ok := byDay[day]; ok {
			p.DailyActiveUsers.Add(r.DailyActiveUsers)
			p.UserInitiatedInteraction.Add(r.UserInitiatedInteractionCount)
			p.CodeGeneration.Add(r.CodeGenerationActivityCount)
			p.CodeAcceptance.Add(r.CodeAcceptanceActivityCount)
			p.LOCAdded.Add(r.LOCAddedSum)
		}
		points = append(points, p)
	}
	return points
}

type breakdownAcc struct {
	rows  map[string]*BreakdownRow
	order []string
}

func newBreakdownAcc() *breakdownAcc {
	return &breakdownAcc{rows: map[string]*BreakdownRow{}}
}

func (b *breakdownAcc) get(key string) *BreakdownRow {
	if key == "" {
		key = "unknown"
	}
	row, ok := b.rows[key]
	if !ok {
		row = &BreakdownRow{Key: key}
		b.rows[key] = row
		b.order = append(b.order, key)
	}
	return row
}

// sorted orders rows by interactions, then code generation, then key.
func (b *breakdownAcc) sorted() []BreakdownRow {
	out := make([]BreakdownRow, 0, len(b.order))
	for _, k := range b.order {
		out = append(out, *b.rows[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i], out[j]
		if a.UserInitiatedInteraction.Value != c.UserInitiatedInteraction.Value {
			return a.UserInitiatedInteraction.Value > c.UserInitiatedInteraction.Value
		}
		if a.CodeGeneration.Value != c.CodeGeneration.Value {
			return a.CodeGeneration.Value > c.CodeGeneration.Value
		}
		return a.Key < c.Key
	})
	return out
}

func buildFeatures(days []*github.CopilotDailyMetrics) []BreakdownRow {
	acc := newBreakdownAcc()
	for _, d := range days {
		for _, f := range d.TotalsByFeature {
			if f == nil {
				continue
			}
			row := acc.get(f.Feature)
			row.UserInitiatedInteraction.Add(f.UserInitiatedInteractionCount)
			row.addCode(f.CopilotMetricsCodeActivity)
		}
	}
	return acc.sorted()
}

func buildLanguages(days []*github.CopilotDailyMetrics) []BreakdownRow {
	acc := newBreakdownAcc()
	for _, d := range days {
		for _, l := range d.TotalsByLanguageFeature {
			if l == nil {
				continue
			}
			acc.get(l.Language).addCode(l.CopilotMetricsCodeActivity)
		}
	}
	return acc.sorted()
}

func buildModels(days []*github.CopilotDailyMetrics) []BreakdownRow {
	acc := newBreakdownAcc()
	for _, d := range days {
		for _, m := range d.TotalsByModelFeature {
			if m == nil {
				continue
			}
			row := acc.get(m.Model)
			row.UserInitiatedInteraction.Add(m.UserInitiatedInteractionCount)
			row.addCode(m.CopilotMetricsCodeActivity)
		}
	}
	return acc.sorted()
}

func buildModelRequestsByFeature(days []*github.CopilotDailyMetrics) []RichValue {
	values := map[richDimension]Counter{}
	for _, day := range days {
		if day == nil {
			continue
		}
		for _, model := range day.TotalsByModelFeature {
			if model == nil {
				continue
			}
			key := richDimension{name: model.Model, group: model.Feature}
			addRichDimension(values, key, counterFromInt(model.UserInitiatedInteractionCount))
		}
	}
	return richDimensionsFromMap(values)
}

func buildModelCodeGenerationByLanguage(days []*github.CopilotDailyMetrics) []RichValue {
	values := map[richDimension]Counter{}
	for _, day := range days {
		if day == nil {
			continue
		}
		for _, language := range day.TotalsByLanguageModel {
			if language == nil {
				continue
			}
			key := richDimension{name: language.Model, group: language.Language}
			addRichDimension(values, key, counterFromInt(language.CodeGenerationActivityCount))
		}
	}
	return richDimensionsFromMap(values)
}

func buildIDEs(days []*github.CopilotDailyMetrics) []BreakdownRow {
	acc := newBreakdownAcc()
	for _, d := range days {
		for _, ide := range d.TotalsByIDE {
			if ide == nil {
				continue
			}
			row := acc.get(ide.IDE)
			row.UserInitiatedInteraction.Add(ide.UserInitiatedInteractionCount)
			row.addCode(ide.CopilotMetricsCodeActivity)
		}
	}
	return acc.sorted()
}

func buildRepositories(records []*github.CopilotRepositoryDailyMetrics) []RepositoryRow {
	rows := map[int64]*RepositoryRow{}
	var order []int64
	for _, r := range records {
		row, ok := rows[r.RepoID]
		if !ok {
			row = &RepositoryRow{Name: repositoryName(r), Visibility: r.RepoVisibility}
			rows[r.RepoID] = row
			order = append(order, r.RepoID)
		}
		row.ActiveDays++
		pr := r.PullRequests
		if pr == nil {
			continue
		}
		row.Created.Add(pr.TotalCreated)
		row.CreatedByCopilot.Add(pr.TotalCreatedByCopilot)
		row.Reviewed.Add(pr.TotalReviewed)
		row.ReviewedByCopilot.Add(pr.TotalReviewedByCopilot)
		row.Merged.Add(pr.TotalMerged)
		row.MergedCreatedByCopilot.Add(pr.TotalMergedCreatedByCopilot)
		row.CreatedPair.Add(pr.TotalCreated, pr.TotalCreatedByCopilot)
		row.ReviewedPair.Add(pr.TotalReviewed, pr.TotalReviewedByCopilot)
		row.MergedPair.Add(pr.TotalMerged, pr.TotalMergedCreatedByCopilot)
		row.CopilotSuggestions.Add(pr.TotalCopilotSuggestions)
		row.CopilotAppliedSuggestions.Add(pr.TotalCopilotAppliedSuggestions)
	}
	out := make([]RepositoryRow, 0, len(order))
	for _, id := range order {
		out = append(out, *rows[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		ac := a.CreatedByCopilot.Value + a.ReviewedByCopilot.Value
		bc := b.CreatedByCopilot.Value + b.ReviewedByCopilot.Value
		if ac != bc {
			return ac > bc
		}
		if a.Created.Value != b.Created.Value {
			return a.Created.Value > b.Created.Value
		}
		return a.Name < b.Name
	})
	return out
}

func repositoryName(r *github.CopilotRepositoryDailyMetrics) string {
	if r.RepoOwnerName != "" {
		return r.RepoOwnerName + "/" + r.RepoName
	}
	return r.RepoName
}

func (a *ActivityTotals) addUserDay(u UserDay) {
	a.UserInitiatedInteraction.Add(u.UserInitiatedInteractions)
	a.CodeGeneration.Add(u.CodeGenerationActivities)
	a.CodeAcceptance.Add(u.CodeAcceptanceActivities)
	a.LOCAdded.Add(u.LOCAdded)
	a.LOCDeleted.Add(u.LOCDeleted)
}

func sortByActivity[T any](rows []T, totals func(T) ActivityTotals, key func(T) string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := totals(rows[i]), totals(rows[j])
		if a.UserInitiatedInteraction.Value != b.UserInitiatedInteraction.Value {
			return a.UserInitiatedInteraction.Value > b.UserInitiatedInteraction.Value
		}
		if a.CodeGeneration.Value != b.CodeGeneration.Value {
			return a.CodeGeneration.Value > b.CodeGeneration.Value
		}
		return key(rows[i]) < key(rows[j])
	})
}

// buildTeams joins user-day activity with the same day's team membership on user ID. A user in
// multiple teams is counted in each team, so team totals are not additive across teams.
func buildTeams(users []UserDay, memberships []*github.CopilotUserTeamsDailyMetrics) []TeamRow {
	type userDayKey struct {
		id  int64
		day string
	}
	activity := make(map[userDayKey]UserDay, len(users))
	for _, u := range users {
		activity[userDayKey{u.UserID, u.Day}] = u
	}
	rows := map[int64]*TeamRow{}
	members := map[int64]map[int64]bool{}
	active := map[int64]map[int64]bool{}
	var order []int64
	for _, m := range memberships {
		row, ok := rows[m.TeamID]
		if !ok {
			row = &TeamRow{TeamID: m.TeamID, Slug: m.Slug}
			rows[m.TeamID] = row
			members[m.TeamID] = map[int64]bool{}
			active[m.TeamID] = map[int64]bool{}
			order = append(order, m.TeamID)
		}
		if row.Slug == "" {
			row.Slug = m.Slug
		}
		members[m.TeamID][m.UserID] = true
		u, ok := activity[userDayKey{m.UserID, m.Day}]
		if !ok {
			continue
		}
		active[m.TeamID][m.UserID] = true
		row.addUserDay(u)
		row.AICredits.Add(u.AICreditsUsed)
	}
	out := make([]TeamRow, 0, len(order))
	for _, id := range order {
		row := rows[id]
		row.Members = len(members[id])
		row.ActiveUsers = len(active[id])
		out = append(out, *row)
	}
	sortByActivity(out, func(r TeamRow) ActivityTotals { return r.ActivityTotals }, func(r TeamRow) string { return r.Slug })
	return out
}

func buildUsers(users []UserDay) []UserRow {
	rows := map[int64]*UserRow{}
	surfaces := map[int64]map[string]bool{}
	var order []int64
	for _, u := range users {
		row, ok := rows[u.UserID]
		if !ok {
			row = &UserRow{Login: u.Login}
			rows[u.UserID] = row
			surfaces[u.UserID] = map[string]bool{}
			order = append(order, u.UserID)
		}
		row.ActiveDays++
		row.addUserDay(u)
		row.AICredits.Add(u.AICreditsUsed)
		row.Acceptance.add(u.CodeAcceptanceActivities, u.CodeGenerationActivities)
		set := surfaces[u.UserID]
		for name, used := range map[string]bool{
			"agent":        u.UsedAgent,
			"chat":         u.UsedChat,
			"cli":          u.UsedCLI,
			"app":          u.UsedCopilotApp,
			"cloud-agent":  u.UsedCopilotCloudAgent,
			"code-review":  u.UsedCopilotCodeReviewActive,
			"coding-agent": u.UsedCopilotCodingAgent,
		} {
			if used {
				set[name] = true
			}
		}
	}
	out := make([]UserRow, 0, len(order))
	for _, id := range order {
		row := rows[id]
		for name := range surfaces[id] {
			row.Surfaces = append(row.Surfaces, name)
		}
		sort.Strings(row.Surfaces)
		out = append(out, *row)
	}
	sortByActivity(out, func(r UserRow) ActivityTotals { return r.ActivityTotals }, func(r UserRow) string { return r.Login })
	return out
}

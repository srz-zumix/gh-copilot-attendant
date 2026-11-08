package sessionstats

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

const activityDateFormat = "2006-01-02"

// SessionActivity counts sessions with recorded events, not continuous active time.
type SessionActivity struct {
	Version         int
	TimeZone        string
	AsOf            time.Time
	AgentModes      []string
	StartDate       string
	EndDate         string
	EmptyReason     string
	ScannedSessions int
	UntimedSessions int
	UntimedEvents   int
	MalformedEvents int
	Days            []ActivityDay
	Coverage        []ActivityDayCoverage
}

// ActivityDay holds distinct session counts for each local wall-clock hour.
type ActivityDay struct {
	Date  string
	Hours [24]int
}

// ActivityDayCoverage describes hours affected by bounds or clock transitions.
type ActivityDayCoverage struct {
	Date  string
	Hours [24]string
}

type activityHour struct {
	date string
	hour int
}

type activityCollector struct {
	opts            copilotext.PermissionStatsOptions
	location        *time.Location
	agentModes      map[string]struct{}
	days            map[string]*ActivityDay
	firstEvent      time.Time
	lastEvent       time.Time
	scannedSessions int
	untimedSessions int
	untimedEvents   int
	malformedEvents int
}

func newActivityCollector(opts copilotext.PermissionStatsOptions, location *time.Location) *activityCollector {
	agentModes := make(map[string]struct{}, len(opts.AgentMode))
	for _, mode := range opts.AgentMode {
		agentModes[mode] = struct{}{}
	}
	return &activityCollector{
		opts: opts, location: location, agentModes: agentModes, days: make(map[string]*ActivityDay),
	}
}

// record returns whether the event lacks a usable timestamp.
func (c *activityCollector) record(line []byte, seen map[activityHour]struct{}) bool {
	var envelope struct {
		Type      json.RawMessage `json:"type"`
		Timestamp json.RawMessage `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		c.malformedEvents++
		return false
	}
	var eventType string
	if len(envelope.Type) == 0 || json.Unmarshal(envelope.Type, &eventType) != nil || eventType == "" {
		c.malformedEvents++
		return false
	}
	if !c.matchesAgentMode(eventType, envelope.Data) {
		return false
	}
	var timestamp time.Time
	if len(envelope.Timestamp) == 0 || json.Unmarshal(envelope.Timestamp, &timestamp) != nil || timestamp.IsZero() {
		c.untimedEvents++
		return true
	}
	if (!c.opts.Since.IsZero() && timestamp.Before(c.opts.Since)) ||
		(!c.opts.Until.IsZero() && !timestamp.Before(c.opts.Until)) {
		return false
	}
	if c.firstEvent.IsZero() || timestamp.Before(c.firstEvent) {
		c.firstEvent = timestamp
	}
	if c.lastEvent.IsZero() || timestamp.After(c.lastEvent) {
		c.lastEvent = timestamp
	}
	local := timestamp.In(c.location)
	key := activityHour{date: local.Format(activityDateFormat), hour: local.Hour()}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	day := c.days[key.date]
	if day == nil {
		day = &ActivityDay{Date: key.date}
		c.days[key.date] = day
	}
	day.Hours[key.hour]++
	return false
}

func (c *activityCollector) matchesAgentMode(eventType string, data json.RawMessage) bool {
	if len(c.agentModes) == 0 {
		return true
	}
	if eventType != "permission.requested" {
		return false
	}
	var request struct {
		RequestID string          `json:"requestId"`
		AgentMode json.RawMessage `json:"agentMode"`
	}
	if err := json.Unmarshal(data, &request); err != nil || request.RequestID == "" {
		return false
	}
	var mode string
	if len(request.AgentMode) > 0 {
		_ = json.Unmarshal(request.AgentMode, &mode)
	}
	if mode == "" {
		mode = "unknown"
	}
	_, ok := c.agentModes[mode]
	return ok
}

func (c *activityCollector) finish(asOf time.Time) (SessionActivity, error) {
	result := SessionActivity{
		Version: 1, TimeZone: c.location.String(), AsOf: asOf.In(c.location),
		AgentModes:      make([]string, 0, len(c.agentModes)),
		ScannedSessions: c.scannedSessions, UntimedSessions: c.untimedSessions,
		UntimedEvents: c.untimedEvents, MalformedEvents: c.malformedEvents,
		Days: make([]ActivityDay, 0, len(c.days)), Coverage: []ActivityDayCoverage{},
	}
	for mode := range c.agentModes {
		result.AgentModes = append(result.AgentModes, mode)
	}
	sort.Strings(result.AgentModes)
	for _, day := range c.days {
		result.Days = append(result.Days, *day)
	}
	sort.Slice(result.Days, func(i, j int) bool { return result.Days[i].Date < result.Days[j].Date })
	if !c.opts.Since.IsZero() && !c.opts.Until.IsZero() && !c.opts.Since.Before(c.opts.Until) {
		result.EmptyReason = "empty-window"
		return result, nil
	}

	if !c.opts.Since.IsZero() {
		result.StartDate = c.opts.Since.In(c.location).Format(activityDateFormat)
	} else if len(result.Days) > 0 {
		result.StartDate = result.Days[0].Date
	}
	if !c.opts.Until.IsZero() {
		result.EndDate = c.opts.Until.Add(-time.Nanosecond).In(c.location).Format(activityDateFormat)
	} else {
		result.EndDate = asOf.In(c.location).Format(activityDateFormat)
	}
	if len(result.Days) > 0 {
		first, last := result.Days[0].Date, result.Days[len(result.Days)-1].Date
		if result.StartDate == "" || first < result.StartDate {
			result.StartDate = first
		}
		if last > result.EndDate {
			result.EndDate = last
		}
	}
	if result.StartDate == "" {
		result.EndDate = ""
		result.EmptyReason = "no-dated-events"
		return result, nil
	}
	if c.opts.Until.IsZero() && len(result.Days) == 0 &&
		!c.opts.Since.IsZero() && !c.opts.Since.Before(asOf) {
		result.StartDate, result.EndDate = "", ""
		result.EmptyReason = "no-dated-events"
		return result, nil
	}
	start := c.opts.Since
	if start.IsZero() {
		start = c.firstEvent
	}
	end := c.opts.Until
	if end.IsZero() {
		end = asOf
		if !c.lastEvent.IsZero() && !c.lastEvent.Before(end) {
			end = c.lastEvent.Add(time.Nanosecond)
		}
	}
	result.StartDate, result.EndDate = c.transitionDateExtrema(result.StartDate, result.EndDate, start, end)
	if result.EndDate < result.StartDate {
		if !c.opts.Since.IsZero() && !c.opts.Until.IsZero() {
			result.StartDate, result.EndDate = result.EndDate, result.StartDate
		} else {
			result.StartDate, result.EndDate = "", ""
			result.EmptyReason = "no-dated-events"
			return result, nil
		}
	}
	coverage, err := c.coverage(result.StartDate, result.EndDate)
	if err != nil {
		return SessionActivity{}, fmt.Errorf("calculate session activity coverage: %w", err)
	}
	result.Coverage = coverage
	return result, nil
}

func (c *activityCollector) transitionDateExtrema(first, last string, start, end time.Time) (string, string) {
	for cursor := start; cursor.Before(end); {
		_, next := cursor.In(c.location).ZoneBounds()
		if next.IsZero() || !next.After(cursor) || !next.Before(end) {
			break
		}
		for _, date := range []string{
			next.Add(-time.Nanosecond).In(c.location).Format(activityDateFormat),
			next.In(c.location).Format(activityDateFormat),
		} {
			if date < first {
				first = date
			}
			if date > last {
				last = date
			}
		}
		cursor = next
	}
	return first, last
}

func (c *activityCollector) coverage(first, last string) ([]ActivityDayCoverage, error) {
	start, err := time.Parse(activityDateFormat, first)
	if err != nil {
		return nil, fmt.Errorf("parse first local date: %w", err)
	}
	end, err := time.Parse(activityDateFormat, last)
	if err != nil {
		return nil, fmt.Errorf("parse last local date: %w", err)
	}
	dates := map[string]struct{}{first: {}, last: {}}
	// Date-line shifts can skip or repeat a whole civil date, not just an hour.
	limit := end.Add(72 * time.Hour)
	for cursor := start.Add(-48 * time.Hour); cursor.Before(limit); {
		_, next := cursor.In(c.location).ZoneBounds()
		if next.IsZero() || !next.After(cursor) || !next.Before(limit) {
			break
		}
		before := next.Add(-time.Nanosecond).In(c.location).Format(activityDateFormat)
		after := next.In(c.location).Format(activityDateFormat)
		if before > after {
			before, after = after, before
		}
		transitionStart, err := time.Parse(activityDateFormat, before)
		if err != nil {
			return nil, fmt.Errorf("parse transition date: %w", err)
		}
		transitionEnd, err := time.Parse(activityDateFormat, after)
		if err != nil {
			return nil, fmt.Errorf("parse transition date: %w", err)
		}
		if transitionStart.Before(start) {
			transitionStart = start
		}
		if transitionEnd.After(end) {
			transitionEnd = end
		}
		for date := transitionStart; !date.After(transitionEnd); date = date.AddDate(0, 0, 1) {
			dates[date.Format(activityDateFormat)] = struct{}{}
		}
		cursor = next
	}
	ordered := make([]string, 0, len(dates))
	for date := range dates {
		ordered = append(ordered, date)
	}
	sort.Strings(ordered)
	coverage := make([]ActivityDayCoverage, 0, len(ordered))
	for _, date := range ordered {
		day, err := time.Parse(activityDateFormat, date)
		if err != nil {
			return nil, fmt.Errorf("parse coverage date: %w", err)
		}
		entry := ActivityDayCoverage{Date: date}
		for hour := range entry.Hours {
			entry.Hours[hour] = c.hourCoverage(day.Add(time.Duration(hour) * time.Hour))
		}
		coverage = append(coverage, entry)
	}
	return coverage, nil
}

// hourCoverage compares the actual UTC intervals mapping to a local clock hour.
func (c *activityCollector) hourCoverage(wall time.Time) string {
	var total, selected time.Duration
	limit := wall.Add(49 * time.Hour)
	for cursor := wall.Add(-48 * time.Hour); cursor.Before(limit); {
		local := cursor.In(c.location)
		_, offset := local.Zone()
		_, next := local.ZoneBounds()
		if next.IsZero() || next.After(limit) {
			next = limit
		}
		if !next.After(cursor) {
			break
		}
		start := wall.Add(-time.Duration(offset) * time.Second)
		end := start.Add(time.Hour)
		if start.Before(cursor) {
			start = cursor
		}
		if end.After(next) {
			end = next
		}
		if start.Before(end) {
			total += end.Sub(start)
			if !c.opts.Since.IsZero() && start.Before(c.opts.Since) {
				start = c.opts.Since
			}
			if !c.opts.Until.IsZero() && end.After(c.opts.Until) {
				end = c.opts.Until
			}
			if start.Before(end) {
				selected += end.Sub(start)
			}
		}
		cursor = next
	}
	switch {
	case total == 0:
		return "nonexistent"
	case selected == 0:
		return "outside"
	case selected < total:
		return "partial"
	default:
		return "full"
	}
}

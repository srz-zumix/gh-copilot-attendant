package sessionstats

import (
	"sort"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

// FusionUsageMetric distinguishes recorded subtotals from missing values.
type FusionUsageMetric[T int64 | float64] struct {
	Total           T
	ObservedRecords int
	CompleteRecords int
	ExcludedRecords int
}

// FusionUsageMetrics contains metrics derived only from tagged usage records.
type FusionUsageMetrics struct {
	AIU                 FusionUsageMetric[float64]
	PremiumRequests     FusionUsageMetric[float64]
	PromptTokens        FusionUsageMetric[int64]
	UncachedInputTokens FusionUsageMetric[int64]
	CacheReadTokens     FusionUsageMetric[int64]
	CacheWriteTokens    FusionUsageMetric[int64]
	OutputTokens        FusionUsageMetric[int64]
	APIDurationMs       FusionUsageMetric[float64]
}

// FusionUsageGroup aggregates tagged records by one recorded dimension.
type FusionUsageGroup struct {
	Key          string
	Sessions     int
	UsageRecords int
	Metrics      FusionUsageMetrics
}

// FusionUsageStats is a versioned, independently aggregated tagged-usage dataset.
type FusionUsageStats struct {
	Version      int
	Sessions     int
	UsageRecords int
	Metrics      FusionUsageMetrics
	ByModel      []FusionUsageGroup
	ModelGroups  int
	ByCWD        []FusionUsageGroup
	CWDGroups    int
	ByRole       []FusionUsageGroup
	RoleGroups   int
	PhaseGroups  int
}

type metricObservation[T int64 | float64] struct {
	total    T
	observed bool
	complete bool
	excluded bool
}

type usageObservation struct {
	aiu, premium, duration               metricObservation[float64]
	prompt, input, cacheRead, cacheWrite metricObservation[int64]
	output                               metricObservation[int64]
}

func valueOrZero[T int64 | float64](value *T) T {
	if value != nil {
		return *value
	}
	var zero T
	return zero
}

func scalar[T int64 | float64](value *T) metricObservation[T] {
	return metricObservation[T]{total: valueOrZero(value), observed: value != nil, complete: value != nil}
}

func (u rawUsage) role(ev rawEvent) string {
	if ev.AgentID != "" || u.ParentToolCallID != "" || u.Initiator == "sub-agent" ||
		u.InteractionType == "conversation-subagent" {
		return copilotext.UsageRoleSubagent
	}
	switch u.Initiator {
	case "user", "agent":
		return copilotext.UsageRoleMain
	default:
		return copilotext.UsageRoleUnknown
	}
}

type tokenObservation struct {
	total   int64
	present bool
	seen    bool
	missing bool
}

func (t *tokenObservation) add(value *int64) {
	t.present = true
	if value == nil {
		t.missing = true
		return
	}
	t.total += *value
	t.seen = true
}

func (t tokenObservation) metric() metricObservation[int64] {
	return metricObservation[int64]{total: t.total, observed: t.seen, complete: t.present && !t.missing}
}

func (u rawUsage) metrics() usageObservation {
	var result usageObservation
	if u.CopilotUsage != nil && u.CopilotUsage.TotalNanoAIU != nil {
		result.aiu = metricObservation[float64]{
			total: float64(*u.CopilotUsage.TotalNanoAIU) / 1e9, observed: true, complete: true,
		}
	}
	if u.Initiator == "" || u.Initiator == "user" {
		result.premium = scalar(u.Cost)
	} else {
		// The shared collector excludes non-user initiators from this multiplier.
		result.premium = metricObservation[float64]{excluded: true}
	}
	result.duration = scalar(u.Duration)
	if u.CopilotUsage != nil && len(u.CopilotUsage.TokenDetails) > 0 {
		var input, read, write, output tokenObservation
		unknown := false
		for _, token := range u.CopilotUsage.TokenDetails {
			switch token.TokenType {
			case "input":
				input.add(token.TokenCount)
			case "cache_read":
				read.add(token.TokenCount)
			case "cache_write":
				write.add(token.TokenCount)
			case "output":
				output.add(token.TokenCount)
			default:
				unknown = true
			}
		}
		result.input = input.metric()
		result.cacheRead = read.metric()
		result.cacheWrite = write.metric()
		result.output = output.metric()
		result.output.complete = result.output.complete && !unknown
		result.prompt = metricObservation[int64]{
			total:    input.total + read.total + write.total,
			observed: input.seen || read.seen || write.seen,
			complete: result.input.complete && result.cacheRead.complete && result.cacheWrite.complete && !unknown,
		}
		return result
	}
	result.prompt = scalar(u.InputTokens)
	result.cacheRead = scalar(u.CacheReadTokens)
	result.cacheWrite = scalar(u.CacheWriteTokens)
	result.output = scalar(u.OutputTokens)
	if u.InputTokens != nil && u.CacheReadTokens != nil && u.CacheWriteTokens != nil {
		result.input = metricObservation[int64]{
			total: *u.InputTokens - *u.CacheReadTokens - *u.CacheWriteTokens, observed: true, complete: true,
		}
	}
	return result
}

func (m *FusionUsageMetric[T]) add(observation metricObservation[T]) {
	m.Total += observation.total
	if observation.observed {
		m.ObservedRecords++
	}
	if observation.complete {
		m.CompleteRecords++
	}
	if observation.excluded {
		m.ExcludedRecords++
	}
}

func (m *FusionUsageMetrics) add(record usageObservation) {
	m.AIU.add(record.aiu)
	m.PremiumRequests.add(record.premium)
	m.PromptTokens.add(record.prompt)
	m.UncachedInputTokens.add(record.input)
	m.CacheReadTokens.add(record.cacheRead)
	m.CacheWriteTokens.add(record.cacheWrite)
	m.OutputTokens.add(record.output)
	m.APIDurationMs.add(record.duration)
}

type fusionUsageCounter struct {
	value    FusionUsageGroup
	sessions map[string]struct{}
}

func newFusionUsageCounter(key string) *fusionUsageCounter {
	return &fusionUsageCounter{value: FusionUsageGroup{Key: key}, sessions: make(map[string]struct{})}
}

func (c *fusionUsageCounter) add(session string, record usageObservation) {
	c.sessions[session] = struct{}{}
	c.value.UsageRecords++
	c.value.Metrics.add(record)
}

type fusionUsageCollector struct {
	total   *fusionUsageCounter
	byModel map[string]*fusionUsageCounter
	byCWD   map[string]*fusionUsageCounter
	byRole  map[string]*fusionUsageCounter
}

func newFusionUsageCollector() *fusionUsageCollector {
	return &fusionUsageCollector{
		total:   newFusionUsageCounter(""),
		byModel: make(map[string]*fusionUsageCounter),
		byCWD:   make(map[string]*fusionUsageCounter),
		byRole:  make(map[string]*fusionUsageCounter),
	}
}

func addUsageGroup(groups map[string]*fusionUsageCounter, key, session string, record usageObservation) {
	entry := groups[key]
	if entry == nil {
		entry = newFusionUsageCounter(key)
		groups[key] = entry
	}
	entry.add(session, record)
}

func (c *fusionUsageCollector) add(session, cwd, model, role string, record usageObservation) {
	c.total.add(session, record)
	addUsageGroup(c.byModel, model, session, record)
	addUsageGroup(c.byCWD, cwd, session, record)
	addUsageGroup(c.byRole, role, session, record)
}

func finishUsageGroups(groups map[string]*fusionUsageCounter, top int) []FusionUsageGroup {
	result := make([]FusionUsageGroup, 0, len(groups))
	for _, entry := range groups {
		entry.value.Sessions = len(entry.sessions)
		result = append(result, entry.value)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if (left.Metrics.AIU.ObservedRecords > 0) != (right.Metrics.AIU.ObservedRecords > 0) {
			return left.Metrics.AIU.ObservedRecords > 0
		}
		if left.Metrics.AIU.Total != right.Metrics.AIU.Total {
			return left.Metrics.AIU.Total > right.Metrics.AIU.Total
		}
		if left.UsageRecords != right.UsageRecords {
			return left.UsageRecords > right.UsageRecords
		}
		return left.Key < right.Key
	})
	if top > 0 && len(result) > top {
		result = result[:top]
	}
	return result
}

func (c *fusionUsageCollector) finish(top, phaseGroups int) FusionUsageStats {
	return FusionUsageStats{
		Version: 1, Sessions: len(c.total.sessions), UsageRecords: c.total.value.UsageRecords,
		Metrics: c.total.value.Metrics,
		ByModel: finishUsageGroups(c.byModel, top), ModelGroups: len(c.byModel),
		ByCWD: finishUsageGroups(c.byCWD, top), CWDGroups: len(c.byCWD),
		ByRole: finishUsageGroups(c.byRole, top), RoleGroups: len(c.byRole),
		PhaseGroups: phaseGroups,
	}
}

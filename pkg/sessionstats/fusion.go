package sessionstats

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
	"github.com/srz-zumix/go-gh-extension/pkg/logger"
)

// Stats extends the shared session statistics without changing their JSON fields.
type Stats struct {
	*copilotext.PermissionStats
	Fusion FusionStats
}

// FusionStats reports only information explicitly recorded in Fusion events.
type FusionStats struct {
	Sessions                    int
	ResolvedTurns               int
	UnidentifiedResolutions     int
	TurnsWithUsage              int
	PhasesWithUsage             int
	UsageRecords                int
	AIU                         float64
	AIURecords                  int
	MissingFusionIDRecords      int
	MissingPhaseIdentityRecords int
	ByPattern                   []FusionPattern
	ByPhase                     []FusionPhase
	Usage                       FusionUsageStats
}

// FusionPattern counts resolved Fusion turns and resolutions without identifiers.
type FusionPattern struct {
	Key                     string
	Sessions                int
	ResolvedTurns           int
	UnidentifiedResolutions int
}

// FusionPhase groups recorded usage by Fusion pattern, phase, role, and model.
type FusionPhase struct {
	Pattern                     string
	Phase                       string
	Role                        string
	Model                       string
	Sessions                    int
	Turns                       int
	Phases                      int
	UsageRecords                int
	AIU                         float64
	AIURecords                  int
	MissingFusionIDRecords      int
	MissingPhaseIdentityRecords int
}

// Collect combines the shared session statistics with recorded Fusion activity.
func Collect(opts copilotext.PermissionStatsOptions) (*Stats, error) {
	type fusionResult struct {
		stats FusionStats
		err   error
	}
	fusionResults := make(chan fusionResult, 1)
	go func() {
		fusion, err := CollectFusion(opts)
		fusionResults <- fusionResult{stats: fusion, err: err}
	}()
	stats, err := copilotext.CollectPermissionStats(opts)
	fusion := <-fusionResults
	if err != nil {
		return nil, err
	}
	if fusion.err != nil {
		return nil, fusion.err
	}
	return &Stats{PermissionStats: stats, Fusion: fusion.stats}, nil
}

type rawEvent struct {
	ID        string          `json:"id"`
	AgentID   string          `json:"agentId"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

type rawResolution struct {
	FusionID string `json:"fusionId"`
	Pattern  string `json:"pattern"`
}

type rawFusion struct {
	FusionID  string `json:"fusionId"`
	Pattern   string `json:"pattern"`
	PhaseID   string `json:"phaseId"`
	PhaseKind string `json:"phaseKind"`
	Role      string `json:"role"`
}

type rawUsage struct {
	Model            string   `json:"model"`
	Initiator        string   `json:"initiator"`
	InteractionType  string   `json:"interactionType"`
	ParentToolCallID string   `json:"parentToolCallId"`
	APICallID        string   `json:"apiCallId"`
	InputTokens      *int64   `json:"inputTokens"`
	CacheReadTokens  *int64   `json:"cacheReadTokens"`
	CacheWriteTokens *int64   `json:"cacheWriteTokens"`
	OutputTokens     *int64   `json:"outputTokens"`
	Cost             *float64 `json:"cost"`
	Duration         *float64 `json:"duration"`
	Accounting       struct {
		SourceSessionID string `json:"sourceSessionId"`
		UsageID         string `json:"usageId"`
		Sequence        *int64 `json:"sequence"`
	} `json:"accounting"`
	CopilotUsage *struct {
		TotalNanoAIU *int64 `json:"totalNanoAiu"`
		TokenDetails []struct {
			TokenType  string `json:"tokenType"`
			TokenCount *int64 `json:"tokenCount"`
		} `json:"tokenDetails"`
	} `json:"copilotUsage"`
	Fusion *rawFusion `json:"fusion"`
}

// identity follows the shared usage collector's accounting precedence.
func (u rawUsage) identity(ev rawEvent, sessionID string) string {
	source := u.Accounting.SourceSessionID
	if source == "" {
		source = ev.AgentID
	}
	if source == "" {
		source = sessionID
	}
	if u.Accounting.UsageID != "" {
		return "usage:" + source + ":" + u.Accounting.UsageID
	}
	if u.Accounting.Sequence != nil {
		return "sequence:" + source + ":" + strconv.FormatInt(*u.Accounting.Sequence, 10)
	}
	if u.APICallID != "" {
		return "api:" + source + ":" + u.APICallID
	}
	if ev.ID != "" {
		return "event:" + ev.ID
	}
	return ""
}

// validate rejects known invalid values before deduplication without treating missing values as zero.
func (u rawUsage) validate() error {
	if u.Model == "" && u.Fusion == nil {
		return fmt.Errorf("missing model")
	}
	if valueOrZero(u.Duration) < 0 || ((u.Initiator == "" || u.Initiator == "user") && valueOrZero(u.Cost) < 0) {
		return fmt.Errorf("negative usage value")
	}
	if u.CopilotUsage != nil {
		if u.CopilotUsage.TotalNanoAIU != nil && *u.CopilotUsage.TotalNanoAIU < 0 {
			return fmt.Errorf("negative usage value")
		}
		if len(u.CopilotUsage.TokenDetails) > 0 {
			for _, token := range u.CopilotUsage.TokenDetails {
				if valueOrZero(token.TokenCount) < 0 {
					return fmt.Errorf("negative token count")
				}
			}
			return nil
		}
	}
	input, read, write := valueOrZero(u.InputTokens), valueOrZero(u.CacheReadTokens), valueOrZero(u.CacheWriteTokens)
	if input < 0 || read < 0 || write < 0 || valueOrZero(u.OutputTokens) < 0 ||
		(u.InputTokens != nil && (read > input || write > input-read)) {
		return fmt.Errorf("negative usage value")
	}
	return nil
}

type phaseKey struct {
	pattern string
	phase   string
	role    string
	model   string
}

type turnIdentity struct {
	session string
	fusion  string
}

type phaseIdentity struct {
	turnIdentity
	phase string
}

type patternCounter struct {
	value    FusionPattern
	sessions map[string]struct{}
}

type phaseCounter struct {
	value    FusionPhase
	sessions map[string]struct{}
	turns    map[turnIdentity]struct{}
	phases   map[phaseIdentity]struct{}
}

type fusionCollector struct {
	opts     copilotext.PermissionStatsOptions
	stats    FusionStats
	patterns map[string]*patternCounter
	phases   map[phaseKey]*phaseCounter
	turns    map[turnIdentity]struct{}
	phaseIDs map[phaseIdentity]struct{}
	usage    *fusionUsageCollector
}

type sessionCollector struct {
	collector    *fusionCollector
	session      copilotext.Session
	seenUsage    map[string]struct{}
	seenResolved map[string]struct{}
	hasActivity  bool
}

// CollectFusion scans the same session scope as the shared collector. Time bounds
// select individual resolution and usage-record timestamps.
func CollectFusion(opts copilotext.PermissionStatsOptions) (FusionStats, error) {
	root := opts.Root
	if root == "" {
		home, err := copilotext.CopilotHome()
		if err != nil {
			return FusionStats{}, err
		}
		root = filepath.Join(home, "session-state")
	}
	sessions, err := copilotext.ListSessions(root)
	if err != nil {
		return FusionStats{}, err
	}

	c := fusionCollector{
		opts: opts, patterns: make(map[string]*patternCounter), phases: make(map[phaseKey]*phaseCounter),
		turns: make(map[turnIdentity]struct{}), phaseIDs: make(map[phaseIdentity]struct{}),
		usage: newFusionUsageCollector(),
	}
	for _, s := range sessions {
		if opts.SessionID != "" && s.ID != opts.SessionID {
			continue
		}
		matches, err := matchesScope(opts.Scope, s.CWD)
		if err != nil {
			return FusionStats{}, fmt.Errorf("match session %q scope: %w", s.ID, err)
		}
		if !matches {
			continue
		}
		sc := sessionCollector{
			collector: &c, session: s, seenUsage: make(map[string]struct{}),
			seenResolved: make(map[string]struct{}),
		}
		if err := sc.scan(); err != nil {
			return FusionStats{}, fmt.Errorf("read Fusion events for session %q: %w", s.ID, err)
		}
		if sc.hasActivity {
			c.stats.Sessions++
		}
	}
	return c.finish(), nil
}

func matchesScope(scope copilotext.SessionScope, cwd string) (bool, error) {
	if scope.ExactCWD == "" && scope.UnderDir == "" {
		return true, nil
	}
	if cwd == "" {
		return false, nil
	}
	resolved, err := copilotext.ResolveSessionScope(context.Background(), false, cwd, "")
	if err != nil {
		return false, err
	}
	if scope.ExactCWD != "" {
		return resolved.ExactCWD == scope.ExactCWD, nil
	}
	rel, err := filepath.Rel(scope.UnderDir, resolved.ExactCWD)
	if err != nil {
		return false, nil
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func (sc *sessionCollector) scan() (rerr error) {
	path := filepath.Join(sc.session.Dir, "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil && rerr == nil {
			rerr = fmt.Errorf("close %q: %w", path, err)
		}
	}()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Avoid decoding unrelated event payloads, while accepting escaped JSON type names.
		if !bytes.Contains(line, []byte(`"session.fusion_resolved"`)) &&
			!bytes.Contains(line, []byte(`"session.usage_record"`)) &&
			!bytes.Contains(line, []byte(`\u`)) {
			if !json.Valid(line) {
				logger.Warn("skipping unparsable Fusion session event", "dir", sc.session.Dir, "error", "invalid JSON")
			}
			continue
		}
		var ev rawEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			logger.Warn("skipping unparsable Fusion session event", "dir", sc.session.Dir, "error", err)
			continue
		}
		switch ev.Type {
		case "session.fusion_resolved":
			sc.resolved(ev)
		case "session.usage_record":
			sc.usage(ev)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}
	return nil
}

func (sc *sessionCollector) within(ev rawEvent) bool {
	opts := sc.collector.opts
	if ev.Timestamp.IsZero() && (!opts.Since.IsZero() || !opts.Until.IsZero()) {
		logger.Warn("skipping Fusion event without a timestamp in a bounded window", "dir", sc.session.Dir, "type", ev.Type)
		return false
	}
	return (opts.Since.IsZero() || !ev.Timestamp.Before(opts.Since)) &&
		(opts.Until.IsZero() || ev.Timestamp.Before(opts.Until))
}

func recorded(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func (sc *sessionCollector) resolved(ev rawEvent) {
	var data *rawResolution
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		logger.Warn("skipping invalid Fusion resolution", "dir", sc.session.Dir, "error", err)
		return
	}
	if data == nil {
		logger.Warn("skipping Fusion resolution without data", "dir", sc.session.Dir)
		return
	}
	key := ""
	if data.FusionID != "" {
		key = "fusion:" + data.FusionID
	} else if ev.ID != "" {
		key = "event:" + ev.ID
	}
	if key != "" {
		if _, ok := sc.seenResolved[key]; ok {
			return
		}
		sc.seenResolved[key] = struct{}{}
	}
	c := sc.collector
	if !sc.within(ev) {
		return
	}
	sc.hasActivity = true
	pattern := recorded(data.Pattern)
	entry := c.patterns[pattern]
	if entry == nil {
		entry = &patternCounter{value: FusionPattern{Key: pattern}, sessions: make(map[string]struct{})}
		c.patterns[pattern] = entry
	}
	entry.sessions[sc.session.Dir] = struct{}{}
	if data.FusionID == "" {
		c.stats.UnidentifiedResolutions++
		entry.value.UnidentifiedResolutions++
		return
	}
	c.stats.ResolvedTurns++
	entry.value.ResolvedTurns++
}

func (sc *sessionCollector) usage(ev rawEvent) {
	var data struct {
		Usage *rawUsage `json:"usage"`
	}
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		logger.Warn("skipping unparsable Fusion usage record", "dir", sc.session.Dir, "error", err)
		return
	}
	if data.Usage == nil {
		logger.Warn("skipping Fusion usage record without usage", "dir", sc.session.Dir)
		return
	}
	u := *data.Usage
	if err := u.validate(); err != nil {
		logger.Warn("skipping invalid Fusion usage record", "dir", sc.session.Dir, "error", err)
		return
	}
	// The first valid copy of an identity is canonical, matching the shared usage
	// collector: later replays neither add a Fusion tag nor move the record into
	// another time window, so disjoint windows never count the same record twice.
	key := u.identity(ev, sc.session.ID)
	if key != "" {
		if _, ok := sc.seenUsage[key]; ok {
			return
		}
		sc.seenUsage[key] = struct{}{}
	}
	c := sc.collector
	if u.Fusion == nil || !sc.within(ev) {
		return
	}
	sc.hasActivity = true
	c.usage.add(sc.session.Dir, sc.session.CWD, u.Model, u.role(ev), u.metrics())
	f := u.Fusion
	group := phaseKey{recorded(f.Pattern), recorded(f.PhaseKind), recorded(f.Role), u.Model}
	entry := c.phases[group]
	if entry == nil {
		entry = &phaseCounter{
			value:    FusionPhase{Pattern: group.pattern, Phase: group.phase, Role: group.role, Model: group.model},
			sessions: make(map[string]struct{}), turns: make(map[turnIdentity]struct{}), phases: make(map[phaseIdentity]struct{}),
		}
		c.phases[group] = entry
	}
	entry.sessions[sc.session.Dir] = struct{}{}
	entry.value.UsageRecords++
	c.stats.UsageRecords++
	if u.CopilotUsage != nil && u.CopilotUsage.TotalNanoAIU != nil {
		aiu := float64(*u.CopilotUsage.TotalNanoAIU) / 1e9
		entry.value.AIU += aiu
		entry.value.AIURecords++
		c.stats.AIU += aiu
		c.stats.AIURecords++
	}
	if f.FusionID == "" {
		entry.value.MissingFusionIDRecords++
		c.stats.MissingFusionIDRecords++
	} else {
		turn := turnIdentity{session: sc.session.Dir, fusion: f.FusionID}
		entry.turns[turn] = struct{}{}
		c.turns[turn] = struct{}{}
		if f.PhaseID != "" {
			phase := phaseIdentity{turnIdentity: turn, phase: f.PhaseID}
			entry.phases[phase] = struct{}{}
			c.phaseIDs[phase] = struct{}{}
		}
	}
	if f.FusionID == "" || f.PhaseID == "" {
		entry.value.MissingPhaseIdentityRecords++
		c.stats.MissingPhaseIdentityRecords++
	}
}

func (c *fusionCollector) finish() FusionStats {
	c.stats.TurnsWithUsage = len(c.turns)
	c.stats.PhasesWithUsage = len(c.phaseIDs)
	c.stats.Usage = c.usage.finish(c.opts.Top, len(c.phases))
	c.stats.ByPattern = make([]FusionPattern, 0, len(c.patterns))
	for _, entry := range c.patterns {
		entry.value.Sessions = len(entry.sessions)
		c.stats.ByPattern = append(c.stats.ByPattern, entry.value)
	}
	sort.Slice(c.stats.ByPattern, func(i, j int) bool {
		left, right := c.stats.ByPattern[i], c.stats.ByPattern[j]
		if left.ResolvedTurns != right.ResolvedTurns {
			return left.ResolvedTurns > right.ResolvedTurns
		}
		return left.Key < right.Key
	})
	c.stats.ByPhase = make([]FusionPhase, 0, len(c.phases))
	for _, entry := range c.phases {
		entry.value.Sessions = len(entry.sessions)
		entry.value.Turns = len(entry.turns)
		entry.value.Phases = len(entry.phases)
		c.stats.ByPhase = append(c.stats.ByPhase, entry.value)
	}
	sort.Slice(c.stats.ByPhase, func(i, j int) bool {
		left, right := c.stats.ByPhase[i], c.stats.ByPhase[j]
		if left.UsageRecords != right.UsageRecords {
			return left.UsageRecords > right.UsageRecords
		}
		if left.Pattern != right.Pattern {
			return left.Pattern < right.Pattern
		}
		if left.Phase != right.Phase {
			return left.Phase < right.Phase
		}
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		return left.Model < right.Model
	})
	if c.opts.Top > 0 {
		if len(c.stats.ByPattern) > c.opts.Top {
			c.stats.ByPattern = c.stats.ByPattern[:c.opts.Top]
		}
		if len(c.stats.ByPhase) > c.opts.Top {
			c.stats.ByPhase = c.stats.ByPhase[:c.opts.Top]
		}
	}
	return c.stats
}

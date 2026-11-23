package sessionstats

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/srz-zumix/gh-copilot-attendant/pkg/permissionhook"
	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
)

const maxHookRecordBytes = 1024

// HookActivity describes recorded allow and deny hook decisions, independently of permission requests.
type HookActivity struct {
	Version int
	State   string
	Data    *HookActivityData
}

// HookActivityData contains observations from a readable snapshot of the hook log.
type HookActivityData struct {
	Invocations         int
	PermissionRequests  int
	PreToolUses         int
	Decisions           HookDecisions
	Outputs             HookOutputs
	Processing          HookProcessing
	Unidentified        HookUnidentified
	Attribution         string
	MalformedRecords    int
	UnsupportedVersions int
	ByTool              []HookToolCount
	ByDecisionTool      HookDecisionTools
	// ByTarget groups allowed and denied invocations by tool, decision, and target.
	ByTarget []HookTargetCount
}

// HookDecisions counts decisions calculated by the hook.
type HookDecisions struct {
	Allow int
	Deny  int
	None  int
}

// HookOutputs counts whether the hook attempted and completed its output.
type HookOutputs struct {
	NotAttempted   int
	WriteSucceeded int
	WriteFailed    int
}

// HookProcessing counts the hook's processing outcomes.
type HookProcessing struct {
	Success     int
	InputError  int
	OutputError int
}

// HookUnidentified counts time-window records that cannot be attributed to the selected scope.
type HookUnidentified struct {
	Missing     int
	Unmatched   int
	Ambiguous   int
	Unscopable  int
	Unavailable int
}

// HookToolCount is a per-tool invocation count; an empty key denotes an unknown tool.
type HookToolCount struct {
	Key   string
	Total int
}

// HookDecisionTools groups tool counts separately for each calculated decision.
type HookDecisionTools struct {
	Allow []HookToolCount
	Deny  []HookToolCount
}

// HookTargetCount counts decided invocations with the same tool, decision, and target.
// An empty Target means the record has no target, as for older logs or tools without one.
type HookTargetCount struct {
	Tool      string
	Decision  string
	Target    string
	Truncated bool
	Total     int
}

type hookTargetKey struct {
	tool, decision, target string
	truncated              bool
}

type hookSession struct {
	session    copilotext.Session
	matches    bool
	unscopable bool
	duplicates int
}

// CollectHookActivity reads the profile's hook log without modifying it.
// A custom session-state root selects the hook log in its parent profile.
func CollectHookActivity(opts copilotext.PermissionStatsOptions) HookActivity {
	result := HookActivity{Version: 1, State: "unreadable"}
	root := opts.Root
	var home string
	var err error
	if root == "" {
		home, err = copilotext.CopilotHome()
		if err != nil {
			return result
		}
		root = filepath.Join(home, "session-state")
	} else {
		root, err = filepath.Abs(root)
		if err != nil {
			return result
		}
		home = filepath.Dir(root)
	}
	path, err := permissionhook.HookLogPathForHome(home)
	if err != nil {
		return result
	}
	file, err := permissionhook.OpenHookLog(path)
	if err != nil {
		switch {
		case errors.Is(err, permissionhook.ErrLogUnsupported):
			result.State = "unsupported"
		case errors.Is(err, os.ErrNotExist):
			result.State = "missing"
		}
		return result
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return result
	}

	data := &HookActivityData{Attribution: "available", ByTool: []HookToolCount{}, ByTarget: []HookTargetCount{}}
	sessions, err := copilotext.ListSessions(root)
	if err != nil {
		data.Attribution = "unavailable"
	}
	index := hookSessionIndex(sessions, opts.Scope)
	tools := make(map[string]int)
	allowTools := make(map[string]int)
	denyTools := make(map[string]int)
	targets := make(map[hookTargetKey]int)
	scoped := opts.SessionID != "" || opts.Scope.ExactCWD != "" || opts.Scope.UnderDir != ""
	complete := scanHookLog(file, info.Size(), data, func(record permissionhook.HookRecord) {
		if record.Decision == "none" {
			return
		}
		if (!opts.Since.IsZero() && record.Timestamp.Before(opts.Since)) ||
			(!opts.Until.IsZero() && !record.Timestamp.Before(opts.Until)) {
			return
		}
		identified := false
		if record.SessionID == "" {
			data.Unidentified.Missing++
		} else if data.Attribution == "unavailable" {
			data.Unidentified.Unavailable++
		} else if session, ok := index[record.SessionID]; !ok {
			data.Unidentified.Unmatched++
		} else if session.duplicates > 1 {
			data.Unidentified.Ambiguous++
		} else if session.unscopable {
			data.Unidentified.Unscopable++
		} else {
			identified = true
			if (opts.SessionID != "" && record.SessionID != opts.SessionID) || !session.matches {
				return
			}
		}
		if scoped && !identified {
			return
		}
		data.add(record)
		tools[record.Tool]++
		switch record.Decision {
		case "allow":
			allowTools[record.Tool]++
		case "deny":
			denyTools[record.Tool]++
		}
		targets[hookTargetKey{record.Tool, record.Decision, record.Target, record.TargetTruncated}]++
	})
	data.ByTool = hookToolCounts(tools, opts.Top)
	data.ByDecisionTool = HookDecisionTools{
		Allow: hookToolCounts(allowTools, opts.Top),
		Deny:  hookToolCounts(denyTools, opts.Top),
	}
	for key, total := range targets {
		data.ByTarget = append(data.ByTarget, HookTargetCount{
			Tool: key.tool, Decision: key.decision, Target: key.target, Truncated: key.truncated, Total: total,
		})
	}
	sort.Slice(data.ByTarget, func(i, j int) bool {
		a, b := data.ByTarget[i], data.ByTarget[j]
		if a.Total != b.Total {
			return a.Total > b.Total
		}
		if a.Decision != b.Decision {
			return a.Decision < b.Decision
		}
		if a.Tool != b.Tool {
			return a.Tool < b.Tool
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return !a.Truncated && b.Truncated
	})
	if opts.Top > 0 && len(data.ByTarget) > opts.Top {
		data.ByTarget = data.ByTarget[:opts.Top]
	}
	result.State = "available"
	if !complete {
		result.State = "partial"
	}
	result.Data = data
	return result
}

func hookToolCounts(tools map[string]int, top int) []HookToolCount {
	rows := make([]HookToolCount, 0, len(tools))
	for key, total := range tools {
		rows = append(rows, HookToolCount{Key: key, Total: total})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		return rows[i].Key < rows[j].Key
	})
	if top > 0 && len(rows) > top {
		rows = rows[:top]
	}
	return rows
}

func hookSessionIndex(sessions []copilotext.Session, scope copilotext.SessionScope) map[string]hookSession {
	index := make(map[string]hookSession, len(sessions))
	for _, session := range sessions {
		if session.ID == "" {
			continue
		}
		entry := index[session.ID]
		entry.duplicates++
		entry.session = session
		index[session.ID] = entry
	}
	for id, entry := range index {
		if entry.duplicates == 1 {
			if (scope.ExactCWD != "" || scope.UnderDir != "") && entry.session.CWD == "" {
				entry.unscopable = true
			} else {
				var err error
				entry.matches, err = matchesScope(scope, entry.session.CWD)
				entry.unscopable = err != nil
			}
		}
		index[id] = entry
	}
	return index
}

func (d *HookActivityData) add(record permissionhook.HookRecord) {
	d.Invocations++
	switch record.Event {
	case "permissionRequest":
		d.PermissionRequests++
	case "preToolUse":
		d.PreToolUses++
	}
	switch record.Decision {
	case "allow":
		d.Decisions.Allow++
	case "deny":
		d.Decisions.Deny++
	case "none":
		d.Decisions.None++
	}
	switch record.Output {
	case "not_attempted":
		d.Outputs.NotAttempted++
	case "write_succeeded":
		d.Outputs.WriteSucceeded++
	case "write_failed":
		d.Outputs.WriteFailed++
	}
	switch record.Status {
	case "success":
		d.Processing.Success++
	case "input_error":
		d.Processing.InputError++
	case "output_error":
		d.Processing.OutputError++
	}
}

// scanHookLog consumes at most the initial file size and recovers after bad lines.
func scanHookLog(source io.Reader, size int64, data *HookActivityData, accept func(permissionhook.HookRecord)) bool {
	limited := &io.LimitedReader{R: source, N: size}
	reader := bufio.NewReaderSize(limited, maxHookRecordBytes+1)
	for {
		line, err := reader.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = reader.ReadSlice('\n')
			}
			data.MalformedRecords++
			if err == nil {
				continue
			}
			return errors.Is(err, io.EOF) && limited.N == 0
		}
		if err != nil {
			if len(bytes.TrimSpace(line)) != 0 {
				data.MalformedRecords++
			}
			return errors.Is(err, io.EOF) && limited.N == 0
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if len(line) > maxHookRecordBytes {
			data.MalformedRecords++
			continue
		}
		var envelope struct {
			Version *int `json:"version"`
		}
		if json.Unmarshal(line, &envelope) != nil || envelope.Version == nil || *envelope.Version < 1 {
			data.MalformedRecords++
			continue
		}
		if *envelope.Version != 1 {
			data.UnsupportedVersions++
			continue
		}
		var record permissionhook.HookRecord
		if json.Unmarshal(line, &record) != nil || !record.Valid() {
			data.MalformedRecords++
			continue
		}
		accept(record)
	}
}

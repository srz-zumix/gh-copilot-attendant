package permissionhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"
)

// maxRecordBytes is the largest serialized record, excluding its delimiting newlines.
const maxRecordBytes = 1024

var safeToolPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var sessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ErrLogUnsupported indicates that private hook logging is unavailable in this build.
var ErrLogUnsupported = errors.New("private hook logging is unsupported on this platform")

// HookRecord describes a hook invocation, not the CLI's final permission decision.
type HookRecord struct {
	Version   int       `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Event     string    `json:"event"`
	Tool      string    `json:"tool,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Decision  string    `json:"decision"`
	Status    string    `json:"status"`
	Output    string    `json:"output"`
	// Target is the command, paths, or URL of an allowed or denied call.
	Target string `json:"target,omitempty"`
	// TargetTruncated reports that Target was shortened to fit the record.
	TargetTruncated bool `json:"target_truncated,omitempty"`
}

// Valid reports whether a version 1 record could have been produced by the hook.
func (r HookRecord) Valid() bool {
	if r.Version != 1 || r.Timestamp.IsZero() ||
		(r.Event != "permissionRequest" && r.Event != "preToolUse") ||
		(r.Tool != "" && !safeToolPattern.MatchString(r.Tool)) ||
		(r.SessionID != "" && !sessionIDPattern.MatchString(r.SessionID)) {
		return false
	}
	if r.Decision != "none" && r.Decision != "allow" && r.Decision != "deny" {
		return false
	}
	if (r.Decision == "none" && (r.Target != "" || r.TargetTruncated)) || !utf8.ValidString(r.Target) {
		return false
	}
	switch r.Status {
	case "success":
		return (r.Decision == "none" && r.Output == "not_attempted") ||
			(r.Decision != "none" && r.Output == "write_succeeded")
	case "input_error":
		return r.Decision == "none" && r.Output == "not_attempted"
	case "output_error":
		return r.Decision != "none" && r.Output == "write_failed"
	default:
		return false
	}
}

func safeToolName(name string) string {
	if safeToolPattern.MatchString(name) {
		return name
	}
	return ""
}

func sessionIDFromInput(top map[string]json.RawMessage) string {
	var id string
	for _, key := range []string{"sessionId", "session_id"} {
		raw, ok := top[key]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) != nil || !sessionIDPattern.MatchString(value) || (id != "" && id != value) {
			return ""
		}
		id = value
	}
	return id
}

// HookLogPath identifies the log for the current Copilot profile without creating it.
func HookLogPath() (string, error) {
	home := os.Getenv("COPILOT_HOME")
	if home == "" {
		base, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(base, ".copilot")
	}
	return HookLogPathForHome(home)
}

// HookLogPathForHome identifies the hook log belonging to a Copilot profile.
func HookLogPathForHome(home string) (string, error) {
	return filepath.Abs(filepath.Join(home, "gh-copilot-attendant", "permission-hook", "events.jsonl"))
}

// OpenHookLog opens an existing private hook log without changing its contents.
func OpenHookLog(path string) (*os.File, error) {
	return openPrivateLog(path)
}

func appendHookRecord(record HookRecord) error {
	path, err := HookLogPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	// JSON escaping can expand the target, so shrink it until the record fits.
	for len(data) > maxRecordBytes && record.Target != "" {
		record.Target, _ = truncateTarget(record.Target, len(record.Target)/2)
		record.TargetTruncated = true
		if data, err = json.Marshal(record); err != nil {
			return err
		}
	}
	if len(data) > maxRecordBytes {
		return fmt.Errorf("hook log record too large")
	}
	// A leading newline lets a reader recover after an interrupted prior write.
	data = append(append([]byte{'\n'}, data...), '\n')
	return appendPrivateRecord(path, data)
}

package permissionhook

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// maxTargetBytes bounds the recorded target before JSON escaping.
const maxTargetBytes = 512

// decisionTarget describes what a decided tool call acted on: the command line for shell
// tools, the paths for path-scoped tools, or the URL for web_fetch. Other tools have no
// target because their name identifies the decision.
func decisionTarget(req *Request) string {
	if req == nil {
		return ""
	}
	if shellTools[req.ToolName] {
		return redactBashAssignments(req.Command)
	}
	var keys []string
	switch {
	case req.ToolName == "view":
		keys = []string{"path"}
	case isPathScopedTool(req.ToolName):
		keys = []string{"paths"}
	case req.ToolName == "web_fetch":
		keys = []string{"url"}
	default:
		return ""
	}
	var values []string
	for _, key := range keys {
		raw, ok := req.Arguments[key]
		if !ok {
			continue
		}
		var single string
		if json.Unmarshal(raw, &single) == nil {
			values = append(values, single)
			continue
		}
		var many []string
		if json.Unmarshal(raw, &many) == nil {
			values = append(values, many...)
		}
	}
	if len(values) == 0 && isPathScopedTool(req.ToolName) {
		values = append(values, req.CWD)
	}
	return strings.Join(values, " ")
}

func redactBashAssignments(command string) string {
	segments, err := parseBashCommand(command)
	if err != nil {
		return ""
	}
	var spans [][2]int
	for _, segment := range segments {
		for _, word := range segment {
			if assignment := word.assignment; assignment != nil {
				if assignment.valueStart < word.start || assignment.valueEnd > word.end ||
					assignment.valueStart > assignment.valueEnd || assignment.valueEnd > len(command) {
					return ""
				}
				spans = append(spans, [2]int{assignment.valueStart, assignment.valueEnd})
			}
		}
	}
	if len(spans) == 0 {
		return command
	}
	var out strings.Builder
	last := 0
	for _, span := range spans {
		if span[0] < last {
			return ""
		}
		out.WriteString(command[last:span[0]])
		out.WriteString("[REDACTED]")
		last = span[1]
	}
	out.WriteString(command[last:])
	return out.String()
}

// truncateTarget shortens s to at most limit bytes without splitting a UTF-8 sequence.
func truncateTarget(s string, limit int) (string, bool) {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	if len(s) <= limit {
		return s, false
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

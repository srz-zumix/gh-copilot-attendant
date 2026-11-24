package permissionhook

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

// Behavior is the permission decision returned to Copilot CLI.
type Behavior string

const (
	// BehaviorNone means no decision: the normal permission flow continues.
	BehaviorNone Behavior = ""
	// BehaviorAllow approves the tool call.
	BehaviorAllow Behavior = "allow"
	// BehaviorDeny rejects the tool call.
	BehaviorDeny Behavior = "deny"
)

// Decision is the result of evaluating a request against the config.
type Decision struct {
	Behavior Behavior `json:"behavior"`
	Message  string   `json:"message,omitempty"`
}

// shellTools are tool names whose command line is subject to command rules.
// Only bash is parsed; other shells never receive an allow decision.
var shellTools = map[string]bool{"bash": true, "powershell": true}

// Decide evaluates a request against the permissionRequest rules.
// Deny rules win over allow rules. Shell tools are approved only when every simple
// command in the parsed command line matches an allow rule or preset; tool rules
// never approve shell tools wholesale.
func Decide(cfg *Config, req *Request) Decision {
	if cfg == nil || req == nil || req.ToolName == "" {
		return Decision{}
	}
	rules := cfg.Hooks.PermissionRequest

	if pattern, ok := matchToolRules(rules.Deny.Tools, req.ToolName); ok {
		return denyDecision(fmt.Sprintf("tool %q matches deny rule %q", req.ToolName, pattern))
	}

	if req.OpaqueInput {
		return Decision{}
	}
	if shellTools[req.ToolName] {
		return decideShell(rules, req)
	}

	if req.SandboxBypass {
		return Decision{}
	}
	if _, ok := matchToolRules(rules.Allow.Tools, req.ToolName); ok {
		return Decision{Behavior: BehaviorAllow}
	}
	return decideScopedTools(rules.Allow.ScopedTools, req)
}

// DecidePreToolUse evaluates scoped tool rules before a tool executes.
func DecidePreToolUse(cfg *Config, req *Request) Decision {
	if cfg == nil || req == nil || !isPreToolUseSupported(req.ToolName) {
		return Decision{}
	}
	rules := cfg.Hooks.PermissionRequest
	if pattern, ok := matchToolRules(rules.Deny.Tools, req.ToolName); ok {
		return denyDecision(fmt.Sprintf("tool %q matches deny rule %q", req.ToolName, pattern))
	}
	if req.OpaqueInput || req.SandboxBypass {
		return Decision{}
	}
	if req.ToolName == "list_repositories" && len(req.Arguments) == 0 {
		if _, ok := matchToolRules(rules.Allow.Tools, req.ToolName); ok {
			return Decision{Behavior: BehaviorAllow}
		}
	}
	return decideScopedTools(rules.Allow.ScopedTools, req)
}

// DecideAllRulesPreToolUse evaluates all rules before a tool executes.
func DecideAllRulesPreToolUse(cfg *Config, req *Request) Decision {
	if cfg == nil || req == nil {
		return Decision{}
	}
	if req.ToolName == "list_repositories" && (req.OpaqueInput || len(req.Arguments) != 0) {
		if pattern, ok := matchToolRules(cfg.Hooks.PermissionRequest.Deny.Tools, req.ToolName); ok {
			return denyDecision(fmt.Sprintf("tool %q matches deny rule %q", req.ToolName, pattern))
		}
		return Decision{}
	}
	return Decide(cfg, req)
}

func decideShell(rules PermissionRequestConfig, req *Request) Decision {
	if req.ToolName != "bash" || req.Command == "" {
		return Decision{}
	}
	parsed, err := parseBashCommandDetails(req.Command)
	if err != nil {
		return Decision{}
	}
	segments := parsed.segments
	argvs := make([][]string, len(segments))
	normalized := make([][]string, len(segments))
	hasAssignments := false
	for i, segment := range segments {
		for _, word := range segment {
			argvs[i] = append(argvs[i], word.value)
			if word.assignment != nil {
				hasAssignments = true
				continue
			}
			normalized[i] = append(normalized[i], word.value)
		}
	}
	for i := range argvs {
		if _, ok := matchCommandRules(rules.Deny.Commands, argvs[i]); ok {
			return denyDecision("command matches a deny rule")
		}
		if hasAssignments {
			if _, ok := matchCommandRules(rules.Deny.Commands, normalized[i]); ok {
				return denyDecision("command matches a deny rule")
			}
		}
	}
	if hasAssignments {
		for i := range segments {
			if dynamicAssignmentMayMatchDeny(rules.Deny.Commands, segments[i], normalized[i]) {
				return Decision{}
			}
		}
	}
	if req.SandboxBypass {
		return Decision{}
	}
	priorDisplayOnly := true
	displayPathUsed := false
	allDisplay := true
	for i := range segments {
		command := argvs[i]
		if hasAssignments {
			if len(normalized[i]) == 0 {
				return Decision{}
			}
			command = normalized[i]
		}
		executable := path.Base(command[0])
		isGH := executable == "gh" || executable == "gh.exe"
		ghRead := false
		if rules.Allow.GHRead || (hasAssignments && isGH) {
			ghRead = ghReadCommandAllowed(command, rules.EnvironmentAssignments, assignmentsForSegment(segments[i]))
		}
		if hasAssignments && isGH && !ghRead {
			return Decision{}
		}
		_, commandAllowed := matchCommandRules(rules.Allow.Commands, command)
		gitRead := rules.Allow.GitRead && gitReadCommandAllowed(command)
		allowedByRuleOrPreset := commandAllowed || (rules.Allow.GHRead && ghRead) || gitRead
		displayAllowed, displayUsesPath := false, false
		if rules.Allow.ShellDisplay && hasSafeShellDisplayArguments(req) && !hasUnquotedTilde(req.Command, segments[i]) {
			piped := i > 0 && isDisplayPipe(req.Command, segments[i-1], segments[i])
			displayAllowed, displayUsesPath = shellDisplayCommandAllowed(command, req.CWD, rules.Allow.ScopedTools, piped)
		}
		if !allowedByRuleOrPreset {
			if !displayAllowed || parsed.hasRedirect || (displayUsesPath && !priorDisplayOnly) {
				return Decision{}
			}
			displayPathUsed = displayPathUsed || displayUsesPath
		}
		priorDisplayOnly = priorDisplayOnly && displayAllowed
		allDisplay = allDisplay && displayAllowed
		if hasAssignments {
			for _, word := range segments[i] {
				if word.assignment != nil && !rules.EnvironmentAssignments.allows(word.assignment.name) {
					return Decision{}
				}
			}
		}
	}
	if (parsed.hasBackground || parsed.hasPipe) && displayPathUsed && !allDisplay {
		return Decision{}
	}
	return Decision{Behavior: BehaviorAllow}
}

func assignmentsForSegment(segment []bashWord) []*bashAssignment {
	var assignments []*bashAssignment
	for _, word := range segment {
		if word.assignment != nil {
			assignments = append(assignments, word.assignment)
		}
	}
	return assignments
}

func dynamicAssignmentMayMatchDeny(rules []string, segment []bashWord, normalized []string) bool {
	assignments := assignmentsForSegment(segment)
	dynamic := false
	for _, assignment := range assignments {
		dynamic = dynamic || assignment.reference != ""
	}
	if !dynamic {
		return false
	}
	for _, text := range rules {
		rule, err := ParseCommandRule(text)
		if err != nil {
			continue
		}
		if (!rule.Prefix && len(rule.Words) != len(assignments)+len(normalized)) ||
			(rule.Prefix && len(rule.Words) > len(assignments)+len(normalized)) {
			continue
		}
		match := true
		for i, ruleWord := range rule.Words {
			if i < len(assignments) {
				assignment := assignments[i]
				if assignment.reference == "" {
					if ruleWord != segment[i].value {
						match = false
						break
					}
				} else if !strings.HasPrefix(ruleWord, assignment.name+"=") {
					match = false
					break
				}
				continue
			}
			commandWord := i - len(assignments)
			if commandWord >= len(normalized) || ruleWord != normalized[commandWord] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func denyDecision(reason string) Decision {
	return Decision{Behavior: BehaviorDeny, Message: "Denied by gh-copilot-attendant: " + reason}
}

// WriteDecision writes a decision or an evaluation warning. Unmatched calls produce
// no output, so Copilot CLI falls through to its normal permission flow.
func WriteDecision(w io.Writer, d Decision) error {
	if d.Behavior == BehaviorNone {
		if d.Message != "" {
			return writeEvaluationWarning(w, d.Message)
		}
		return nil
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

// WritePreToolDecision writes the preToolUse representation of a decision.
func WritePreToolDecision(w io.Writer, d Decision) error {
	if d.Behavior == BehaviorNone {
		if d.Message != "" {
			return writeEvaluationWarning(w, d.Message)
		}
		return nil
	}
	output := struct {
		Decision Behavior `json:"permissionDecision"`
		Reason   string   `json:"permissionDecisionReason,omitempty"`
	}{Decision: d.Behavior, Reason: d.Message}
	data, err := json.Marshal(output)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

package permissionhook

import (
	"encoding/json"
	"fmt"
	"io"
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
// command in the parsed command line matches an allow command rule; tool rules never
// approve shell tools wholesale.
func Decide(cfg *Config, req *Request) Decision {
	if cfg == nil || req == nil || req.ToolName == "" {
		return Decision{}
	}
	rules := cfg.Hooks.PermissionRequest

	if p, ok := matchToolRules(rules.Deny.Tools, req.ToolName); ok {
		return denyDecision(fmt.Sprintf("tool %q matches deny rule %q", req.ToolName, p))
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
	return Decision{}
}

func decideShell(rules PermissionRequestConfig, req *Request) Decision {
	if req.ToolName != "bash" || req.Command == "" {
		return Decision{}
	}
	segments, err := ParseBashCommand(req.Command)
	if err != nil {
		return Decision{}
	}
	for _, argv := range segments {
		if p, ok := matchCommandRules(rules.Deny.Commands, argv); ok {
			return denyDecision(fmt.Sprintf("command matches deny rule %q", p))
		}
	}
	if req.SandboxBypass {
		return Decision{}
	}
	for _, argv := range segments {
		if _, ok := matchCommandRules(rules.Allow.Commands, argv); !ok {
			return Decision{}
		}
	}
	return Decision{Behavior: BehaviorAllow}
}

func denyDecision(reason string) Decision {
	return Decision{Behavior: BehaviorDeny, Message: "Denied by gh-copilot-attendant: " + reason}
}

// WriteDecision writes the hook output. No output is written when there is no decision,
// so that Copilot CLI falls through to its normal permission flow.
func WriteDecision(w io.Writer, d Decision) error {
	if d.Behavior == BehaviorNone {
		return nil
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

package permissionhook

import (
	"encoding/json"
	"io"
	"time"
)

// Run handles one permissionRequest hook invocation: it loads the config at configPath
// (resolved via ResolveConfigPath), reads the hook payload from r, and writes the
// decision to w. A missing or disabled config file produces no decision.
// Configuration and input errors produce a progress warning without a decision.
func Run(r io.Reader, w io.Writer, configPath string) error {
	return run(r, w, configPath, "permissionRequest", nil, Decide, WriteDecision)
}

// RunPreToolUse handles a preToolUse hook invocation for scoped tools.
func RunPreToolUse(r io.Reader, w io.Writer, configPath string) error {
	return run(r, w, configPath, "preToolUse", validatePreToolConfig, DecidePreToolUse, WritePreToolDecision)
}

// RunAllRulesPreToolUse handles all configured rules before a tool executes.
func RunAllRulesPreToolUse(r io.Reader, w io.Writer, configPath string) error {
	return run(r, w, configPath, "preToolUse", nil, DecideAllRulesPreToolUse, WritePreToolDecision)
}

func run(r io.Reader, w io.Writer, configPath, event string, validate func(*Config) error, decide func(*Config, *Request) Decision, write func(io.Writer, Decision) error) error {
	started := time.Now().UTC()
	p, err := ResolveConfigPath(configPath)
	if err != nil {
		return writeEvaluationWarning(w, "could not resolve hook configuration; run gh copilot-attendant hook status")
	}
	cfg, found, err := LoadConfig(p)
	if err != nil {
		return writeEvaluationWarning(w, "could not load or validate hook configuration; run gh copilot-attendant hook status")
	}
	if cfg.Hooks.Disabled {
		return nil
	}
	if found && validate != nil {
		if err := validate(cfg); err != nil {
			return writeEvaluationWarning(w, "hook configuration is incompatible with scoped mode; run gh copilot-attendant hook status")
		}
	}
	if !found {
		return nil
	}
	req, err := ParseRequest(r)
	if err != nil {
		return writeEvaluationWarning(w, "could not read or parse hook input")
	}
	decision := decide(cfg, req)
	record := HookRecord{
		Version: 1, Timestamp: started, Event: event, Tool: safeToolName(req.ToolName),
		SessionID: req.SessionID, Decision: string(decision.Behavior), Status: "success", Output: "not_attempted",
	}
	if decision.Behavior == BehaviorAllow || decision.Behavior == BehaviorDeny {
		record.Target, record.TargetTruncated = truncateTarget(decisionTarget(req), maxTargetBytes)
		if !cfg.Hooks.Logging.Disabled {
			defer func() {
				if err := appendHookRecord(record); err != nil {
					loggingDiagnostic()
				}
			}()
		}
	}
	err = write(w, decision)
	if err != nil {
		record.Status = "output_error"
		record.Output = "write_failed"
	} else if decision.Behavior != BehaviorNone {
		record.Output = "write_succeeded"
	}
	return err
}

func writeEvaluationWarning(w io.Writer, message string) error {
	return json.NewEncoder(w).Encode(struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{
		Type:    "progress",
		Message: "gh-copilot-attendant warning: " + message + "; no permission decision",
	})
}

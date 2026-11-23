package permissionhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// Request is the normalized subset of a permissionRequest hook payload.
type Request struct {
	ToolName string
	// SessionID is present only when explicitly supplied in a recognized format.
	SessionID string
	// Command is the shell command line for shell tools. Empty when absent.
	Command string
	// CWD is the working directory supplied by the hook.
	CWD string
	// SandboxBypass is true when the tool call asks to run outside the sandbox.
	SandboxBypass bool
	// Arguments contains the original tool arguments.
	Arguments map[string]json.RawMessage
	// OpaqueInput means the tool arguments cannot be inspected as an object.
	OpaqueInput bool
}

var toolNameKeys = []string{"toolName", "tool_name"}
var toolInputKeys = []string{"toolInput", "tool_input", "toolArgs"}

// ParseRequest decodes a permissionRequest hook payload. It accepts camelCase
// (toolName/toolInput/toolArgs) and snake_case (tool_name/tool_input) fields; the tool
// input may be an object, a JSON-encoded object string, or opaque arguments for
// tools that do not require structured input. Duplicate keys and conflicting
// values are rejected so that the decision never depends on which interpretation wins.
func ParseRequest(r io.Reader) (*Request, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	top, err := decodeObjectStrict(data)
	if err != nil {
		return nil, fmt.Errorf("invalid hook input: %w", err)
	}

	req := &Request{}
	req.SessionID = sessionIDFromInput(top)
	hasName := false
	for _, k := range toolNameKeys {
		raw, ok := top[k]
		if !ok {
			continue
		}
		var name string
		if isJSONNull(raw) || json.Unmarshal(raw, &name) != nil {
			return nil, fmt.Errorf("invalid hook input: %s must be a string", k)
		}
		if hasName && req.ToolName != name {
			return nil, errors.New("invalid hook input: conflicting tool names")
		}
		req.ToolName = name
		hasName = true
	}
	if raw, ok := top["cwd"]; ok {
		if isJSONNull(raw) || json.Unmarshal(raw, &req.CWD) != nil {
			return nil, errors.New("invalid hook input: cwd must be a string")
		}
	}

	var seenInput bool
	var opaqueInput json.RawMessage
	for _, k := range toolInputKeys {
		raw, ok := top[k]
		if !ok || isJSONNull(raw) {
			continue
		}
		input, opaque, err := decodeToolInput(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid hook input: %s: %w", k, err)
		}
		if opaque {
			if shellTools[req.ToolName] || isPreToolUseSupported(req.ToolName) {
				return nil, fmt.Errorf("invalid hook input: %s: expected a JSON object", k)
			}
			if seenInput && (!req.OpaqueInput || !equalJSON(opaqueInput, raw)) {
				return nil, errors.New("invalid hook input: conflicting tool arguments")
			}
			req.OpaqueInput = true
			opaqueInput = raw
			seenInput = true
			continue
		}
		if !seenInput {
			req.Arguments = input
			seenInput = true
			continue
		}
		if req.OpaqueInput {
			return nil, errors.New("invalid hook input: conflicting tool arguments")
		}
		if len(req.Arguments) != len(input) {
			return nil, errors.New("invalid hook input: conflicting tool arguments")
		}
		for key, value := range input {
			if previous, exists := req.Arguments[key]; !exists || !equalJSON(previous, value) {
				return nil, fmt.Errorf("invalid hook input: conflicting tool argument %q", key)
			}
		}
	}
	if shellTools[req.ToolName] {
		if raw, ok := req.Arguments["command"]; ok {
			if isJSONNull(raw) || json.Unmarshal(raw, &req.Command) != nil {
				return nil, errors.New("invalid hook input: command must be a string")
			}
		}
	}
	for _, key := range []string{"requestSandboxBypass", "request_sandbox_bypass"} {
		if raw, ok := req.Arguments[key]; ok {
			var bypass bool
			if isJSONNull(raw) || json.Unmarshal(raw, &bypass) != nil {
				return nil, fmt.Errorf("invalid hook input: %s must be a boolean", key)
			}
			req.SandboxBypass = req.SandboxBypass || bypass
		}
	}
	return req, nil
}

func equalJSON(a, b json.RawMessage) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

func decodeToolInput(raw json.RawMessage) (map[string]json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, false, err
		}
		decoded := bytes.TrimSpace([]byte(s))
		if len(decoded) == 0 || decoded[0] != '{' || !json.Valid(decoded) {
			return nil, true, nil
		}
		trimmed = decoded
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, true, nil
	}
	input, err := decodeObjectStrict(trimmed)
	return input, false, err
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// decodeObjectStrict decodes a JSON object into its raw members, rejecting duplicate keys
// and trailing data.
func decodeObjectStrict(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("expected a JSON object")
	}
	out := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, errors.New("expected an object key")
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out[key] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after JSON object")
	}
	return out, nil
}

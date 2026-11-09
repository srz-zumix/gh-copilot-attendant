package permissionhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Request is the normalized subset of a permissionRequest hook payload.
type Request struct {
	ToolName string
	// Command is the shell command line for shell tools. Empty when absent.
	Command string
	// SandboxBypass is true when the tool call asks to run outside the sandbox.
	SandboxBypass bool
}

var toolNameKeys = []string{"toolName", "tool_name"}
var toolInputKeys = []string{"toolInput", "tool_input", "toolArgs"}

// ParseRequest decodes a permissionRequest hook payload. It accepts camelCase
// (toolName/toolInput/toolArgs) and snake_case (tool_name/tool_input) fields; the tool
// input may be an object or a JSON-encoded object string. Duplicate keys and
// conflicting values are rejected so that the decision never depends on which
// interpretation wins.
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
	for _, k := range toolNameKeys {
		raw, ok := top[k]
		if !ok {
			continue
		}
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			return nil, fmt.Errorf("invalid hook input: %s must be a string", k)
		}
		if req.ToolName != "" && req.ToolName != name {
			return nil, errors.New("invalid hook input: conflicting tool names")
		}
		req.ToolName = name
	}

	hasCommand := false
	for _, k := range toolInputKeys {
		raw, ok := top[k]
		if !ok || isJSONNull(raw) {
			continue
		}
		input, err := decodeToolInput(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid hook input: %s: %w", k, err)
		}
		if raw, ok := input["command"]; ok {
			var cmd string
			if err := json.Unmarshal(raw, &cmd); err != nil {
				return nil, fmt.Errorf("invalid hook input: %s.command must be a string", k)
			}
			if hasCommand && req.Command != cmd {
				return nil, errors.New("invalid hook input: conflicting commands")
			}
			req.Command = cmd
			hasCommand = true
		}
		if raw, ok := input["requestSandboxBypass"]; ok && !isJSONNull(raw) {
			var bypass bool
			if err := json.Unmarshal(raw, &bypass); err != nil {
				return nil, fmt.Errorf("invalid hook input: %s.requestSandboxBypass must be a boolean", k)
			}
			req.SandboxBypass = req.SandboxBypass || bypass
		}
	}
	return req, nil
}

func decodeToolInput(raw json.RawMessage) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		return decodeObjectStrict([]byte(s))
	}
	return decodeObjectStrict(trimmed)
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

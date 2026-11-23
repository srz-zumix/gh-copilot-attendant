package permissionhook

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ToggleResult describes the outcome of SetDisabled.
type ToggleResult struct {
	// Path is the config file that was inspected or written.
	Path string
	// Changed is true when the file was created or modified.
	Changed bool
	// Created is true when a new config file was written.
	Created bool
}

// SetDisabled sets or clears hooks.disabled in the config file at p, keeping the
// other rules and comments. Disabling a missing config creates a file holding only
// the flag; enabling a missing config is a no-op. Enabling fails when the
// resulting config would be invalid, leaving the file unchanged.
func SetDisabled(p string, disabled bool) (ToggleResult, error) {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	res := ToggleResult{Path: p}
	data, err := os.ReadFile(p)
	mode := os.FileMode(0o644)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
		if !disabled {
			return res, nil
		}
		res.Created = true
	} else if info, statErr := os.Stat(p); statErr == nil {
		mode = info.Mode().Perm()
	}

	out, changed, err := setDisabledYAML(data, disabled)
	if err != nil {
		return res, fmt.Errorf("invalid config %s: %w", p, err)
	}
	if !changed {
		return res, nil
	}
	if _, err := ParseConfig(out); err != nil {
		return res, fmt.Errorf("invalid config %s: %w", p, err)
	}
	if err := writeFileAtomic(p, out, mode); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}

func setDisabledYAML(data []byte, disabled bool) ([]byte, bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, false, errors.New("top level must be a mapping")
	}
	root := doc.Content[0]
	hooks := mappingValue(root, "hooks")
	if hooks == nil || (hooks.Kind == yaml.ScalarNode && hooks.Tag == "!!null") {
		if !disabled {
			return nil, false, nil
		}
		if hooks == nil {
			hooks = &yaml.Node{Kind: yaml.MappingNode}
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "hooks"}, hooks)
		} else {
			*hooks = yaml.Node{Kind: yaml.MappingNode}
		}
	}
	if hooks.Kind != yaml.MappingNode {
		return nil, false, errors.New("hooks must be a mapping")
	}

	idx := mappingIndex(hooks, "disabled")
	if !disabled {
		if idx < 0 {
			return nil, false, nil
		}
		var current bool
		if err := hooks.Content[idx+1].Decode(&current); err == nil && !current {
			return nil, false, nil
		}
		hooks.Content = append(hooks.Content[:idx], hooks.Content[idx+2:]...)
	} else {
		if idx >= 0 {
			var current bool
			if err := hooks.Content[idx+1].Decode(&current); err == nil && current {
				return nil, false, nil
			}
			hooks.Content[idx+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		} else {
			hooks.Content = append([]*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "disabled"},
				{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
			}, hooks.Content...)
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, false, err
	}
	if err := enc.Close(); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

func mappingIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if i := mappingIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

func writeFileAtomic(p string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}

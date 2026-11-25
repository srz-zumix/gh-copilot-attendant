package permissionhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

var scopedInputKeys = map[string]map[string]bool{
	"view": {
		"path": true, "view_range": true, "forceReadLargeFiles": true,
	},
	"rg": {
		"pattern": true, "paths": true, "output_mode": true, "glob": true, "type": true,
		"-i": true, "-A": true, "-B": true, "-C": true, "-n": true, "head_limit": true, "multiline": true,
	},
	"grep": {
		"pattern": true, "paths": true, "output_mode": true, "glob": true, "type": true,
		"-i": true, "-A": true, "-B": true, "-C": true, "-n": true, "head_limit": true, "multiline": true,
	},
	"glob": {
		"pattern": true, "paths": true,
	},
	"web_fetch": {
		"url": true, "raw": true, "max_length": true, "start_index": true,
	},
}

func isPathScopedTool(name string) bool {
	switch name {
	case "view", "rg", "grep", "glob":
		return true
	default:
		return false
	}
}

func isPreToolUseSupported(name string) bool {
	return isPathScopedTool(name) || name == "web_fetch" || name == "list_repositories"
}

func validatePreToolConfig(cfg *Config) error {
	rules := cfg.Hooks.PermissionRequest
	if rules.Allow.GHRead || rules.Allow.GitRead || rules.Allow.ShellDisplay || len(rules.Allow.Commands) > 0 || len(rules.Deny.Commands) > 0 {
		return errors.New("scoped-tools mode does not apply command rules")
	}
	for _, tool := range rules.Allow.Tools {
		if tool != "list_repositories" {
			return fmt.Errorf("scoped-tools mode does not apply unscoped allow rule %q", tool)
		}
	}
	for _, tool := range rules.Deny.Tools {
		if !isPreToolUseSupported(tool) {
			return fmt.Errorf("scoped-tools mode does not apply deny rule %q", tool)
		}
	}
	return nil
}

func validateScopedToolRule(rule ScopedToolRule) error {
	if len(rule.Tools) == 0 {
		return errors.New("tools must not be empty")
	}
	hasPaths, hasURLs := len(rule.Paths) > 0, len(rule.URLs) > 0
	if hasPaths == hasURLs {
		return errors.New("specify either paths or urls")
	}
	if hasURLs && !rule.AllowDespiteNativeDeny {
		return errors.New("scoped URL rules require allowDespiteNativeDeny: true because hook approval can override native URL denials")
	}
	if hasPaths && rule.AllowDespiteNativeDeny {
		return errors.New("allowDespiteNativeDeny is only supported for URL rules")
	}
	for _, tool := range rule.Tools {
		if (hasPaths && !isPathScopedTool(tool)) || (hasURLs && tool != "web_fetch") {
			return fmt.Errorf("tool %q does not support this scope", tool)
		}
	}
	for _, p := range rule.Paths {
		if p == "" || strings.TrimSpace(p) != p || strings.ContainsRune(p, 0) || !filepath.IsAbs(p) {
			return fmt.Errorf("path %q must be an absolute path", p)
		}
	}
	for _, u := range rule.URLs {
		if !validScopedURL(u) {
			return fmt.Errorf("invalid URL %q", u)
		}
	}
	return nil
}

func matchesScopedTools(rules []ScopedToolRule, req *Request) bool {
	for _, rule := range rules {
		matchesTool := false
		for _, tool := range rule.Tools {
			if tool == req.ToolName {
				matchesTool = true
				break
			}
		}
		if !matchesTool || !hasKnownScopedArguments(req) {
			continue
		}
		if len(rule.Paths) > 0 && matchScopedPaths(rule.Paths, req) {
			return true
		}
		if len(rule.URLs) > 0 && rule.AllowDespiteNativeDeny && matchScopedURLs(rule.URLs, req) {
			return true
		}
	}
	return false
}

func hasKnownScopedArguments(req *Request) bool {
	known, ok := scopedInputKeys[req.ToolName]
	if !ok || req.Arguments == nil {
		return false
	}
	for key := range req.Arguments {
		if !known[key] && key != "requestSandboxBypass" && key != "request_sandbox_bypass" {
			return false
		}
	}
	return true
}

func stringArgument(args map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := args[key]
	if !ok || isJSONNull(raw) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func pathArguments(req *Request) ([]string, string, bool) {
	if req.ToolName == "view" {
		p, ok := stringArgument(req.Arguments, "path")
		return []string{p}, "", ok && p != ""
	}
	pattern, ok := stringArgument(req.Arguments, "pattern")
	if !ok {
		return nil, "", false
	}
	prefix := ""
	if req.ToolName == "glob" {
		prefix, ok = globPrefix(pattern)
		if !ok {
			return nil, "", false
		}
	}
	raw, ok := req.Arguments["paths"]
	if !ok {
		return []string{"."}, prefix, true
	}
	if isJSONNull(raw) {
		return nil, "", false
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}, prefix, one != ""
	}
	var paths []string
	if json.Unmarshal(raw, &paths) != nil || len(paths) == 0 {
		return nil, "", false
	}
	for _, p := range paths {
		if p == "" {
			return nil, "", false
		}
	}
	return paths, prefix, true
}

func matchScopedPaths(paths []string, req *Request) bool {
	targets, prefix, ok := pathArguments(req)
	if !ok {
		return false
	}
	var roots []string
	for _, root := range paths {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			roots = append(roots, resolved)
		}
	}
	if len(roots) == 0 {
		return false
	}
	for _, target := range targets {
		resolved, ok := resolveTarget(target, req.CWD)
		if !ok || !withinRoots(resolved, roots) {
			return false
		}
		if prefix != "" && prefix != "." {
			resolvedPrefix, ok := resolveTarget(prefix, resolved)
			if !ok || !withinRoots(resolvedPrefix, roots) {
				return false
			}
		}
	}
	return true
}

func resolveTarget(target, cwd string) (string, bool) {
	if target == "" || strings.ContainsRune(target, 0) {
		return "", false
	}
	if !filepath.IsAbs(target) {
		if !filepath.IsAbs(cwd) || filepath.VolumeName(target) != "" || strings.HasPrefix(target, "-") {
			return "", false
		}
		// Avoid cleaning ".." before resolving preceding symlinks.
		target = cwd + string(filepath.Separator) + target
	}
	resolved, err := filepath.EvalSymlinks(target)
	return resolved, err == nil
}

func withinRoots(target string, roots []string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, target)
		if err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// globPrefix rejects patterns that could escape their search roots and returns
// the literal path before the first wildcard.
func globPrefix(pattern string) (string, bool) {
	if pattern == "" || filepath.IsAbs(pattern) || filepath.VolumeName(pattern) != "" ||
		strings.Contains(pattern, "..") || strings.ContainsAny(pattern, "\\~$!()\x00\r\n") {
		return "", false
	}
	braces := 0
	inClass := false
	for _, c := range pattern {
		switch c {
		case '{':
			if !inClass {
				braces++
			}
		case '}':
			if !inClass {
				braces--
				if braces < 0 {
					return "", false
				}
			}
		case '[':
			if inClass {
				return "", false
			}
			inClass = true
		case ']':
			if !inClass {
				return "", false
			}
			inClass = false
		case '/':
			if braces != 0 || inClass {
				return "", false
			}
		}
	}
	if braces != 0 || inClass {
		return "", false
	}
	if i := strings.IndexAny(pattern, "*?[{"); i >= 0 {
		prefix := pattern[:i]
		if slash := strings.LastIndex(prefix, "/"); slash >= 0 {
			return prefix[:slash], true
		}
		return ".", true
	}
	return pattern, true
}

func validScopedURL(raw string) bool {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") &&
		u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

func matchScopedURLs(urls []string, req *Request) bool {
	requested, ok := stringArgument(req.Arguments, "url")
	if !ok || !validScopedURL(requested) {
		return false
	}
	for _, allowed := range urls {
		if requested == allowed {
			return true
		}
	}
	return false
}

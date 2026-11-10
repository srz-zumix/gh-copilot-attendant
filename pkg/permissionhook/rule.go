package permissionhook

import (
	"errors"
	"path"
	"strings"
)

// CommandRule is a parsed shell command rule.
// "git status" matches only that exact argv; "go test:*" matches the prefix
// alone or followed by any additional arguments.
type CommandRule struct {
	Raw    string
	Words  []string
	Prefix bool
}

// ParseCommandRule parses a command rule string.
func ParseCommandRule(s string) (CommandRule, error) {
	rule := CommandRule{Raw: s}
	body := strings.TrimSpace(s)
	if strings.HasSuffix(body, ":*") {
		rule.Prefix = true
		body = strings.TrimSuffix(body, ":*")
	}
	rule.Words = strings.Fields(body)
	if len(rule.Words) == 0 {
		return rule, errors.New("empty command rule")
	}
	return rule, nil
}

// Match reports whether argv is matched by the rule.
func (r CommandRule) Match(argv []string) bool {
	if len(argv) < len(r.Words) || (!r.Prefix && len(argv) != len(r.Words)) {
		return false
	}
	for i, w := range r.Words {
		if argv[i] != w {
			return false
		}
	}
	return true
}

func matchCommandRules(rules []string, argv []string) (string, bool) {
	for _, s := range rules {
		r, err := ParseCommandRule(s)
		if err != nil {
			continue
		}
		if r.Match(argv) {
			return s, true
		}
	}
	return "", false
}

func matchToolRules(patterns []string, toolName string) (string, bool) {
	for _, p := range patterns {
		if ok, err := path.Match(p, toolName); err == nil && ok {
			return p, true
		}
	}
	return "", false
}

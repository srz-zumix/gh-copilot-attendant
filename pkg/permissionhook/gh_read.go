package permissionhook

import (
	"net/url"
	"strings"
)

type readCommandSpec struct {
	minArgs int
	maxArgs int
	flags   map[string]bool
}

func readSpec(minArgs, maxArgs int, flags map[string]bool) readCommandSpec {
	return readCommandSpec{minArgs: minArgs, maxArgs: maxArgs, flags: flags}
}

var commonReadFlags = map[string]bool{
	"-R": true, "--repo": true, "--hostname": true,
	"--json": true, "--jq": true, "--template": true,
}

var ghReadCommands = map[string]readCommandSpec{
	"pr view": readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{"--comments": false, "--exit-status": false})),
	"pr list": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{
		"--state": true, "--limit": true, "--author": true, "--assignee": true, "--base": true,
		"--head": true, "--label": true, "--search": true, "--draft": false, "--app": true,
	})),
	"pr status":  readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{})),
	"pr checks":  readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{"--required": false})),
	"pr diff":    readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{"--patch": false, "--name-only": false, "--color": true})),
	"issue view": readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{"--comments": false})),
	"issue list": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{
		"--state": true, "--limit": true, "--assignee": true, "--author": true, "--label": true,
		"--milestone": true, "--search": true, "--mention": true, "--reason": true,
	})),
	"issue status": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{})),
	"repo view":    readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{"--branch": true})),
	"repo list": readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--visibility": true, "--source": false, "--fork": false,
		"--archived": false, "--no-archived": false, "--private": false, "--public": false,
	})),
	"run view": readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{
		"--log": false, "--job": true, "--attempt": true, "--exit-status": false,
	})),
	"run list": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{
		"--branch": true, "--commit": true, "--event": true, "--status": true,
		"--user": true, "--limit": true, "--workflow": true,
	})),
	"workflow view": readSpec(1, 1, mergeFlags(commonReadFlags, map[string]bool{})),
	"workflow list": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{"--all": false})),
	"release view":  readSpec(0, 1, mergeFlags(commonReadFlags, map[string]bool{"--latest": false, "--verify-tag": false})),
	"release list": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--exclude-drafts": false, "--exclude-pre-releases": false,
	})),
	"gist view": readSpec(1, 1, mergeFlags(commonReadFlags, map[string]bool{})),
	"gist list": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{"--public": false, "--secret": false, "--limit": true})),
	"search code": readSpec(1, 64, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--language": true, "--repo": true, "--owner": true,
		"--filename": true, "--extension": true,
	})),
	"search repos": readSpec(1, 64, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--language": true, "--topic": true, "--owner": true,
		"--archived": false, "--stars": true,
	})),
	"search issues": readSpec(1, 64, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--state": true, "--assignee": true, "--author": true,
		"--label": true, "--repo": true, "--owner": true,
	})),
	"search prs": readSpec(1, 64, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--state": true, "--assignee": true, "--author": true,
		"--label": true, "--repo": true, "--owner": true,
	})),
	"search commits": readSpec(1, 64, mergeFlags(commonReadFlags, map[string]bool{
		"--limit": true, "--author": true, "--repo": true, "--owner": true,
	})),
	"status": readSpec(0, 0, mergeFlags(commonReadFlags, map[string]bool{
		"-e": true, "--exclude": true, "-o": true, "--org": true,
	})),
}

func mergeFlags(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool, len(a)+len(b))
	for key, takesValue := range a {
		out[key] = takesValue
	}
	for key, takesValue := range b {
		out[key] = takesValue
	}
	return out
}

func ghReadCommandAllowed(argv []string, env EnvironmentAssignmentConfig, assignments []*bashAssignment) bool {
	if len(argv) < 2 || argv[0] != "gh" {
		return false
	}
	args := argv[1:]
	var hosts []string
	for len(args) > 0 {
		if value, ok := strings.CutPrefix(args[0], "--hostname="); ok {
			hosts = append(hosts, value)
			args = args[1:]
			continue
		}
		if args[0] != "--hostname" {
			break
		}
		if len(args) < 2 {
			return false
		}
		hosts = append(hosts, args[1])
		args = args[2:]
	}
	if len(args) < 1 {
		return false
	}
	if args[0] == "auth" {
		return len(args) >= 2 && args[1] == "status" &&
			ghAuthStatusAllowed(args[2:], env, assignments, hosts)
	}
	if args[0] == "api" {
		return ghAPIAllowed(args[1:], env, assignments, hosts)
	}
	command := args[0]
	commandArgs := args[1:]
	if command != "status" {
		if len(args) < 2 {
			return false
		}
		command += " " + args[1]
		commandArgs = args[2:]
	}
	spec, ok := ghReadCommands[command]
	if !ok {
		return false
	}
	positions, selectors, ok := parseReadArgs(commandArgs, spec.flags)
	if !ok || len(positions) < spec.minArgs || len(positions) > spec.maxArgs {
		return false
	}
	for _, value := range selectors {
		if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			host, ok := hostFromURL(value)
			if !ok {
				return false
			}
			hosts = append(hosts, host)
		} else {
			hosts = append(hosts, value)
		}
	}
	for _, position := range positions {
		if strings.HasPrefix(position, "http://") || strings.HasPrefix(position, "https://") {
			host, ok := hostFromURL(position)
			if !ok {
				return false
			}
			hosts = append(hosts, host)
		}
	}
	return assignmentsTrustRouting(assignments, env, hosts)
}

func parseReadArgs(args []string, flags map[string]bool) ([]string, []string, bool) {
	var positionals, selectors []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || arg == "-w" || arg == "--web" {
			return nil, nil, false
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if !strings.HasPrefix(arg, "--") && strings.HasPrefix(arg, "-R") && len(arg) > 2 && !hasValue {
			name, value, hasValue = "-R", arg[2:], true
		}
		takesValue, ok := flags[name]
		if !ok || (takesValue && hasValue && value == "") || (!takesValue && hasValue) {
			return nil, nil, false
		}
		if takesValue && !hasValue {
			i++
			if i >= len(args) || args[i] == "" {
				return nil, nil, false
			}
			value = args[i]
		}
		if takesValue {
			if name == "-R" || name == "--repo" || name == "--hostname" {
				selectors = append(selectors, value)
			}
		}
	}
	return positionals, selectors, true
}

func ghAuthStatusAllowed(args []string, env EnvironmentAssignmentConfig, assignments []*bashAssignment, hosts []string) bool {
	seenNoRefresh := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-refresh":
			seenNoRefresh = true
		case "--hostname":
			i++
			if i >= len(args) {
				return false
			}
			hosts = append(hosts, args[i])
		default:
			return false
		}
	}
	return seenNoRefresh && assignmentsTrustRouting(assignments, env, hosts)
}

func ghAPIAllowed(args []string, env EnvironmentAssignmentConfig, assignments []*bashAssignment, hosts []string) bool {
	methodCount := 0
	explicitGet := false
	fieldCount := 0
	var endpoints []string
	afterSeparator := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if afterSeparator {
			endpoints = append(endpoints, arg)
			continue
		}
		if arg == "--" {
			afterSeparator = true
			continue
		}
		if arg == "--input" || strings.HasPrefix(arg, "--input=") || arg == "-i" {
			return false
		}
		if arg == "-X" || arg == "--method" || strings.HasPrefix(arg, "--method=") ||
			strings.HasPrefix(arg, "-X") && len(arg) > 2 {
			methodCount++
			if methodCount > 1 {
				return false
			}
			value := ""
			switch {
			case strings.HasPrefix(arg, "--method="):
				value = strings.TrimPrefix(arg, "--method=")
			case strings.HasPrefix(arg, "-X") && len(arg) > 2:
				value = arg[2:]
			default:
				i++
				if i >= len(args) {
					return false
				}
				value = args[i]
			}
			explicitGet = strings.EqualFold(value, "GET")
			if !explicitGet {
				return false
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			name, value, hasValue := strings.Cut(arg, "=")
			if !strings.HasPrefix(arg, "--") && strings.HasPrefix(arg, "-R") && len(arg) > 2 && !hasValue {
				name, value, hasValue = "-R", arg[2:], true
			}
			takesValue, known := map[string]bool{
				"--hostname": true, "--repo": true, "-R": true,
				"-f": true, "--raw-field": true, "-F": true, "--field": true,
				"-H": true, "--header": true, "-q": true, "--jq": true,
				"--template": true, "-t": true, "--preview": true,
			}[name]
			if !known {
				if strings.HasPrefix(arg, "-f") && len(arg) > 2 {
					name, value, hasValue, takesValue = "-f", arg[2:], true, true
				} else if strings.HasPrefix(arg, "-F") && len(arg) > 2 {
					name, value, hasValue, takesValue = "-F", arg[2:], true, true
				} else if strings.HasPrefix(arg, "-H") && len(arg) > 2 {
					name, value, hasValue, takesValue = "-H", arg[2:], true, true
				} else if strings.HasPrefix(arg, "-q") && len(arg) > 2 {
					name, value, hasValue, takesValue = "-q", arg[2:], true, true
				} else if strings.HasPrefix(arg, "-t") && len(arg) > 2 {
					name, value, hasValue, takesValue = "-t", arg[2:], true, true
				} else {
					return false
				}
			}
			if !takesValue {
				return false
			}
			if !hasValue {
				i++
				if i >= len(args) {
					return false
				}
				value = args[i]
			}
			fieldValue := value
			if _, afterEquals, ok := strings.Cut(value, "="); ok {
				fieldValue = afterEquals
			}
			if value == "" || strings.HasPrefix(fieldValue, "@") &&
				(name == "-F" || name == "--field" || name == "-f" || name == "--raw-field") {
				return false
			}
			switch name {
			case "--hostname", "--repo", "-R":
				hosts = append(hosts, value)
			case "-f", "--raw-field", "-F", "--field":
				fieldCount++
			case "-H", "--header":
				return false
			}
			continue
		}
		endpoints = append(endpoints, arg)
	}
	if methodCount == 0 && fieldCount != 0 {
		return false
	}
	if methodCount > 0 && !explicitGet || len(endpoints) != 1 {
		return false
	}
	endpoint := endpoints[0]
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		host, ok := hostFromURL(endpoint)
		if !ok {
			return false
		}
		hosts = append(hosts, host)
	} else if strings.Contains(endpoint, "://") {
		return false
	}
	return assignmentsTrustRouting(assignments, env, hosts)
}

func assignmentsTrustRouting(assignments []*bashAssignment, env EnvironmentAssignmentConfig, hosts []string) bool {
	seen := map[string]bool{}
	for _, assignment := range assignments {
		switch assignment.name {
		case "GH_HOST", "GH_REPO":
			if seen[assignment.name] || assignment.reference != "" {
				return false
			}
			seen[assignment.name] = true
			if assignment.name == "GH_HOST" {
				hosts = append(hosts, assignment.value)
				continue
			}
			if strings.HasPrefix(assignment.value, "http://") || strings.HasPrefix(assignment.value, "https://") {
				host, ok := hostFromURL(assignment.value)
				if !ok {
					return false
				}
				hosts = append(hosts, host)
			} else if !validRepositoryName(assignment.value) {
				return false
			}
		}
	}
	for _, host := range hosts {
		if strings.Contains(host, "/") && !strings.Contains(host, ":") {
			if validRepositoryName(host) {
				continue
			}
			return false
		}
		if !env.trustsHost(host) {
			return false
		}
	}
	return true
}

func validRepositoryName(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, c := range part {
			if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') &&
				!(c >= '0' && c <= '9') && !strings.ContainsRune("._-", c) {
				return false
			}
		}
	}
	return true
}

func hostFromURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", false
	}
	host := u.Host
	if _, ok := normalizeTrustedHost(host); !ok {
		return "", false
	}
	return host, true
}

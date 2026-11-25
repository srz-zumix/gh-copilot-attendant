package permissionhook

import (
	"regexp"
	"strings"
)

type displayValueKind byte

const (
	displayText displayValueKind = iota
	displayInteger
	displayCutList
	displaySortKey
)

type displayOptions struct {
	shortFlags  string
	longFlags   string
	shortValues map[byte]displayValueKind
	longValues  map[string]displayValueKind
}

var displayCommandOptions = map[string]displayOptions{
	"nl": {
		shortFlags: "p",
		longFlags:  "--no-renumber",
		shortValues: map[byte]displayValueKind{
			'b': displayText, 'd': displayText, 'f': displayText, 'h': displayText,
			'i': displayInteger, 'l': displayInteger, 'n': displayText, 's': displayText,
			'v': displayInteger, 'w': displayInteger,
		},
		longValues: map[string]displayValueKind{
			"--body-numbering": displayText, "--section-delimiter": displayText,
			"--footer-numbering": displayText, "--header-numbering": displayText,
			"--line-increment": displayInteger, "--join-blank-lines": displayInteger,
			"--number-format": displayText, "--number-separator": displayText,
			"--starting-line-number": displayInteger, "--number-width": displayInteger,
		},
	},
	"cut": {
		shortFlags:  "nsz",
		longFlags:   "--no-partial --only-delimited --zero-terminated --complement",
		shortValues: map[byte]displayValueKind{'b': displayCutList, 'c': displayCutList, 'd': displayText, 'f': displayCutList},
		longValues: map[string]displayValueKind{
			"--bytes": displayCutList, "--characters": displayCutList,
			"--delimiter": displayText, "--fields": displayCutList,
			"--output-delimiter": displayText,
		},
	},
	"uniq": {
		shortFlags:  "cdiuz",
		longFlags:   "--count --repeated --ignore-case --unique --zero-terminated",
		shortValues: map[byte]displayValueKind{'f': displayInteger, 's': displayInteger, 'w': displayInteger},
		longValues: map[string]displayValueKind{
			"--skip-fields": displayInteger, "--skip-chars": displayInteger, "--check-chars": displayInteger,
		},
	},
	"stat": {
		shortFlags: "L",
		longFlags:  "--dereference",
	},
	"sort": {
		shortFlags:  "bcCdfghiMmnrsuVz",
		longFlags:   "--ignore-leading-blanks --dictionary-order --ignore-case --general-numeric-sort --ignore-nonprinting --month-sort --human-numeric-sort --numeric-sort --reverse --version-sort --merge --stable --unique --zero-terminated",
		shortValues: map[byte]displayValueKind{'k': displaySortKey, 't': displayText},
		longValues:  map[string]displayValueKind{"--key": displaySortKey, "--field-separator": displayText},
	},
	"jq": {
		shortFlags: "nRscraSCMje",
		longFlags:  "--null-input --raw-input --slurp --compact-output --raw-output --ascii-output --sort-keys --color-output --monochrome-output --join-output --exit-status --tab --unbuffered",
	},
	"tr": {
		shortFlags: "Ccdstu",
		longFlags:  "--complement --delete --squeeze-repeats --truncate-set1",
	},
	"basename": {
		shortFlags:  "az",
		longFlags:   "--multiple --zero",
		shortValues: map[byte]displayValueKind{'s': displayText},
		longValues:  map[string]displayValueKind{"--suffix": displayText},
	},
	"dirname": {
		shortFlags: "z",
		longFlags:  "--zero",
	},
}

var (
	displayJQSelector = regexp.MustCompile(`^[[:space:]]*\.([A-Za-z_][A-Za-z0-9_]*)?(\.[A-Za-z_][A-Za-z0-9_]*|\[([0-9]+)?\])*\??([[:space:]]*\|[[:space:]]*\.([A-Za-z_][A-Za-z0-9_]*)?(\.[A-Za-z_][A-Za-z0-9_]*|\[([0-9]+)?\])*\??)*[[:space:]]*$`)
	displaySedScript  = regexp.MustCompile(`^(p|([0-9]+|\$)(,([0-9]+|\$))?p)(;[[:space:]]*(p|([0-9]+|\$)(,([0-9]+|\$))?p))*$`)
)

func shellDisplayExtendedCommandAllowed(argv []string, cwd string, scoped []ScopedToolRule, piped bool) (bool, bool) {
	command := argv[0]
	operands, flags, afterSeparator, ok := parseDisplayOptions(argv[1:], displayCommandOptions[command])
	if !ok {
		return false, false
	}
	if !afterSeparator && (command == "sort" || command == "uniq") {
		for _, operand := range operands {
			if strings.HasPrefix(operand, "+") {
				return false, false
			}
		}
	}
	switch command {
	case "basename", "dirname":
		return len(operands) > 0, false
	case "tr":
		return piped && len(operands) >= 1 && len(operands) <= 2, false
	case "jq":
		if len(operands) == 0 || !displayJQSelector.MatchString(operands[0]) {
			return false, false
		}
		operands = operands[1:]
		if len(operands) == 0 {
			return piped || flags["-n"] || flags["--null-input"], false
		}
	case "stat":
		if len(operands) == 0 {
			return false, false
		}
		return shellDisplayTargetsAllowed(operands, cwd, scoped, true, false, false)
	case "nl", "uniq":
		if len(operands) > 1 {
			return false, false
		}
	}
	if len(operands) == 0 {
		return piped, false
	}
	return shellDisplayTargetsAllowed(operands, cwd, scoped, false, true, piped)
}

// parseDisplayOptions stops at the first operand so that BSD and POSIXLY_CORRECT
// option parsing cannot turn a later option or its value into an unchecked path.
func parseDisplayOptions(args []string, spec displayOptions) ([]string, map[string]bool, bool, bool) {
	flags := map[string]bool{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return args[i+1:], flags, true, true
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			return args[i:], flags, false, true
		}
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue := strings.Cut(arg, "=")
			if containsDisplayFlag(spec.longFlags, name) {
				if hasValue {
					return nil, nil, false, false
				}
				flags[name] = true
				continue
			}
			kind, known := spec.longValues[name]
			if !known {
				return nil, nil, false, false
			}
			if !hasValue {
				i++
				if i >= len(args) {
					return nil, nil, false, false
				}
				value = args[i]
			}
			if !validDisplayOptionValue(kind, value) {
				return nil, nil, false, false
			}
			continue
		}
		for j := 1; j < len(arg); j++ {
			flag := arg[j]
			if strings.ContainsRune(spec.shortFlags, rune(flag)) {
				flags["-"+string(flag)] = true
				continue
			}
			kind, known := spec.shortValues[flag]
			if !known {
				return nil, nil, false, false
			}
			value := arg[j+1:]
			if value == "" {
				i++
				if i >= len(args) {
					return nil, nil, false, false
				}
				value = args[i]
			}
			if !validDisplayOptionValue(kind, value) {
				return nil, nil, false, false
			}
			break
		}
	}
	return nil, flags, false, true
}

func containsDisplayFlag(flags, flag string) bool {
	for _, allowed := range strings.Fields(flags) {
		if flag == allowed {
			return true
		}
	}
	return false
}

func validDisplayOptionValue(kind displayValueKind, value string) bool {
	switch kind {
	case displayText:
		return true
	case displayInteger:
		return displayNumber(value)
	case displayCutList, displaySortKey:
		digit := false
		for _, c := range value {
			if c >= '0' && c <= '9' {
				digit = true
			} else if kind == displayCutList && !strings.ContainsRune(",- \t", c) {
				return false
			} else if kind == displaySortKey && !strings.ContainsRune(",.bcdfghinrMV", c) {
				return false
			}
		}
		return digit
	default:
		return false
	}
}

func shellDisplaySedAllowed(args []string, cwd string, scoped []ScopedToolRule, piped bool) (bool, bool) {
	hasScript := false
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		switch {
		case arg == "-n" || arg == "--quiet" || arg == "--silent":
			i++
		case arg == "-e" || arg == "--expression" || arg == "-ne":
			i++
			if i >= len(args) || !displaySedScript.MatchString(strings.TrimSpace(args[i])) {
				return false, false
			}
			hasScript = true
			i++
		case strings.HasPrefix(arg, "-e") && len(arg) > 2:
			if !displaySedScript.MatchString(strings.TrimSpace(arg[2:])) {
				return false, false
			}
			hasScript = true
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return false, false
			}
			goto operands
		}
	}
operands:
	if !hasScript {
		if i >= len(args) || !displaySedScript.MatchString(strings.TrimSpace(args[i])) {
			return false, false
		}
		i++
	}
	if i == len(args) {
		return piped, false
	}
	return shellDisplayTargetsAllowed(args[i:], cwd, scoped, false, true, piped)
}

func shellDisplayFindAllowed(args []string, cwd string, scoped []ScopedToolRule) (bool, bool) {
	i := 0
	if i < len(args) && args[i] == "-P" {
		i++
	}
	start := i
	for i < len(args) && !strings.HasPrefix(args[i], "-") && args[i] != "!" && args[i] != "(" && args[i] != ")" {
		i++
	}
	paths := args[start:i]
	if len(paths) == 0 {
		paths = []string{"."}
	}
	allowed, _ := shellDisplayTargetsAllowed(paths, cwd, scoped, true, false, false)
	if !allowed {
		return false, false
	}
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "-print", "-print0", "-empty", "-prune", "-depth", "-xdev", "-mount",
			"-true", "-false", "-a", "-and", "-o", "-or", "-not", "!", "(", ")":
			i++
		case "-name", "-iname", "-path", "-ipath", "-type", "-maxdepth", "-mindepth":
			i++
			if i >= len(args) || args[i] == "" || strings.HasPrefix(args[i], "-") {
				return false, false
			}
			if (arg == "-maxdepth" || arg == "-mindepth") && !displayNumber(args[i]) {
				return false, false
			}
			if arg == "-type" && (len(args[i]) != 1 || !strings.Contains("bcdpfls", args[i])) {
				return false, false
			}
			i++
		default:
			return false, false
		}
	}
	return true, true
}

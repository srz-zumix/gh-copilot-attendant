package permissionhook

import (
	"os"
	"path/filepath"
	"strings"
)

func hasSafeShellDisplayArguments(req *Request) bool {
	for key := range req.Arguments {
		switch key {
		case "command", "description", "mode", "initial_wait", "requestSandboxBypass", "request_sandbox_bypass":
		default:
			return false
		}
	}
	return true
}

func isDisplayPipe(command string, previous, current []bashWord) bool {
	if len(previous) == 0 || len(current) == 0 {
		return false
	}
	between := strings.TrimSpace(command[previous[len(previous)-1].end:current[0].start])
	return between == "|" || between == "|&"
}

func hasUnquotedTilde(command string, segment []bashWord) bool {
	for _, word := range segment {
		if word.assignment == nil && word.start < len(command) && command[word.start] == '~' {
			return true
		}
	}
	return false
}

// shellDisplayCommandAllowed reports whether the command matches the preset and
// whether its result depends on the working directory or a filesystem path.
func shellDisplayCommandAllowed(argv []string, cwd string, scoped []ScopedToolRule, piped bool) (bool, bool) {
	if len(argv) == 0 {
		return false, false
	}
	switch argv[0] {
	case "echo":
		return true, false
	case "printf":
		return safePrintf(argv[1:]), false
	case "pwd":
		if len(argv) > 2 || (len(argv) == 2 && argv[1] != "-L" && argv[1] != "-P") {
			return false, false
		}
		return shellDisplayPathAllowed(".", cwd, scoped, true), true
	case "ls", "cat", "head", "tail", "wc":
		return shellDisplayFilesAllowed(argv, cwd, scoped, piped)
	case "nl", "cut", "uniq", "stat", "sort", "jq", "tr", "basename", "dirname":
		return shellDisplayExtendedCommandAllowed(argv, cwd, scoped, piped)
	case "sed":
		return shellDisplaySedAllowed(argv[1:], cwd, scoped, piped)
	case "find":
		return shellDisplayFindAllowed(argv[1:], cwd, scoped)
	default:
		return false, false
	}
}

func shellDisplayFilesAllowed(argv []string, cwd string, scoped []ScopedToolRule, piped bool) (bool, bool) {
	command := argv[0]
	paths := []string{}
	afterSeparator := false
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if !afterSeparator && arg == "--" {
			if len(paths) > 0 {
				return false, false
			}
			afterSeparator = true
			continue
		}
		if !afterSeparator && strings.HasPrefix(arg, "-") && arg != "-" {
			if len(paths) > 0 {
				return false, false
			}
			consumed, ok := shellDisplayOption(command, argv[i:])
			if !ok {
				return false, false
			}
			i += consumed - 1
			continue
		}
		if !afterSeparator && command == "tail" && strings.HasPrefix(arg, "+") {
			return false, false
		}
		paths = append(paths, arg)
	}
	if len(paths) == 0 {
		if command == "ls" {
			paths = append(paths, ".")
		} else {
			return piped, false
		}
	}
	return shellDisplayTargetsAllowed(paths, cwd, scoped, command == "ls", command != "ls", piped)
}

func shellDisplayTargetsAllowed(targets []string, cwd string, scoped []ScopedToolRule, allowDirectory, allowStdin, piped bool) (bool, bool) {
	usesPath := false
	for _, target := range targets {
		if target == "-" && allowStdin {
			if !piped {
				return false, false
			}
			continue
		}
		if !shellDisplayPathAllowed(target, cwd, scoped, allowDirectory) {
			return false, false
		}
		usesPath = true
	}
	return true, usesPath
}

func shellDisplayOption(command string, args []string) (int, bool) {
	arg := args[0]
	if command == "head" || command == "tail" {
		if len(arg) > 1 && arg[0] == '-' && displayNumber(arg[1:]) {
			return 1, true
		}
		for _, option := range []string{"-n", "-c", "--lines", "--bytes"} {
			switch {
			case arg == option:
				return 2, len(args) > 1 && displayNumber(args[1])
			case strings.HasPrefix(arg, option+"="):
				return 1, displayNumber(strings.TrimPrefix(arg, option+"="))
			case len(option) == 2 && strings.HasPrefix(arg, option):
				return 1, displayNumber(strings.TrimPrefix(arg, option))
			}
		}
	}
	short := ""
	long := ""
	switch command {
	case "ls":
		short = "aAdfghilnopqrstux1GSF"
		long = "--all --almost-all --directory --human-readable --inode --long --numeric-uid-gid --reverse --size --classify"
	case "cat":
		short = "AbEnstuvT"
		long = "--number --number-nonblank --squeeze-blank --show-all --show-ends --show-nonprinting --show-tabs"
	case "head", "tail":
		short = "qv"
		long = "--quiet --silent --verbose"
	case "wc":
		short = "clmwL"
		long = "--bytes --chars --lines --words --max-line-length"
	}
	if strings.HasPrefix(arg, "--") {
		for _, allowed := range strings.Fields(long) {
			if arg == allowed {
				return 1, true
			}
		}
		return 0, false
	}
	if len(arg) < 2 {
		return 0, false
	}
	for _, flag := range arg[1:] {
		if !strings.ContainsRune(short, flag) {
			return 0, false
		}
	}
	return 1, true
}

func displayNumber(value string) bool {
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		value = value[1:]
	}
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func shellDisplayPathAllowed(target, cwd string, scoped []ScopedToolRule, allowDirectory bool) bool {
	roots := []string{}
	if filepath.IsAbs(cwd) {
		if root, err := filepath.EvalSymlinks(cwd); err == nil {
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				roots = append(roots, root)
			}
		}
	}
	for _, rule := range scoped {
		for _, path := range rule.Paths {
			if root, err := filepath.EvalSymlinks(path); err == nil {
				roots = append(roots, root)
			}
		}
	}
	resolved, ok := resolveTarget(target, cwd)
	if !ok || !withinRoots(resolved, roots) {
		return false
	}
	info, err := os.Stat(resolved)
	return err == nil && (info.Mode().IsRegular() || (allowDirectory && info.IsDir()))
}

func safePrintf(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "--" {
		args = args[1:]
	} else if strings.HasPrefix(args[0], "-") {
		return false
	}
	if len(args) == 0 {
		return false
	}
	format := args[0]
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i >= len(format) {
			return false
		}
		if format[i] == '%' {
			continue
		}
		for i < len(format) && strings.ContainsRune("#0- +'I", rune(format[i])) {
			i++
		}
		if i < len(format) && format[i] == '*' {
			i++
		} else {
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		if i < len(format) && format[i] == '.' {
			i++
			if i < len(format) && format[i] == '*' {
				i++
			} else {
				for i < len(format) && format[i] >= '0' && format[i] <= '9' {
					i++
				}
			}
		}
		if i >= len(format) || !strings.ContainsRune("diouxXfFeEgGaAcspqb", rune(format[i])) {
			return false
		}
	}
	return true
}

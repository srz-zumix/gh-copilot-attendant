package permissionhook

import "strings"

type gitReadSpec struct {
	flags       map[string]byte
	listOnly    bool
	numericArgs bool
}

func gitFlags(noValue, requiredValue, optionalValue string) map[string]byte {
	flags := make(map[string]byte)
	for _, flag := range strings.Fields(noValue) {
		flags[flag] = 'n'
	}
	for _, flag := range strings.Fields(requiredValue) {
		flags[flag] = 'v'
	}
	for _, flag := range strings.Fields(optionalValue) {
		flags[flag] = 'o'
	}
	return flags
}

var gitHistoryFlags = gitFlags(
	`--oneline --all --branches --tags --remotes --graph --first-parent
	--no-merges --merges --reverse --follow --stat --numstat --shortstat
	--name-only --name-status --no-patch --patch --raw --full-index --no-color
	--no-ext-diff --no-textconv --summary --no-decorate --quiet -q -p -s
	--cached --staged --no-index --check --exit-code --quiet -z --no-renames`,
	`--max-count -n --skip --since --until --after --before --author
	--committer --grep --format --date --diff-filter`,
	`--pretty --decorate --color --abbrev --stat --find-renames`,
)

var gitReadCommands = map[string]gitReadSpec{
	"status": {flags: gitFlags(
		`--short -s --branch -b -sb -bs --long --show-stash --ahead-behind
		--no-ahead-behind --no-renames --renames -z --null --no-optional-locks
		-uno -unormal -uall`,
		``, `--porcelain --untracked-files --ignored --ignore-submodules --column --find-renames -u`)},
	"log":  {flags: gitHistoryFlags, numericArgs: true},
	"show": {flags: gitHistoryFlags, numericArgs: true},
	"diff": {flags: gitHistoryFlags},
	"branch": {listOnly: true, flags: gitFlags(
		`--list -l --show-current --all -a --remotes -r --verbose -v -vv
		--no-color --ignore-case -i --omit-empty`,
		`--sort --format --points-at`, `--color --abbrev --column`)},
	"tag": {listOnly: true, flags: gitFlags(
		`--list -l --ignore-case -i --omit-empty --no-color`,
		`--sort --format --points-at`, `--color --column`)},
	"rev-parse": {flags: gitFlags(
		`--verify --quiet -q --show-toplevel --show-prefix --show-cdup
		--git-dir --git-common-dir --is-inside-work-tree --is-inside-git-dir
		--is-bare-repository --is-shallow-repository --show-object-format
		--symbolic --symbolic-full-name --all --branches --tags --remotes
		--local-env-vars --absolute-git-dir --show-superproject-working-tree`,
		`--git-path`, `--short --abbrev-ref --show-ref-format`)},
	"ls-files": {flags: gitFlags(
		`-z -t -v -f -c -d -m -o -i -s -k -u --cached --deleted --modified
		--others --ignored --stage --killed --unmerged --directory --eol
		--empty-directory --resolve-undo --exclude-standard --full-name
		--recurse-submodules --error-unmatch --debug --deduplicate --sparse`,
		`-x --exclude -X --exclude-from --exclude-per-directory --with-tree --format`,
		`--abbrev`)},
	"ls-tree": {flags: gitFlags(
		`-d -r -t -z -l --long --name-only --name-status --object-only
		--full-name --full-tree`, `--format`, `--abbrev`)},
	"show-ref": {flags: gitFlags(
		`--head -d --dereference -s --branches --tags --verify -q --quiet --exists`,
		``, `--hash --abbrev --exclude-existing`)},
	"for-each-ref": {flags: gitFlags(
		`--shell -s --perl -p --python --tcl --omit-empty --ignore-case
		--include-root-refs`,
		`--count --format --start-after --exclude --sort --points-at`,
		`--color --merged --no-merged --contains --no-contains`)},
	"describe": {flags: gitFlags(
		`--contains --debug --all --tags --long --first-parent --exact-match --always`,
		`--candidates --match --exclude`, `--abbrev --dirty --broken`)},
	"cat-file":   {flags: gitFlags(`-e -t -s -p --batch-check`, ``, ``)},
	"rev-list":   {flags: gitHistoryFlags, numericArgs: true},
	"diff-tree":  {flags: gitHistoryFlags},
	"diff-index": {flags: gitHistoryFlags},
	"diff-files": {flags: gitHistoryFlags},
}

func gitReadCommandAllowed(argv []string) bool {
	if len(argv) < 2 || argv[0] != "git" {
		return false
	}
	i := 1
	for i < len(argv) {
		switch argv[i] {
		case "--no-pager", "--no-optional-locks", "--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs":
			i++
		case "-C":
			if i+1 >= len(argv) || argv[i+1] == "" || strings.HasPrefix(argv[i+1], "-") {
				return false
			}
			i += 2
		default:
			goto command
		}
	}
command:
	if i >= len(argv) {
		return false
	}
	name := argv[i]
	i++
	if name == "remote" {
		return i == len(argv) || (i+1 == len(argv) && argv[i] == "-v")
	}
	if name == "worktree" {
		return i+1 == len(argv) && argv[i] == "list"
	}
	spec, ok := gitReadCommands[name]
	if !ok {
		return false
	}
	list := false
	positional := false
	afterSeparator := false
	for ; i < len(argv); i++ {
		arg := argv[i]
		if !afterSeparator && arg == "--" {
			afterSeparator = true
			continue
		}
		if afterSeparator || !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = true
			continue
		}
		if spec.numericArgs && isGitNumericOption(arg) {
			continue
		}
		flag, value, hasValue := strings.Cut(arg, "=")
		kind, known := spec.flags[flag]
		if !known || (hasValue && (kind == 'n' || value == "")) {
			return false
		}
		if kind == 'v' && !hasValue {
			i++
			if i >= len(argv) || argv[i] == "" || strings.HasPrefix(argv[i], "-") {
				return false
			}
		}
		if flag == "--list" || flag == "-l" {
			list = true
		}
	}
	return !spec.listOnly || !positional || list
}

func isGitNumericOption(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for _, c := range arg[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

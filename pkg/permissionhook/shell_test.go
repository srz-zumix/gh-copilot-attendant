package permissionhook

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBashCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want [][]string
	}{
		{"simple", "git status", [][]string{{"git", "status"}}},
		{"extra spaces", "  ls   -la  ", [][]string{{"ls", "-la"}}},
		{"single quotes", "grep 'a b' file", [][]string{{"grep", "a b", "file"}}},
		{"single quotes keep specials", "echo '$HOME;`x`'", [][]string{{"echo", "$HOME;`x`"}}},
		{"double quotes", `echo "a b"`, [][]string{{"echo", "a b"}}},
		{"double quote escapes", `echo "a\"b\\c\$d\x"`, [][]string{{"echo", `a"b\c$d\x`}}},
		{"empty quoted word", `echo ""`, [][]string{{"echo", ""}}},
		{"backslash escape", `echo a\;b`, [][]string{{"echo", "a;b"}}},
		{"line continuation", "git \\\nstatus", [][]string{{"git", "status"}}},
		{"and", "go build ./... && go test ./...", [][]string{{"go", "build", "./..."}, {"go", "test", "./..."}}},
		{"or", "ls || pwd", [][]string{{"ls"}, {"pwd"}}},
		{"pipe", "git log | head -5", [][]string{{"git", "log"}, {"head", "-5"}}},
		{"pipe stderr", "go test |& tail", [][]string{{"go", "test"}, {"tail"}}},
		{"semicolon and newline", "pwd; ls\nwc -l", [][]string{{"pwd"}, {"ls"}, {"wc", "-l"}}},
		{"background", "sleep 1 & ls", [][]string{{"sleep", "1"}, {"ls"}}},
		{"fd dup", "go test ./... 2>&1 | tail", [][]string{{"go", "test", "./..."}, {"tail"}}},
		{"dev null", "ls >/dev/null 2> /dev/null", [][]string{{"ls"}}},
		{"amp dev null", "ls &>/dev/null", [][]string{{"ls"}}},
		{"append dev null", "ls >> /dev/null", [][]string{{"ls"}}},
		{"quoted digit is word", `echo "2">/dev/null`, [][]string{{"echo", "2"}}},
		{"quoted glob literal", `ls '*.go' "p?sh" \[a]`, [][]string{{"ls", "*.go", "p?sh", "[a]"}}},
		{"escaped digit is word before redirect", `rm \3>/dev/null build.tmp`, [][]string{{"rm", "3", "build.tmp"}}},
		{"escaped digit command before redirect", `\3>/dev/null`, [][]string{{"3"}}},
		{"hash inside word", "echo a#b", [][]string{{"echo", "a#b"}}},
		{"constant leading assignment", "GH_TOKEN=synthetic gh pr view", [][]string{{"GH_TOKEN=synthetic", "gh", "pr", "view"}}},
		{"simple reference assignment", "GH_TOKEN=$TOKEN gh pr view", [][]string{{"GH_TOKEN=$TOKEN", "gh", "pr", "view"}}},
		{"braced reference assignment", `GH_TOKEN="${TOKEN}" gh pr view`, [][]string{{"GH_TOKEN=${TOKEN}", "gh", "pr", "view"}}},
		{"multiple assignments", `GH_TOKEN=synthetic NO_COLOR=1 gh pr view`, [][]string{{"GH_TOKEN=synthetic", "NO_COLOR=1", "gh", "pr", "view"}}},
		{"quoted fixed assignment", `GH_TOKEN='synthetic value' gh pr view`, [][]string{{"GH_TOKEN=synthetic value", "gh", "pr", "view"}}},
		{"escaped dollar is literal", `GH_TOKEN=\$TOKEN gh pr view`, [][]string{{"GH_TOKEN=$TOKEN", "gh", "pr", "view"}}},
		{"quoted assignment-looking word", `"GH_TOKEN=value" gh pr view`, [][]string{{"GH_TOKEN=value", "gh", "pr", "view"}}},
		{"escaped assignment delimiter is command word", `GH_TOKEN\=value gh pr view`, [][]string{{"GH_TOKEN=value", "gh", "pr", "view"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBashCommand(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseBashCommandUnsupported(t *testing.T) {
	inputs := []string{
		"",
		"   ",
		"echo $HOME",
		"echo ${HOME}",
		"echo $(rm -rf /)",
		"echo `rm -rf /`",
		`echo "$(rm -rf /)"`,
		"echo \"`id`\"",
		"echo $'\\x41'",
		"(cd /tmp && ls)",
		"{ ls; }",
		"echo {a,b}",
		"cat <(ls)",
		"cat < file",
		"cat <<< text",
		"ls > out.txt",
		"ls >> out.txt",
		"ls 2>err.txt",
		"ls >&out.txt",
		"ls >| /dev/null",
		"ls >/dev/null2",
		"! ls",
		`echo "!!"`,
		"ls # comment",
		"echo 'unterminated",
		`echo "unterminated`,
		"echo \\",
		"ls *.go",
		"git p?sh origin main",
		"ls [ab].txt",
		"GH_TOKEN=$(printenv) gh pr view",
		"GH_TOKEN=$TOKEN-suffix gh pr view",
		"GH_TOKEN=$TOKEN$OTHER gh pr view",
		"GH_TOKEN=~/.config gh pr view",
		"ls ~",
		"ls ~/bin",
		"ls ~root",
		"GH_TOKEN=prefix:~/bin gh pr view",
		"PATH+=/tmp gh pr view",
		`PATH\+=/tmp gh pr view`,
		`"PATH"+=/tmp gh pr view`,
		"gh pr view $TOKEN",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			_, err := ParseBashCommand(in)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnsupportedSyntax))
		})
	}
}

func TestParseBashAssignmentProvenance(t *testing.T) {
	tests := []struct {
		command string
		name    string
		value   string
		dynamic string
		raw     string
	}{
		{`TOKEN=secret gh pr view`, "TOKEN", "secret", "", "secret"},
		{`TOKEN="$SOURCE" gh pr view`, "TOKEN", "$SOURCE", "SOURCE", `"$SOURCE"`},
		{`TOKEN=\$SOURCE gh pr view`, "TOKEN", "$SOURCE", "", `\$SOURCE`},
	}
	for _, tt := range tests {
		segments, err := parseBashCommand(tt.command)
		require.NoError(t, err, tt.command)
		require.NotEmpty(t, segments)
		require.NotNil(t, segments[0][0].assignment, tt.command)
		assignment := segments[0][0].assignment
		assert.Equal(t, tt.name, assignment.name)
		assert.Equal(t, tt.value, assignment.value)
		assert.Equal(t, tt.dynamic, assignment.reference)
		assert.Equal(t, tt.raw, tt.command[assignment.valueStart:assignment.valueEnd])
	}

	for _, command := range []string{
		`"TOKEN=secret" gh pr view`,
		`TOKEN\=secret gh pr view`,
	} {
		segments, err := parseBashCommand(command)
		require.NoError(t, err, command)
		require.NotEmpty(t, segments)
		assert.Nil(t, segments[0][0].assignment, command)
	}
}

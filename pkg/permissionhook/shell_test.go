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
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			_, err := ParseBashCommand(in)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnsupportedSyntax))
		})
	}
}

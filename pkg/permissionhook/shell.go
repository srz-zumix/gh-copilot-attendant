package permissionhook

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupportedSyntax is returned when a shell command uses syntax that the
// conservative parser refuses to interpret (expansions, substitutions, subshells, etc.).
var ErrUnsupportedSyntax = errors.New("unsupported shell syntax")

// ParseBashCommand splits a bash command line into simple commands (argv lists)
// separated by ";", "&", "&&", "||", "|", "|&" or newlines.
//
// The parser is intentionally conservative: any construct whose meaning depends on
// runtime expansion (parameter/command/arithmetic expansion, subshells, brace groups,
// history expansion, unquoted pathname expansion (*, ?, [), comments, input redirection, or output redirection to anything
// other than /dev/null or a file descriptor) yields ErrUnsupportedSyntax.
func ParseBashCommand(s string) ([][]string, error) {
	p := &bashParser{src: s}
	if err := p.parse(); err != nil {
		return nil, err
	}
	if len(p.segments) == 0 {
		return nil, fmt.Errorf("%w: empty command", ErrUnsupportedSyntax)
	}
	return p.segments, nil
}

type bashParser struct {
	src        string
	segments   [][]string
	current    []string
	word       strings.Builder
	inWord     bool
	wordQuoted bool
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnsupportedSyntax}, args...)...)
}

func (p *bashParser) endWord() {
	if p.inWord {
		p.current = append(p.current, p.word.String())
	}
	p.word.Reset()
	p.inWord = false
	p.wordQuoted = false
}

func (p *bashParser) endSegment() {
	p.endWord()
	if len(p.current) > 0 {
		p.segments = append(p.segments, p.current)
	}
	p.current = nil
}

func (p *bashParser) peek(i int) byte {
	if i < len(p.src) {
		return p.src[i]
	}
	return 0
}

func (p *bashParser) parse() error {
	s := p.src
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case ' ', '\t':
			p.endWord()
			i++
		case '\n':
			p.endSegment()
			i++
		case '\\':
			if i+1 >= len(s) {
				return unsupported("trailing backslash")
			}
			if s[i+1] != '\n' {
				p.word.WriteByte(s[i+1])
				p.inWord = true
				p.wordQuoted = true
			}
			i += 2
		case '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return unsupported("unterminated single quote")
			}
			p.word.WriteString(s[i+1 : i+1+end])
			p.inWord = true
			p.wordQuoted = true
			i += end + 2
		case '"':
			next, err := p.parseDoubleQuoted(i + 1)
			if err != nil {
				return err
			}
			p.inWord = true
			p.wordQuoted = true
			i = next
		case ';':
			p.endSegment()
			i++
		case '&':
			switch p.peek(i + 1) {
			case '&':
				p.endSegment()
				i += 2
			case '>':
				p.endWord()
				next, err := p.parseRedirectTarget(i + 2)
				if err != nil {
					return err
				}
				i = next
			default:
				p.endSegment()
				i++
			}
		case '|':
			p.endSegment()
			i++
			if n := p.peek(i); n == '|' || n == '&' {
				i++
			}
		case '>':
			if p.inWord && !p.wordQuoted && isDigits(p.word.String()) {
				p.word.Reset()
				p.inWord = false
			} else {
				p.endWord()
			}
			next, err := p.parseRedirectTarget(i + 1)
			if err != nil {
				return err
			}
			i = next
		case '<':
			return unsupported("input redirection")
		case '$', '`':
			return unsupported("expansion %q", c)
		case '(', ')', '{', '}':
			return unsupported("grouping %q", c)
		case '!':
			return unsupported("history expansion or negation")
		case '*', '?', '[':
			return unsupported("pathname expansion %q", c)
		case '#':
			if !p.inWord {
				return unsupported("comment")
			}
			p.word.WriteByte(c)
			i++
		default:
			p.word.WriteByte(c)
			p.inWord = true
			i++
		}
	}
	p.endSegment()
	return nil
}

// parseDoubleQuoted consumes a double-quoted string starting after the opening quote
// and returns the index after the closing quote.
func (p *bashParser) parseDoubleQuoted(i int) (int, error) {
	s := p.src
	for i < len(s) {
		c := s[i]
		switch c {
		case '"':
			return i + 1, nil
		case '\\':
			if i+1 >= len(s) {
				return 0, unsupported("unterminated double quote")
			}
			switch n := s[i+1]; n {
			case '$', '`', '"', '\\':
				p.word.WriteByte(n)
				i += 2
			case '\n':
				i += 2
			default:
				p.word.WriteByte('\\')
				i++
			}
		case '$', '`':
			return 0, unsupported("expansion %q in double quotes", c)
		case '!':
			return 0, unsupported("history expansion in double quotes")
		default:
			p.word.WriteByte(c)
			i++
		}
	}
	return 0, unsupported("unterminated double quote")
}

// parseRedirectTarget accepts only output redirections to /dev/null or fd duplication
// (e.g. "2>&1"). i points just after the first '>' (or "&>").
func (p *bashParser) parseRedirectTarget(i int) (int, error) {
	s := p.src
	if p.peek(i) == '>' {
		i++
	}
	if p.peek(i) == '&' {
		i++
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start || !isWordBoundary(p.peek(i)) {
			return 0, unsupported("redirection to file descriptor")
		}
		return i, nil
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	start := i
	for i < len(s) && !isWordBoundary(s[i]) {
		i++
	}
	if s[start:i] != "/dev/null" {
		return 0, unsupported("output redirection")
	}
	return i, nil
}

func isWordBoundary(c byte) bool {
	switch c {
	case 0, ' ', '\t', '\n', ';', '&', '|':
		return true
	}
	return false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

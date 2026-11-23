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
// The parser is intentionally conservative: it rejects runtime expansions except a
// whole simple variable reference in a leading assignment. It also rejects subshells,
// brace groups, history expansion, unquoted pathname expansion (*, ?, [), comments,
// input redirection, and output redirection other than /dev/null or a file descriptor.
func ParseBashCommand(s string) ([][]string, error) {
	parsed, err := parseBashCommand(s)
	if err != nil {
		return nil, err
	}
	segments := make([][]string, len(parsed))
	for i, segment := range parsed {
		for _, word := range segment {
			segments[i] = append(segments[i], word.value)
		}
	}
	return segments, nil
}

type bashAssignment struct {
	name           string
	value          string
	valueStart     int
	valueEnd       int
	reference      string
	referenceStart int
	referenceEnd   int
}

type bashWord struct {
	value      string
	start      int
	end        int
	assignment *bashAssignment
}

func parseBashCommand(s string) ([][]bashWord, error) {
	p, err := parseBashCommandDetails(s)
	if err != nil {
		return nil, err
	}
	return p.segments, nil
}

func parseBashCommandDetails(s string) (*bashParser, error) {
	p := &bashParser{src: s, leading: true}
	if err := p.parse(); err != nil {
		return nil, err
	}
	if len(p.segments) == 0 {
		return nil, fmt.Errorf("%w: empty command", ErrUnsupportedSyntax)
	}
	return p, nil
}

type bashParser struct {
	src                    string
	segments               [][]bashWord
	current                []bashWord
	word                   strings.Builder
	inWord                 bool
	wordQuoted             bool
	wordStart              int
	leading                bool
	assignmentName         string
	assignmentValueStart   int
	reference              string
	referenceStart         int
	referenceEnd           int
	assignmentDynamicCount int
	candidateAssignment    bool
	hasRedirect            bool
	hasBackground          bool
	hasPipe                bool
}

func unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnsupportedSyntax}, args...)...)
}

func (p *bashParser) beginWord(start int) {
	if !p.inWord {
		p.inWord = true
		p.wordStart = start
		p.candidateAssignment = p.leading
	}
}

func (p *bashParser) endWord(end int) error {
	if p.inWord {
		if p.assignmentName != "" && p.assignmentDynamicCount > 0 {
			rawValue := p.src[p.assignmentValueStart:end]
			ref := p.src[p.referenceStart:p.referenceEnd]
			if rawValue != ref && rawValue != `"`+ref+`"` {
				return unsupported("complex assignment expansion")
			}
		}
		value := p.word.String()
		if p.assignmentName == "" && p.leading && isAppendedAssignment(value) {
			return unsupported("appended assignment")
		}
		word := bashWord{value: value, start: p.wordStart, end: end}
		if p.assignmentName != "" {
			word.assignment = &bashAssignment{
				name: p.assignmentName, value: strings.TrimPrefix(value, p.assignmentName+"="),
				valueStart: p.assignmentValueStart, valueEnd: end, reference: p.reference,
				referenceStart: p.referenceStart, referenceEnd: p.referenceEnd,
			}
			p.leading = true
		} else {
			p.leading = false
		}
		p.current = append(p.current, word)
	}
	p.word.Reset()
	p.inWord = false
	p.wordQuoted = false
	p.wordStart = 0
	p.assignmentName = ""
	p.assignmentValueStart = 0
	p.reference = ""
	p.referenceStart = 0
	p.referenceEnd = 0
	p.assignmentDynamicCount = 0
	p.candidateAssignment = false
	return nil
}

func (p *bashParser) endSegment(end int) error {
	if err := p.endWord(end); err != nil {
		return err
	}
	if len(p.current) > 0 {
		p.segments = append(p.segments, p.current)
	}
	p.current = nil
	p.leading = true
	return nil
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
			if err := p.endWord(i); err != nil {
				return err
			}
			i++
		case '\n':
			if err := p.endSegment(i); err != nil {
				return err
			}
			i++
		case '\\':
			if i+1 >= len(s) {
				return unsupported("trailing backslash")
			}
			if s[i+1] != '\n' {
				p.beginWord(i)
				if p.assignmentName == "" {
					p.candidateAssignment = false
				}
				p.word.WriteByte(s[i+1])
				p.wordQuoted = true
			}
			i += 2
		case '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return unsupported("unterminated single quote")
			}
			p.beginWord(i)
			if p.assignmentName == "" {
				p.candidateAssignment = false
			}
			p.word.WriteString(s[i+1 : i+1+end])
			p.wordQuoted = true
			i += end + 2
		case '"':
			p.beginWord(i)
			if p.assignmentName == "" {
				p.candidateAssignment = false
			}
			next, err := p.parseDoubleQuoted(i + 1)
			if err != nil {
				return err
			}
			p.wordQuoted = true
			i = next
		case ';':
			if err := p.endSegment(i); err != nil {
				return err
			}
			i++
		case '&':
			switch p.peek(i + 1) {
			case '&':
				if err := p.endSegment(i); err != nil {
					return err
				}
				i += 2
			case '>':
				p.hasRedirect = true
				if err := p.endWord(i); err != nil {
					return err
				}
				next, err := p.parseRedirectTarget(i + 2)
				if err != nil {
					return err
				}
				i = next
			default:
				p.hasBackground = true
				if err := p.endSegment(i); err != nil {
					return err
				}
				i++
			}
		case '|':
			if err := p.endSegment(i); err != nil {
				return err
			}
			i++
			if p.peek(i) == '|' {
				i++
			} else {
				p.hasPipe = true
				if p.peek(i) == '&' {
					i++
				}
			}
		case '>':
			p.hasRedirect = true
			if p.inWord && !p.wordQuoted && isDigits(p.word.String()) {
				p.word.Reset()
				p.inWord = false
				p.wordStart = 0
				p.candidateAssignment = false
			} else {
				if err := p.endWord(i); err != nil {
					return err
				}
			}
			next, err := p.parseRedirectTarget(i + 1)
			if err != nil {
				return err
			}
			i = next
		case '<':
			return unsupported("input redirection")
		case '$':
			next, err := p.writeReference(i)
			if err != nil {
				return err
			}
			i = next
		case '`':
			return unsupported("command substitution")
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
			if c == '~' && !p.inWord {
				return unsupported("tilde expansion")
			}
			p.beginWord(i)
			if c == '=' && p.assignmentName == "" && p.candidateAssignment {
				name := p.word.String()
				if validShellName(name) {
					p.assignmentName = name
					p.assignmentValueStart = i + 1
				} else if strings.HasSuffix(name, "+") && validShellName(strings.TrimSuffix(name, "+")) {
					return unsupported("appended assignment")
				}
			}
			if c == '~' && p.assignmentName != "" &&
				(p.word.Len() == len(p.assignmentName)+1 || strings.HasSuffix(p.word.String(), ":")) {
				return unsupported("tilde expansion in assignment")
			}
			p.word.WriteByte(c)
			i++
		}
	}
	return p.endSegment(len(s))
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
			if c == '`' {
				return 0, unsupported("command substitution in double quotes")
			}
			next, err := p.writeReference(i)
			if err != nil {
				return 0, err
			}
			i = next
		case '!':
			return 0, unsupported("history expansion in double quotes")
		default:
			p.word.WriteByte(c)
			i++
		}
	}
	return 0, unsupported("unterminated double quote")
}

func (p *bashParser) writeReference(start int) (int, error) {
	if p.assignmentName == "" || !p.leading {
		return 0, unsupported("expansion outside a leading assignment")
	}
	i := start + 1
	braced := i < len(p.src) && p.src[i] == '{'
	if braced {
		i++
	}
	nameStart := i
	for i < len(p.src) && shellNameByte(p.src[i], i == nameStart) {
		i++
	}
	if i == nameStart {
		return 0, unsupported("unsupported assignment expansion")
	}
	name := p.src[nameStart:i]
	if braced {
		if i >= len(p.src) || p.src[i] != '}' {
			return 0, unsupported("unsupported assignment expansion")
		}
		i++
	}
	p.word.WriteString(p.src[start:i])
	p.reference = name
	p.referenceStart = start
	p.referenceEnd = i
	p.assignmentDynamicCount++
	return i, nil
}

func validShellName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !shellNameByte(name[i], i == 0) {
			return false
		}
	}
	return true
}

func isAppendedAssignment(word string) bool {
	name, _, ok := strings.Cut(word, "+=")
	return ok && validShellName(name)
}

func shellNameByte(c byte, first bool) bool {
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' ||
		!first && c >= '0' && c <= '9'
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

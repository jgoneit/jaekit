package bundle

import (
	"errors"
	"strings"
)

// ErrShellSyntax means the command uses shell syntax that ha does not
// interpret because it runs commands without a shell.
var ErrShellSyntax = errors.New("shell syntax")

// ErrQuote means a quote is not closed.
var ErrQuote = errors.New("unclosed quote")

// ErrEmpty means the command has no words.
var ErrEmpty = errors.New("empty command")

// SplitCommand turns a plan command string into argv without a shell.
// Words are separated by spaces and tabs. Single quotes keep text literally;
// double quotes allow \" and \\; a backslash outside quotes escapes the next
// character. Unquoted pipes, redirects, `&`, `;`, backquotes, and `$` are
// rejected because nothing would interpret them.
func SplitCommand(s string) ([]string, error) {
	var args []string
	var b strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			if inWord {
				args = append(args, b.String())
				b.Reset()
				inWord = false
			}
		case c == '\'':
			inWord = true
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, ErrQuote
			}
			b.WriteString(s[i+1 : i+1+end])
			i += end + 1
		case c == '"':
			inWord = true
			j := i + 1
			for ; j < len(s); j++ {
				if s[j] == '\\' && j+1 < len(s) && (s[j+1] == '"' || s[j+1] == '\\') {
					b.WriteByte(s[j+1])
					j++
					continue
				}
				if s[j] == '"' {
					break
				}
				b.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, ErrQuote
			}
			i = j
		case c == '\\':
			inWord = true
			if i+1 < len(s) {
				b.WriteByte(s[i+1])
				i++
			}
		case strings.IndexByte("|&;<>`$", c) >= 0:
			return nil, ErrShellSyntax
		default:
			inWord = true
			b.WriteByte(c)
		}
	}
	if inWord {
		args = append(args, b.String())
	}
	if len(args) == 0 {
		return nil, ErrEmpty
	}
	return args, nil
}

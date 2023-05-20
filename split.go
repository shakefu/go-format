package format

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Splitter splits the string
type Splitter struct {
	orig    string
	rest    string
	context Context

	count int

	err error
	cur string
}

// NewSplitter ...
func NewSplitter(text string, context Context) *Splitter {
	return &Splitter{
		orig:    text,
		rest:    text,
		context: context,
	}
}

func nipString(content string) (res string, rest string, err error) {
	content = content[1:] // Passing first character
	var pos int
	for {
		pos = strings.IndexByte(content, '"')
		if pos < 0 {
			err = fmt.Errorf("didn't find string end at %s", content)
			return
		}
		res += content[:pos]
		if strings.HasSuffix(res, "\\") {
			res += content[pos : pos+1]
			content = content[pos+1:]
		} else {
			content = content[pos+1:]
			break
		}
	}

	res = strings.Replace(res, "\\t", "\t", -1)
	res = strings.Replace(res, "\\n", "\n", -1)
	res = strings.Replace(res, "\\\"", "\"", -1)
	res = strings.Replace(res, "\\r", "\r", -1)
	res = strings.Replace(res, "\\\\", "\\", -1)
	rest = content
	return
}

func nipIdentifier(content string) (string, string, error) {
	if len(content) == 0 {
		return "", "", fmt.Errorf("expected heading identifier or }, got empty string instead")
	}
	if content[0] == '|' || content[0] == '}' {
		return "", content, nil
	}
	i := 0
	for i < len(content) && isWord(rune(content[i])) {
		i++
	}
	if content[0] == '+' || content[0] == '-' {
		// must be date arithmetic
		return "", content, nil
	}
	if i == 0 {
		return "", "", fmt.Errorf("expected heading identifier in %s", content)
	}

	return content[:i], content[i:], nil
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}

func isSimpleWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func nipOpenIdentifier(content string) (string, string, error) {
	i := 0
	for i < len(content) && unicode.IsDigit(rune(content[i])) {
		i++
	}
	if i > 0 {
		return content[:i], content[i:], nil
	}
	for i < len(content) && isSimpleWord(rune(content[i])) {
		i++
	}

	return content[:i], content[i:], nil
}

func locateABuck(rest string) int {
	var off int
	for i := strings.IndexByte(rest, '$'); i >= 0; i = strings.IndexByte(rest, '$') {
		if i == len(rest)-1 {
			return i + off
		}

		if rest[i+1] != '$' {
			return i + off
		}

		off += i + 2
		rest = rest[i+2:]
	}

	return -1
}

// Split cut the next chunk and apply context formatting once it is a piece of format
func (s *Splitter) Split() bool {
	if len(s.rest) == 0 {
		return false
	}

	pos := locateABuck(s.rest)
	if pos < 0 {
		s.cur = s.rest
		s.rest = ""
		return true
	} else if pos > 0 {
		s.cur = s.rest[:pos]
		s.rest = s.rest[pos:]
		return true
	}

	var ident string
	var err error
	var formatter Formatter

	if len(s.rest) == 0 || s.rest == "$" {
		ident = strconv.Itoa(s.count)
		s.count++
		formatter, err = s.context.GetFormatter(ident)
		if err != nil {
			s.err = err
			return false
		}
		s.cur, s.err = formatter.Format("")
		if len(s.rest) > 0 {
			s.rest = s.rest[len(s.rest):]
		}
		return s.err == nil
	} else if isWord(rune(s.rest[1])) {
		// This is a simple subsitution
		s.rest = s.rest[1:]
		ident, s.rest, err = nipOpenIdentifier(s.rest)
		if err != nil {
			s.err = err
			return false
		}
		formatter, err = s.context.GetFormatter(ident)
		if err != nil {
			s.err = err
			return false
		}
		s.cur, s.err = formatter.Format("")
		return s.err == nil
	} else if s.rest[1] != '{' {
		ident = strconv.Itoa(s.count)
		s.count++
		formatter, err := s.context.GetFormatter(ident)
		if err != nil {
			s.err = err
			return false
		}
		s.cur, s.err = formatter.Format("")
		s.rest = s.rest[1:]
		return s.err == nil
	}

	// This is a format!
	s.rest = s.rest[2:]
	s.rest = strings.TrimLeft(s.rest, " \t\r\n")

	// Nip the leading identifier
	ident, s.rest, err = nipIdentifier(s.rest)
	if err != nil {
		s.err = err
		return false
	}
	if len(ident) == 0 {
		ident = strconv.Itoa(s.count)
		s.count++
	}
	formatter, err = s.context.GetFormatter(ident)
	if err != nil {
		s.err = err
		return false
	}

	s.rest = strings.TrimLeft(s.rest, " \t\r\n")
	if strings.HasPrefix(s.rest, "}") {
		// It is just a substitution
		s.cur, s.err = formatter.Format("")
		s.rest = s.rest[1:]
		return s.err == nil
	}

	if !strings.HasPrefix(s.rest, "|") {
		// There is a clarification
		pos := strings.IndexByte(s.rest, '|')
		if pos < 0 {
			// Let the clarification needs a format
			s.err = fmt.Errorf("couldn't find clarification end in %s", s.rest)
			return false
		}
		clarification := s.rest[:pos]
		s.rest = s.rest[pos:]
		formatter, s.err = formatter.Clarify(clarification)
		if s.err != nil {
			return false
		}
	}

	s.rest = s.rest[1:]
	s.rest = strings.TrimLeft(s.rest, " \t\r\n")
	if len(s.rest) == 0 {
		s.err = fmt.Errorf("couldn't find format end in %s", s.rest)
		return false
	}

	var format string
	if s.rest[0] == '"' {
		format, s.rest, s.err = nipString(s.rest)
		if s.err != nil {
			return false
		}
		pos = strings.IndexByte(s.rest, '}')
		if pos < 0 {
			s.err = fmt.Errorf("couldn't find format end in %s", s.rest)
			return false
		}
		s.rest = s.rest[pos+1:]
	} else {
		pos = strings.IndexByte(s.rest, '}')
		if pos < 0 {
			s.err = fmt.Errorf("couldn't find format end in %s", s.rest)
			return false
		}
		format = strings.TrimSpace(s.rest[:pos])
		s.rest = s.rest[pos+1:]
	}

	s.cur, s.err = formatter.Format(format)
	return s.err == nil
}

// Text ...
func (s *Splitter) Text() string {
	return strings.Replace(s.cur, "$$", "$", -1)
}

// Err ...
func (s *Splitter) Err() error {
	return s.err
}

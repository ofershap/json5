package json5

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SyntaxError describes a JSON5 syntax problem with position.
type SyntaxError struct {
	msg    string
	Offset int64 // byte offset
	Line   int   // 1-based
	Column int   // 1-based, in runes
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("json5: syntax error at line %d, column %d (offset %d): %s", e.Line, e.Column, e.Offset, e.msg)
}

type scanner struct {
	data []byte
	pos  int // byte offset
	line int // 1-based
	col  int // 1-based rune column
}

func newScanner(data []byte) *scanner { return &scanner{data: data, line: 1, col: 1} }

func (s *scanner) eof() bool { return s.pos >= len(s.data) }

func (s *scanner) peek() rune {
	if s.eof() {
		return -1
	}
	r, _ := utf8.DecodeRune(s.data[s.pos:])
	return r
}

func (s *scanner) peekAt(n int) rune {
	p := s.pos
	for i := 0; i < n; i++ {
		if p >= len(s.data) {
			return -1
		}
		_, sz := utf8.DecodeRune(s.data[p:])
		p += sz
	}
	if p >= len(s.data) {
		return -1
	}
	r, _ := utf8.DecodeRune(s.data[p:])
	return r
}

func (s *scanner) next() rune {
	if s.eof() {
		return -1
	}
	r, sz := utf8.DecodeRune(s.data[s.pos:])
	s.pos += sz
	if r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029 {
		s.line++
		s.col = 1
	} else {
		s.col++
	}
	return r
}

func (s *scanner) errf(format string, args ...any) *SyntaxError {
	return &SyntaxError{msg: fmt.Sprintf(format, args...), Offset: int64(s.pos), Line: s.line, Column: s.col}
}

func isLineTerminator(r rune) bool { return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029 }

func isWhitespace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', ' ', 0x00A0, 0xFEFF:
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// skipSpace consumes whitespace, line terminators and comments.
func (s *scanner) skipSpace() error {
	for {
		r := s.peek()
		if isWhitespace(r) || isLineTerminator(r) {
			s.next()
			continue
		}
		if r == '/' {
			r2 := s.peekAt(1)
			if r2 == '/' {
				s.next()
				s.next()
				for !s.eof() && !isLineTerminator(s.peek()) {
					s.next()
				}
				continue
			}
			if r2 == '*' {
				s.next()
				s.next()
				closed := false
				for !s.eof() {
					if s.peek() == '*' && s.peekAt(1) == '/' {
						s.next()
						s.next()
						closed = true
						break
					}
					s.next()
				}
				if !closed {
					return s.errf("unterminated block comment")
				}
				continue
			}
			return nil
		}
		return nil
	}
}

func isIDStart(r rune) bool {
	return r == '$' || r == '_' || unicode.IsLetter(r)
}

func isIDContinue(r rune) bool {
	return isIDStart(r) || unicode.IsDigit(r) ||
		unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) ||
		unicode.Is(unicode.Pc, r) || r == 0x200C || r == 0x200D
}

// scanIdentifier scans an IdentifierName (unquoted object key).
func (s *scanner) scanIdentifier() (string, error) {
	var b strings.Builder
	r := s.peek()
	if r == '\\' && s.peekAt(1) == 'u' {
		rn, err := s.scanUnicodeEscape()
		if err != nil {
			return "", err
		}
		r = rn
	} else {
		s.next()
	}
	if !isIDStart(r) {
		return "", s.errf("invalid identifier start %q", r)
	}
	b.WriteRune(r)
	for {
		r = s.peek()
		if r == '\\' && s.peekAt(1) == 'u' {
			rn, err := s.scanUnicodeEscape()
			if err != nil {
				return "", err
			}
			if !isIDContinue(rn) {
				return "", s.errf("invalid identifier character %q", rn)
			}
			b.WriteRune(rn)
			continue
		}
		if !isIDContinue(r) {
			break
		}
		s.next()
		b.WriteRune(r)
	}
	return b.String(), nil
}

// scanUnicodeEscape consumes \uHHHH and returns the rune.
func (s *scanner) scanUnicodeEscape() (rune, error) {
	s.next() // backslash
	s.next() // u
	var v rune
	for i := 0; i < 4; i++ {
		r := s.peek()
		d, ok := hexVal(r)
		if !ok {
			return 0, s.errf("invalid unicode escape")
		}
		s.next()
		v = v*16 + d
	}
	return v, nil
}

func hexVal(r rune) (rune, bool) {
	switch {
	case r >= '0' && r <= '9':
		return r - '0', true
	case r >= 'a' && r <= 'f':
		return r - 'a' + 10, true
	case r >= 'A' && r <= 'F':
		return r - 'A' + 10, true
	}
	return 0, false
}

// scanString scans a single- or double-quoted string.
func (s *scanner) scanString() (string, error) {
	quote := s.next() // ' or "
	var b strings.Builder
	for {
		if s.eof() {
			return "", s.errf("unterminated string")
		}
		r := s.next()
		if r == quote {
			return b.String(), nil
		}
		if isLineTerminator(r) {
			return "", s.errf("unescaped line terminator in string")
		}
		if r != '\\' {
			b.WriteRune(r)
			continue
		}
		// escape
		e := s.peek()
		switch {
		case isLineTerminator(e):
			// line continuation
			if e == '\r' && s.peekAt(1) == '\n' {
				s.next()
			}
			s.next()
		case e == '\'' || e == '"' || e == '\\' || e == '/':
			s.next()
			b.WriteRune(e)
		case e == 'b':
			s.next()
			b.WriteByte('\b')
		case e == 'f':
			s.next()
			b.WriteByte('\f')
		case e == 'n':
			s.next()
			b.WriteByte('\n')
		case e == 'r':
			s.next()
			b.WriteByte('\r')
		case e == 't':
			s.next()
			b.WriteByte('\t')
		case e == 'v':
			s.next()
			b.WriteByte('\v')
		case e == '0':
			s.next()
			if d := s.peek(); d >= '0' && d <= '9' {
				return "", s.errf("\\0 must not be followed by a digit")
			}
			b.WriteByte(0)
		case e == 'x':
			s.next()
			var v rune
			for i := 0; i < 2; i++ {
				d, ok := hexVal(s.peek())
				if !ok {
					return "", s.errf("invalid \\x escape")
				}
				s.next()
				v = v*16 + d
			}
			b.WriteRune(v)
		case e == 'u':
			rn, err := s.scanUnicodeEscape()
			if err != nil {
				return "", err
			}
			b.WriteRune(rn)
		default:
			// any other escaped char is the char itself
			s.next()
			b.WriteRune(e)
		}
	}
}

// scanNumber scans a JSON5 number literal and returns canonical JSON text.
func (s *scanner) scanNumber() (string, error) {
	start := s.pos
	neg := false
	if r := s.peek(); r == '+' || r == '-' {
		s.next()
		neg = r == '-'
	}
	// Infinity / NaN
	if s.peek() == 'I' {
		for _, c := range "Infinity" {
			if s.peek() != c {
				return "", s.errf("invalid number")
			}
			s.next()
		}
		if neg {
			return "-Infinity", nil
		}
		return "Infinity", nil
	}
	if s.peek() == 'N' {
		for _, c := range "NaN" {
			if s.peek() != c {
				return "", s.errf("invalid number")
			}
			s.next()
		}
		if neg {
			// -NaN: value is NaN; sign is insignificant
			return "NaN", nil
		}
		return "NaN", nil
	}
	// hex
	if s.peek() == '0' && (s.peekAt(1) == 'x' || s.peekAt(1) == 'X') {
		s.next()
		s.next()
		hstart := s.pos
		for {
			if _, ok := hexVal(s.peek()); !ok {
				break
			}
			s.next()
		}
		if s.pos == hstart {
			return "", s.errf("invalid hex number")
		}
		var v uint64
		var overflow bool
		for _, c := range s.data[hstart:s.pos] {
			d, _ := hexVal(rune(c))
			nv := v*16 + uint64(d)
			if nv < v {
				overflow = true
			}
			v = nv
		}
		if overflow {
			return "", s.errf("hex number overflows uint64")
		}
		if neg {
			return fmt.Sprintf("-%d", v), nil
		}
		return fmt.Sprintf("%d", v), nil
	}
	// decimal: [int][.frac][exp] | .frac[exp]
	digits := false
	intStart := s.pos
	for {
		r := s.peek()
		if r >= '0' && r <= '9' {
			s.next()
			digits = true
		} else {
			break
		}
	}
	intLen := s.pos - intStart
	if intLen > 1 && s.data[intStart] == '0' {
		// leading zeros are not valid JSON5 (ES5.1 NumericLiteral)
		return "", s.errf("invalid number with leading zero")
	}
	if s.peek() == '.' {
		s.next()
		for {
			r := s.peek()
			if r >= '0' && r <= '9' {
				s.next()
				digits = true
			} else {
				break
			}
		}
	}
	if !digits {
		return "", s.errf("invalid number")
	}
	if r := s.peek(); r == 'e' || r == 'E' {
		s.next()
		if r := s.peek(); r == '+' || r == '-' {
			s.next()
		}
		expDigits := false
		for {
			r := s.peek()
			if r >= '0' && r <= '9' {
				s.next()
				expDigits = true
			} else {
				break
			}
		}
		if !expDigits {
			return "", s.errf("invalid exponent")
		}
	}
	lit := string(s.data[start:s.pos])
	// canonicalize: leading '+' and '.' forms for the canonical JSON emitter
	if lit[0] == '+' {
		lit = lit[1:]
	}
	if strings.HasPrefix(lit, ".") {
		lit = "0" + lit
	} else if strings.HasPrefix(lit, "-.") {
		lit = "-0" + lit[1:]
	}
	if strings.HasSuffix(lit, ".") {
		lit += "0"
	}
	return lit, nil
}

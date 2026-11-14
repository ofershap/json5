package json5

import (
	"encoding/json"
	"math"
	"strconv"
)

type parser struct {
	s         *scanner
	useNumber bool
}

// parseAny parses one JSON5 value from the scanner.
func (p *parser) parseAny() (any, error) {
	s := p.s
	if err := s.skipSpace(); err != nil {
		return nil, err
	}
	if s.eof() {
		return nil, s.errf("unexpected end of input")
	}
	r := s.peek()
	switch {
	case r == '{':
		return p.parseObject()
	case r == '[':
		return p.parseArray()
	case r == '\'' || r == '"':
		return s.scanString()
	case r == 't':
		return p.parseKeyword("true", true)
	case r == 'f':
		return p.parseKeyword("false", false)
	case r == 'n':
		return p.parseKeyword("null", nil)
	case r == 'I' || r == 'N' || r == '+' || r == '-' || r == '.' || (r >= '0' && r <= '9'):
		lit, err := s.scanNumber()
		if err != nil {
			return nil, err
		}
		return p.numberValue(lit)
	case r == '\\' && s.peekAt(1) == 'u':
		return nil, s.errf("unexpected unicode escape outside string or identifier")
	}
	return nil, s.errf("unexpected character %q", r)
}

func (p *parser) numberValue(lit string) (any, error) {
	switch lit {
	case "Infinity":
		return math.Inf(1), nil
	case "-Infinity":
		return math.Inf(-1), nil
	case "NaN":
		return math.NaN(), nil
	}
	if p.useNumber {
		return json.Number(lit), nil
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			// JS semantics: overflow -> +/-Inf, underflow -> 0; f already holds it
			return f, nil
		}
		return nil, p.s.errf("invalid number %q", lit)
	}
	return f, nil
}

func (p *parser) parseKeyword(kw string, val any) (any, error) {
	for _, c := range kw {
		if p.s.peek() != c {
			return nil, p.s.errf("invalid literal, expected %q", kw)
		}
		p.s.next()
	}
	// reject identifier continuation (e.g. truex)
	if r := p.s.peek(); isIDContinue(r) {
		return nil, p.s.errf("invalid literal, expected %q", kw)
	}
	return val, nil
}

func (p *parser) parseObject() (any, error) {
	s := p.s
	s.next() // {
	obj := map[string]any{}
	if err := s.skipSpace(); err != nil {
		return nil, err
	}
	if s.peek() == '}' {
		s.next()
		return obj, nil
	}
	for {
		if err := s.skipSpace(); err != nil {
			return nil, err
		}
		var key string
		var err error
		switch r := s.peek(); {
		case r == '\'' || r == '"':
			key, err = s.scanString()
		default:
			key, err = s.scanIdentifier()
		}
		if err != nil {
			return nil, err
		}
		if err := s.skipSpace(); err != nil {
			return nil, err
		}
		if s.peek() != ':' {
			return nil, s.errf("expected ':' after object key")
		}
		s.next()
		v, err := p.parseAny()
		if err != nil {
			return nil, err
		}
		obj[key] = v
		if err := s.skipSpace(); err != nil {
			return nil, err
		}
		switch s.peek() {
		case ',':
			s.next()
			if err := s.skipSpace(); err != nil {
				return nil, err
			}
			if s.peek() == '}' { // trailing comma
				s.next()
				return obj, nil
			}
		case '}':
			s.next()
			return obj, nil
		default:
			return nil, s.errf("expected ',' or '}' in object")
		}
	}
}

func (p *parser) parseArray() (any, error) {
	s := p.s
	s.next() // [
	arr := []any{}
	if err := s.skipSpace(); err != nil {
		return nil, err
	}
	if s.peek() == ']' {
		s.next()
		return arr, nil
	}
	for {
		v, err := p.parseAny()
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
		if err := s.skipSpace(); err != nil {
			return nil, err
		}
		switch s.peek() {
		case ',':
			s.next()
			if err := s.skipSpace(); err != nil {
				return nil, err
			}
			if s.peek() == ']' { // trailing comma
				s.next()
				return arr, nil
			}
		case ']':
			s.next()
			return arr, nil
		default:
			return nil, s.errf("expected ',' or ']' in array")
		}
	}
}

package json5

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Marshal returns the JSON encoding of v. Unlike encoding/json,
// NaN and Infinity floats encode as their JSON5 literals, which are
// valid JSON5 but not strict JSON.
func Marshal(v any) ([]byte, error) {
	return marshal(v, Style{})
}

// MarshalIndent is like Marshal with indentation (encoding/json parity).
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return marshal(v, Style{Indent: indent, Prefix: prefix})
}

// Style controls JSON5-styled output produced by MarshalStyled.
type Style struct {
	Indent         string // e.g. "  "; empty for compact
	Prefix         string // line prefix (encoding/json parity)
	SingleQuotes   bool   // prefer ' for strings
	TrailingCommas bool   // emit trailing commas in objects/arrays
	UnquotedKeys   bool   // emit identifier-safe object keys unquoted
}

// MarshalStyled returns the JSON5-styled encoding of v.
func MarshalStyled(v any, s Style) ([]byte, error) { return marshal(v, s) }

func marshal(v any, s Style) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeValue(&buf, reflect.ValueOf(v), s, 0); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s Style) newline(buf *bytes.Buffer, depth int) {
	if s.Indent == "" {
		return
	}
	buf.WriteByte('\n')
	buf.WriteString(s.Prefix)
	for i := 0; i < depth; i++ {
		buf.WriteString(s.Indent)
	}
}

func (s Style) sep(buf *bytes.Buffer) {
	if s.Indent == "" {
		buf.WriteByte(':')
	} else {
		buf.WriteString(": ")
	}
}

func encodeValue(buf *bytes.Buffer, rv reflect.Value, st Style, depth int) error {
	if !rv.IsValid() {
		buf.WriteString("null")
		return nil
	}
	if rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			buf.WriteString("null")
			return nil
		}
		return encodeValue(buf, rv.Elem(), st, depth)
	}
	switch rv.Kind() {
	case reflect.Bool:
		buf.WriteString(strconv.FormatBool(rv.Bool()))
	case reflect.String:
		writeString(buf, rv.String(), st)
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		switch {
		case math.IsNaN(f):
			buf.WriteString("NaN")
		case math.IsInf(f, 1):
			buf.WriteString("Infinity")
		case math.IsInf(f, -1):
			buf.WriteString("-Infinity")
		default:
			buf.WriteString(strconv.FormatFloat(f, 'g', -1, rv.Type().Bits()))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		buf.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		buf.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			buf.WriteString("null")
			return nil
		}
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			// []byte: stdlib encodes as base64 string
			writeString(buf, encodeBase64(rv.Bytes()), st)
			return nil
		}
		n := rv.Len()
		buf.WriteByte('[')
		for i := 0; i < n; i++ {
			if i > 0 {
				buf.WriteByte(',')
			}
			st.newline(buf, depth+1)
			if err := encodeValue(buf, rv.Index(i), st, depth+1); err != nil {
				return err
			}
		}
		if n > 0 && st.TrailingCommas {
			buf.WriteByte(',')
		}
		st.newline(buf, depth)
		buf.WriteByte(']')
	case reflect.Map:
		if rv.IsNil() {
			buf.WriteString("null")
			return nil
		}
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("json5: unsupported map key type %s", rv.Type().Key())
		}
		keys := rv.MapKeys()
		strs := make([]string, len(keys))
		for i, k := range keys {
			strs[i] = k.String()
		}
		sort.Strings(strs)
		buf.WriteByte('{')
		for i, k := range strs {
			if i > 0 {
				buf.WriteByte(',')
			}
			st.newline(buf, depth+1)
			if st.UnquotedKeys && isIdentifier(k) {
				buf.WriteString(k)
			} else {
				writeString(buf, k, st)
			}
			st.sep(buf)
			if err := encodeValue(buf, rv.MapIndex(reflect.ValueOf(k)), st, depth+1); err != nil {
				return err
			}
		}
		if len(strs) > 0 && st.TrailingCommas {
			buf.WriteByte(',')
		}
		st.newline(buf, depth)
		buf.WriteByte('}')
	case reflect.Struct:
		// delegate struct field/tag semantics to encoding/json, then restyle
		b, err := json.Marshal(rv.Interface())
		if err != nil {
			return err
		}
		var gen any
		dec := NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if err := dec.Decode(&gen); err != nil {
			return err
		}
		return encodeValue(buf, reflect.ValueOf(gen), st, depth)
	default:
		return fmt.Errorf("json5: unsupported type %s", rv.Type())
	}
	return nil
}

// isIdentifier reports whether s is a valid JSON5 IdentifierName.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !isIDStart(r) {
				return false
			}
		} else if !isIDContinue(r) {
			return false
		}
	}
	switch s { // reserved words stay quoted for safety
	case "true", "false", "null", "NaN", "Infinity":
		return false
	}
	return true
}

func writeString(buf *bytes.Buffer, s string, st Style) {
	q := byte('"')
	if st.SingleQuotes && !strings.ContainsRune(s, '\'') {
		q = '\''
	}
	buf.WriteByte(q)
	for _, r := range s {
		switch r {
		case rune(q):
			buf.WriteByte('\\')
			buf.WriteRune(r)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\v':
			buf.WriteString(`\v`)
		case 0:
			buf.WriteString(`\0`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\x%02x`, r)
			} else if r == 0x2028 || r == 0x2029 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte(q)
}

var b64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func encodeBase64(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); i += 3 {
		var n uint32
		rem := len(b) - i
		n = uint32(b[i]) << 16
		if rem > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(b[i+2])
		}
		sb.WriteByte(b64chars[n>>18&63])
		sb.WriteByte(b64chars[n>>12&63])
		if rem > 1 {
			sb.WriteByte(b64chars[n>>6&63])
		} else {
			sb.WriteByte('=')
		}
		if rem > 2 {
			sb.WriteByte(b64chars[n&63])
		} else {
			sb.WriteByte('=')
		}
	}
	return sb.String()
}

// Encoder writes JSON5 values to an output stream.
type Encoder struct {
	w  io.Writer
	st Style
}

// NewEncoder returns an encoder writing strict-JSON-compatible output.
func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

// SetStyle switches the encoder to styled JSON5 output.
func (e *Encoder) SetStyle(s Style) { e.st = s }

// Encode writes the encoding of v followed by a newline.
func (e *Encoder) Encode(v any) error {
	b, err := marshal(v, e.st)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = e.w.Write(b)
	return err
}

// quoteRuneASCII is kept for symmetry with scanner; unused helpers avoided.
var _ = utf8.RuneLen
var _ = unicode.IsLetter

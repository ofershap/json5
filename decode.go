// Package json5 implements encoding and decoding of JSON5 (https://json5.org),
// the human-friendly JSON extension, with an encoding/json-compatible API.
package json5

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
)

// Unmarshal parses JSON5-encoded data and stores the result in the value
// pointed to by v, following encoding/json semantics (including json tags).
// Numbers decode as float64 unless the Decoder.UseNumber path is used.
// NaN and Infinity decode as their float64 values.
func Unmarshal(data []byte, v any) error {
	p := &parser{s: newScanner(data)}
	val, err := p.parseAny()
	if err != nil {
		return err
	}
	if err := p.s.skipSpace(); err != nil {
		return err
	}
	if !p.s.eof() {
		return p.s.errf("unexpected data after top-level value")
	}
	return assign(val, v)
}

// Valid reports whether data is valid JSON5.
func Valid(data []byte) bool {
	var v any
	return Unmarshal(data, &v) == nil
}

// assign places a parsed value into v. *interface{} targets (and other
// generic targets) receive the tree directly; typed targets round-trip
// through canonical JSON so encoding/json owns all tag semantics.
func assign(val any, v any) error {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &json.InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	elem := rv.Elem()
	if val == nil {
		switch elem.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
			elem.SetZero()
			return nil
		}
	} else if tv := reflect.ValueOf(val); tv.Type().AssignableTo(elem.Type()) {
		// direct fast path: float64 (incl. NaN/Inf), string, bool,
		// map[string]any and []any targets receive the tree as-is,
		// preserving json.Number under Decoder.UseNumber.
		elem.Set(tv)
		return nil
	}
	if elem.Kind() == reflect.Interface && elem.NumMethod() == 0 {
		elem.Set(reflect.ValueOf(val))
		return nil
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, val); err != nil {
		return err
	}
	if err := json.Unmarshal(buf.Bytes(), v); err != nil {
		return err
	}
	return nil
}

// writeCanonical writes a parsed tree as canonical strict JSON.
// NaN and Infinity have no JSON representation; they error unless the
// target accepted the raw tree (assign fast path).
func writeCanonical(buf *bytes.Buffer, val any) error {
	switch t := val.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		b, _ := json.Marshal(t)
		buf.Write(b)
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return fmt.Errorf("json5: NaN/Infinity cannot decode into typed targets; use a float64 or interface{} destination")
		}
		buf.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
	case json.Number:
		buf.WriteString(t.String())
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	default:
		return fmt.Errorf("json5: internal: unexpected value type %T", val)
	}
	return nil
}

// Decoder reads and decodes JSON5 values from an input stream.
type Decoder struct {
	r         io.Reader
	buf       []byte
	sc        *scanner
	useNumber bool
	err       error
}

// NewDecoder returns a new decoder reading from r.
func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: r} }

// UseNumber causes the Decoder to unmarshal numbers into json.Number
// instead of float64 for interface{} values.
func (d *Decoder) UseNumber() { d.useNumber = true }

func (d *Decoder) fill() error {
	if d.sc == nil {
		data, err := io.ReadAll(d.r)
		if err != nil {
			return err
		}
		d.buf = data
		d.sc = newScanner(data)
	}
	return nil
}

// Decode reads the next JSON5-encoded value from its input and stores it
// in the value pointed to by v. At end of input it returns io.EOF.
func (d *Decoder) Decode(v any) error {
	if d.err != nil {
		return d.err
	}
	if err := d.fill(); err != nil {
		d.err = err
		return err
	}
	if err := d.sc.skipSpace(); err != nil {
		d.err = err
		return err
	}
	if d.sc.eof() {
		return io.EOF
	}
	p := &parser{s: d.sc, useNumber: d.useNumber}
	val, err := p.parseAny()
	if err != nil {
		d.err = err
		return err
	}
	return assign(val, v)
}

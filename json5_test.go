package json5

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strings"
	"testing"
)

func TestUnmarshalStructTags(t *testing.T) {
	type Server struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		TLS  bool   `json:"tls,omitempty"`
	}
	in := `{
		// server config
		host: 'localhost',
		port: 0x1F90,
		tls: true,
	}`
	var s Server
	if err := Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	if s.Host != "localhost" || s.Port != 8080 || !s.TLS {
		t.Fatalf("got %+v", s)
	}
}

func TestNumbers(t *testing.T) {
	cases := map[string]float64{
		"0xdecaf":  912559,
		".5":       0.5,
		"5.":       5,
		"+1":       1,
		"-.25":     -0.25,
		"1e3":      1000,
		"9e999":    math.Inf(1),
		"-9e999":   math.Inf(-1),
		"Infinity": math.Inf(1),
	}
	for in, want := range cases {
		var got float64
		if err := Unmarshal([]byte(in), &got); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %v want %v", in, got, want)
		}
	}
	var nan float64
	if err := Unmarshal([]byte("NaN"), &nan); err != nil || !math.IsNaN(nan) {
		t.Fatalf("NaN: %v %v", nan, err)
	}
}

func TestUseNumber(t *testing.T) {
	d := NewDecoder(strings.NewReader(`{a: 0xdecaf, b: 12345678901234567890123}`))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["a"] != json.Number("912559") {
		t.Fatalf("hex via UseNumber: %#v", m["a"])
	}
	if m["b"] != json.Number("12345678901234567890123") {
		t.Fatalf("big int via UseNumber: %#v", m["b"])
	}
}

func TestDecoderStream(t *testing.T) {
	d := NewDecoder(strings.NewReader("{a:1}\n[1,2]\n'x'\n"))
	var o map[string]any
	var a []any
	var s string
	if err := d.Decode(&o); err != nil || o["a"] != 1.0 {
		t.Fatalf("obj: %v %#v", err, o)
	}
	if err := d.Decode(&a); err != nil || len(a) != 2 {
		t.Fatalf("arr: %v %#v", err, a)
	}
	if err := d.Decode(&s); err != nil || s != "x" {
		t.Fatalf("str: %v %#v", err, s)
	}
	if err := d.Decode(&s); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	in := map[string]any{"a": 1.0, "b": "x", "c": []any{true, nil}}
	b, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Marshal output is not strict JSON: %v (%s)", err, b)
	}
	var back map[string]any
	if err := Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
}

func TestMarshalNaNInf(t *testing.T) {
	b, err := Marshal(map[string]any{"n": math.NaN(), "i": math.Inf(-1)})
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := Unmarshal(b, &back); err != nil {
		t.Fatalf("round trip NaN/Inf: %v (%s)", err, b)
	}
	if !math.IsNaN(back["n"].(float64)) || back["i"] != math.Inf(-1) {
		t.Fatalf("got %#v", back)
	}
}

func TestMarshalStyled(t *testing.T) {
	v := map[string]any{"name": "ofer", "ids": []any{1.0, 2.0}}
	b, err := MarshalStyled(v, Style{Indent: "  ", UnquotedKeys: true, SingleQuotes: true, TrailingCommas: true})
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "name: 'ofer',") {
		t.Fatalf("styled output missing unquoted key/single quote: %s", got)
	}
	if !Valid(b) {
		t.Fatalf("styled output is not valid JSON5: %s", got)
	}
}

func TestStructMarshal(t *testing.T) {
	type Cfg struct {
		Name string `json:"name"`
		N    int    `json:"n,omitempty"`
	}
	b, err := MarshalStyled(Cfg{Name: "x"}, Style{UnquotedKeys: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{name:"x"}` {
		t.Fatalf("got %s", b)
	}
}

func TestErrorPosition(t *testing.T) {
	err := Unmarshal([]byte("{\n  a: 1,\n  b: ,\n}"), new(any))
	se, ok := err.(*SyntaxError)
	if !ok {
		t.Fatalf("expected *SyntaxError, got %T (%v)", err, err)
	}
	if se.Line != 3 {
		t.Fatalf("expected line 3, got %d (%v)", se.Line, se)
	}
}

func TestValid(t *testing.T) {
	if !Valid([]byte("{a: 1,}")) {
		t.Fatal("should be valid")
	}
	if Valid([]byte("{a: }")) {
		t.Fatal("should be invalid")
	}
}

func TestEncoder(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	if err := e.Encode(map[string]any{"a": 1.0}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "{\"a\":1}\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func FuzzUnmarshal(f *testing.F) {
	seeds := []string{
		"{a: 1}", "[1,2,3]", "'x'", "0xdecaf", ".5", "NaN", "-Infinity",
		"{/* c */a: [1,], b: \"s\"}", "true", "null", "{\\u0061: 1}",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		var v any
		// must never panic; errors are fine
		_ = Unmarshal([]byte(in), &v)
	})
}

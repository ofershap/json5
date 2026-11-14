package json5

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The corpus in testdata/json5-tests is vendored from
// https://github.com/json5/json5-tests (MIT, see testdata/json5-tests/LICENSE.md).
// golden.json is generated with the reference implementation (npm json5@2).

type goldenEntry struct {
	OK       any    `json:"ok"` // true, false, or "skip"
	Value    any    `json:"value"`
	Note     string `json:"note"`
	MustFail bool   `json:"mustFail"`
	Input    string `json:"input"`
}

func loadGolden(t *testing.T) map[string]goldenEntry {
	t.Helper()
	data, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	out := map[string]goldenEntry{}
	for k, r := range raw {
		var e goldenEntry
		if err := json.Unmarshal(r, &e); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		out[k] = e
	}
	return out
}

func denormalize(v any) any {
	switch t := v.(type) {
	case string:
		switch t {
		case "__NaN__":
			return math.NaN()
		case "__Inf__":
			return math.Inf(1)
		case "__-Inf__":
			return math.Inf(-1)
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = denormalize(e)
		}
		return t
	case map[string]any:
		for k, e := range t {
			t[k] = denormalize(e)
		}
		return t
	}
	return v
}

func deepEqual(a, b any) bool {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		if math.IsNaN(af) && math.IsNaN(bf) {
			return true
		}
		return af == bf
	}
	aa, aok := a.([]any)
	bb, bok := b.([]any)
	if aok && bok {
		if len(aa) != len(bb) {
			return false
		}
		for i := range aa {
			if !deepEqual(aa[i], bb[i]) {
				return false
			}
		}
		return true
	}
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		if len(am) != len(bm) {
			return false
		}
		for k := range am {
			if !deepEqual(am[k], bm[k]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

func TestCorpusValid(t *testing.T) {
	golden := loadGolden(t)
	root := "testdata/json5-tests"
	var files []string
	for name := range golden {
		if strings.HasSuffix(name, ".json5") || strings.HasSuffix(name, ".json") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	pass := 0
	for _, name := range files {
		e := golden[name]
		okFlag, _ := e.OK.(bool)
		if !okFlag {
			t.Errorf("%s: reference itself rejects (%s) - investigate", name, e.Note)
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got any
		if err := Unmarshal(data, &got); err != nil {
			t.Errorf("%s: parse error: %v", name, err)
			continue
		}
		want := denormalize(e.Value)
		if !deepEqual(got, want) {
			t.Errorf("%s: value mismatch\n got: %#v\nwant: %#v", name, got, want)
			continue
		}
		pass++
	}
	t.Logf("valid corpus: %d/%d pass", pass, len(files))
}

func TestCorpusInvalid(t *testing.T) {
	golden := loadGolden(t)
	root := "testdata/json5-tests"
	pass, total := 0, 0
	for name, e := range golden {
		if !strings.HasSuffix(name, ".errorSpec") || !e.MustFail {
			continue
		}
		total++
		data, err := os.ReadFile(filepath.Join(root, e.Input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got any
		if err := Unmarshal(data, &got); err == nil {
			t.Errorf("%s: accepted invalid input %s (value %#v)", name, e.Input, got)
			continue
		}
		pass++
	}
	t.Logf("invalid corpus: %d/%d rejected", pass, total)
	if total == 0 {
		t.Fatal("no invalid cases found - harness bug")
	}
}

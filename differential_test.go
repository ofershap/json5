package json5

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// testdata/differential.json is generated with the reference implementation
// (npm json5@2): 500 randomized documents (with comment/trailing-comma
// injections) plus 12 hand-written invalid inputs. The Go parser must match
// the reference's accept/reject decision and the parsed value.

type diffCase struct {
	Input string `json:"input"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Note  string `json:"note"`
}

func TestDifferential(t *testing.T) {
	data, err := os.ReadFile("testdata/differential.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases map[string]diffCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range doc.Cases {
		names = append(names, n)
	}
	sort.Strings(names)
	mismatches := 0
	for _, name := range names {
		c := doc.Cases[name]
		var got any
		err := Unmarshal([]byte(c.Input), &got)
		if c.OK {
			if err != nil {
				t.Errorf("%s: reference accepts, we reject: %v", name, err)
				mismatches++
				continue
			}
			if !deepEqual(got, denormalize(c.Value)) {
				t.Errorf("%s: value mismatch on %q", name, c.Input)
				mismatches++
			}
		} else {
			if err == nil {
				t.Errorf("%s: reference rejects, we accept %q -> %#v", name, c.Input, got)
				mismatches++
			}
		}
	}
	t.Logf("differential: %d cases, %d mismatches", len(names), mismatches)
}

package json5

import "testing"

var benchDoc = []byte(`{
	// application config
	name: 'gailwind',
	version: '0.1.0',
	ports: [8080, 0x1F91, 9000,],
	limits: {cpu: .5, memory: 512e6, retries: 3,},
	features: {flags: {darkMode: true, betaUI: false,},},
	note: "multi\nline",
}`)

func BenchmarkUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var v any
		if err := Unmarshal(benchDoc, &v); err != nil {
			b.Fatal(err)
		}
	}
}

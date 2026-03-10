package deduplication

import (
	"testing"
)

var sampleText = `
Rust ownership tutorial explains memory safety.
Rust manages memory without garbage collection.
Ownership rules ensure safety and performance.
`

func BenchmarkSimHashPipeline(b *testing.B) {

	for i := 0; i < b.N; i++ {

		tokens := Tokenize(sampleText)

		shingles := Shingles(tokens, ShingleSize)

		SimHash(shingles)
	}
}

func BenchmarkCleanHTML(b *testing.B) {

	html := `<html><body><h1>Hello</h1><p>Rust tutorial</p></body></html>`

	for i := 0; i < b.N; i++ {
		CleanData(html, SourceHTML)
	}
}

func BenchmarkSimhashQuery(b *testing.B) {

	index := NewSimIndex()

	for i := 0; i < 10000; i++ {
		index.Add(uint64(i))
	}

	hash := uint64(1234)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		index.IsNearDuplicate(hash, 3)
	}
}

func BenchmarkSimhashIndexMemory(b *testing.B) {

	index := NewSimIndex()

	for i := 0; i < b.N; i++ {

		hash := uint64(i)

		index.Add(hash)
	}
}

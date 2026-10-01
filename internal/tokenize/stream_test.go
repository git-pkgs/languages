package tokenize_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/git-pkgs/languages/internal/tokenize"
)

func TestStreamMatchesScanner(t *testing.T) {
	for _, data := range []string{
		"package main\nfunc main() { println(value) }\n",
		"/*! block */ name // comment\n/// docs\n//! directive\n-- comment\n# comment\n#tag",
		"'escaped\\\"' `multiline\nstring` \"quote\\\"end\" after",
		"/*/**/*/ x / - # ", "source 世界\U0001f642 end", "word 12.3e10 and_123",
		"/word -word #word //", "///", "//!", "/* tail */word",
		"\"escaped\\\\\"word 'quote\\'inside'after",
		strings.Repeat("a", 4096) + " known /*" + strings.Repeat("/", 4096),
	} {
		for chunk := 1; chunk <= 17; chunk++ {
			assertStream(t, []byte(data), chunk, 512)
		}
	}
}

func assertStream(t *testing.T, data []byte, chunk, limit int) {
	t.Helper()
	var want, got []string
	tokenize.Scan(data, func(token []byte) {
		if len(token) <= limit {
			want = append(want, string(token))
		}
	})
	stream := tokenize.NewStream(make([]byte, limit))
	visit := func(token []byte) { got = append(got, string(token)) }
	for start := 0; start < len(data); start += chunk {
		part := bytes.Clone(data[start:min(start+chunk, len(data))])
		stream.Write(part, visit)
		clear(part)
		stream.Write(nil, visit)
	}
	stream.Finish(visit)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chunk %d: got %q, want %q", chunk, got, want)
	}
}

func FuzzStream(f *testing.F) {
	f.Add([]byte("/// doc\nfn f(x) { /* comment */ println(\"value\"); }"), uint8(3))
	f.Add([]byte("word 世界\U0001f642 `quoted` "), uint8(1))
	f.Fuzz(func(t *testing.T, data []byte, chunk uint8) { assertStream(t, data, int(chunk)+1, 512) })
}

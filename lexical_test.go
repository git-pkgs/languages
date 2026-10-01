package languages

import (
	"bytes"
	"testing"
)

func TestLexicalChunks(t *testing.T) {
	for _, source := range []string{
		"/* comment\n*/package main\n",
		"`raw string\nuse strict;\n`\npackage main\n",
		"\"\"\"docstring\nuse strict;\n\"\"\"\npackage main\n",
		"\"escaped\\\"inside\"\npackage main\n",
		"# comment \"\"\"\npackage main\n// comment /*\n/*x*/ package main\n",
		"; comment\n-- comment\n//comment\n/\n-\n/*tail*",
	} {
		for chunk := 1; chunk <= 9; chunk++ {
			assertLexicalChunks(t, []byte(source), chunk)
		}
	}
}

func assertLexicalChunks(t *testing.T, data []byte, chunk int) {
	t.Helper()
	var whole, streamed lexicalState
	for _, line := range bytes.Split(data, []byte("\n")) {
		want := whole.codeStart(line)
		streamed.startLine()
		for len(line) > 0 {
			n := min(chunk, len(line))
			part := bytes.Clone(line[:n])
			streamed.write(part)
			clear(part)
			line = line[n:]
		}
		got := streamed.finishLine()
		if got != int64(want) || whole.block != streamed.block || whole.quote != streamed.quote || whole.triple != streamed.triple {
			t.Fatalf("chunk %d: start=%d want=%d; whole=%+v streamed=%+v", chunk, got, want, whole, streamed)
		}
	}
}

func FuzzLexicalChunks(f *testing.F) {
	f.Add([]byte("/* multiline\ncomment */ package main\n\"\"\"text\nmore\"\"\""), uint8(1))
	f.Fuzz(func(t *testing.T, data []byte, chunk uint8) {
		assertLexicalChunks(t, data, int(chunk)+1)
	})
}

package classifier

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/git-pkgs/languages/internal/tokenize"
)

func TestRepeatedTokenScores(t *testing.T) {
	source := []byte("package main\nimport fmt\nfunc main() { println(value) }\n")
	// Distinct tokens exercise cache eviction as well as repeated punctuation.
	for i := 0; i < len(indexData); i += indexWidth {
		start, length := indexUint(i), indexUint(i+fieldWidth)
		if length == 8 {
			source = append(source, tokenData[start:start+length]...)
			source = append(source, ' ')
			if len(source) > 2048 {
				break
			}
		}
	}
	source = append(source, '\n')
	var single, repeated Analysis
	Analyze(source, &single)
	const copies = 8
	Analyze(bytes.Repeat(source, copies), &repeated)
	if repeated.Tokens != copies*single.Tokens {
		t.Fatalf("tokens: %d, want %d", repeated.Tokens, copies*single.Tokens)
	}
	for i, score := range single.Scores {
		want := int64(priors[i]) + copies*(score-int64(priors[i]))
		if repeated.Scores[i] != want {
			t.Fatalf("%s: %d, want %d", Names[i], repeated.Scores[i], want)
		}
	}
}

func TestCollidingTokenScores(t *testing.T) {
	var source []byte
	var found int
	for i := 0; i < len(indexData) && found < 12; i += indexWidth {
		start, length := indexUint(i), indexUint(i+fieldWidth)
		token := []byte(tokenData[start : start+length])
		if len(token) != 8 || tokenHash(token)%tokenCacheSize != 0 {
			continue
		}
		scanner := tokenize.NewScanner(token)
		if !bytes.Equal(scanner.Next(), token) || scanner.Next() != nil {
			continue
		}
		source = append(source, token...)
		source = append(source, ' ')
		found++
	}
	if found != 12 {
		t.Fatalf("only found %d colliding tokens", found)
	}
	source = append(source, "someIdentifierAbsentFromTheModel \n"...)
	assertUncachedScores(t, bytes.Repeat(source, 32))
}

func FuzzCachedScores(f *testing.F) {
	f.Add([]byte("function render(e){return e.map(function(x){return x.value+1})};"))
	f.Add([]byte("type Record struct { ID int; Name string; Enabled bool }\n"))
	f.Fuzz(func(t *testing.T, data []byte) { assertUncachedScores(t, data) })
}

func assertUncachedScores(t *testing.T, data []byte) {
	t.Helper()
	var want Analysis
	reset(&want)
	tokenize.Scan(data, func(token []byte) {
		_, offset, count := lookup(token)
		if count > 0 {
			want.Tokens++
		}
		for range count {
			language := binary.LittleEndian.Uint16([]byte(weightData[offset:]))
			weight := binary.LittleEndian.Uint16([]byte(weightData[offset+postingWidth/2:]))
			want.Scores[language] += int64(weight)
			offset += postingWidth
		}
	})
	var got Analysis
	Analyze(data, &got)
	if got != want {
		t.Fatal("cached scores differ from per-token accumulation")
	}
}

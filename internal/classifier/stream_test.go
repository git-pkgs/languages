package classifier

import (
	"bytes"
	"testing"
)

func TestStreamScores(t *testing.T) {
	data := []byte("function render(e){return e.map(function(x){return x.value+1})};\n/* comment */\n")
	for _, chunk := range []int{1, 2, 3, 7, 32, 1024} {
		var stream Stream
		for _, input := range [][]byte{bytes.Repeat(data, 200), []byte("package main\n"), nil} {
			var got, want Analysis
			Analyze(input, &want)
			stream.Reset(&got)
			for offset := 0; offset < len(input); offset += chunk {
				stream.Write(input[offset:min(offset+chunk, len(input))])
			}
			stream.Finish()
			if got != want {
				t.Fatalf("chunk %d: streamed scores differ", chunk)
			}
		}
	}
}

func TestStreamLargeScores(t *testing.T) {
	data := []byte("function render(e){return e.map(function(x){return x.value+1})};\n")
	var single, repeated Analysis
	Analyze(data, &single)
	var stream Stream
	stream.Reset(&repeated)
	const copies = 200000
	for range copies {
		stream.Write(data)
	}
	stream.Finish()
	if repeated.Tokens != copies*single.Tokens {
		t.Fatal(repeated.Tokens)
	}
	for i, score := range single.Scores {
		if want := int64(priors[i]) + copies*(score-int64(priors[i])); repeated.Scores[i] != want {
			t.Fatalf("%s: got %d, want %d", Names[i], repeated.Scores[i], want)
		}
	}
}

func FuzzStreamScores(f *testing.F) {
	f.Add([]byte("// comment\nfunc main() { println(\"value\") }"), uint8(1))
	f.Fuzz(func(t *testing.T, data []byte, size uint8) {
		var got, want Analysis
		Analyze(data, &want)
		var stream Stream
		stream.Reset(&got)
		chunk := int(size) + 1
		for offset := 0; offset < len(data); offset += chunk {
			stream.Write(data[offset:min(offset+chunk, len(data))])
		}
		stream.Finish()
		if got != want {
			t.Fatal("streamed scores differ")
		}
	})
}

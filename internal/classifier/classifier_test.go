package classifier_test

import (
	"bytes"
	"testing"

	"github.com/git-pkgs/languages/internal/classifier"
)

func TestClassifyAmongCandidates(t *testing.T) {
	for _, test := range []struct{ source, want string }{
		{"package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"hello\") }\n", "Go"},
		{"from pathlib import Path\ndef read(path):\n    return Path(path).read_text()\n", "Python"},
		{"use std::io;\nfn main() { println!(\"Hello\"); }\n", "Rust"},
	} {
		var a classifier.Analysis
		classifier.Analyze([]byte(test.source), &a)
		first, _ := a.Best(func(i int) bool {
			name := classifier.Names[i]
			return name == "Go" || name == "Python" || name == "Rust"
		})
		if first < 0 || classifier.Names[first] != test.want {
			t.Fatalf("%q: got model index %d, want %s", test.source, first, test.want)
		}
	}
}

func TestCandidatesAndReuse(t *testing.T) {
	var a classifier.Analysis
	classifier.Analyze([]byte("package main\nfunc main() {}\n"), &a)
	before := a
	first, second := a.Best(func(i int) bool { return classifier.Names[i] == "Go" })
	if first < 0 || classifier.Names[first] != "Go" || second != -1 || a != before {
		t.Fatal(first, second, "candidate restriction mutated scores")
	}
	classifier.Analyze(nil, &a)
	first, second = a.Best(nil)
	if first != -1 || second != -1 || a.Tokens != 0 {
		t.Fatal("empty input retained evidence")
	}
}

func TestFullInputAndAllocations(t *testing.T) {
	data := []byte("package main\nfunc main() {}\n")
	var a classifier.Analysis
	if got := testing.AllocsPerRun(100, func() {
		classifier.Analyze(data, &a)
		a.Best(nil)
	}); got != 0 {
		t.Fatalf("allocations: %v", got)
	}
	before := a
	data = append(bytes.Repeat([]byte(" "), 128*1024), data...)
	classifier.Analyze(data, &a)
	if a != before {
		t.Fatal("large input lost trailing token evidence")
	}
}

func BenchmarkAnalyze1KB(b *testing.B) {
	data := bytes.Repeat([]byte("package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"hello\") }\n"), 32)[:1024]
	var a classifier.Analysis
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		classifier.Analyze(data, &a)
		a.Best(nil)
	}
}

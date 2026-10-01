package tokenize_test

import (
	"reflect"
	"testing"

	"github.com/git-pkgs/languages/internal/tokenize"
)

func TestScan(t *testing.T) {
	for _, test := range []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"const value = 12;", []string{"const", "value", "=", ";"}},
		{"#include <stdio.h>\nint main() {}", []string{"#", "include", "<", "stdio", ".", "h", ">", "int", "main", "(", ")", "{", "}"}},
		{"// ignored\n-- ignored\n# ignored\n/* ignored */x", []string{"x"}},
		{"x /* unfinished", []string{"x"}},
		{"x = \"escaped \\\" value\" + 'text'", []string{"x", "=", "+"}},
		{"x = `multiline\nvalue`", []string{"x", "="}},
		{"x = \"unfinished\\", []string{"x", "="}},
		{"var café = 0x12;", []string{"var", "caf", "é", "=", ";"}},
		{"⍴ array ← 1 2 3", []string{"⍴", "array", "←"}},
		{"//! Module documentation\n/// Member documentation\n// ordinary comment", []string{"//!", "///"}},
		{"func function_name123() {}", []string{"func", "function_name123", "(", ")", "{", "}"}},
	} {
		var got []string
		tokenize.Scan([]byte(test.input), func(token []byte) { got = append(got, string(token)) })
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q: got %q, want %q", test.input, got, test.want)
		}
	}
}

func TestScanDoesNotMutateOrAllocate(t *testing.T) {
	const source = "// header\nconst value = call(\"text\");\n"
	data := []byte(source)
	count := 0
	if got := testing.AllocsPerRun(100, func() {
		count = 0
		tokenize.Scan(data, func([]byte) { count++ })
	}); got != 0 {
		t.Fatalf("allocations: %v", got)
	}
	if count != 7 || string(data) != source {
		t.Fatalf("tokens %d, data %q", count, data)
	}
}

func FuzzScan(f *testing.F) {
	for _, input := range []string{"", "'\\", "/* unfinished", "#include <stdio.h>", "fn main() {}"} {
		f.Add([]byte(input))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		before := string(data)
		total := 0
		tokenize.Scan(data, func(token []byte) {
			if len(token) == 0 {
				t.Fatal("empty token")
			}
			total += len(token)
		})
		if total > len(data) || string(data) != before {
			t.Fatal("tokens exceed or mutate input")
		}
	})
}

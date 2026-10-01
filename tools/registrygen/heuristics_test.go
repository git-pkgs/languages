package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func TestGenerateHeuristicsSnapshot(t *testing.T) {
	output := filepath.Join(t.TempDir(), "heuristics.go")
	command := exec.Command("go", "run", ".", "-heuristics", "-source", "heuristics.yml", "-revision-file", "revision", "-out", output)
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generator command: %v\n%s", err, data)
	}
	want, err := os.ReadFile("../../heuristics_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("heuristics differ from pinned source")
	}
}

func TestRubyRegexFlags(t *testing.T) {
	for _, tt := range []struct {
		pattern, input string
		want           bool
	}{
		{"^target$", "header\ntarget\nfooter", true},
		{"a.b", "a\nb", false},
		{"(?m)a.b", "a\nb", true},
		{"(?-m)^a.b$", "header\na-b\nfooter", true},
		{"(?im:^a.b$)", "header\nA\nB\nfooter", true},
	} {
		pattern, err := compilePattern([]string{tt.pattern})
		if err != nil {
			t.Fatal(err)
		}
		if got := regexp.MustCompile(pattern).MatchString(tt.input); got != tt.want {
			t.Errorf("%q: %t", tt.pattern, got)
		}
	}
}

func TestCompileHeuristicConditions(t *testing.T) {
	spec := heuristicSpec{And: []heuristicSpec{{Named: "keyword"}, {Negative: stringList{"forbidden"}}}}
	conditions, err := compileConditions(spec, map[string]stringList{"keyword": {"first", "second"}})
	if err != nil || len(conditions) != 2 {
		t.Fatalf("%+v %v", conditions, err)
	}
	if conditions[0].negative || !conditions[1].negative || !regexp.MustCompile(conditions[0].pattern).MatchString("second") {
		t.Fatal(conditions)
	}
	if _, err := compileConditions(heuristicSpec{Named: "missing"}, nil); err == nil {
		t.Fatal("accepted unknown named pattern")
	}
	if _, err := compilePattern([]string{"(?=unsupported)"}); err == nil {
		t.Fatal("accepted unsupported regex")
	}
}

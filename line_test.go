package languages

import (
	"bytes"
	"strings"
	"testing"
)

func TestRuleWindows(t *testing.T) {
	for _, r := range rules {
		if len(r.prefix) >= ruleWindow || len(r.contains) > ruleWindow || len(r.suffix) > ruleWindow {
			t.Fatalf("rule %s exceeds the streaming window", r.id)
		}
	}
}

func TestLineSummary(t *testing.T) {
	lines := []string{
		"", "import from ", "import x from ", "import x from y", "import \t\r",
		"def function(name):", "def method\t", "module Example", "module Example;",
		"const value: number = 1;", "let $value \u2000: number = 1", "var 1bad: type = 1",
		"const name.with.dot: number = 1", "func run(\u2000)\u2000 { body }",
		"func run(a int) {}", "func run() -> Int {}", "func run() : Int {}", "func run()",
		"function value = \u2000name(x)", "function value = |closure|", "require \u2000('json')",
		"require ((call))", "require lower", "#lang racket\u2000", "#lang racket\u2000 other",
		"#lang racket \u2000other", "#lang typed/racket\u2000\t", "#lang scribble/text ",
		"ancestor(X,Y) :- parent(X,Y).", "ancestor(\"X\") :- parent(X,Y).",
		"<?php", "<?php \t", "#include<vector>", "<!doctype html>", "SELECT ID FROM table",
	}
	for _, r := range rules {
		lines = append(lines, r.prefix+r.contains+r.suffix)
		lines = append(lines, r.prefix+strings.Repeat("x", 32780)+r.contains+r.suffix+" \t\r")
	}
	for _, line := range lines {
		for _, chunk := range []int{1, 2, 3, 7, 31, 32, 33, 1024, 32768} {
			assertLineSummary(t, []byte(line), chunk, false)
			assertLineSummary(t, []byte(line), chunk, true)
		}
	}
}

func assertLineSummary(t *testing.T, line []byte, chunk int, terminated bool) {
	t.Helper()
	var summary lineSummary
	for at := 0; at < len(line); at += chunk {
		part := bytes.Clone(line[at:min(at+chunk, len(line))])
		summary.write(part)
		clear(part)
	}
	got := summary.finish(terminated)
	trimmed := bytes.TrimRight(line, " \t\r")
	for i := range rules {
		want := len(trimmed) >= minSignalBytes && matches(trimmed, &rules[i], terminated)
		matched := got[i/wordBits]&(1<<uint(i%wordBits)) != 0
		if want != matched {
			t.Fatalf("rule %s chunk %d complete=%t matched=%t want=%t line=%q", rules[i].id, chunk, terminated, matched, want, line)
		}
	}
}

func FuzzLineSummary(f *testing.F) {
	for _, prefix := range []string{"require ", "func ", "function ", "const ", "#lang ", "import ", "module ", "def ", "ancestor("} {
		f.Add([]byte(prefix), []byte("value : type = func()\u2000"), uint8(1), true)
	}
	f.Fuzz(func(t *testing.T, prefix, body []byte, chunk uint8, terminated bool) {
		line := append(bytes.Clone(prefix), body...)
		assertLineSummary(t, line, int(chunk)+1, terminated)
	})
}

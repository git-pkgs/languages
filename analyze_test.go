package languages_test

import (
	"bytes"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/git-pkgs/languages"
)

func analyze(data string) languages.Analysis {
	var a languages.Analysis
	languages.Analyze([]byte(data), true, &a)
	return a
}

func TestConfidenceRank(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   languages.Confidence
	}{
		{"", languages.None},
		{"import os\n", languages.Low},
		{"use strict;\n", languages.Medium},
		{"#!/bin/bash\n", languages.High},
	} {
		a := analyze(tt.source)
		got := a.Result().Confidence
		if got != tt.want || (got.Rank() >= languages.Medium.Rank()) != (tt.want == languages.Medium || tt.want == languages.High) {
			t.Errorf("%q: confidence %q, rank %d", tt.source, got, got.Rank())
		}
	}
	for rank, confidence := range []languages.Confidence{languages.None, languages.Low, languages.Medium, languages.High} {
		if got := confidence.Rank(); got != rank {
			t.Errorf("%q: rank %d, want %d", confidence, got, rank)
		}
	}
	for _, confidence := range []languages.Confidence{"", "invalid"} {
		if confidence.Rank() != languages.None.Rank() {
			t.Errorf("%q should rank as none", confidence)
		}
	}
}

func TestControlBytes(t *testing.T) {
	for b := byte(0); b < 0x20; b++ {
		a := analyze("#!/bin/bash\nprintf '" + string([]byte{b}) + "[31mred'\n")
		allowed := strings.IndexByte("\t\n\f\r\x1b\x02\x03\b\v\x0f\x1a\x1f", b) >= 0
		if a.Binary == allowed || allowed && a.Result().Language != languages.Bash || !allowed && a.Count != 0 {
			t.Errorf("control byte %#x: binary %t, result %+v", b, a.Binary, a.Result())
		}
	}
	for _, data := range []string{"\x00", "\x01\x02\x03", "abc\x00def"} {
		if a := analyze(data); !a.Binary || a.Count != 0 {
			t.Errorf("binary control sequence accepted: %q", data)
		}
	}
}

func TestContent(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"ruby", "#!/usr/bin/env ruby\n# frozen_string_literal: true\nrequire 'json'\n", languages.Ruby},
		{"python", "from pathlib import Path\ndef read(path):\n    return Path(path).read_text()\n", languages.Python},
		{"go", "package main\nimport (\"fmt\")\nfunc main() { fmt.Println(1) }", languages.Go},
		{"rust", "use std::io;\nfn main() {}", languages.Rust},
		{"java", "package demo;\nimport java.util.List;\npublic class Demo {\npublic static void main(String[] args) {}\n}\n", languages.Java},
		{"cpp", "#include <vector>\nusing namespace std;", languages.CPP},
		{"objc", "#import <Foundation/Foundation.h>\n@interface Thing : NSObject", languages.ObjectiveC},
		{"objc-interface", "@interface Thing : NSObject", languages.ObjectiveC},
		{"matlab", "function result = add(a,b)\nresult=a+b;\nend", languages.MATLAB},
		{"bash", "#!/usr/bin/env -S bash -eu\necho hello", languages.Bash},
		{"perl", "use strict;\nuse warnings;\nsub main { print 1; }", languages.Perl},
		{"prolog", " :- module(family, [ancestor/2]).\nancestor(X,Y) :- parent(X,Y).", languages.Prolog},
		{"raku", "use v6;\nunit module Example;", languages.Raku},
		{"lisp", "(in-package :demo)\n(defun sum (a b) (+ a b))", languages.CommonLisp},
		{"racket", "#lang racket\n(define (sum a b) (+ a b))", languages.Racket},
		{"clojure", "(ns sample.core)\n(defn sum [a b] (+ a b))", languages.Clojure},
		{"php", "<?php\nnamespace Demo;", languages.PHP},
		{"lua", "local json = require('json')\nlocal function sum(a,b) return a+b end", languages.Lua},
		{"csharp", "using System;\nnamespace Demo;", languages.CSharp},
		{"html", "<!doctype html>\n<html lang=\"en\">", languages.HTML},
		{"xml", "<?xml version=\"1.0\"?>\n<project/>", languages.XML},
		{"sql", "CREATE TABLE users (id integer);\nSELECT id FROM users;", languages.SQL},
		{"sql-lowercase", "select id from users;", languages.SQL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := analyze(tt.source)
			if got := a.Result(); got.Language != tt.want {
				t.Fatalf("got %+v, want %s", got, tt.want)
			}
		})
	}
}

func TestCaseFoldDispatch(t *testing.T) {
	for _, source := range []string{"select id from users;", "SELECT id FROM users;", "<!DoCtYpE html>", "@implementation Thing"} {
		a := analyze(source)
		if a.Count == 0 {
			t.Fatal(source)
		}
	}
}

func TestAmbiguity(t *testing.T) {
	for _, tt := range []struct {
		source     string
		candidates []languages.Language
	}{
		{"#include <stdio.h>\nint main(void) { return 0; }", []languages.Language{languages.C, languages.CPP, languages.ObjectiveC}},
		{"function sum(a,b) { return a+b; }", []languages.Language{languages.JavaScript, languages.TypeScript, languages.JSX, languages.TSX}},
		{"export interface Props { title: string; }", []languages.Language{languages.TypeScript, languages.TSX}},
		{"const title: string = 'Example';", []languages.Language{languages.TypeScript, languages.TSX}},
		{"return <Panel/>;", []languages.Language{languages.JSX, languages.TSX}},
		{"if [ -f x ]; then\n echo x\nfi", []languages.Language{languages.Shell, languages.Bash, languages.Zsh}},
		{"(define (sum a b) (+ a b))", []languages.Language{languages.Scheme, languages.Racket}},
		{"{% extends 'base.html' %}", []languages.Language{languages.Jinja, languages.Twig}},
	} {
		a := analyze(tt.source)
		r := a.Result()
		if r.Language != languages.Unknown || r.Candidates.Len() != len(tt.candidates) {
			t.Fatalf("%q: %+v", tt.source, r)
		}
		for _, l := range tt.candidates {
			if !r.Candidates.Has(l) {
				t.Errorf("missing %s for %q", l, tt.source)
			}
		}
	}
}

func TestPathAndCombination(t *testing.T) {
	if c := languages.AnalyzePath("src/demo.pl"); !c.Candidates.Has(languages.Perl) || !c.Candidates.Has(languages.Prolog) {
		t.Fatal(c)
	}
	for _, source := range []string{"use strict;\nuse warnings;", ":- use_module(library(lists))."} {
		a := analyze(source)
		before := a
		got := languages.Combine(&a, languages.AnalyzePath("demo.pl"))
		if got.Language != a.Result().Language || a != before {
			t.Fatal(got)
		}
	}
	a := analyze("function sum(a,b) { return a+b; }")
	if r := languages.Combine(&a, languages.AnalyzePath("src/sum.ts")); r.Language != languages.TypeScript {
		t.Fatal(r)
	}
	if r := languages.Combine(&a, languages.AnalyzePath("sum.py")); r.Conflict || r.Language != languages.Python || r.Confidence != languages.Low {
		t.Fatal(r)
	}
	if got := languages.AnalyzePath(`C:\src\Gemfile`).Result().Language; got != languages.Ruby {
		t.Fatal(got)
	}
	if c := languages.AnalyzePath("src/python/README"); !c.Candidates.Empty() {
		t.Fatal(c)
	}
}

func TestReuseAndBytes(t *testing.T) {
	data := bytes.Repeat([]byte("# frozen_string_literal: true\n"), 10000)
	var a languages.Analysis
	languages.Analyze(data, true, &a)
	if a.Bytes != int64(len(data)) || a.Prefix || a.Count != 1 {
		t.Fatal(a.Bytes, a.Prefix, a.Count)
	}
	before := a
	clear(data)
	if a != before {
		t.Fatal("retained input")
	}
	languages.Analyze(nil, true, &a)
	if a.Count != 0 || a.Bytes != 0 || a.Result().Confidence != languages.None {
		t.Fatal(a)
	}
	languages.Analyze([]byte("\x00package main\nfunc main(){}"), true, &a)
	if !a.Binary || !a.Result().Candidates.Empty() {
		t.Fatal(a)
	}
	languages.Analyze([]byte("#!/usr/bin/python"), false, &a)
	if a.Count != 0 {
		t.Fatal("partial interpreter token accepted")
	}
	languages.Analyze([]byte("\xef\xbb\xbf#!/usr/bin/python3\n"), false, &a)
	if a.Result().Language != languages.Python || a.Signals[0].Offset != 3 {
		t.Fatal(a)
	}
}

func TestAbstentionAndShielding(t *testing.T) {
	for _, s := range []string{"", "hello world", "print(1)", "parent(alice,bob).", "#!/usr/bin/notpython\n", "#!/usr/bin/env -u ruby python\n", "/*\nfunc fake() {\n*/", "\"\"\"\nfunc fake() {\n\"\"\"", "const text = `\nfunc fake() {\n`;", "// use strict;", "<htmlish>"} {
		a := analyze(s)
		if a.Result().Candidates.Has(languages.Go) || a.Result().Candidates.Has(languages.Perl) || a.Result().Candidates.Has(languages.HTML) {
			t.Errorf("false signal: %q %+v", s, a.Result())
		}
	}
}

func TestEvidenceAndConfidence(t *testing.T) {
	a := analyze("#!/usr/bin/env ruby\n# frozen_string_literal: true\nrequire 'json'\n")
	if a.Count != 3 || a.Result().Confidence != languages.High {
		t.Fatal(a.Result(), a.Count)
	}
	for _, m := range a.Signals[:a.Count] {
		e := m.Evidence()
		if e.ID == "" || e.Description == "" || !e.Languages.Has(languages.Ruby) {
			t.Fatal(e)
		}
	}
	b := analyze("require 'json'\nrequire 'set'\n")
	if b.Count != 1 || b.Result().Confidence != languages.Medium {
		t.Fatal(b.Result(), b.Count)
	}
}

func TestContradictoryShebang(t *testing.T) {
	a := analyze("#!/usr/bin/ruby\ndef main():\n    return 1\n")
	r := a.Result()
	if !r.Conflict || r.Language != languages.Unknown || !r.Candidates.Has(languages.Ruby) || !r.Candidates.Has(languages.Python) {
		t.Fatal(r)
	}
	if combined := languages.Combine(&a, languages.AnalyzePath("main.rb")); !combined.Conflict || combined.Language != languages.Unknown {
		t.Fatal(combined)
	}
}

func TestTypedBindingAndProlog(t *testing.T) {
	a := analyze("const title = {name: 'Example'};")
	if !a.Result().Candidates.Has(languages.JavaScript) {
		t.Fatal(a.Result())
	}
	for _, source := range []string{"#!/usr/bin/env swipl\n", ":- dynamic parent/2.\n"} {
		a = analyze(source)
		if a.Result().Language != languages.Prolog {
			t.Fatal(a.Result())
		}
	}
}

func TestEvidenceOffsets(t *testing.T) {
	a := analyze("  use strict;\n")
	if a.Count != 1 || a.Signals[0].Offset != 2 {
		t.Fatal(a.Count, a.Signals[0])
	}
	a = analyze("\u00a0use strict;\n")
	if a.Count != 0 {
		t.Fatal("non-ASCII whitespace treated as Perl indentation")
	}
}

func TestExtensionlessAndMisleadingNames(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   languages.Language
	}{
		{"use strict;\nuse warnings;", languages.Perl},
		{":- module(family, [ancestor/2]).\nancestor(X,Y) :- parent(X,Y).", languages.Prolog},
		{"package main\nfunc main() {}", languages.Go},
	} {
		a := analyze(tt.source)
		snapshot := a
		for _, name := range []string{"", "blob", "wrong.py", "source.pl"} {
			_ = languages.Combine(&a, languages.AnalyzePath(name))
			if a != snapshot || a.Result().Language != tt.want {
				t.Fatal(name, a.Result())
			}
		}
	}
}

func TestParallelAndAllocations(t *testing.T) {
	data := []byte("package main\nimport (\"fmt\")\nfunc main() {}")
	var a languages.Analysis
	if n := testing.AllocsPerRun(100, func() { languages.Analyze(data, false, &a); _ = a.Result() }); n != 0 && !raceEnabled {
		t.Fatal(n)
	}
	want := a
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			var got languages.Analysis
			for range 100 {
				languages.Analyze(data, false, &got)
				if !reflect.DeepEqual(got, want) {
					t.Error("nondeterministic")
				}
			}
		})
	}
	wg.Wait()
}

func FuzzAnalyze(f *testing.F) {
	for _, s := range []string{"", "#!/usr/bin/env -S ruby\n", "/*", "\x00\xff", "\"\"\"", ":- module(x, [])."} {
		f.Add([]byte(s), false)
	}
	f.Fuzz(func(t *testing.T, data []byte, complete bool) {
		var a, b languages.Analysis
		languages.Analyze(data, complete, &a)
		languages.Analyze(data, complete, &b)
		if a != b || a.Count > languages.MaxSignals || a.Bytes != int64(len(data)) {
			t.Fatal("invariant")
		}
		_ = a.Result()
		for _, m := range a.Signals[:a.Count] {
			if m.Offset >= uint64(a.Bytes) {
				t.Fatal("offset")
			}
		}
	})
}

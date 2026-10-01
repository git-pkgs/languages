package languages_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestAnalyzeFullFile(t *testing.T) {
	header := strings.Repeat("# header\n", 16000)
	for _, test := range []struct {
		name, suffix string
		language     languages.Language
		binary       bool
	}{
		{"late declaration", "use strict;\nuse warnings;\n", languages.Perl, false},
		{"footer modeline", "# vim: ft=python\n", languages.Python, false},
		{"late binary byte", "\x00", languages.Unknown, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(header + test.suffix)
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			if a.Bytes != int64(len(data)) || a.Prefix || a.Binary != test.binary || a.Result().Language != test.language {
				t.Fatalf("bytes=%d prefix=%t binary=%t result=%+v", a.Bytes, a.Prefix, a.Binary, a.Result())
			}
			if !test.binary && (a.Count == 0 || a.Signals[0].Offset != uint64(len(header))) {
				t.Fatalf("unexpected offsets: %+v", a.Signals[:a.Count])
			}
			languages.Analyze(data[:1024], false, &a)
			if a.Bytes != 1024 || !a.Prefix || a.Binary || a.Count != 0 {
				t.Fatalf("explicit prefix: bytes=%d prefix=%t binary=%t signals=%+v", a.Bytes, a.Prefix, a.Binary, a.Signals[:a.Count])
			}
		})
	}
}

func TestFullFileClassifier(t *testing.T) {
	source := []byte("module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n")
	var small, large languages.Analysis
	languages.Analyze(source, true, &small)
	data := append(bytes.Repeat([]byte(" \n"), 128*1024), source...)
	languages.Analyze(data, true, &large)
	if small.Result().Language != languages.GoModule || !small.Result().Statistical || small.Result() != large.Result() {
		t.Fatalf("small=%+v large=%+v", small.Result(), large.Result())
	}
}

func TestFullFileLexicalBoundaries(t *testing.T) {
	for _, quotes := range []struct{ open, close string }{
		{"/*", "*/"}, {"`", "`"}, {`"""`, `"""`}, {"'''", "'''"},
	} {
		for split := 0; split <= len(quotes.open); split++ {
			header := strings.Repeat(" ", 32*1024-split) + quotes.open + "\nuse strict;\n"
			header += strings.Repeat("x", 128*1024) + "\nuse warnings;\n" + quotes.close + "\n"
			var a languages.Analysis
			languages.Analyze([]byte(header+"package main\nimport (\"fmt\")\nfunc main() { fmt.Println(1) }\n"), true, &a)
			if a.Prefix || a.Binary || a.Result().Language != languages.Go {
				t.Fatalf("%q split %d: %+v", quotes.open, split, a.Result())
			}
			for _, match := range a.Signals[:a.Count] {
				if match.Evidence().Languages.Has(languages.Perl) {
					t.Fatalf("%q split %d: comment or string contributed evidence: %+v", quotes.open, split, match.Evidence())
				}
			}
		}
	}
}

func TestFullFileLongLines(t *testing.T) {
	for _, test := range []struct {
		source, want, reject string
	}{
		{"def " + strings.Repeat("x", 128*1024) + "(name):\n", "python.def", "ruby.def"},
		{"import " + strings.Repeat("pkg.", 32768) + "other;\n", "java.import", "python.import"},
		{"const " + strings.Repeat("x", 128*1024) + ": number = 1;\n", "ts.binding", ""},
		{"# " + strings.Repeat("x", 128*1024) + "frozen_string_literal: true\n", "ruby.frozen", ""},
		{"SELECT " + strings.Repeat("id,", 65536) + "id FROM records;\n", "sql.select", ""},
	} {
		var a languages.Analysis
		languages.Analyze([]byte(test.source), true, &a)
		found := false
		for _, match := range a.Signals[:a.Count] {
			id := match.Evidence().ID
			found = found || id == test.want
			if id == test.reject {
				t.Fatalf("unexpected %s evidence", id)
			}
		}
		if !found || a.Prefix || a.Bytes != int64(len(test.source)) {
			t.Fatalf("missing full-line evidence for %s: %+v", test.want, a.Signals[:a.Count])
		}
	}
}

func TestFullFileLongModelines(t *testing.T) {
	for _, body := range []string{
		"mode:" + strings.Repeat("\u2000", 65536) + "python",
		"coding: " + strings.Repeat("x", 128*1024) + "; mode: python",
		"python" + strings.Repeat(" ", 128*1024),
	} {
		for _, header := range []string{"", strings.Repeat("# header\n", 16000)} {
			source := header + "# -*- " + body + " -*-\n"
			var a languages.Analysis
			languages.Analyze([]byte(source), true, &a)
			if a.Prefix || a.Binary || a.Result().Language != languages.Python || a.Count != 1 || a.Signals[0].Offset != uint64(len(header)) {
				t.Fatalf("full modeline: result=%+v evidence=%+v", a.Result(), a.Signals[:a.Count])
			}
		}
	}
}

func TestFullFileLongVimModelines(t *testing.T) {
	for _, line := range []string{
		"vim" + strings.Repeat("7", 65536) + ": set ft=python:",
		"vim: option=" + strings.Repeat("x", 128*1024) + " ft=python",
		"vim: ft=python" + strings.Repeat("\u2000", 65536),
	} {
		header := strings.Repeat("# header\n", 16000)
		var a languages.Analysis
		languages.Analyze([]byte(header+"# "+line+"\n"), true, &a)
		if a.Prefix || a.Binary || a.Result().Language != languages.Python || a.Count != 1 || a.Signals[0].Offset != uint64(len(header)) {
			t.Fatalf("full Vim modeline: result=%+v evidence=%+v", a.Result(), a.Signals[:a.Count])
		}
	}
}

func TestFullFileLongShebangs(t *testing.T) {
	for _, source := range []string{
		"#!/usr/bin/" + strings.Repeat("directory/", 16384) + "python3\npass\n",
		"#!/usr/bin/python" + strings.Repeat("3.0", 65536) + "\npass\n",
		"#!/usr/bin/env NAME=" + strings.Repeat("x", 128*1024) + " python3\npass\n",
		"#!/bin/sh\nexec " + strings.Repeat("\u2000", 65536) + "python3 \"$0\"\n",
		"#!/bin/sh\nexec python3 " + strings.Repeat("x", 128*1024) + " \"$0\"\n",
	} {
		var a languages.Analysis
		languages.Analyze([]byte(source), true, &a)
		if a.Prefix || a.Binary || a.Result().Language != languages.Python || a.Result().Confidence != languages.High {
			t.Fatalf("full shebang: result=%+v evidence=%+v", a.Result(), a.Signals[:a.Count])
		}
		languages.Analyze([]byte(source[:1024]), false, &a)
		if a.Bytes != 1024 || !a.Prefix {
			t.Fatalf("explicit shebang prefix: bytes=%d prefix=%t", a.Bytes, a.Prefix)
		}
	}
}

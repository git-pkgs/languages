package languages_test

import (
	"bytes"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestDetect(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
		conflict     bool
	}{
		{"app.js", "const count = 1;\n", languages.JavaScript, false},
		{"app.ts", "const count = 1;\n", languages.TypeScript, false},
		{"", "use strict;\nuse warnings;\n", languages.Perl, false},
		{"Gemfile", "source 'https://rubygems.org'\n", languages.Ruby, false},
		{"demo.pl", ":- use_module(library(lists)).\n", languages.Prolog, false},
		{"wrong.py", "#!/usr/bin/ruby\n", languages.Unknown, true},
		{"photo.py", "\x00", languages.Unknown, true},
		{"app.go", "", languages.Go, false},
		{"", "", languages.Unknown, false},
		{"", "#!/usr/bin/ruby", languages.Unknown, false},
	} {
		t.Run(tt.name+"/"+tt.source, func(t *testing.T) {
			got := languages.Detect(tt.name, []byte(tt.source))
			if got.Language != tt.want || got.Conflict != tt.conflict {
				t.Fatalf("got %+v, want %s, conflict %t", got, tt.want, tt.conflict)
			}
		})
	}
}

func TestDetectMatchesReusableAnalysis(t *testing.T) {
	inputs := []string{
		"", "package main\nfunc main() {}\n", "use strict;\nuse warnings;\n",
		"#!/usr/bin/ruby\nputs 'hello'\n", ":- use_module(library(lists)).\n",
		"{\"openapi\":\"3.1.0\"}\n", "<?xml version=\"1.0\"?>\n<plist/>\n",
		"/* comment */\n#include <vector>\n", "\x00package main\n",
		"\xff\xfep\x00a\x00s\x00s\x00\n\x00",
		"\x00\x00\xfe\xff\x00\x00\x00p\x00\x00\x00a\x00\x00\x00s\x00\x00\x00s",
	}
	for _, input := range inputs {
		data := []byte(input)
		var analysis languages.Analysis
		languages.Analyze(data, false, &analysis)
		for _, name := range []string{"", "source", "main.go", "main.py", "main.c", "main.h", "app.ts", "source.pl", "config.json", "config.plist", "Gemfile", ".releaserc", "file.unknown"} {
			if got, want := languages.Detect(name, data), analysis.Detect(name); got != want {
				t.Fatalf("%s %q: got %+v, want %+v", name, input, got, want)
			}
		}
	}
}

func TestDetectCompleteAndReuse(t *testing.T) {
	var a languages.Analysis
	languages.Analyze([]byte("#!/usr/bin/ruby"), true, &a)
	if got := a.Detect(""); got.Language != languages.Ruby {
		t.Fatal(got)
	}
	languages.Analyze([]byte("const count = 1;\n"), false, &a)
	before := a
	for _, tt := range []struct {
		name string
		want languages.Language
	}{{"app.js", languages.JavaScript}, {"app.ts", languages.TypeScript}, {"", languages.Unknown}} {
		if got := a.Detect(tt.name); got.Language != tt.want || a != before {
			t.Fatal(tt.name, got)
		}
	}
}

func TestDetectPreservesAmbiguityAndConflict(t *testing.T) {
	var a languages.Analysis
	languages.Analyze([]byte("#!/usr/bin/python3\n# SPDX-License-Identifier: MIT\nprint(1)\n"), true, &a)
	before := a
	intrinsic := a.Result()
	combined := a.Detect("script.rb")
	if intrinsic.Language != languages.Python || intrinsic.Confidence != languages.High {
		t.Fatal(intrinsic)
	}
	if combined.Language != languages.Unknown || !combined.Conflict || combined.Confidence != languages.Low || combined.Candidates != languages.NewSet(languages.Python, languages.Ruby) {
		t.Fatal(combined)
	}
	if a != before || a.Result() != intrinsic {
		t.Fatal("filename changed intrinsic evidence")
	}
	want := languages.NewSet(languages.Perl, languages.Raku, languages.Prolog)
	if got := languages.Detect("source.pl", nil); got.Language != languages.Unknown || got.Conflict || got.Candidates != want || got.Confidence != languages.Low {
		t.Fatal(got)
	}
}

func TestDetectEvidenceStrength(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
		confidence   languages.Confidence
		conflict     bool
	}{
		{"module.py", "import org.example.Widget;\n", languages.Python, languages.Low, false},
		{"module.rs", "use Example::Thing;\n", languages.Rust, languages.Medium, false},
		{"module.pm", "use Example::Thing;\n", languages.Perl, languages.Low, false},
		{"script.sh", "set -e\n", languages.Shell, languages.Low, false},
		{"script.fish", "set -g greeting hello\n", languages.Fish, languages.Medium, false},
		{"tmux.conf", "set -g status on\n", languages.Shell, languages.Medium, false},
		{"module.py", "use strict;\n", languages.Unknown, languages.Low, true},
	} {
		got := languages.Detect(tt.name, []byte(tt.source))
		if got.Language != tt.want || got.Confidence != tt.confidence || got.Conflict != tt.conflict {
			t.Fatalf("%s: %+v", tt.name, got)
		}
	}
	for _, tt := range []struct {
		source string
		want   languages.Set
	}{
		{"use Example::Thing;\n", languages.NewSet(languages.Rust, languages.Perl, languages.Raku)},
		{"set -e\n", languages.NewSet(languages.Shell, languages.Bash, languages.Zsh, languages.Fish)},
	} {
		if got := languages.Detect("", []byte(tt.source)); got.Candidates != tt.want || got.Language != languages.Unknown {
			t.Fatal(got)
		}
	}
}

func TestDetectBoundsAndAllocations(t *testing.T) {
	const prefixBytes = 1024
	data := append(bytes.Repeat([]byte("\n"), prefixBytes), []byte("#!/usr/bin/ruby\nuse strict;\nuse warnings;\n")...)
	if got := languages.Detect("", data[:prefixBytes]); !got.Candidates.Empty() {
		t.Fatal(got)
	}
	if got := languages.Detect("", data); got.Language != languages.Perl {
		t.Fatal(got)
	}
	data = append(bytes.Repeat([]byte("\n"), 64*1024), []byte("use strict;\nuse warnings;\n")...)
	if got := languages.Detect("", data); got.Language != languages.Perl {
		t.Fatal(got)
	}
	data = []byte("const count = 1;\n")
	if n := testing.AllocsPerRun(100, func() { _ = languages.Detect("app.js", data) }); n != 0 && !raceEnabled {
		t.Fatal(n)
	}
}

func TestClassifierPreservesPathConflict(t *testing.T) {
	const imports = "import os\nimport sys\nprint(os.path.join(sys.argv[0], \"example\"))\n"
	for _, test := range []struct {
		prefix string
		want   languages.Language
	}{
		{"use strict;\nuse warnings;\n", languages.Perl},
		{"# frozen_string_literal: true\n", languages.Ruby},
	} {
		source := []byte(test.prefix + imports)
		var a languages.Analysis
		languages.Analyze(source, false, &a)
		before := a
		for _, got := range []languages.Result{
			languages.Detect("example.py", source),
			a.Detect("example.py"),
			languages.Combine(&a, languages.AnalyzePath("example.py")),
		} {
			if !got.Conflict || got.Language != languages.Unknown || got.Confidence != languages.Low || got.Statistical || got.Candidates != languages.NewSet(test.want, languages.Python) {
				t.Errorf("%q: %+v", test.prefix, got)
			}
		}
		if a != before {
			t.Fatal("combining path changed cached analysis")
		}
	}
}

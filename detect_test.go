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
	if combined.Language != languages.Unknown || !combined.Conflict || combined.Confidence != languages.Low || combined.Candidates != 1<<languages.Python|1<<languages.Ruby {
		t.Fatal(combined)
	}
	if a != before || a.Result() != intrinsic {
		t.Fatal("filename changed intrinsic evidence")
	}
	want := languages.Set(1<<languages.Perl | 1<<languages.Raku | 1<<languages.Prolog)
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
		{"module.pm", "use Example::Thing;\n", languages.Unknown, languages.Medium, false},
		{"script.sh", "set -e\n", languages.Unknown, languages.Medium, false},
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
		{"use Example::Thing;\n", 1<<languages.Rust | 1<<languages.Perl | 1<<languages.Raku},
		{"set -e\n", 1<<languages.Shell | 1<<languages.Bash | 1<<languages.Zsh | 1<<languages.Fish},
	} {
		if got := languages.Detect("", []byte(tt.source)); got.Candidates != tt.want || got.Language != languages.Unknown {
			t.Fatal(got)
		}
	}
}

func TestDetectBoundsAndAllocations(t *testing.T) {
	data := append(bytes.Repeat([]byte("\n"), languages.DefaultBytes), []byte("#!/usr/bin/ruby\nuse strict;\nuse warnings;\n")...)
	if got := languages.Detect("", data[:languages.DefaultBytes]); got.Candidates != 0 {
		t.Fatal(got)
	}
	if got := languages.Detect("", data); got.Language != languages.Perl {
		t.Fatal(got)
	}
	data = append(bytes.Repeat([]byte("\n"), languages.MaxBytes), []byte("use strict;\nuse warnings;\n")...)
	if got := languages.Detect("", data); got.Candidates != 0 {
		t.Fatal(got)
	}
	data = []byte("const count = 1;\n")
	if n := testing.AllocsPerRun(100, func() { _ = languages.Detect("app.js", data) }); n != 0 {
		t.Fatal(n)
	}
}

package languages_test

import (
	"testing"

	"github.com/git-pkgs/languages"
)

func TestInterpreterFamilies(t *testing.T) {
	for _, executable := range []string{"/usr/bin/perl6", "/usr/bin/env perl6", "/usr/bin/env -S perl6 -w", "/usr/bin/raku"} {
		if got := languages.Detect("script.pl", []byte("#!"+executable+"\nuse v6;\n")); got.Language != languages.Raku || got.Conflict {
			t.Fatal(executable, got)
		}
	}
	for _, executable := range []string{"perl", "perl5", "perl5.40"} {
		if got := languages.Detect("script.pl", []byte("#!/usr/bin/env "+executable+"\nuse strict;\n")); got.Language != languages.Perl || got.Conflict {
			t.Fatal(executable, got)
		}
	}
	for _, name := range []string{"app.js", "app.ts", "app.jsx", "app.tsx"} {
		got := languages.Detect(name, []byte("#!/usr/bin/env node\nconst count = 1;\n"))
		want := map[string]languages.Language{"app.js": languages.JavaScript, "app.ts": languages.TypeScript, "app.jsx": languages.JSX, "app.tsx": languages.TSX}[name]
		if got.Language != want || got.Conflict || got.Confidence != languages.High {
			t.Fatal(name, got)
		}
	}
	for _, name := range []string{"", "run", "bin/js"} {
		if got := languages.Detect(name, []byte("#!/usr/bin/env node\nconsole.log('hello');\n")); got.Language != languages.JavaScript || got.Conflict {
			t.Errorf("%s: %+v", name, got)
		}
	}
	var a languages.Analysis
	languages.Analyze([]byte("#!/usr/bin/env node\nconst value: number = 1;\n"), true, &a)
	if got := a.Detect("app.ts"); got.Language != languages.TypeScript || got.Conflict {
		t.Fatal(got)
	}
	if got := a.Detect("app.rb"); !got.Conflict || got.Language != languages.Unknown {
		t.Fatal(got)
	}
	languages.Analyze([]byte("#!/usr/bin/env perl6"), false, &a)
	if !a.Result().Candidates.Empty() {
		t.Fatal(a.Result())
	}
	languages.Analyze([]byte("#!/usr/bin/env perl6"), true, &a)
	if a.Result().Language != languages.Raku {
		t.Fatal(a.Result())
	}
}

func TestShellDialectWithGenericFilename(t *testing.T) {
	for _, name := range []string{"job.slurm", "gradlew", "settime.cgi", "script.sh", ".profile"} {
		for interpreter, want := range map[string]languages.Language{"bash": languages.Bash, "zsh": languages.Zsh} {
			data := []byte("#!/usr/bin/env " + interpreter + "\nset -eu\necho \"${HOME}\"\n")
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			before := a
			if got := a.Detect(name); got.Language != want || got.Conflict || got.Confidence != languages.High {
				t.Errorf("%s with %s: %+v", name, interpreter, got)
			}
			if a != before {
				t.Fatal("filename changed cached analysis")
			}
			if got := a.Detect("script.py"); !got.Conflict || got.Language != languages.Unknown {
				t.Errorf("lost interpreter conflict: %+v", got)
			}
		}
	}
	if got := languages.Detect("job.slurm", nil); got.Language != languages.Shell {
		t.Fatal(got)
	}
}

func TestLispDeclarationsWithSharedForms(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"greet.hy", "#!/usr/bin/env hy\n(defn greet [name] (print (+ \"Hello \" name)))\n", languages.Hy},
		{"greet.lfe", ";; -*- mode: lfe -*-\n(defmodule greet (export (hello 0)))\n(defun hello () (io:format \"Hello~n\"))\n", languages.LFE},
	} {
		for _, name := range []string{tt.name, ""} {
			if got := languages.Detect(name, []byte(tt.source)); got.Language != tt.want || got.Conflict {
				t.Errorf("%s: %+v", name, got)
			}
		}
		if got := languages.Detect("greet.py", []byte(tt.source)); !got.Conflict || got.Language != languages.Unknown {
			t.Errorf("lost declaration conflict: %+v", got)
		}
	}
}

func TestFilenameCollisions(t *testing.T) {
	for _, name := range []string{"translation.ts", "tile.tsx", "schema.sch", "element.rs"} {
		if got := languages.Detect(name, []byte("<?xml version=\"1.0\"?>\n<root/>\n")); got.Language != languages.XML || got.Conflict {
			t.Fatal(name, got)
		}
	}
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"header.hh", "#include <string>\n", languages.CPP},
		{"library.sls", "(define (square x) (* x x))\n", languages.Scheme},
		{"schema.sch", "(define (square x) (* x x))\n", languages.Scheme},
		{"build.cake", "using System;\n", languages.CSharp},
		{"build.boot", "(ns project.build)\n", languages.Clojure},
		{"autoload.al", "use strict;\n", languages.Perl},
		{"schema.mysql", "CREATE TABLE users (id integer);\n", languages.SQL},
		{"buzzer.sch", "EESchema Schematic File Version 2\nLIBS:device\n", languages.KiCadSchematic},
		{"top.sls", "base:\n  '*':\n    - packages\n", languages.Salt},
		{"build.cake", "fs = require 'fs'\n", languages.CoffeeScript},
	} {
		if got := languages.Detect(tt.name, []byte(tt.source)); got.Language != tt.want || got.Conflict {
			t.Fatal(tt.name, got)
		}

	}
}

func TestDocumentFilename(t *testing.T) {
	if got := languages.Detect("README.mysql", []byte("Instructions for connecting to the database.\n")); got.Language != languages.Text {
		t.Fatal(got)
	}
}

func TestPodWithPerlExamples(t *testing.T) {
	for _, source := range []string{
		"=pod\n\n=head1 EXAMPLE\n\n    use strict;\n    use warnings;\n\n=cut\n",
		"use strict;\nuse warnings;\npackage Example;\nsub value { return 1; }\n",
	} {
		var a languages.Analysis
		languages.Analyze([]byte(source), true, &a)
		before := a
		if got := a.Detect("Example.pod"); got.Language != languages.Pod || got.Conflict {
			t.Fatal(got)
		}
		if a != before {
			t.Fatal("filename changed cached analysis")
		}
		if got := a.Detect("Example.py"); !got.Conflict {
			t.Fatal("lost Perl conflict", got)
		}
	}
	if got := languages.Detect("Example.pod", []byte("#!/usr/bin/env python3\nprint('hello')\n")); !got.Conflict {
		t.Fatal("lost interpreter conflict", got)
	}
}

func TestSharedSyntaxConstraints(t *testing.T) {
	for _, source := range []string{

		"#lang en\nA document written in English.\n",
		"package Example::Client;\n",
		"import _NodePath",
	} {
		if got := languages.Detect("", []byte(source)); got.Language != languages.Unknown {
			t.Fatal(source, got)
		}
	}
	for _, source := range []string{
		"package example;\nimport example.io.Console;\n",
		"using System;\nusing System.IO;\n",
	} {
		if got := languages.Detect("", []byte(source)); got.Confidence == languages.High {
			t.Fatal(source, got)
		}
	}
	for _, tt := range []struct {
		source string
		want   languages.Language
	}{
		{"func value() -> Int {\n return 1\n}\n", languages.Swift},
		{"func lessThanTen(number: Int) -> Bool {\n return number < 10\n}\n", languages.Swift},
		{"function main = |args| {\n println(1)\n}\n", languages.Golo},
		{"<?hh // strict\nfinal class :ui:nav {}\n", languages.Hack},
		{"function result = add(a,b)\nresult = a+b;\nend\n", languages.MATLAB},
		{"#lang typed/racket\n", languages.Racket},
		{"module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n", languages.GoModule},
	} {
		if got := languages.Detect("", []byte(tt.source)); got.Language != tt.want {
			t.Fatal(tt.source, got)
		}
	}
}

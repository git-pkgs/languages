package languages_test

import (
	"testing"

	"github.com/git-pkgs/languages"
)

func TestSharedSyntaxUsesFilename(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         languages.Language
	}{
		{"ranks.swift", "let mask: Self = (1 << bit) - 1\n", languages.Swift},
		{"merge.kt", "var previousFlow: Job? = null\n", languages.Kotlin},
		{"flags.ts", "export enum Flags {\n None = 0,\n Global = 1 << 0,\n Unicode = 1 << 1,\n Any = Global | Unicode,\n}\n", languages.TypeScript},
		{"values.pyx", "#cython: language_level=3\ncimport cython\ncdef int width = 16\ndef read(value):\n    return value\n", languages.Cython},
		{"values.pyx", "def square(value):\n    return value * value\n", languages.Cython},
		{"settings.plist", "<?xml version=\"1.0\"?>\n<plist version=\"1.0\"><dict/></plist>\n", languages.XMLPropertyList},
		{"icon.svg", "<?xml version=\"1.0\"?>\n<svg xmlns=\"http://www.w3.org/2000/svg\"/>\n", languages.SVG},
		{"page.xsp-config", "<?xml version=\"1.0\"?>\n<faces-config/>\n", languages.XPages},
		{"build.xml", "<?xml version=\"1.0\"?>\n<project name=\"example\"/>\n", languages.AntBuildSystem},
		{"init.el", "(defun greet (name)\n  (interactive \"sName: \")\n  (message \"Hello %s\" name))\n", languages.EmacsLisp},
	} {
		var a languages.Analysis
		languages.Analyze([]byte(test.source), true, &a)
		if got := a.Detect(test.name); got.Language != test.want || got.Conflict {
			t.Fatalf("%s: %+v", test.name, got)
		}
	}
	if got := languages.Detect("values.rb", []byte("def square(value):\n    return value * value\n")); !got.Conflict || got.Language != languages.Unknown {
		t.Fatal("unrelated filename lost its conflict", got)
	}
}

func TestGoSwiftFunctionSyntax(t *testing.T) {
	const shared = "func greet() {\n    print(\"Hello\")\n}\n"
	for _, test := range []struct {
		name, source string
		want         languages.Set
	}{
		{"greet.swift", shared, languages.NewSet(languages.Swift)},
		{"greet.go", shared, languages.NewSet(languages.Go)},
		{"", shared, languages.NewSet(languages.Go, languages.Swift)},
		{"", "func main() { println(1) }\n", languages.NewSet(languages.Go, languages.Swift)},
		{"", "package main\n" + shared, languages.NewSet(languages.Go)},
		{"", "func greet(name string) { println(name) }\n", languages.NewSet(languages.Go)},
		{"", "func value() int { return 1 }\n", languages.NewSet(languages.Go)},
		{"", "func greet(name: String) { print(name) }\n", languages.NewSet(languages.Swift)},
		{"", "func value() -> Int { return 1 }\n", languages.NewSet(languages.Swift)},
	} {
		got := languages.Detect(test.name, []byte(test.source))
		if got.Candidates != test.want || got.Language != test.want.Only() || got.Conflict {
			t.Errorf("%q %q: %+v", test.name, test.source, got)
		}
	}
}

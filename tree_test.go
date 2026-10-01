package languages_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestTreeRollups(t *testing.T) {
	var tree languages.Tree
	var analysis languages.Analysis
	for _, file := range []struct {
		name, source string
		size         int64
	}{
		{"web/app.ts", "const count = 1;\n", 2000},
		{"api/main.go", "package main\nfunc main() {}\n", 5000},
		{"api/helper.go", "package main\n", 1000},
		{"bindings/native/library.c", "#include <stdio.h>\n", 1000},
		{"main.go", "package main\n", 1000},
	} {
		languages.Analyze([]byte(file.source), false, &analysis)
		if err := tree.Add(file.name, file.size, &analysis); err != nil {
			t.Fatal(err)
		}
	}
	analysis = languages.Analysis{}
	root := tree.Root()
	want := []languages.LanguageTotals{
		{Language: "Go", FileTotals: languages.FileTotals{Files: 3, Bytes: 7000}},
		{Language: "TypeScript", FileTotals: languages.FileTotals{Files: 1, Bytes: 2000}},
		{Language: "C", FileTotals: languages.FileTotals{Files: 1, Bytes: 1000}},
	}
	if root.Path != "." || root.Summary.Files != 5 || root.Summary.Bytes != 10000 || root.Summary.Incomplete != 5 || !reflect.DeepEqual(root.Summary.Languages, want) {
		t.Fatalf("root: %+v", root)
	}
	if len(root.Children) != 3 || root.Children[0].Path != "api" || root.Children[1].Path != "bindings" || root.Children[2].Path != "web" {
		t.Fatal(root.Children)
	}
	native, ok := tree.Subtree("bindings/native")
	if !ok || native.Path != "bindings/native" || native.Summary.Files != 1 || native.Summary.Bytes != 1000 {
		t.Fatal(native, ok)
	}
	api, ok := tree.Subtree("api")
	if !ok || api.Summary.Files != 2 || api.Summary.Bytes != 6000 {
		t.Fatal(api, ok)
	}
	api.Summary.Languages[0].Bytes = 0
	root.Children[0].Summary.Languages[0].Language = "changed"
	again, _ := tree.Subtree("api")
	if again.Summary.Languages[0].Language != "Go" || again.Summary.Languages[0].Bytes != 6000 {
		t.Fatal("snapshot shares mutable state", again)
	}
	for _, name := range []string{"missing", "api/main.go", "../api", ""} {
		if _, ok := tree.Subtree(name); ok {
			t.Fatalf("unexpected subtree %q", name)
		}
	}
}

func TestTreeUnselectedFiles(t *testing.T) {
	var tree languages.Tree
	for _, file := range []struct{ name, source string }{
		{"unknown", ""},
		{"ambiguous.pl", ""},
		{"wrong.py", "#!/usr/bin/ruby\n"},
		{"binary.go", "\x00"},
	} {
		var analysis languages.Analysis
		languages.Analyze([]byte(file.source), true, &analysis)
		if err := tree.Add(file.name, 100, &analysis); err != nil {
			t.Fatal(err)
		}
	}
	summary := tree.Root().Summary
	want := languages.FileTotals{Files: 1, Bytes: 100}
	if summary.Files != 4 || summary.Bytes != 400 || summary.Incomplete != 0 || len(summary.Languages) != 0 || summary.Unknown != want || summary.Ambiguous != want || summary.Conflicts != want || summary.Binary != want {
		t.Fatal(summary)
	}
}

func TestTreeRejectsInvalidEntries(t *testing.T) {
	var tree languages.Tree
	var empty languages.Analysis
	languages.Analyze(nil, true, &empty)
	if err := tree.Add("api/main.go", 0, &empty); err != nil {
		t.Fatal(err)
	}
	before := tree.Root()
	for _, name := range []string{"", ".", "/main.go", "api/../main.go", "api//main.go", "api/", "api/main.go", "api", "api/main.go/child"} {
		if err := tree.Add(name, 0, &empty); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if err := tree.Add("negative", -1, &empty); err == nil {
		t.Fatal("accepted negative size")
	}
	if err := tree.Add("nil", 0, nil); err == nil {
		t.Fatal("accepted nil analysis")
	}
	var content languages.Analysis
	languages.Analyze([]byte("package main\n"), true, &content)
	if err := tree.Add("small.go", 1, &content); err == nil {
		t.Fatal("accepted size smaller than content")
	}
	if !reflect.DeepEqual(tree.Root(), before) {
		t.Fatal("invalid entry changed tree")
	}
}

func TestTreeJSONAndOrdering(t *testing.T) {
	var tree languages.Tree
	var empty languages.Analysis
	languages.Analyze(nil, true, &empty)
	for _, name := range []string{"b.py", "a.go"} {
		if err := tree.Add(name, 0, &empty); err != nil {
			t.Fatal(err)
		}
	}
	encoded, err := json.Marshal(tree.Root())
	if err != nil {
		t.Fatal(err)
	}
	var decoded languages.Directory
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Summary.Languages[0].Language != "Go" || decoded.Summary.Languages[1].Language != "Python" {
		t.Fatal(string(encoded))
	}
	var blank languages.Tree
	if root := blank.Root(); root.Path != "." || root.Summary.Files != 0 || root.Summary.Languages == nil {
		t.Fatal(root)
	}
}

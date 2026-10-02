// wasmcheck exercises the module API under native Go and WebAssembly.
package main

import (
	"context"
	"strings"
	"testing/fstest"

	"github.com/git-pkgs/languages"
)

func main() {
	const javascript = "const count = 1;\n"
	for _, sample := range []struct {
		name, content string
		want          languages.Language
		conflict      bool
	}{
		{"app.ts", javascript, languages.TypeScript, false},
		{"app.js", javascript, languages.JavaScript, false},
		{"page.html", "<!DOCTYPE html>\n<html><p>{{.Title}}</p></html>", languages.GoTemplate, false},
		{"main.swift", "func value() -> Int { return 1 }\n", languages.Swift, false},
		{"main.zig", "", languages.Zig, false},
		{"", "module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n", languages.GoModule, false},
		{"", "#!/usr/bin/env julia\n", languages.Julia, false},
		{"run", "#!/usr/bin/env node\nconsole.log('hello');\n", languages.JavaScript, false},
		{"automation.js", "#!/usr/bin/osascript -l JavaScript\nfunction run() {}\n", languages.JavaScript, false},
		{"package.mo", "within Example;\npackage Widgets\nend Widgets;\n", languages.Modelica, false},
		{"example.pod", "=pod\n\n=head1 EXAMPLE\n\n    use strict;\n    use warnings;\n", languages.Pod, false},
		{"", "#!/usr/bin/python3\n", languages.Python, false},
		{"Gemfile", "source 'https://rubygems.org'\n", languages.Ruby, false},
		{"", javascript, languages.Unknown, false},
		{"", "#!/usr/bin/ruby", languages.Unknown, false},
		{"wrong.py", "#!/usr/bin/ruby\n", languages.Unknown, true},
		{"binary.go", "\x00package main\n", languages.Unknown, true},
		{"document.go", "%PDF-1.7\npackage main\n", languages.Unknown, true},
		{"script.py", "\xff\xfep\x00a\x00s\x00s\x00\n\x00", languages.Python, false},
		{"script.py", "\x00\x00\xfe\xff\x00\x00\x00p\x00\x00\x00a\x00\x00\x00s\x00\x00\x00s", languages.Python, false},
	} {
		result := languages.Detect(sample.name, []byte(sample.content))
		if result.Language != sample.want || result.Conflict != sample.conflict {
			panic("unexpected detection for " + sample.name + ": " + result.Language.String())
		}
	}
	var content languages.Analysis
	languages.Analyze([]byte("#!/usr/bin/ruby"), true, &content)
	if content.Detect("").Language != languages.Ruby || content.Prefix {
		panic("complete input not detected")
	}
	const largeHeaderBytes = 64 * 1024
	data := []byte(strings.Repeat("\n", largeHeaderBytes) + "use strict;\nuse warnings;\n")
	if languages.Detect("", data).Language != languages.Perl {
		panic("large input lost trailing evidence")
	}
	const headerLines, modelineSpaces = 16000, 65536
	modeline := strings.Repeat("# header\n", headerLines) + "# -*- mode:" + strings.Repeat("\u2000", modelineSpaces) + "python -*-\n"
	languages.Analyze([]byte(modeline), true, &content)
	if content.Prefix || content.Result().Language != languages.Python || content.Bytes != int64(len(modeline)) {
		panic("large footer modeline not detected")
	}
	const api = `{"openapi":"3.1.0","info":{"title":"Example"}}`
	if err := languages.AnalyzeReader(context.Background(), strings.NewReader(api), languages.ReadOptions{Filename: "api.json"}, &content); err != nil {
		panic(err)
	}
	if content.Detect("api.json").Language != languages.OASv3Json {
		panic("reader lost filename heuristic")
	}
	vim := strings.Repeat("# header\n", headerLines) + "# vim: ft=python" + strings.Repeat("\u2000", modelineSpaces) + "\n"
	if err := languages.AnalyzeReader(context.Background(), strings.NewReader(vim), languages.ReadOptions{}, &content); err != nil {
		panic(err)
	}
	if content.Prefix || content.Result().Language != languages.Python || content.Bytes != int64(len(vim)) {
		panic("reader lost large Vim modeline")
	}
	checkTree()
	println("module detection checks passed")
}

func checkTree() {
	source := []byte("package main\n")
	var analysis languages.Analysis
	languages.Analyze(source, true, &analysis)
	var tree languages.Tree
	if err := tree.Add("api/main.go", int64(len(source)), &analysis); err != nil {
		panic(err)
	}
	api, ok := tree.Subtree("api")
	if !ok || api.Summary.Files != 1 || api.Summary.Languages[0].Language != "Go" {
		panic("unexpected subtree totals")
	}
	files := fstest.MapFS{"api/main.go": {Data: source}}
	scanned, err := languages.Scan(context.Background(), files, languages.ScanOptions{})
	if err != nil {
		panic(err)
	}
	if scanned.Root().Summary.Bytes != api.Summary.Bytes || scanned.Root().Summary.Files != 1 {
		panic("unexpected scan totals")
	}
}

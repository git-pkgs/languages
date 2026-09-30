// wasmcheck exercises the module API under native Go and WebAssembly.
package main

import (
	"strings"

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
		{"", "#!/usr/bin/python3\n", languages.Python, false},
		{"Gemfile", "source 'https://rubygems.org'\n", languages.Ruby, false},
		{"", javascript, languages.Unknown, false},
		{"", "#!/usr/bin/ruby", languages.Unknown, false},
		{"wrong.py", "#!/usr/bin/ruby\n", languages.Unknown, true},
		{"binary.go", "\x00package main\n", languages.Unknown, true},
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
	data := []byte(strings.Repeat("\n", languages.MaxBytes) + "use strict;\nuse warnings;\n")
	if languages.Detect("", data).Candidates != 0 {
		panic("byte bound exceeded")
	}
	println("module detection checks passed")
}

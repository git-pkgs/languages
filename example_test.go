package languages_test

import (
	"fmt"
	"github.com/git-pkgs/languages"
)

func ExampleDetect() {
	result := languages.Detect("app.ts", []byte("const count = 1;\n"))
	fmt.Println(result.Language)
	// Output: TypeScript
}

func ExampleAnalyze() {
	var content languages.Analysis
	languages.Analyze([]byte("use strict;\nuse warnings;\n"), false, &content)
	r := content.Result()
	fmt.Println(r.Language, r.Confidence)
	fmt.Println(languages.AnalyzePath("source.pl").Candidates.Has(languages.Prolog))
	fmt.Println(languages.Combine(&content, languages.AnalyzePath("source.pl")).Language)
	// Output:
	// Perl high
	// true
	// Perl
}

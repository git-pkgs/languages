package languages_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/git-pkgs/languages"
)

func BenchmarkReaderFilename(b *testing.B) {
	for _, sample := range []struct{ name, source string }{
		{"main.go", "package main\nfunc main() {}\n"},
		{"app.ts", "export const count: number = 1;\n"},
		{"api.json", `{"openapi":"3.1.0","info":{"title":"Example"}}`},
	} {
		for _, mode := range []string{"generic", "named"} {
			b.Run(sample.name+"/"+mode, func(b *testing.B) {
				data := bytes.Repeat([]byte(" "), 1024)
				copy(data, sample.source)
				options := languages.ReadOptions{}
				if mode == "named" {
					options.Filename = sample.name
				}
				reader := bytes.NewReader(data)
				var analysis languages.Analysis
				if err := languages.AnalyzeReader(context.Background(), reader, options, &analysis); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(data)))
				for b.Loop() {
					reader.Reset(data)
					if err := languages.AnalyzeReader(context.Background(), reader, options, &analysis); err != nil {
						b.Fatal(err)
					}
					_ = analysis.Detect(sample.name)
				}
			})
		}
	}
}

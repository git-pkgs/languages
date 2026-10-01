package languages_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/git-pkgs/languages"
)

func BenchmarkDirectoryScan(b *testing.B) {
	const files = 128
	for _, size := range []int{1024, benchmarkBytes} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			root := b.TempDir()
			sources := []struct{ name, content string }{
				{"main.go", "package main\nfunc main() {}\n"},
				{"app.ts", "export const count: number = 1;\n"},
				{"main.py", "def greet(name):\n    print(name)\n"},
				{"run.rb", "#!/usr/bin/ruby\nputs 'hello'\n"},
			}
			for i := range files {
				source := sources[i%len(sources)]
				dir := filepath.Join(root, fmt.Sprintf("service-%03d", i/len(sources)))
				if err := os.MkdirAll(dir, 0700); err != nil {
					b.Fatal(err)
				}
				data := bytes.Repeat([]byte(source.content), size/len(source.content)+1)[:size]
				if err := os.WriteFile(filepath.Join(dir, source.name), data, 0600); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.SetBytes(int64(files * size))
			for b.Loop() {
				tree, err := languages.Scan(context.Background(), os.DirFS(root), languages.ScanOptions{})
				if err != nil {
					b.Fatal(err)
				}
				if got := tree.Root().Summary; got.Files != files || got.Bytes != int64(files*size) {
					b.Fatal(got)
				}
			}
			b.ReportMetric(files, "files/op")
		})
	}
}

func BenchmarkTreeAggregation(b *testing.B) {
	var analysis languages.Analysis
	const source = "package main\nfunc main() {}\n"
	languages.Analyze([]byte(source), true, &analysis)
	for _, count := range []int{100, 1000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			names := make([]string, count)
			for i := range names {
				names[i] = fmt.Sprintf("services/%03d/src/file-%04d.go", i/10, i)
			}
			b.ReportAllocs()
			for b.Loop() {
				var tree languages.Tree
				for _, name := range names {
					if err := tree.Add(name, int64(len(source)), &analysis); err != nil {
						b.Fatal(err)
					}
				}
				if got := tree.Root().Summary.Files; got != int64(count) {
					b.Fatal(got)
				}
			}
			b.ReportMetric(float64(count), "files/op")
		})
	}
}

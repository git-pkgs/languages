package languages_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/git-pkgs/languages"
)

func BenchmarkLargeFiles(b *testing.B) {
	for _, fixture := range []struct{ name, filename, source string }{
		{"generated", "generated.go", "type Record struct { ID int; Name string; Enabled bool }\n"},
		{"minified", "bundle.js", "function render(e){return e.map(function(x){return x.value+1})};"},
		{"comments", "source", "// Copyright Example. Permission to use this software is granted.\n"},
		{"declared", "script", "#!/usr/bin/env python3\nprint('example')\n"},
	} {
		for _, size := range []int{64 << 10, 1 << 20, 16 << 20} {
			b.Run(fixture.name+"/"+strconv.Itoa(size), func(b *testing.B) {
				data := bytes.Repeat([]byte(fixture.source), size/len(fixture.source)+1)[:size]
				b.ReportAllocs()
				for b.Loop() {
					_ = languages.Detect(fixture.filename, data)
				}
			})
		}
	}
}

func BenchmarkLargeFileReader(b *testing.B) {
	for _, fixture := range []struct {
		name, source string
		encoded      bool
	}{
		{"generated", "type Record struct { ID int; Name string; Enabled bool }\n", false},
		{"minified", "function render(e){return e.map(function(x){return x.value+1})};", false},
		{"long_comment", " Copyright Example. Permission to use this software is granted.", false},
		{"unicode", "# 日本語 \U0001f642\nprint('example')\n", false},
		{"utf16", "# 日本語 \U0001f642\nprint('example')\n", true},
	} {
		for _, size := range []int{1024, 64 << 10, 1 << 20, 16 << 20} {
			b.Run(fixture.name+"/"+strconv.Itoa(size), func(b *testing.B) {
				data := bytes.Repeat([]byte(fixture.source), size/len(fixture.source)+1)[:size]
				if fixture.name == "long_comment" {
					copy(data, "//")
				}
				if fixture.encoded {
					data = encodeSource(string(data), 2, binary.LittleEndian)
				}
				reader := bytes.NewReader(data)
				var analysis languages.Analysis
				if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{}, &analysis); err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for b.Loop() {
					reader.Reset(data)
					if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{}, &analysis); err != nil {
						b.Fatal(err)
					}
					if analysis.Bytes != int64(len(data)) || analysis.Prefix || analysis.Binary {
						b.Fatal(analysis.Bytes, analysis.Prefix, analysis.Binary)
					}
				}
			})
		}
	}
}

func BenchmarkLargeFileScan(b *testing.B) {
	const source = "type Record struct { ID int; Name string; Enabled bool }\n"
	for _, size := range []int{64 << 10, 1 << 20, 16 << 20} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			root := b.TempDir()
			data := bytes.Repeat([]byte(source), size/len(source)+1)[:size]
			if err := os.WriteFile(filepath.Join(root, "generated.go"), data, 0600); err != nil {
				b.Fatal(err)
			}
			if _, err := languages.Scan(context.Background(), os.DirFS(root), languages.ScanOptions{}); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				tree, err := languages.Scan(context.Background(), os.DirFS(root), languages.ScanOptions{})
				if err != nil {
					b.Fatal(err)
				}
				if got := tree.Root().Summary; got.Files != 1 || got.Bytes != int64(size) {
					b.Fatal(got)
				}
			}
		})
	}
}

package languages_test

import (
	"errors"
	"github.com/git-pkgs/languages"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkCorpusPrefixes(b *testing.B) {
	root := os.Getenv("LANGUAGES_BENCH_CORPUS")
	if root == "" {
		b.Skip("set LANGUAGES_BENCH_CORPUS to a sampled corpus directory")
	}
	const maxSamples = 1024
	const prefixBytes = 1024
	var inputs [][]byte
	var names []string
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() || entry.Name() == "provenance.jsonl" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		label, _, found := strings.Cut(filepath.ToSlash(rel), "/")
		if !found || languages.Parse(label) == languages.Unknown {
			return nil
		}
		if len(inputs) == maxSamples {
			return fs.SkipAll
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		buf := make([]byte, prefixBytes)
		n, readErr := io.ReadFull(f, buf)
		closeErr := f.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		inputs = append(inputs, buf[:n])
		names = append(names, entry.Name())
		total += int64(n)
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	if len(inputs) == 0 {
		b.Fatal("no samples")
	}
	for _, mode := range []string{"content", "combined"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(total / int64(len(inputs)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				index := i % len(inputs)
				name := ""
				if mode == "combined" {
					name = names[index]
				}
				_ = languages.Detect(name, inputs[index])
			}
		})
	}
}

package languages_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/git-pkgs/languages"
)

type chunkReader struct {
	*bytes.Reader
	chunk int
}

func (r chunkReader) Read(p []byte) (int, error) {
	return r.Reader.Read(p[:min(len(p), r.chunk)])
}

func TestAnalyzeReader(t *testing.T) {
	for _, data := range [][]byte{
		nil, []byte("#!/usr/bin/env -S ruby\nputs 1\n"),
		[]byte("/* header */\npackage main\nfunc main() {}\n"),
		[]byte(strings.Repeat("# header\n", 16000) + "# vim: ft=python\n"),
		[]byte("def " + strings.Repeat("x", 128*1024) + "(name):\n"),
		[]byte("# header\n" + strings.Repeat("x", 65536) + "\x00"),
		[]byte("GIF89a" + strings.Repeat("a", 65536)),
		[]byte("<html>\x01" + strings.Repeat(" ", 65536) + "\xff\n"),
		[]byte("\x01" + strings.Repeat(" ", 65536) + "\xff\n"),
		[]byte("module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n"),
	} {
		for _, chunk := range []int{1, 3, 511, 1024, 32768} {
			for _, prefix := range []bool{false, true} {
				assertReader(t, data, chunk, prefix)
			}
		}
	}
}

func assertReader(t *testing.T, data []byte, chunk int, prefix bool) {
	t.Helper()
	var want, got languages.Analysis
	languages.Analyze(data, !prefix, &want)
	reader := chunkReader{bytes.NewReader(data), chunk}
	if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{Prefix: prefix}, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("chunk=%d prefix=%t size=%d: got bytes=%d binary=%t prefix=%t result=%+v evidence=%+v; want bytes=%d binary=%t prefix=%t result=%+v evidence=%+v", chunk, prefix, len(data), got.Bytes, got.Binary, got.Prefix, got.Result(), got.Signals[:got.Count], want.Bytes, want.Binary, want.Prefix, want.Result(), want.Signals[:want.Count])
	}
}

func TestAnalyzeReaderEncoding(t *testing.T) {
	for _, width := range []int{2, 4} {
		for _, order := range []binary.AppendByteOrder{binary.LittleEndian, binary.BigEndian} {
			for _, source := range []string{
				"#!/usr/bin/python3\n# 日本語 \U0001f642\n",
				strings.Repeat("# 日本語 \U0001f642\n", 6000) + "def greet(name):\n    print(name)\n",
				"# 日本語 \U0001f642\n# -*- ruby -*-\n",
			} {
				data := encodeSource(source, width, order)
				for _, chunk := range []int{1, 3, 511, 1024, 32768} {
					assertReader(t, data, chunk, false)
					for remove := 1; remove < width; remove++ {
						assertReader(t, data[:len(data)-remove], chunk, true)
						assertReader(t, data[:len(data)-remove], chunk, false)
					}
				}
			}
		}
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestAnalyzeReaderLimits(t *testing.T) {
	data := []byte("#!/usr/bin/ruby\n" + strings.Repeat("# header\n", 9000) + "# vim: ft=python\n")
	for _, limit := range []int64{1, 1024, 65537, int64(len(data)), int64(len(data) + 1)} {
		reader := bytes.NewReader(data)
		var got, want languages.Analysis
		if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{Bytes: limit}, &got); err != nil {
			t.Fatal(err)
		}
		n := min(limit, int64(len(data)))
		languages.Analyze(data[:n], n < limit, &want)
		if got != want || reader.Len() != len(data)-int(n) {
			t.Fatalf("limit=%d bytes=%d prefix=%t unread=%d", limit, got.Bytes, got.Prefix, reader.Len())
		}
	}
	for _, prefix := range []bool{false, true} {
		reader := readerFunc(func(p []byte) (int, error) { return copy(p, data), io.EOF })
		var got languages.Analysis
		if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{Bytes: 1024, Prefix: prefix}, &got); err != nil || got.Prefix != prefix || got.Bytes != 1024 {
			t.Fatalf("data with EOF: bytes=%d prefix=%t err=%v", got.Bytes, got.Prefix, err)
		}
	}
}

func TestAnalyzeReaderErrors(t *testing.T) {
	failure := errors.New("read failed")
	for _, test := range []struct {
		reader  io.Reader
		options languages.ReadOptions
		want    error
	}{
		{readerFunc(func(p []byte) (int, error) { return copy(p, "#!/usr/bin/ruby\n"), failure }), languages.ReadOptions{}, failure},
		{readerFunc(func([]byte) (int, error) { return 0, nil }), languages.ReadOptions{}, io.ErrNoProgress},
	} {
		got := languages.Analysis{Bytes: 123, Binary: true}
		err := languages.AnalyzeReader(context.Background(), test.reader, test.options, &got)
		if !errors.Is(err, test.want) || got != (languages.Analysis{}) {
			t.Fatalf("error=%v bytes=%d binary=%t", err, got.Bytes, got.Binary)
		}
	}
	var got languages.Analysis
	if err := languages.AnalyzeReader(context.Background(), strings.NewReader(""), languages.ReadOptions{Bytes: -1}, &got); err == nil {
		t.Fatal("accepted negative byte budget")
	}
}

func TestAnalyzeReaderCancellation(t *testing.T) {
	for _, before := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if before {
			cancel()
		}
		calls := 0
		reader := readerFunc(func(p []byte) (int, error) {
			calls++
			cancel()
			return copy(p, "#!/usr/bin/ruby\n"), io.EOF
		})
		var got languages.Analysis
		err := languages.AnalyzeReader(ctx, reader, languages.ReadOptions{}, &got)
		cancel()
		if !errors.Is(err, context.Canceled) || got != (languages.Analysis{}) || before && calls != 0 || !before && calls != 1 {
			t.Fatalf("before=%t calls=%d err=%v", before, calls, err)
		}
	}
}

func FuzzAnalyzeReader(f *testing.F) {
	for _, data := range [][]byte{
		[]byte("#!/usr/bin/env -S ruby\nputs 1\n"), []byte("# vim: ft=python\n"),
		encodeSource("# 日本語\ndef greet(name):\n    print(name)\n", 2, binary.LittleEndian),
	} {
		f.Add(data, uint8(1), false)
	}
	f.Fuzz(func(t *testing.T, data []byte, chunk uint8, prefix bool) {
		assertReader(t, data, int(chunk)+1, prefix)
	})
}

func TestAnalyzeReaderAllocations(t *testing.T) {
	for _, lines := range []int{100, 16000} {
		data := []byte("#!/usr/bin/ruby\n" + strings.Repeat("# header\n", lines))
		reader := bytes.NewReader(data)
		var analysis languages.Analysis
		allocations := testing.AllocsPerRun(5, func() {
			reader.Reset(data)
			if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{}, &analysis); err != nil {
				t.Fatal(err)
			}
		})
		if allocations > 1 || analysis.Bytes != int64(len(data)) || analysis.Prefix || analysis.Result().Language != languages.Ruby {
			t.Fatalf("lines=%d allocations=%g bytes=%d result=%+v", lines, allocations, analysis.Bytes, analysis.Result())
		}
	}
}

func TestAnalyzeReaderDeclarations(t *testing.T) {
	const module = "module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n"
	for _, test := range []struct {
		name, header, body    string
		language              languages.Language
		conflict, statistical bool
	}{
		{"shebang", "#!/usr/bin/ruby\n", "puts 'hello'\n", languages.Ruby, false, false},
		{"modeline", "# -*- ruby -*-\n", "puts 'hello'\n", languages.Ruby, false, false},
		{"conflict", "#!/usr/bin/ruby\n", "use strict;\n", languages.Unknown, true, false},
		{"wrapper", "#!/bin/sh\nexec python3 \"$0\"\n", "print('hello')\n", languages.Python, false, false},
		{"unknown wrapper", "#!/bin/sh\nexec unknown \"$0\"\n", module, languages.GoModule, false, true},
		{"suppressed modeline", "# -*- ruby -*-\nUseVimball\n", module, languages.GoModule, false, true},
		{"no declaration", "", module, languages.GoModule, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.header + strings.Repeat("\n", 5) + strings.Repeat(test.body, 8000))
			for _, chunk := range []int{7, 32768} {
				var got languages.Analysis
				reader := chunkReader{bytes.NewReader(data), chunk}
				if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{}, &got); err != nil {
					t.Fatal(err)
				}
				result := got.Result()
				if result.Language != test.language || result.Conflict != test.conflict || result.Statistical != test.statistical || got.Bytes != int64(len(data)) || got.Prefix {
					t.Fatalf("chunk %d: bytes=%d prefix=%t result=%+v", chunk, got.Bytes, got.Prefix, result)
				}
			}
		})
	}
}

func TestAnalyzeReaderFilename(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{"main.go", "package main\nfunc main() {}\n"},
		{"api.json", `{"openapi":"3.1.0","info":{"title":"Example"}}`},
		{"settings.json", `{"name":"Ada","enabled":true}`},
		{"app.ts", "export const count: number = 1;\n"},
		{".releaserc", "branches:\n  - main\nplugins: []\n"},
		{"source", "module example.org/project\nrequire (\n example.org/library v1.0.0\n)\n"},
		{"wrong.py", "#!/usr/bin/ruby\nputs 'hello'\n"},
		{"data.go", "\x00binary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, options := range []languages.ReadOptions{
				{Filename: test.name}, {Filename: test.name, Bytes: 16}, {Filename: test.name, Prefix: true},
			} {
				data := []byte(test.source)
				assertNamedReader(t, data, options)
				assertNamedReader(t, encodeSource(test.source, 2, binary.LittleEndian), options)
			}
		})
	}
}

func assertNamedReader(t *testing.T, data []byte, options languages.ReadOptions) {
	t.Helper()
	length, complete := len(data), !options.Prefix
	if options.Bytes > 0 && options.Bytes <= int64(length) {
		length, complete = int(options.Bytes), false
	}
	var want languages.Analysis
	languages.Analyze(data[:length], complete, &want)
	for _, chunk := range []int{1, 511} {
		var got languages.Analysis
		reader := chunkReader{bytes.NewReader(data), chunk}
		if err := languages.AnalyzeReader(context.Background(), reader, options, &got); err != nil {
			t.Fatal(err)
		}
		if got.Result() != want.Result() || got.Detect(options.Filename) != want.Detect(options.Filename) || got.Bytes != want.Bytes || got.Prefix != want.Prefix || got.Binary != want.Binary || got.Signals != want.Signals || got.Count != want.Count {
			t.Fatalf("options=%+v chunk=%d: got %+v, want %+v", options, chunk, got.Detect(options.Filename), want.Detect(options.Filename))
		}
		if reader.Len() != len(data)-length {
			t.Fatalf("read beyond budget: %d unread, want %d", reader.Len(), len(data)-length)
		}
	}
}

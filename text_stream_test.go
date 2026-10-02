package languages

import (
	"bytes"
	"strings"
	"testing"
)

func TestTextStream(t *testing.T) {
	for _, source := range []string{
		"", "\xef\xbb\xbf", "\xef\xbb\xbf#!/usr/bin/ruby\n", "#!/bin/sh\nexec python3 \"$0\"\npass\n",
		"# -*- ruby -*-\n#!/usr/bin/ruby\n", "#!/usr/bin/ruby\n# vim: ft=python\n",
		"# -*- ruby -*-\nUseVimball\n", "# ruby\n# vim: ft=ruby", "# vim: ft=ruby\n",
		strings.Repeat("# header\n", 5) + "# vim: ft=ruby\n" + strings.Repeat("# footer\n", 5),
		strings.Repeat("# header\n", 5) + "# vim: ft=ruby\n" + strings.Repeat("# footer\n", 4),
		strings.Repeat("# header\n", 8) + "# vim: ft=ruby\n\n", "\n\n\n\n\n\n# -*- ruby -*-",
		"/* header */ package main\nfunc main() {}\n", "/* header\nuse strict;\n*/package main\n",
		"\"\"\"docstring\nuse strict;\n\"\"\"\nfrom pathlib import Path\n",
		"const count = 1;\n", "module example.org/project\nrequire (\nexample.org/library v1.0.0\n)\n",
		"/", "-", "'", "\"\"", "/*", "# frozen_string_literal: true\n",
	} {
		for _, chunk := range []int{1, 2, 3, 7, 32, 1024} {
			assertTextStream(t, []byte(source), chunk, true)
			assertTextStream(t, []byte(source), chunk, false)
		}
	}
}

func assertTextStream(t *testing.T, data []byte, chunk int, complete bool) {
	t.Helper()
	var want Analysis
	analyzeTextBuffer(data, complete, &want, allHeuristics)
	stream := textStreams.Get().(*textStream)
	defer textStreams.Put(stream)
	stream.reset()
	for at := 0; at < len(data); at += chunk {
		part := bytes.Clone(data[at:min(at+chunk, len(data))])
		location := sourceMap{base: uint64(at)}
		stream.write(part, location.at)
		clear(part)
	}
	got := stream.finish(complete, allHeuristics)
	got.Bytes = int64(len(data))
	if got != want {
		t.Fatalf("chunk %d complete=%t: got result=%+v signals=%+v, want result=%+v signals=%+v; data=%q", chunk, complete, got.Result(), got.Signals[:got.Count], want.Result(), want.Signals[:want.Count], data)
	}
}

func FuzzTextStream(f *testing.F) {
	f.Add([]byte("<!DOCTYPE html>\n<html>{{/* ignored */}}{{.Title}}</html>"), uint8(1), true)
	f.Add([]byte("{{define \"page\"}}{{printf \"}}\" .Title}}{{end}}"), uint8(7), false)
	for _, source := range []string{"#!/bin/sh\nexec ruby \"$0\"\n", "\xef\xbb\xbf#!/usr/bin/python3\n", "# vim: ft=ruby\n", "package main\nfunc main() {}\n"} {
		f.Add([]byte(source), uint8(1), true)
	}
	f.Fuzz(func(t *testing.T, data []byte, chunk uint8, complete bool) {
		assertTextStream(t, data, int(chunk)+1, complete)
	})
}

func TestTextStreamModelineBoundaries(t *testing.T) {
	header := strings.Repeat("# header\n", 4000)
	for _, modeline := range []string{"# -*- python -*-\n", "# vim: set ft=python:\n", "# -*- invalid -*- vim: ft=python\n"} {
		for _, padding := range []int{0, 1, 7, 31} {
			data := []byte(header + strings.Repeat(" ", padding) + modeline)
			for _, chunk := range []int{511, textChunkBytes - 1, textChunkBytes} {
				assertTextStream(t, data, chunk, true)
				assertTextStream(t, data, chunk, false)
			}
		}
	}
}

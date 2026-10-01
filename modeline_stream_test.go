package languages

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestModeFieldCapacity(t *testing.T) {
	for _, entry := range languageAliases {
		if utf8.RuneCountInString(entry.name)*utf8.UTFMax > modeFieldBytes {
			t.Fatalf("alias %q exceeds the streaming field", entry.name)
		}
	}
}

func TestEmacsStream(t *testing.T) {
	for _, line := range []string{
		"# -*- python -*-", "# -*- mode: ruby; coding: utf-8 -*-", "-*- c -*-",
		"/* -*- coding: utf-8; mode: common-lisp -*- */", "-*- mode: unknown; mode: ruby -*-",
		"-*- MODE: python -*-", "-*- MoDe\u2000: \u2000pYtHoN\u2000 -*-",
		"-*- mode: ruby; mode: python -*-", "-*- ;; -*-", "-*- missing end", "-*- invalid: ruby -*-",
		"-*- python -*- -*- ruby -*-", "-*- mode: \xffpython -*-", "-*- mode: python\xe2 -*-",
		"prefix" + strings.Repeat("x", 65536) + "-*- mode: " + strings.Repeat("\u2000", 65536) + "ruby -*-",
		"-*- coding: " + strings.Repeat("x", 65536) + "; mode: python -*-",
		"-*- mode: py" + strings.Repeat(" ", 65536) + "thon -*-",
	} {
		for _, chunk := range []int{1, 2, 3, 7, 32, 1024, 32768} {
			assertEmacsStream(t, []byte(line), chunk)
		}
	}
}

func assertEmacsStream(t *testing.T, line []byte, chunk int) {
	t.Helper()
	want := emacsLineMode(line)
	var stream emacsStream
	for at := 0; at < len(line); at += chunk {
		part := bytes.Clone(line[at:min(at+chunk, len(line))])
		stream.write(part)
		clear(part)
	}
	if got := stream.result(); got != want {
		t.Fatalf("chunk %d: got %s want %s line=%q", chunk, got, want, line)
	}
}

func FuzzEmacsStream(f *testing.F) {
	for _, source := range []string{"-*- mode: ruby -*-", "-*- coding: utf-8; mode: python -*-", "-*- common-lisp -*-"} {
		f.Add([]byte(source), uint8(1))
	}
	f.Fuzz(func(t *testing.T, line []byte, chunk uint8) { assertEmacsStream(t, line, int(chunk)+1) })
}

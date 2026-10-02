package languages

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/git-pkgs/scan"
)

func TestDetectEveryHeuristicGroup(t *testing.T) {
	inputs := []string{
		"", "const count: number = 1;\n", "use strict;\npackage Demo::Client;\n",
		"{\"openapi\":\"3.1.0\"}\n", "openapi: 3.1.0\npaths: {}\n",
		"\t\tHP: 100\n", "#include <vector>\nint main() {}\n",
		".Dd October 1\n.Dt TEST 1\n.Sh NAME\n", "BEGIN\nRETURN foo\nEND\nGOSUB 100\n",
		"<?xml version=\"1.0\"?>\n<TS version=\"2.1\"></TS>\n",
		"#!/usr/bin/ruby\nputs 'hello'\n", "\x00binary",
		"\xff\xfe{\x00}\x00\n\x00", "\xff\xfe\x00\x00{\x00\x00\x00}\x00\x00\x00",
	}
	analyses := make([]Analysis, len(inputs))
	for i, input := range inputs {
		Analyze([]byte(input), false, &analyses[i])
	}
	for _, group := range heuristicGroups {
		t.Run(group.extensions[0], func(t *testing.T) {
			t.Parallel()
			for _, extension := range group.extensions {
				for _, name := range []string{"src/sample" + extension, "src/SAMPLE" + strings.ToUpper(extension)} {
					for i, input := range inputs {
						if got, want := Detect(name, []byte(input)), analyses[i].Detect(name); got != want {
							t.Fatalf("%s %q: got %+v, want %+v", name, input, got, want)
						}
					}
				}
			}
		})
	}
}

func TestHeuristicScannerMatchesRegexp(t *testing.T) {
	inputs := [][]byte{
		nil,
		[]byte("package main\nfunc main() {}\n"),
		[]byte("\t\tHP: 10\n---\n"),
		[]byte("{\"openapi\":\"3.1.0\"}"),
		[]byte("first line\n#define DEBUG 1\n#include <vector>\n"),
		[]byte("use strict;\npackage Demo::Client;\n"),
		[]byte(".Dd October 1\n.Dt TEST 1\n.Sh NAME\n"),
		[]byte("BEGIN\nRETURN foo\nEND\nGOSUB 100\n"),
		bytes.Repeat([]byte("x\n\t "), 256),
	}
	compiled := make([]*regexp.Regexp, len(heuristicPatterns))
	for i, pattern := range heuristicPatterns {
		compiled[i] = regexp.MustCompile(pattern)
	}
	database := sharedHeuristics().database
	scratch := scan.NewScratch(database)
	for _, input := range inputs {
		var matched [len(heuristicPatterns)]bool
		if err := database.Scan(input, scratch, func(match scan.Match) error {
			matched[match.ID] = true
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for i, pattern := range compiled {
			if want := pattern.Match(input); matched[i] != want {
				t.Errorf("pattern %d %q, input %q: got %t, want %t", i, pattern, input, matched[i], want)
			}
		}
	}
}

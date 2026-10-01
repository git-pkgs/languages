package languages

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandCapacity(t *testing.T) {
	for _, r := range interpreterRules {
		if len(r.prefix)-1 > commandBytes {
			t.Fatal(r.prefix)
		}
	}
}

func TestHeaderStream(t *testing.T) {
	for _, source := range []string{
		"#!/usr/bin/python3\n", "#!/usr/bin/ruby", "#!/usr/bin/ruby   ", "#!/usr/bin/ruby -v",
		"#!/usr/bin/ruby\u2000", "#!/usr/bin/ruby\u2000 -v", "#!\u2000/usr/bin/ruby\u2000",
		"#!/usr/bin/env -ivS python3 -u\n", "#!/usr/bin/env -Si python3\n", "#!/usr/bin/env -\n",
		"#!/usr/bin/env -u HOME KEY=value ruby\n", "#!/usr/bin/env --ignore-environment --split-string ruby\n",
		"#!/usr/bin/env --unset=HOME ruby\n", "#!/usr/bin/env -- ruby\n", "#!/usr/bin/env -- ruby",
		"#!/usr/bin/env --unknown ruby\n", "#!/usr/bin/env NAME=value\n", "#!/usr/bin/env -u\n",
		"#!/usr/bin/osascript -l JavaScript\n", "#!/usr/bin/osascript -lJavaScript\n", "#!/usr/bin/env osascript -s s -l JavaScript\n",
		"#!/bin/sh\nexec ruby \"$0\"\n", "#!/bin/sh\nexec env -S python3 \"$0\"\n", "#!/bin/sh\nexec /usr/bin/env python3 \"$0\"\n",
		"#!/bin/sh\nexec python3 \"$0\"", "#!/bin/sh\nexec python3 \"other.py\"\n", "#!/bin/sh\n\u2000exec python3 \"$0\"\n",
		"#!/bin/sh\nexec env --unknown \"$0\"\nexec ruby \"$0\"\n", "#!/bin/sh\nexec / \"$0\"\nexec ruby \"$0\"\n",
		"#!/bin/sh -l\nexec osascript \"$0\"\n", "#!/bin/sh\nexec osascript -l JavaScript \"$0\"\n",
		"#!/bin/sh\n# first\n# second\n# third\nexec python3 \"$0\"\n", "#!/bin/sh\n# first\n# second\n# third\n# fourth\nexec python3 \"$0\"\n",
		"#!/usr/bin/perl6\n", "#!/usr/bin/" + strings.Repeat("x", 65536) + "/ruby\n",
		"#!/usr/bin/python" + strings.Repeat("3.0", 65536) + "\n", "#!/usr/bin/python" + strings.Repeat("3.0", 65536) + "bad\n",
		"#!/usr/bin/env NAME=" + strings.Repeat("x", 65536) + " ruby\n",
		"#!/usr/bin/env -" + strings.Repeat("iv", 65536) + "S python3\n",
	} {
		for _, chunk := range []int{1, 2, 3, 7, 1024, 32768} {
			assertHeaderStream(t, []byte(source), chunk, true)
			assertHeaderStream(t, []byte(source), chunk, false)
		}
	}
}

func assertHeaderStream(t *testing.T, data []byte, chunk int, complete bool) {
	t.Helper()
	var analysis Analysis
	analysis.Prefix = !complete
	var seen [2]uint64
	line, _, terminated := bytes.Cut(data, []byte("\n"))
	if bytes.HasPrefix(line, []byte("#!")) {
		detectShebangLine(data, line, terminated || complete, 0, &analysis, &seen)
	}
	want := -1
	if analysis.Count > 0 {
		want = int(analysis.Signals[0].Rule)
	}
	var stream headerStream
	for at := 0; at < len(data); at += chunk {
		part := bytes.Clone(data[at:min(at+chunk, len(data))])
		stream.write(part)
		clear(part)
	}
	if got := stream.finish(complete); got != want {
		t.Fatalf("chunk %d complete=%t: got rule %d want %d data=%q", chunk, complete, got, want, data)
	}
}

func FuzzHeaderStream(f *testing.F) {
	for _, prefix := range []string{"#!/usr/bin/", "#!/usr/bin/env ", "#!/bin/sh\nexec ", "#!/bin/sh\nexec env ", "#!/bin/sh -l\nexec "} {
		f.Add([]byte(prefix), []byte("ruby \"$0\"\n"), uint8(1), true)
	}
	f.Fuzz(func(t *testing.T, prefix, body []byte, chunk uint8, complete bool) {
		assertHeaderStream(t, append(bytes.Clone(prefix), body...), int(chunk)+1, complete)
	})
}

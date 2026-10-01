package cli_test

import (
	"bytes"
	"encoding/json"
	"github.com/git-pkgs/languages/internal/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, tt := range []struct {
		args            []string
		input, language string
		conflict        bool
	}{
		{nil, "#!/bin/bash\nprintf '\x1b[31mred\x1b[0m\\n'\n", "Bash", false},
		{nil, "use strict;\nuse warnings;\n", "Perl", false},
		{[]string{"-name", "wrong.py"}, "use strict;\nuse warnings;\n", "", true},
		{[]string{"-mode", "content", "-name", "wrong.py"}, "use strict;\nuse warnings;\n", "Perl", false},
		{[]string{"-name", "app.ts", "-"}, "const count = 1;\n", "TypeScript", false},
		{[]string{"-mode", "combined", "-name", "demo.pl"}, ":- use_module(library(lists)).\n", "Prolog", false},
		{[]string{"-mode", "combined", "-name", "wrong.py"}, "use strict;\nuse warnings;\n", "", true},
		{[]string{"-mode", "path", "Gemfile"}, "", "Ruby", false},
		{[]string{"-name", "settings.plist"}, "<?xml version=\"1.0\"?>\n<plist><dict/></plist>\n", "XML Property List", false},
		{[]string{"-name", "script.RB"}, "# vim: set ft=ruby:\n", "Ruby", false},
		{[]string{"-name", "example.1ssl"}, ".TH EXAMPLE 1\n.SH NAME\nexample\n", "Roff Manpage", false},
		{[]string{"-name", "script.py"}, "#!/usr/bin/env -vS python3 -u\npass\n", "Python", false},
		{[]string{"-name", "script.py"}, "\xff\xfep\x00a\x00s\x00s\x00\n\x00", "Python", false},
	} {
		var out bytes.Buffer
		if err := cli.Run(tt.args, strings.NewReader(tt.input), &out, &out); err != nil {
			t.Fatal(err)
		}
		var got cli.Output
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Language != tt.language || got.Conflict != tt.conflict {
			t.Fatal(out.String())
		}
	}
}

func TestCLIUsesSourceFilename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.js")
	if err := os.WriteFile(path, []byte("const count = 1;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		args         []string
		language     string
		pathEvidence bool
	}{
		{[]string{path}, "JavaScript", true},
		{[]string{"-name", "app.ts", path}, "TypeScript", true},
		{[]string{"-mode", "content", path}, "", false},
	} {
		var output bytes.Buffer
		if err := cli.Run(tt.args, strings.NewReader(""), &output, &output); err != nil {
			t.Fatal(err)
		}
		var got cli.Output
		if err := json.Unmarshal(output.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Language != tt.language || (got.Path != nil) != tt.pathEvidence || got.Bytes == 0 {
			t.Fatal(got)
		}
	}
}

func TestCLIBoundedRead(t *testing.T) {
	in := strings.NewReader("#!/usr/bin/ruby\n" + strings.Repeat("x", 4096))
	var out bytes.Buffer
	if err := cli.Run([]string{"-bytes", "128"}, in, &out, &out); err != nil {
		t.Fatal(err)
	}
	var got cli.Output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Bytes != 128 || !got.Prefix || in.Len() != 4096+len("#!/usr/bin/ruby\n")-128 {
		t.Fatal(got, in.Len())
	}
	for _, args := range [][]string{{"-bytes", "-1"}, {"-bytes", "9223372036854775808"}, {"-mode", "bad"}, {"-mode", "path"}} {
		if err := cli.Run(args, strings.NewReader(""), &out, &out); err == nil {
			t.Fatal(args)
		}
	}
}

func TestAlreadyTruncatedInput(t *testing.T) {
	var output bytes.Buffer
	if err := cli.Run([]string{"-prefix"}, strings.NewReader("#!/usr/bin/ruby"), &output, &output); err != nil {
		t.Fatal(err)
	}
	var got cli.Output
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Prefix || got.Language != "" {
		t.Fatal(got)
	}
}

func TestCLIByteLimits(t *testing.T) {
	const perl = "use strict;\nuse warnings;\n"
	delayed := strings.Repeat("# header\n", 256) + perl
	for _, test := range []struct {
		name, source, language string
		args                   []string
		bytes                  int
		prefix                 bool
	}{
		{"default reaches source", delayed, "Perl", nil, len(delayed), false},
		{"explicit short prefix", delayed, "", []string{"-bytes", "1024"}, 1024, true},
		{"short prefix detects source", perl, "Perl", []string{"-bytes", "1024"}, len(perl), false},
		{"default reads full file", strings.Repeat("\n", 65536) + perl, "Perl", nil, 65536 + len(perl), false},
		{"explicit full file", delayed, "Perl", []string{"-bytes", "0"}, len(delayed), false},
		{"larger explicit prefix", strings.Repeat("\n", 65536) + perl, "Perl", []string{"-bytes", "131072"}, 65536 + len(perl), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := strings.NewReader(test.source)
			var output bytes.Buffer
			if err := cli.Run(test.args, input, &output, &output); err != nil {
				t.Fatal(err)
			}
			var got cli.Output
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Language != test.language || got.Bytes != int64(test.bytes) || got.Prefix != test.prefix || input.Len() != len(test.source)-test.bytes {
				t.Fatal(output.String(), input.Len())
			}
		})
	}
}

func TestCLIReadsFileBeyondOneKiB(t *testing.T) {
	source := strings.Repeat("# header\n", 16000) + "use strict;\nuse warnings;\n"
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := cli.Run([]string{path}, strings.NewReader(""), &output, &output); err != nil {
		t.Fatal(err)
	}
	var got cli.Output
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Language != "Perl" || got.Bytes != int64(len(source)) || got.Prefix {
		t.Fatal(output.String())
	}
}

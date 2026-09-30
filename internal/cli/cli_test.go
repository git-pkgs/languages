package cli_test

import (
	"bytes"
	"encoding/json"
	"github.com/git-pkgs/languages/internal/cli"
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
		{[]string{"-name", "wrong.py"}, "use strict;\nuse warnings;\n", "Perl", false},
		{[]string{"-mode", "combined", "-name", "demo.pl"}, ":- use_module(library(lists)).\n", "Prolog", false},
		{[]string{"-mode", "combined", "-name", "wrong.py"}, "use strict;\nuse warnings;\n", "", true},
		{[]string{"-mode", "path", "Gemfile"}, "", "Ruby", false},
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
	for _, args := range [][]string{{"-bytes", "0"}, {"-bytes", "65537"}, {"-mode", "bad"}, {"-mode", "path"}} {
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

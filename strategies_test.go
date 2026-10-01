package languages_test

import (
	"strings"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestEditorModelines(t *testing.T) {
	for _, source := range []string{
		"# -*- ruby -*-\n",
		"# -*-mode:Ruby-*-\n",
		"# -*- coding: utf-8; mode: ruby; -*-\n",
		"# vim: set ft=ruby:\n",
		"# vim: syntax=ruby\n",
		"# vim: ft  =ruby\n",
		"# vim: set\tft=ruby:\n",
		"# Vim>800: noai: filetype=ruby\n",
		"# ex: ft=ruby\n",
		strings.Repeat("# header\n", 8) + "# vim: set ft=ruby:\n",
	} {
		var a languages.Analysis
		languages.Analyze([]byte(source), true, &a)
		if got := a.Detect("script.rb"); got.Language != languages.Ruby || got.Conflict || got.Confidence != languages.High {
			t.Errorf("%q: %+v", source, got)
		}
		if a.Count != 1 || a.Signals[0].Evidence().ID != "modeline" || !a.Signals[0].Evidence().Languages.Has(languages.Ruby) {
			t.Errorf("missing modeline evidence for %q", source)
		}
		if got := a.Detect("script.py"); !got.Conflict || got.Language != languages.Unknown {
			t.Errorf("modeline contradiction lost: %+v", got)
		}
	}
}

func TestStrategyAllocations(t *testing.T) {
	for _, source := range []string{
		"# -*- coding: utf-8; mode: Ruby -*-\n",
		"# vim: set filetype=Ruby:\n",
		"#!/usr/bin/env -vS python3 -u\npass\n",
		"#!/bin/sh\nexec python3 \"$0\" \"$@\"\n",
	} {
		data := []byte(source)
		var a languages.Analysis
		if allocations := testing.AllocsPerRun(100, func() { languages.Analyze(data, true, &a) }); allocations != 0 && !raceEnabled {
			t.Errorf("%q: %v allocations", source, allocations)
		}
	}
}

func TestModelineBoundaries(t *testing.T) {
	for _, source := range []string{
		"# -*-mode:ruby\n",
		"# vim: set ft=ruby\n",
		"prefixvim: ft=ruby\n",
		"# vim: titlestring=\\ ft=ruby\n",
		"# vim<: ft=ruby\n",
		"UseVimball\n# vim: ft=ruby\n",
		strings.Repeat("# header\n", 5) + "# vim: ft=ruby\n" + strings.Repeat("# footer\n", 5),
	} {
		var a languages.Analysis
		languages.Analyze([]byte(source), true, &a)
		for _, match := range a.Signals[:a.Count] {
			if match.Evidence().ID == "modeline" {
				t.Errorf("unexpected modeline: %q", source)
			}
		}
	}
	for _, source := range []string{"# vim: ft=ruby", strings.Repeat("# header\n", 8) + "# vim: ft=ruby\n"} {
		if got := languages.Detect("", []byte(source)); got.Language == languages.Ruby {
			t.Errorf("accepted incomplete modeline/footer: %q", source)
		}
	}
	if got := languages.Detect("", []byte("# -*- common-lisp -*-\n")); got.Language != languages.CommonLisp {
		t.Fatal(got)
	}
}

func TestShebangOptionsAndWrappers(t *testing.T) {
	for _, source := range []string{
		"#!/usr/bin/env -vS python3 -u\npass\n",
		"#!/usr/bin/env -ivS python3 -u\npass\n",
		"#!/usr/bin/env --ignore-environment -u HOME KEY=value python3\npass\n",
		"#!/usr/bin/env -- python3\npass\n",
		"#!/bin/sh\n\"\"\":\"\nexec python3 \"$0\" \"$@\"\n\":\"\"\"\nprint('hello')\n",
		"#!/usr/bin/env sh\nexec /usr/bin/python3 \"$0\" \"$@\"\n",
	} {
		if got := languages.Detect("script.py", []byte(source)); got.Language != languages.Python || got.Conflict {
			t.Errorf("%q: %+v", source, got)
		}
	}
	for _, source := range []string{
		"#!/bin/sh\nexec python3 \"another.py\"\n",
		"#!/bin/sh\n# header\n# header\n# header\n# header\nexec python3 \"$0\"\n",
	} {
		if got := languages.Detect("", []byte(source)); got.Language != languages.Shell {
			t.Errorf("%q: %+v", source, got)
		}
	}
}

func TestOSAScriptLanguageOption(t *testing.T) {
	for _, command := range []string{
		"/usr/bin/osascript -l JavaScript",
		"/usr/bin/env osascript -l JavaScript",
		"/usr/bin/env -S osascript -lJavaScript",
		"/usr/bin/osascript -s s -l JavaScript",
	} {
		data := []byte("#!" + command + "\nfunction run(argv) { return Application.currentApplication(); }\n")
		if got := languages.Detect("automation.js", data); got.Language != languages.JavaScript || got.Conflict {
			t.Errorf("%s: %+v", command, got)
		}
	}
	for _, command := range []string{"/usr/bin/osascript -l JavaScript", "/usr/bin/osascript -lJavaScript", "/usr/bin/osascript -l Unknown"} {
		var a languages.Analysis
		languages.Analyze([]byte("#!"+command+"\n"), true, &a)
		if got := a.Result(); got.Candidates.Has(languages.AppleScript) {
			t.Errorf("language override ignored: %s: %+v", command, got)
		}
	}
	if got := languages.Detect("", []byte("#!/usr/bin/osascript\n")); got.Language != languages.AppleScript {
		t.Fatal(got)
	}
}

func TestManpageDetection(t *testing.T) {
	for _, name := range []string{"example.1ssl", "example.3p.in", "example.0p", "example.9_X"} {
		if got := languages.Detect(name, []byte(".TH EXAMPLE 1\n.SH NAME\nexample \\- example\n")); got.Language != languages.RoffManpage || got.Conflict {
			t.Errorf("%s: %+v", name, got)
		}
	}
	for _, name := range []string{"example.12", "example.0", "example.1-foo"} {
		if got := languages.AnalyzePath(name); got.Candidates.Has(languages.RoffManpage) {
			t.Errorf("unexpected manpage suffix: %s", name)
		}
	}
}

func TestCaseInsensitiveExtensions(t *testing.T) {
	for name, want := range map[string]languages.Language{
		"sample.PY":     languages.Python,
		"script.Rb":     languages.Ruby,
		"view.HTML.ERB": languages.ERB,
		"source.C":      languages.CPP,
	} {
		if got := languages.Detect(name, nil); got.Language != want {
			t.Errorf("%s: %+v", name, got)
		}
	}
	for alias, want := range map[string]languages.Language{"COMMON-LISP": languages.CommonLisp, "PyThOn": languages.Python, "Emacs-Lisp": languages.EmacsLisp} {
		if got := languages.Parse(alias); got != want {
			t.Errorf("Parse(%q) = %v", alias, got)
		}
	}
}

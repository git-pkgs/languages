package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestGenerateSnapshot(t *testing.T) {
	output := filepath.Join(t.TempDir(), "registry.go")
	command := exec.Command("go", "run", ".", "-source", "languages.yml", "-revision-file", "revision", "-out", output)
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generator command: %v\n%s", err, data)
	}
	want, err := os.ReadFile("../../registry_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("registry differs from its pinned metadata; run go generate ./...")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	data := []byte("Swift:\n  extensions: [.swift]\n  aliases: [swiftlang]\n  interpreters: [swift]\nRuby:\n  extensions: [.rb, .shared]\n  filenames: [Gemfile]\nZig:\n  extensions: [.shared]\n")
	first, err := generate(data, testRevision)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		got, err := generate(data, testRevision)
		if err != nil || !bytes.Equal(first, got) {
			t.Fatalf("nondeterministic generation: %v", err)
		}
	}
	for _, want := range []string{testRevision, `"swiftlang"`, `".shared"`, `"Gemfile"`, `"interpreter.swift"`} {
		if !bytes.Contains(first, []byte(want)) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestGenerateRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct{ data, revision string }{
		{"", testRevision},
		{"[", testRevision},
		{"Swift: {}", "main"},
		{"Swift: {}", strings.Repeat("z", 40)},
	} {
		if _, err := generate([]byte(test.data), test.revision); err == nil {
			t.Errorf("accepted metadata %q and revision %q", test.data, test.revision)
		}
	}
}

func TestGeneratedSyntaxAndAliases(t *testing.T) {
	generated, err := generate([]byte("XML:\n  tm_scope: text.xml\nXML Property List:\n  tm_scope: text.xml.plist\n  extensions: [.tmLanguage]\n"), testRevision)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"xml-property-list"`, `".tmlanguage"`, "var xmlLanguages ="} {
		if !bytes.Contains(generated, []byte(value)) {
			t.Errorf("missing %s", value)
		}
	}
}

func TestCommandPreservesOutputOnFailure(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "registry.go")
	if err := os.WriteFile(output, []byte("existing output"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "run", ".", "-source", "languages.yml", "-revision-file", filepath.Join(dir, "missing"), "-out", output)
	if err := command.Run(); err == nil {
		t.Fatal("command accepted missing revision")
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != "existing output" {
		t.Fatalf("output changed after failed generation: %q, %v", got, err)
	}
}

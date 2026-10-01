package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/git-pkgs/languages"
	"github.com/git-pkgs/languages/internal/cli"
)

func treeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"api/main.go":       "package main\n" + strings.Repeat(" ", 2035),
		"api/lib/helper.go": "package lib\n" + strings.Repeat(" ", 1012),
		"web/app.ts":        "const count = 1;\n" + strings.Repeat(" ", 1007),
		".git/config":       "metadata",
	} {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runTree(t *testing.T, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := cli.Run(args, strings.NewReader(""), &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestCLIDirectory(t *testing.T) {
	root := treeFixture(t)
	want := filepath.ToSlash(root) + "/  Go 75.0%, TypeScript 25.0% (3 files, 4.0 KiB)\n" +
		"├── api/  Go 100.0% (2 files, 3.0 KiB)\n" +
		"│   └── lib/  Go 100.0% (1 file, 1.0 KiB)\n" +
		"└── web/  TypeScript 100.0% (1 file, 1.0 KiB)\n"
	if got := runTree(t, root); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := runTree(t, "-depth", "0", root); got != strings.SplitAfter(want, "\n")[0] {
		t.Fatal(got)
	}
	if got := runTree(t, "-depth", "1", root); strings.Contains(got, "lib/") || !strings.Contains(got, "api/") {
		t.Fatal(got)
	}
	if got := runTree(t, "-bytes", "1024", root); !strings.Contains(got, "3 files, 4.0 KiB, 1 partial") {
		t.Fatal(got)
	}
}

func TestCLIDirectoryJSON(t *testing.T) {
	root := treeFixture(t)
	for _, depth := range []string{"-1", "0", "1"} {
		var got languages.Directory
		if err := json.Unmarshal([]byte(runTree(t, "-json", "-depth", depth, root)), &got); err != nil {
			t.Fatal(err)
		}
		if got.Path != "." || got.Summary.Files != 3 || got.Summary.Bytes != 4096 || got.Summary.Languages[0].Language != "Go" {
			t.Fatal(got)
		}
		if depth == "0" {
			if len(got.Children) != 0 {
				t.Fatal(got.Children)
			}
			continue
		}
		if len(got.Children) != 2 || got.Children[0].Path != "api" || got.Children[0].Summary.Bytes != 3072 {
			t.Fatal(got.Children)
		}
		if (len(got.Children[0].Children) == 0) != (depth == "1") {
			t.Fatal(got.Children[0])
		}
	}
	var subtree languages.Directory
	if err := json.Unmarshal([]byte(runTree(t, "-json", filepath.Join(root, "api"))), &subtree); err != nil {
		t.Fatal(err)
	}
	if subtree.Path != "." || subtree.Summary.Files != 2 || len(subtree.Summary.Languages) != 1 || subtree.Children[0].Path != "lib" {
		t.Fatal(subtree)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCLIDirectoryErrors(t *testing.T) {
	root := treeFixture(t)
	for _, args := range [][]string{
		{"-mode", "content", root}, {"-mode", "path", root},
		{"-name", "main.go", root}, {"-prefix", root}, {"-depth", "-2", root},
	} {
		if err := cli.Run(args, strings.NewReader(""), io.Discard, io.Discard); err == nil {
			t.Fatal(args)
		}
	}
	for _, args := range [][]string{{root}, {"-json", root}} {
		if err := cli.Run(args, strings.NewReader(""), failingWriter{}, io.Discard); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}
}

func TestCLIDirectoryCategories(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"unknown": "", "ambiguous.pl": "", "wrong.py": "#!/usr/bin/ruby\n", "binary.go": "\x00",
		"src/main.go": "package main\n",
	} {
		filename := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := runTree(t, root)
	for _, fragment := range []string{"unknown 0.0%", "ambiguous 0.0%", "conflicting", "binary", "30 B"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("missing %q in %s", fragment, got)
		}
	}
	if got := runTree(t, t.TempDir()); !strings.Contains(got, "empty (0 files, 0 B)") {
		t.Fatal(got)
	}
}

func TestCLIDirectoryEscapedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows forbids control characters in filenames")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "line\nbreak")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := runTree(t, root); !strings.Contains(got, "line\\nbreak/") || strings.Contains(got, "line\nbreak") {
		t.Fatal(got)
	}
}

package localscan_test

import (
	"fmt"
	"github.com/git-pkgs/languages"
	"github.com/git-pkgs/languages/internal/evaluate"
	"github.com/git-pkgs/languages/internal/localscan"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalRepositories(t *testing.T) {
	root := t.TempDir()
	for repo := range 2 {
		dir := filepath.Join(root, fmt.Sprintf("repo%d", repo))
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		for file := range 5 {
			source := fmt.Sprintf("package main\nfunc f%d() {}\n", file) + strings.Repeat(" ", 256)
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.go", file)), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	out := filepath.Join(t.TempDir(), "corpus")
	stats, err := localscan.Run(root, "", out)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Repositories != 2 || stats.Files != 10 || stats.Selected != 10 || stats.Samples < 3 || stats.Samples > 6 {
		t.Fatal(stats)
	}
	report, err := evaluate.Run(out, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != stats.Samples || report.Rows[3].Ours.Correct != stats.Samples {
		t.Fatal(report.Files, stats.Samples)
	}
	if stats.SamplesByLanguage[languages.Go] != stats.Samples {
		t.Fatal(stats)
	}
}

func TestExcludedPaths(t *testing.T) {
	root := t.TempDir()
	excluded := filepath.Join(root, "excluded")
	for _, dir := range []string{excluded, filepath.Join(root, "included")} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sep := string(filepath.Separator)
	for _, exclude := range []string{excluded + sep, excluded + sep + ".", root + sep + "included" + sep + ".." + sep + "excluded"} {
		stats, err := localscan.Run(root+sep, exclude, "")
		if err != nil {
			t.Fatal(err)
		}
		if stats.Repositories != 1 || stats.Files != 1 || stats.Excluded != excluded || stats.Root != root {
			t.Fatal(stats)
		}
	}
	stats, err := localscan.Run(root+sep, root+sep+".", "")
	if err != nil || stats.Repositories != 0 || stats.Files != 0 {
		t.Fatal(stats, err)
	}
}

func TestValidatePaths(t *testing.T) {
	root := t.TempDir()
	sep := string(filepath.Separator)
	for _, tt := range []struct {
		name, root, out string
		valid           bool
	}{
		{"no output", root + sep, "", true},
		{"outside", root, root + "-samples", true},
		{"relative root", "relative", "", false},
		{"relative output", root, "relative", false},
		{"same", root + sep, root, false},
		{"child", root + sep, filepath.Join(root, "samples"), false},
		{"root dot", root + sep + ".", filepath.Join(root, "samples"), false},
		{"output dotdot", root, root + "-samples" + sep + ".." + sep + filepath.Base(root) + sep + "samples", false},
		{"outside dotdot", root, root + sep + ".." + sep + "samples", true},
		{"volume root", filepath.VolumeName(root) + sep, root, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := localscan.Validate(tt.root, tt.out); (err == nil) != tt.valid {
				t.Fatalf("Validate(%q, %q) = %v", tt.root, tt.out, err)
			}
		})
	}
}

package evaluate_test

import (
	"bytes"
	"github.com/git-pkgs/languages"
	"github.com/git-pkgs/languages/internal/evaluate"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusBoundaries(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Ruby", "filenames")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("#!/usr/bin/ruby\n"+strings.Repeat(" ", 200)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "large.rb"), []byte(strings.Repeat("x", languages.MaxBytes+2)), 0600); err != nil {
		t.Fatal(err)
	}
	var predictions bytes.Buffer
	calls := 0
	report, err := evaluate.Run(root, func(req evaluate.Request) (evaluate.Response, error) {
		calls++
		if req.Mode == "content" && req.Name != "" {
			t.Fatal("filename leak")
		}
		if req.Mode == "path" && len(req.Content) != 0 {
			t.Fatal("content leak")
		}
		return evaluate.Response{Language: "Ruby"}, nil
	}, &predictions)
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 2 || report.Oversized != 1 || calls != 33 || report.Rows[5].Ours.Total != 1 || report.Rows[0].Ours.Total != 2 {
		t.Fatal(report.Files, report.Oversized, calls)
	}
	if !bytes.Contains(predictions.Bytes(), []byte("prefix_sha256")) {
		t.Fatal("missing provenance")
	}
}

func TestDevelopmentCorpus(t *testing.T) {
	r, err := evaluate.Run("../../testdata/corpus", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Files < 20 {
		t.Fatal(r.Files)
	}
	for _, row := range r.Rows {
		if row.Ours.Total != r.Files || row.Ours.Correct+row.Ours.Wrong+row.Ours.Ambiguous+row.Ours.Unknown != row.Ours.Total {
			t.Fatal(row)
		}
	}
}

func TestCorpusAliases(t *testing.T) {
	for name, source := range map[string]string{
		"Jinja2": "{% extends 'base.html' %}\n", "HTML+Jinja": "{% extends 'base.html' %}\n",
		"HTML+ERB": "<%= title %>\n", "HTML+PHP": "<?php\n", "Matlab": "function result = add(a,b)\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, name)
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sample"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := evaluate.Run(root, func(evaluate.Request) (evaluate.Response, error) {
				return evaluate.Response{Language: name}, nil
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			row := report.Rows[0]
			if report.Files != 1 || report.Unsupported != 0 || row.Ours.CandidateHits != 1 || row.Baseline.Correct != 1 {
				t.Fatalf("files %d, unsupported %d, row %+v", report.Files, report.Unsupported, row)
			}
		})
	}
}

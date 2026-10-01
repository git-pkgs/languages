package evaluate_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	if err := os.WriteFile(filepath.Join(dir, "large.rb"), []byte(strings.Repeat("x", 64*1024+2)), 0600); err != nil {
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
	wantCalls := 2 * len(evaluate.Sizes) * len(evaluate.Modes)
	if report.Files != 2 || calls != wantCalls || report.Rows[len(evaluate.Sizes)-1].Ours.Total != 2 || report.Rows[0].Ours.Total != 2 {
		t.Fatal(report.Files, calls)
	}
	if !bytes.Contains(predictions.Bytes(), []byte("prefix_sha256")) {
		t.Fatal("missing provenance")
	}
}

func TestUnsupportedAndConflictDiagnostics(t *testing.T) {
	root := t.TempDir()
	for path, source := range map[string]string{
		"Ruby/wrong.rb":       "#!/usr/bin/python3\nprint(1)\n",
		"Unsupported/main.py": "#!/usr/bin/python3\nprint(1)\n",
		"Unsupported/plain":   "ordinary words\n",
		"Unsupported/shared":  "const count = 1;\n",
		"Unsupported/large":   strings.Repeat(" ", 64*1024+1),
	} {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var predictions bytes.Buffer
	calls := 0
	report, err := evaluate.Run(root, func(evaluate.Request) (evaluate.Response, error) {
		calls++
		return evaluate.Response{Language: "Ruby"}, nil
	}, &predictions)
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 1 || report.Unsupported != 4 || calls != len(report.Rows) {
		t.Fatal(report.Files, report.Unsupported, calls)
	}
	for _, row := range report.Rows {
		wantTotal := 4
		if row.Ours.Total != 1 || row.Baseline.Correct != 1 || row.ByLanguage[languages.Unknown].Total != 0 || row.Unsupported.Total != wantTotal {
			t.Fatal(row)
		}
		if row.Unsupported.Selected != 1 || row.Unsupported.Selected+row.Unsupported.Ambiguous+row.Unsupported.Unknown != wantTotal {
			t.Fatal(row.Unsupported)
		}
		if row.Mode == "combined" && (row.Ours.Conflicts != 1 || row.Unsupported.Ambiguous != 1 || row.Unsupported.HighSelected != 1) {
			t.Fatal(row)
		}
	}
	checkDiagnosticPredictions(t, &predictions)
}

func checkDiagnosticPredictions(t *testing.T, predictions *bytes.Buffer) {
	t.Helper()
	decoder := json.NewDecoder(predictions)
	var sawConflict, sawUnsupported bool
	for decoder.More() {
		var p evaluate.Prediction
		if err := decoder.Decode(&p); err != nil {
			t.Fatal(err)
		}
		if p.Bytes != 1024 || p.Mode != "combined" {
			continue
		}
		switch filepath.ToSlash(p.Path) {
		case "Ruby/wrong.rb":
			sawConflict = p.Supported && p.Conflict && p.Language == "" && len(p.Evidence) == 1 && p.Evidence[0].ID == "python.shebang"
		case "Unsupported/main.py":
			sawUnsupported = !p.Supported && p.Expected == "Unsupported" && p.Language == "Python" && p.Baseline == "" && !p.Binary
		}
	}
	if !sawConflict || !sawUnsupported {
		t.Fatal(sawConflict, sawUnsupported)
	}
}

func TestUnsupportedOnlyCorpus(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Unsupported")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary.py"), []byte{0}, 0600); err != nil {
		t.Fatal(err)
	}
	var predictions bytes.Buffer
	report, err := evaluate.Run(root, nil, &predictions)
	if err != nil {
		t.Fatal(err)
	}
	if report.Files != 0 || report.Unsupported != 1 {
		t.Fatal(report.Files, report.Unsupported)
	}
	for _, row := range report.Rows {
		if row.Mode == "combined" && (row.Unsupported.Unknown != 1 || row.Unsupported.Conflicts != 1) {
			t.Fatal(row)
		}
	}
	if !bytes.Contains(predictions.Bytes(), []byte(`"binary":true`)) {
		t.Fatal("missing binary diagnostic")
	}
}

func TestLargerPrefixBudgets(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Perl")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	source := strings.Repeat("\n", 16384) + "use strict;\nuse warnings;\n"
	if err := os.WriteFile(filepath.Join(dir, "sample"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := evaluate.Run(root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Mode == "path" {
			continue
		}
		if row.Bytes == 0 || row.Bytes > 16384 {
			if row.Ours.Correct != 1 {
				t.Fatal(row)
			}
		} else if row.Ours.Unknown != 1 {
			t.Fatal(row)
		}
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

func TestFullFileEvaluation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Perl")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	source := strings.Repeat("# header\n", 16000) + "use strict;\nuse warnings;\n"
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var predictions bytes.Buffer
	fullCalls := 0
	report, err := evaluate.Run(root, func(req evaluate.Request) (evaluate.Response, error) {
		if string(req.Content) == source {
			fullCalls++
		}
		return evaluate.Response{Language: "Perl"}, nil
	}, &predictions)
	if err != nil {
		t.Fatal(err)
	}
	if fullCalls != 2 {
		t.Fatalf("baseline full-file calls=%d", fullCalls)
	}
	for _, row := range report.Rows {
		if row.Ours.Total != 1 {
			t.Fatal(row)
		}
		if row.Mode != "path" && row.Bytes == 0 && row.Ours.Correct != 1 {
			t.Fatal(row)
		}
		if row.Bytes > 0 && row.Ours.Unknown != 1 {
			t.Fatal(row)
		}
	}
	decoder := json.NewDecoder(&predictions)
	fullPredictions := 0
	for decoder.More() {
		var p evaluate.Prediction
		if err := decoder.Decode(&p); err != nil {
			t.Fatal(err)
		}
		if p.Bytes == 0 {
			fullPredictions++
			if p.PrefixSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(source))) || p.Mode != "path" && p.Language != "Perl" {
				t.Fatal(p)
			}
		}
	}
	if fullPredictions != len(evaluate.Modes) {
		t.Fatalf("full-file prediction rows=%d", fullPredictions)
	}
}

func TestCorpusAliases(t *testing.T) {
	for name, source := range map[string]string{
		"Jinja2": "{% extends 'base.html' %}\n", "HTML+Jinja": "{% extends 'base.html' %}\n",
		"HTML+ERB": "<%= title %>\n", "HTML+PHP": "<?php\n", "Matlab": "function result = add(a,b)\n", "fish": "#!/usr/bin/fish\n",
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

func TestStrictAndFamilyScores(t *testing.T) {
	root := t.TempDir()
	for label, source := range map[string]string{
		"Shell":       "#!/bin/bash\necho hello\n",
		"fish":        "#!/usr/bin/fish\necho hello\n",
		"Unsupported": "plain text\n",
	} {
		dir := filepath.Join(root, label)
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "sample"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := evaluate.Run(root, func(req evaluate.Request) (evaluate.Response, error) {
		if bytes.Contains(req.Content, []byte("fish")) {
			return evaluate.Response{Language: "fish"}, nil
		}
		return evaluate.Response{Language: "Zsh"}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	row := report.Rows[3]
	if report.Files != 2 || report.Unsupported != 1 {
		t.Fatal(report.Files, report.Unsupported)
	}
	for _, counts := range []evaluate.Counts{row.Ours, row.Baseline} {
		if counts.Correct != 1 || counts.FamilyCorrect != 2 || counts.Wrong != 1 {
			t.Fatal(counts)
		}
	}
}

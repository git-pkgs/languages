package languages_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/git-pkgs/languages"
)

func TestTemplateDevelopment(t *testing.T) {
	files, err := filepath.Glob("testdata/templates/*/*")
	if err != nil || len(files) == 0 {
		t.Fatalf("template fixtures: %v", err)
	}
	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := languages.Parse(filepath.Base(filepath.Dir(path)))
			if want == languages.GoTemplate {
				if _, err := template.New(path).Parse(string(data)); err != nil {
					t.Fatal(err)
				}
			}
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			if got := a.Detect(filepath.Base(path)); got.Language != want {
				t.Fatalf("combined: got %+v, want %s", got, want)
			}
			if want == languages.GoTemplate && a.Result().Language != want {
				t.Fatalf("content: got %+v, want %s", a.Result(), want)
			}
			if want != languages.GoTemplate && a.Result().Language == languages.GoTemplate {
				t.Fatal("source mistaken for Go template")
			}
			assertTemplateReader(t, data, filepath.Base(path), a)
		})
	}
}

func assertTemplateReader(t *testing.T, data []byte, name string, want languages.Analysis) {
	t.Helper()
	for _, chunk := range []int{1, 7, 511, 32768} {
		var got languages.Analysis
		reader := chunkReader{bytes.NewReader(data), chunk}
		if err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{Filename: name}, &got); err != nil {
			t.Fatal(err)
		}
		if got.Result() != want.Result() || got.Detect(name) != want.Detect(name) || got.Signals != want.Signals || got.Count != want.Count {
			t.Fatalf("chunk=%d: got %+v signals=%+v, want %+v signals=%+v", chunk, got.Result(), got.Signals[:got.Count], want.Result(), want.Signals[:want.Count])
		}
	}
}

func TestTemplateActions(t *testing.T) {
	for _, test := range []struct {
		source string
		match  bool
	}{
		{`<p>{{.Name}}</p>`, true},
		{`{{- .Name -}}`, true},
		{"{{\n .Name\n}}", true},
		{`{{define "page"}}Hello{{end}}`, true},
		{"{{template `page` .}}", true},
		{`{{block "page" .}}Hello{{end}}`, true},
		{`{{if (eq .Name "Ada")}}Hello{{end}}`, true},
		{`{{printf "}}" .Name}}`, true},
		{`{{define "quo\"ted"}}Hello{{end}}`, true},
		{`{{/* {{.Fake}} */}}<p>{{.Real}}</p>`, true},
		{`{{/* {{.Fake}} */}}`, false},
		{`{{printf "{{.Name}}"}}`, false},
		{`{{printf "escaped \" {{.Name}}"}}`, false},
		{`{{.Name}`, false},
		{"# File types {{{\n.bat 38;5;36\n# }}}", false},
		{`ignored_dirs {{} {CVS .git .svn} 0}`, false},
		{"{phang}loop {{p_end}\n.field\n{end}}", false},
		{`{{title}}`, false},
		{`{{customer.name}}`, false},
		{`{{templateName "page"}}`, false},
		{`@go-template`, false},
		{`{{ . }}`, false},
		{`{{#if user}}Hello{{/if}}`, false},
		{`{% block body %}<p>{{ user.name }}</p>{% endblock %}`, false},
		{`<p><%= user.name %></p>`, false},
		{"#!/usr/bin/python3\nprint('{{.Name}}')\n", false},
		{"# -*- ruby -*-\nputs '{{.Name}}'\n", false},
	} {
		t.Run(test.source, func(t *testing.T) {
			for _, prefix := range []string{"", "<!DOCTYPE html>\n<html>\n" + strings.Repeat(" ", 32767)} {
				data := []byte(prefix + test.source)
				var a languages.Analysis
				languages.Analyze(data, true, &a)
				// A shebang only declares a language at the start of a file.
				if prefix != "" && strings.HasPrefix(test.source, "#!") {
					continue
				}
				if got := a.Result().Language == languages.GoTemplate; got != test.match {
					t.Fatalf("got %+v, template=%t", a.Result(), test.match)
				}
				assertTemplateReader(t, data, "", a)
			}
		})
	}
}

func TestTemplateReadBudget(t *testing.T) {
	const source = "<p>{{.Title}}</p>"
	end := strings.Index(source, "}}")
	for _, budget := range []int{end, end + 1, end + 2} {
		var a languages.Analysis
		if err := languages.AnalyzeReader(context.Background(), strings.NewReader(source), languages.ReadOptions{Bytes: int64(budget)}, &a); err != nil {
			t.Fatal(err)
		}
		if got := a.Result().Language == languages.GoTemplate; got != (budget == end+2) || !a.Prefix {
			t.Fatalf("budget=%d: %+v prefix=%t", budget, a.Result(), a.Prefix)
		}
	}
}

func TestTemplateLargeAction(t *testing.T) {
	source := "{{define \"" + strings.Repeat("page", 32768) + "\"}}<p>Hello</p>{{end}}"
	var a languages.Analysis
	languages.Analyze([]byte(source), true, &a)
	if a.Result().Language != languages.GoTemplate {
		t.Fatal(a.Result())
	}
	assertTemplateReader(t, []byte(source), "page.html", a)
	if got := a.Detect("wrong.py"); !got.Conflict || got.Language != languages.Unknown {
		t.Fatalf("misleading filename: %+v", got)
	}
}

func TestTemplateEncodedOffsets(t *testing.T) {
	const source = "<p>é {{.Name}}</p>"
	for _, width := range []int{2, 4} {
		data := encodeSource(source, width, binary.LittleEndian)
		var a languages.Analysis
		languages.Analyze(data, true, &a)
		if a.Result().Language != languages.GoTemplate || a.Count != 1 || a.Signals[0].Offset != uint64(6*width) {
			t.Fatalf("width=%d: result=%+v signals=%+v", width, a.Result(), a.Signals[:a.Count])
		}
		assertTemplateReader(t, data, "page.html", a)
	}
}

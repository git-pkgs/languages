package languages_test

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/languages"
)

func TestScanSubdirectory(t *testing.T) {
	files := fstest.MapFS{
		"api/main.go":        {Data: []byte("package main\nfunc main() {}\n")},
		"api/nested/util.go": {Data: []byte("package nested\n")},
		"api/link":           {Data: []byte("../../outside"), Mode: fs.ModeSymlink},
		"api/.git/config":    {Data: []byte("metadata")},
		"web/app.py":         {Data: []byte("print('hello')\n")},
	}
	sub, err := fs.Sub(files, "api")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := languages.Scan(context.Background(), sub, languages.ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	root := tree.Root()
	if root.Summary.Files != 2 || len(root.Summary.Languages) != 1 || root.Summary.Languages[0].Language != "Go" || root.Summary.Incomplete != 0 {
		t.Fatal(root)
	}
	if _, ok := tree.Subtree("nested"); !ok {
		t.Fatal("missing nested directory")
	}
	if _, ok := tree.Subtree("web"); ok {
		t.Fatal("scan escaped selected subtree")
	}
}

func TestScanMatchesReusableAnalysis(t *testing.T) {
	files := fstest.MapFS{
		"main.go":        {Data: []byte("package main\nfunc main() {}\n")},
		"src/app.ts":     {Data: []byte("export const count: number = 1;\n")},
		"src/rules.pl":   {Data: []byte(":- use_module(library(lists)).\n")},
		"api/spec.json":  {Data: []byte("{\"openapi\":\"3.1.0\"}\n")},
		"src/wrong.py":   {Data: []byte("#!/usr/bin/ruby\n")},
		"src/encoded.py": {Data: []byte("\xff\xfep\x00a\x00s\x00s\x00\n\x00")},
		"src/large.go":   {Data: []byte("package main\n" + strings.Repeat(" ", 2048))},
	}
	for _, limit := range []int64{1024, 65537, languages.DefaultBytes} {
		var want languages.Tree
		for name, file := range files {
			length := int64(len(file.Data))
			if limit > 0 {
				length = min(length, limit)
			}
			var analysis languages.Analysis
			languages.Analyze(file.Data[:length], length == int64(len(file.Data)), &analysis)
			if err := want.Add(name, int64(len(file.Data)), &analysis); err != nil {
				t.Fatal(err)
			}
		}
		got, err := languages.Scan(context.Background(), files, languages.ScanOptions{Bytes: limit})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Root(), want.Root()) {
			t.Fatalf("limit %d: got %+v, want %+v", limit, got.Root(), want.Root())
		}
	}
}

func TestScanLimitsAndFullSizes(t *testing.T) {
	const prefixSize = 1024
	const header = "package main\n"
	source := header + strings.Repeat(" ", prefixSize*3)
	files := fstest.MapFS{
		"large.go": {Data: []byte(source)},
		"exact.go": {Data: []byte(header + strings.Repeat(" ", prefixSize-len(header)))},
	}
	tree, err := languages.Scan(context.Background(), files, languages.ScanOptions{Bytes: prefixSize})
	if err != nil {
		t.Fatal(err)
	}
	root := tree.Root().Summary
	if root.Incomplete != 1 || root.Files != 2 || root.Bytes != int64(len(source)+prefixSize) || root.Languages[0].Bytes != root.Bytes {
		t.Fatal(root)
	}
	if _, err := languages.Scan(context.Background(), files, languages.ScanOptions{Bytes: -1}); err == nil {
		t.Fatal("accepted negative byte budget")
	}
}

func TestScanExclusions(t *testing.T) {
	files := fstest.MapFS{
		".git":        {Data: []byte("gitdir: elsewhere")},
		"vendor/a.go": {Data: []byte("package vendor\n")},
		"src/main.go": {Data: []byte("package main\n")},
		"src/skip.py": {Data: []byte("print('hello')\n")},
		"empty":       {Mode: fs.ModeDir},
	}
	all, err := languages.Scan(context.Background(), files, languages.ScanOptions{})
	if err != nil || all.Root().Summary.Files != 3 {
		t.Fatal(all, err)
	}
	filtered, err := languages.Scan(context.Background(), files, languages.ScanOptions{Exclude: func(name string, _ fs.DirEntry) bool {
		return name == "vendor" || name == "src/skip.py"
	}})
	if err != nil || filtered.Root().Summary.Files != 1 {
		t.Fatal(filtered, err)
	}
	if _, ok := filtered.Subtree("vendor"); ok {
		t.Fatal("excluded directory present")
	}
}

func TestScanFullFiles(t *testing.T) {
	header := strings.Repeat("# header\n", 16000)
	files := fstest.MapFS{
		"src/run":    {Data: []byte(header + "use strict;\nuse warnings;\n")},
		"src/editor": {Data: []byte(header + "# vim: ft=python\n")},
		"src/data":   {Data: []byte(header + "\x00")},
	}
	tree, err := languages.Scan(context.Background(), files, languages.ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	root := tree.Root().Summary
	if root.Files != 3 || root.Incomplete != 0 || root.Binary.Files != 1 || len(root.Languages) != 2 {
		t.Fatal(root)
	}
	for _, count := range root.Languages {
		if count.Files != 1 || count.Language != "Perl" && count.Language != "Python" {
			t.Fatal(count)
		}
	}
	prefix, err := languages.Scan(context.Background(), files, languages.ScanOptions{Bytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if root := prefix.Root().Summary; root.Incomplete != 3 || root.Unknown.Files != 3 || root.Binary.Files != 0 {
		t.Fatal(root)
	}
}

type deniedFileFS struct{ fs.FS }

func (f deniedFileFS) Open(name string) (fs.File, error) {
	if name == "denied.go" {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}

func TestScanErrorsAndCancellation(t *testing.T) {
	files := fstest.MapFS{"denied.go": {Data: []byte("package main\n")}}
	tree, err := languages.Scan(context.Background(), deniedFileFS{files}, languages.ScanOptions{})
	if !errors.Is(err, fs.ErrPermission) || tree != nil {
		t.Fatal(tree, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if tree, err := languages.Scan(ctx, files, languages.ScanOptions{}); !errors.Is(err, context.Canceled) || tree != nil {
		t.Fatal(tree, err)
	}
}

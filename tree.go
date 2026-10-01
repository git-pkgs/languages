package languages

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

type FileTotals struct {
	Files int64 `json:"files"`
	Bytes int64 `json:"bytes"`
}

type LanguageTotals struct {
	Language string `json:"language"`
	FileTotals
}

// Summary counts each file once, including files with no selected language.
type Summary struct {
	FileTotals
	Languages  []LanguageTotals `json:"languages"`
	Unknown    FileTotals       `json:"unknown"`
	Ambiguous  FileTotals       `json:"ambiguous"`
	Conflicts  FileTotals       `json:"conflicts"`
	Binary     FileTotals       `json:"binary"`
	Incomplete int64            `json:"incomplete"`
}

// Directory includes totals for all descendants. Path is relative to the tree root.
type Directory struct {
	Path     string      `json:"path"`
	Summary  Summary     `json:"summary"`
	Children []Directory `json:"children,omitempty"`
}

// Tree accumulates file analyses without retaining their content or analysis buffers.
// Its zero value is ready to use. Add must not run concurrently with other operations.
type Tree struct {
	dirs  map[string]*directoryTotals
	files map[string]struct{}
}

type directoryTotals struct {
	summary   Summary
	languages map[Language]FileTotals
	children  map[string]struct{}
}

// Add records one file using its full size and filename-contextual detection.
// Names must be unique, slash-separated relative file paths without dot components.
func (t *Tree) Add(name string, size int64, analysis *Analysis) error {
	if !fs.ValidPath(name) || name == "." {
		return fmt.Errorf("invalid file path %q", name)
	}
	if analysis == nil || size < 0 || analysis.Bytes > size {
		return fmt.Errorf("invalid size or analysis for %q", name)
	}
	if _, exists := t.files[name]; exists || t.dirs[name] != nil {
		return fmt.Errorf("path already added: %q", name)
	}
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if _, exists := t.files[parent]; exists {
			return fmt.Errorf("parent is a file: %q", parent)
		}
	}
	if t.files == nil {
		t.files = make(map[string]struct{})
		t.dirs = make(map[string]*directoryTotals)
	}
	t.files[name] = struct{}{}
	result := analysis.Detect(name)
	child := ""
	for parent := path.Dir(name); ; parent = path.Dir(parent) {
		dir := t.dirs[parent]
		if dir == nil {
			dir = &directoryTotals{languages: make(map[Language]FileTotals), children: make(map[string]struct{})}
			t.dirs[parent] = dir
		}
		dir.add(size, analysis, result)
		if child != "" {
			dir.children[child] = struct{}{}
		}
		if parent == "." {
			break
		}
		child = parent
	}
	return nil
}

func (d *directoryTotals) add(size int64, analysis *Analysis, result Result) {
	d.summary.Files++
	d.summary.Bytes += size
	if analysis.Prefix {
		d.summary.Incomplete++
	}
	var category *FileTotals
	switch {
	case analysis.Binary:
		category = &d.summary.Binary
	case result.Conflict:
		category = &d.summary.Conflicts
	case result.Language != Unknown:
		count := d.languages[result.Language]
		count.Files++
		count.Bytes += size
		d.languages[result.Language] = count
		return
	case result.Candidates.Empty():
		category = &d.summary.Unknown
	default:
		category = &d.summary.Ambiguous
	}
	category.Files++
	category.Bytes += size
}

// Root returns an independent snapshot, with children sorted by path.
func (t *Tree) Root() Directory {
	root, _ := t.Subtree(".")
	return root
}

// Subtree returns a snapshot of an existing directory, retaining root-relative paths.
// Language totals are ordered by bytes, then file count, then language name.
func (t *Tree) Subtree(name string) (Directory, bool) {
	if !fs.ValidPath(name) {
		return Directory{}, false
	}
	dir := t.dirs[name]
	if dir == nil {
		return Directory{Path: name, Summary: Summary{Languages: []LanguageTotals{}}}, name == "."
	}
	result := Directory{Path: name, Summary: dir.summary}
	result.Summary.Languages = make([]LanguageTotals, 0, len(dir.languages))
	for language, count := range dir.languages {
		result.Summary.Languages = append(result.Summary.Languages, LanguageTotals{language.String(), count})
	}
	sort.Slice(result.Summary.Languages, func(i, j int) bool {
		a, b := result.Summary.Languages[i], result.Summary.Languages[j]
		if a.Bytes != b.Bytes {
			return a.Bytes > b.Bytes
		}
		if a.Files != b.Files {
			return a.Files > b.Files
		}
		return strings.Compare(a.Language, b.Language) < 0
	})
	for child := range dir.children {
		snapshot, _ := t.Subtree(child)
		result.Children = append(result.Children, snapshot)
	}
	sort.Slice(result.Children, func(i, j int) bool { return result.Children[i].Path < result.Children[j].Path })
	return result, true
}

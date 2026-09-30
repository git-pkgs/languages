// Package localscan measures local repository scans and builds weakly labelled samples.
package localscan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/git-pkgs/languages"
)

const (
	perLanguage    = 30
	perRepository  = 3
	directoryBatch = 64
	maxDepth       = 32
	minimumSample  = 128
	heapInterval   = 1000
	directoryMode  = 0755
	sampleMode     = 0600
)

type Stats struct {
	Root              string                          `json:"root"`
	Excluded          string                          `json:"excluded_repository"`
	Repositories      int                             `json:"repositories"`
	Files             int                             `json:"files"`
	Bytes             int64                           `json:"bytes_examined"`
	Binary            int                             `json:"binary"`
	Unknown           int                             `json:"unknown"`
	Ambiguous         int                             `json:"ambiguous"`
	Selected          int                             `json:"selected"`
	ReadErrors        int                             `json:"read_errors"`
	ElapsedNS         int64                           `json:"elapsed_ns"`
	DetectionNS       int64                           `json:"detection_ns"`
	HeapBefore        uint64                          `json:"heap_before"`
	MaxSampledHeap    uint64                          `json:"max_sampled_heap"`
	HeapAfter         uint64                          `json:"heap_after"`
	Allocated         uint64                          `json:"allocated_bytes"`
	Samples           int                             `json:"samples"`
	SamplesByLanguage [languages.LanguageCount]int    `json:"samples_by_language"`
	LanguageNames     [languages.LanguageCount]string `json:"language_names"`
}

type candidate struct {
	path, repo string
	key        [sha256.Size]byte
}
type scanner struct {
	stats    Stats
	samples  [languages.LanguageCount][]candidate
	buffer   [languages.DefaultBytes]byte
	analysis languages.Analysis
}

// Run visits local snapshots only. It never fetches or modifies source repos.
func Run(root, exclude, out string) (Stats, error) {
	root = filepath.Clean(root)
	if exclude != "" {
		exclude = filepath.Clean(exclude)
	}
	s := scanner{stats: Stats{Root: root, Excluded: exclude}}
	for l := languages.Python; l < languages.LanguageCount; l++ {
		s.stats.LanguageNames[l] = l.String()
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	s.stats.HeapBefore = before.HeapAlloc
	s.stats.MaxSampledHeap = before.HeapAlloc
	start := time.Now()
	err := s.walk(root, "", 0)
	s.stats.ElapsedNS = time.Since(start).Nanoseconds()
	runtime.ReadMemStats(&after)
	s.stats.HeapAfter = after.HeapAlloc
	s.stats.Allocated = after.TotalAlloc - before.TotalAlloc
	if err != nil {
		return s.stats, err
	}
	if out != "" {
		err = s.export(out)
	}
	return s.stats, err
}

func (s *scanner) walk(dir, repo string, depth int) error {
	if dir == s.stats.Excluded {
		return nil
	}
	if depth > maxDepth {
		s.stats.ReadErrors++
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		repo = dir
		s.stats.Repositories++
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	for {
		entries, err := f.ReadDir(directoryBatch)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if skipped(entry.Name()) {
					continue
				}
				if err := s.walk(path, repo, depth+1); err != nil {
					s.stats.ReadErrors++
				}
			} else if repo != "" && entry.Type().IsRegular() {
				s.file(path, repo)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func skipped(name string) bool {
	switch name {
	case ".git", ".claude", ".codex", "node_modules", "vendor", ".venv", "venv", "__pycache__", ".bundle", "target", "build", "dist":
		return true
	}
	return false
}

func (s *scanner) file(path, repo string) {
	f, err := os.Open(path)
	if err != nil {
		s.stats.ReadErrors++
		return
	}
	n, readErr := io.ReadFull(f, s.buffer[:])
	closeErr := f.Close()
	if closeErr != nil || readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		s.stats.ReadErrors++
		return
	}
	start := time.Now()
	languages.Analyze(s.buffer[:n], n < len(s.buffer), &s.analysis)
	r := s.analysis.Result()
	s.stats.DetectionNS += time.Since(start).Nanoseconds()
	s.stats.Files++
	s.stats.Bytes += int64(n)
	switch {
	case s.analysis.Binary:
		s.stats.Binary++
	case r.Candidates == 0:
		s.stats.Unknown++
	case r.Language == languages.Unknown:
		s.stats.Ambiguous++
	default:
		s.stats.Selected++
	}
	if s.stats.Files%heapInterval == 0 {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		s.stats.MaxSampledHeap = max(s.stats.MaxSampledHeap, mem.HeapAlloc)
	}
	l := label(path)
	if l == languages.Unknown || n < minimumSample || s.analysis.Binary {
		return
	}
	if bytes.Contains(s.buffer[:n], []byte("Code generated")) || bytes.Contains(s.buffer[:n], []byte("DO NOT EDIT")) {
		return
	}
	rel, err := filepath.Rel(s.stats.Root, path)
	if err != nil {
		return
	}
	s.offer(l, candidate{path, repo, sha256.Sum256([]byte(filepath.ToSlash(rel)))})
}

func (s *scanner) offer(l languages.Language, c candidate) {
	list := s.samples[l]
	same, worstSame, worst := 0, -1, -1
	for i, item := range list {
		if worst < 0 || bytes.Compare(item.key[:], list[worst].key[:]) > 0 {
			worst = i
		}
		if item.repo == c.repo {
			same++
			if worstSame < 0 || bytes.Compare(item.key[:], list[worstSame].key[:]) > 0 {
				worstSame = i
			}
		}
	}
	if same >= perRepository {
		worst = worstSame
	} else if len(list) < perLanguage {
		s.samples[l] = append(list, c)
		return
	}
	if worst >= 0 && bytes.Compare(c.key[:], list[worst].key[:]) < 0 {
		list[worst] = c
	}
}

// These labels are independent extension conventions, not content ground truth.
func label(path string) languages.Language {
	switch filepath.Ext(path) {
	case ".py":
		return languages.Python
	case ".rb":
		return languages.Ruby
	case ".go":
		return languages.Go
	case ".rs":
		return languages.Rust
	case ".java":
		return languages.Java
	case ".c":
		return languages.C
	case ".cpp", ".cc", ".cxx":
		return languages.CPP
	case ".js", ".mjs", ".cjs":
		return languages.JavaScript
	case ".ts", ".mts", ".cts":
		return languages.TypeScript
	case ".jsx":
		return languages.JSX
	case ".tsx":
		return languages.TSX
	case ".bash":
		return languages.Bash
	case ".zsh":
		return languages.Zsh
	case ".fish":
		return languages.Fish
	case ".raku":
		return languages.Raku
	case ".prolog":
		return languages.Prolog
	case ".lisp":
		return languages.CommonLisp
	case ".scm":
		return languages.Scheme
	case ".clj":
		return languages.Clojure
	case ".rkt":
		return languages.Racket
	case ".php":
		return languages.PHP
	case ".lua":
		return languages.Lua
	case ".cs":
		return languages.CSharp
	case ".html":
		return languages.HTML
	case ".xml":
		return languages.XML
	case ".jinja", ".j2":
		return languages.Jinja
	case ".twig":
		return languages.Twig
	case ".erb":
		return languages.ERB
	case ".sql":
		return languages.SQL
	}
	return languages.Unknown
}

type provenance struct{ Source, Sample, Language, SHA256, LabelSource string }

func (s *scanner) export(out string) (resultErr error) {
	if err := os.Mkdir(out, directoryMode); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(out, "provenance.jsonl"))
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); resultErr == nil {
			resultErr = err
		}
	}()
	encoder := json.NewEncoder(f)
	var buffer [languages.MaxBytes + 1]byte
	seen := make(map[[sha256.Size]byte]bool)
	for l := languages.Python; l < languages.LanguageCount; l++ {
		slices.SortFunc(s.samples[l], func(a, b candidate) int { return bytes.Compare(a.key[:], b.key[:]) })
		for _, c := range s.samples[l] {
			data, err := readSample(c.path, buffer[:])
			if err != nil {
				return err
			}
			if len(data) > languages.MaxBytes {
				continue
			}
			hash := sha256.Sum256(data)
			if seen[hash] {
				continue
			}
			seen[hash] = true
			rel := filepath.Join(l.String(), hex.EncodeToString(c.key[:8]), filepath.Base(c.path))
			path := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
				return err
			}
			if err := os.WriteFile(path, data, sampleMode); err != nil {
				return err
			}
			if err := encoder.Encode(provenance{c.path, rel, l.String(), hex.EncodeToString(hash[:]), "extension (weak label)"}); err != nil {
				return err
			}
			s.stats.Samples++
			s.stats.SamplesByLanguage[l]++
		}
	}
	return nil
}

func readSample(path string, buf []byte) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	n, readErr := io.ReadFull(f, buf)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return nil, readErr
	}
	return buf[:n], closeErr
}

func Validate(root, out string) error {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return fmt.Errorf("root must be absolute")
	}
	if out != "" {
		out = filepath.Clean(out)
		rootPrefix := strings.TrimSuffix(root, string(filepath.Separator)) + string(filepath.Separator)
		if !filepath.IsAbs(out) || out == root || strings.HasPrefix(out, rootPrefix) {
			return fmt.Errorf("output must be an absolute path outside the scan root")
		}
	}
	return nil
}

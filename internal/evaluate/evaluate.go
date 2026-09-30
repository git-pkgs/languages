package evaluate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/languages"
)

var Sizes = [...]int{128, 256, 512, 1024, 4096, 0}
var Modes = [...]string{"content", "path", "combined"}

type Request struct {
	Mode    string `json:"mode"`
	Name    string `json:"name"`
	Content []byte `json:"content"`
}

type Response struct {
	Language string `json:"language"`
}
type Baseline func(Request) (Response, error)

type Counts struct {
	Total          int `json:"total"`
	Correct        int `json:"correct"`
	Wrong          int `json:"wrong"`
	Ambiguous      int `json:"ambiguous"`
	Unknown        int `json:"unknown"`
	CandidateHits  int `json:"candidate_hits"`
	High           int `json:"high"`
	HighCorrect    int `json:"high_correct"`
	CandidateTotal int `json:"candidate_total"`
}

func (c *Counts) Add(expected languages.Language, result languages.Result) {
	c.Total++
	c.CandidateTotal += result.Candidates.Len()
	if result.Candidates.Has(expected) {
		c.CandidateHits++
	}
	switch {
	case result.Candidates == 0:
		c.Unknown++
	case result.Language == languages.Unknown:
		c.Ambiguous++
	case result.Language == expected:
		c.Correct++
	default:
		c.Wrong++
	}
	if result.Confidence.Rank() >= languages.High.Rank() {
		c.High++
		if result.Language == expected {
			c.HighCorrect++
		}
	}
}

type Row struct {
	Mode                  string                          `json:"mode"`
	Bytes                 int                             `json:"bytes"`
	Ours                  Counts                          `json:"languages"`
	Baseline              Counts                          `json:"baseline"`
	ByLanguage            [languages.LanguageCount]Counts `json:"by_language"`
	Extensionless         Counts                          `json:"extensionless"`
	ExtensionlessBaseline Counts                          `json:"extensionless_baseline"`
}

type Report struct {
	Source      string  `json:"source"`
	Revision    string  `json:"revision,omitempty"`
	Files       int     `json:"files"`
	Unsupported int     `json:"unsupported_files"`
	Oversized   int     `json:"oversized_complete_skipped"`
	Rows        [18]Row `json:"rows"`
}

type Prediction struct {
	Path         string               `json:"path"`
	PrefixSHA256 string               `json:"prefix_sha256"`
	Expected     string               `json:"expected"`
	Mode         string               `json:"mode"`
	Bytes        int                  `json:"bytes"`
	Language     string               `json:"language"`
	Candidates   languages.Set        `json:"candidates"`
	Confidence   languages.Confidence `json:"confidence"`
	Baseline     string               `json:"baseline,omitempty"`
}

// Run reads a Linguist samples directory without copying its files into the repo.
// Complete-file rows omit files larger than MaxBytes; prefix rows include them.
func Run(root string, baseline Baseline, predictions io.Writer) (Report, error) {
	e := evaluator{report: Report{Source: root}, baseline: baseline}
	for m, mode := range Modes {
		for s, size := range Sizes {
			e.report.Rows[m*len(Sizes)+s] = Row{Mode: mode, Bytes: size}
		}
	}
	if predictions != nil {
		e.encoder = json.NewEncoder(predictions)
	}
	err := walk(root, 0, e.file)
	if err == nil && e.report.Files == 0 {
		err = errors.New("no supported language samples found")
	}
	return e.report, err
}

type evaluator struct {
	report   Report
	baseline Baseline
	encoder  *json.Encoder
	buf      [languages.MaxBytes + 1]byte
}

func (e *evaluator) file(path string) error {
	rel, err := filepath.Rel(e.report.Source, path)
	if err != nil {
		return err
	}
	label, _, found := strings.Cut(filepath.ToSlash(rel), "/")
	if !found {
		return nil
	}
	expected := languages.Parse(label)
	if expected == languages.Unknown {
		e.report.Unsupported++
		return nil
	}
	n, err := read(path, e.buf[:])
	if err != nil {
		return err
	}
	e.report.Files++
	if n > languages.MaxBytes {
		e.report.Oversized++
	}
	for s, size := range Sizes {
		if size == 0 && n > languages.MaxBytes {
			continue
		}
		length := n
		if size > 0 {
			length = min(n, size)
		}
		if err := e.sample(rel, expected, s, e.buf[:length], length == n); err != nil {
			return err
		}
	}
	return nil
}

func read(path string, buf []byte) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	n, readErr := io.ReadFull(f, buf)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return n, readErr
	}
	return n, closeErr
}

func (e *evaluator) sample(path string, expected languages.Language, sizeIndex int, content []byte, complete bool) error {
	var a languages.Analysis
	languages.Analyze(content, complete, &a)
	c := languages.AnalyzePath(filepath.Base(path))
	results := [3]languages.Result{a.Result(), c.Result(), languages.Combine(&a, c)}
	for m, mode := range Modes {
		row := &e.report.Rows[m*len(Sizes)+sizeIndex]
		r := results[m]
		row.Ours.Add(expected, r)
		row.ByLanguage[expected].Add(expected, r)
		if filepath.Ext(path) == "" {
			row.Extensionless.Add(expected, r)
		}
		response, err := e.compare(mode, path, content)
		if err != nil {
			return err
		}
		if e.baseline != nil {
			row.Baseline.AddBaseline(expected, response.Language)
			if filepath.Ext(path) == "" {
				row.ExtensionlessBaseline.AddBaseline(expected, response.Language)
			}
		}
		if e.encoder != nil {
			hash := sha256.Sum256(content)
			if err := e.encoder.Encode(Prediction{path, hex.EncodeToString(hash[:]), expected.String(), mode, Sizes[sizeIndex], r.Language.String(), r.Candidates, r.Confidence, response.Language}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *evaluator) compare(mode, path string, content []byte) (Response, error) {
	if e.baseline == nil {
		return Response{}, nil
	}
	req := Request{Mode: mode}
	if mode != "path" {
		req.Content = content
	}
	if mode != "content" {
		req.Name = filepath.Base(path)
	}
	return e.baseline(req)
}

func (c *Counts) AddBaseline(expected languages.Language, name string) {
	l := languages.Parse(name)
	r := languages.Result{Language: l}
	if l != languages.Unknown {
		r.Candidates = 1 << l
	}
	c.Add(expected, r)
	if l == languages.Unknown && name != "" {
		c.Unknown--
		c.Wrong++
	}
}

const (
	maxDepth       = 32
	directoryBatch = 64
)

func walk(root string, depth int, visit func(string) error) error {
	if depth > maxDepth {
		return fmt.Errorf("corpus directory depth exceeds 32: %s", root)
	}
	f, err := os.Open(root)
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
			path := filepath.Join(root, entry.Name())
			if entry.IsDir() {
				if err := walk(path, depth+1, visit); err != nil {
					return err
				}
			} else if entry.Type().IsRegular() {
				if err := visit(path); err != nil {
					return err
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

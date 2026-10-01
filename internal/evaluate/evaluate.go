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

const maxPrefixBytes = 64 * 1024

var Sizes = [...]int{128, 256, 512, 1024, 4096, 16384, maxPrefixBytes, 0}

const pathMode = "path"

var Modes = [...]string{"content", pathMode, "combined"}

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
	FamilyCorrect  int `json:"family_correct"`
	Wrong          int `json:"wrong"`
	Ambiguous      int `json:"ambiguous"`
	Unknown        int `json:"unknown"`
	CandidateHits  int `json:"candidate_hits"`
	High           int `json:"high"`
	HighCorrect    int `json:"high_correct"`
	CandidateTotal int `json:"candidate_total"`
	Conflicts      int `json:"conflicts"`
}

func (c *Counts) Add(expected languages.Language, result languages.Result) {
	c.Total++
	if result.Conflict {
		c.Conflicts++
	}
	c.CandidateTotal += result.Candidates.Len()
	if result.Candidates.Has(expected) {
		c.CandidateHits++
	}
	if result.Language != languages.Unknown && family(result.Language) == family(expected) {
		c.FamilyCorrect++
	}
	switch {
	case result.Candidates.Empty():
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

func family(l languages.Language) languages.Language {
	if l == languages.Bash || l == languages.Zsh {
		return languages.Shell
	}
	return l
}

type Row struct {
	Mode                  string                          `json:"mode"`
	Bytes                 int                             `json:"bytes"`
	Ours                  Counts                          `json:"languages"`
	Baseline              Counts                          `json:"baseline"`
	ByLanguage            [languages.LanguageCount]Counts `json:"by_language"`
	Extensionless         Counts                          `json:"extensionless"`
	ExtensionlessBaseline Counts                          `json:"extensionless_baseline"`
	Unsupported           UnsupportedCounts               `json:"unsupported"`
}

// UnsupportedCounts measures selections outside the supported label set.
// A selection can be a related language or embedded source, so it needs review.
type UnsupportedCounts struct {
	Total        int `json:"total"`
	Selected     int `json:"selected"`
	Ambiguous    int `json:"ambiguous"`
	Unknown      int `json:"unknown"`
	Conflicts    int `json:"conflicts"`
	HighSelected int `json:"high_selected"`
}

func (c *UnsupportedCounts) Add(result languages.Result) {
	c.Total++
	switch {
	case result.Language != languages.Unknown:
		c.Selected++
		if result.Confidence == languages.High {
			c.HighSelected++
		}
	case !result.Candidates.Empty():
		c.Ambiguous++
	default:
		c.Unknown++
	}
	if result.Conflict {
		c.Conflicts++
	}
}

type Report struct {
	Source      string                       `json:"source"`
	Revision    string                       `json:"revision,omitempty"`
	Files       int                          `json:"files"`
	Unsupported int                          `json:"unsupported_files"`
	Rows        [len(Modes) * len(Sizes)]Row `json:"rows"`
}

type Prediction struct {
	Path         string               `json:"path"`
	PrefixSHA256 string               `json:"prefix_sha256"`
	Expected     string               `json:"expected"`
	Supported    bool                 `json:"supported"`
	Mode         string               `json:"mode"`
	Bytes        int                  `json:"bytes"`
	Language     string               `json:"language"`
	Candidates   languages.Set        `json:"candidates"`
	Confidence   languages.Confidence `json:"confidence"`
	Conflict     bool                 `json:"conflict"`
	Binary       bool                 `json:"binary"`
	Statistical  bool                 `json:"statistical"`
	Evidence     []languages.Evidence `json:"evidence,omitempty"`
	Baseline     string               `json:"baseline,omitempty"`
}

// Run reads a Linguist samples directory without copying its files into the repo.
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
	if err == nil && e.report.Files+e.report.Unsupported == 0 {
		err = errors.New("no language samples found")
	}
	return e.report, err
}

type evaluator struct {
	report   Report
	baseline Baseline
	encoder  *json.Encoder
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
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if expected == languages.Unknown {
		e.report.Unsupported++
	} else {
		e.report.Files++
	}
	for s, size := range Sizes {
		length := len(content)
		if size > 0 {
			length = min(length, size)
		}
		if err := e.sample(rel, label, s, content[:length], length == len(content)); err != nil {
			return err
		}
	}
	return nil
}

func (e *evaluator) sample(path, label string, sizeIndex int, content []byte, complete bool) error {
	expected := languages.Parse(label)
	var a languages.Analysis
	languages.Analyze(content, complete, &a)
	c := languages.AnalyzePath(filepath.Base(path))
	results := [3]languages.Result{a.Result(), c.Result(), a.Detect(filepath.Base(path))}
	for m, mode := range Modes {
		row := &e.report.Rows[m*len(Sizes)+sizeIndex]
		r := results[m]
		if expected != languages.Unknown {
			row.Ours.Add(expected, r)
			row.ByLanguage[expected].Add(expected, r)
			if filepath.Ext(path) == "" {
				row.Extensionless.Add(expected, r)
			}
		} else {
			row.Unsupported.Add(r)
		}
		response, err := e.compare(expected, mode, path, content)
		if err != nil {
			return err
		}
		if e.baseline != nil && expected != languages.Unknown {
			row.Baseline.AddBaseline(expected, response.Language)
			if filepath.Ext(path) == "" {
				row.ExtensionlessBaseline.AddBaseline(expected, response.Language)
			}
		}
		if e.encoder != nil {
			prediction := predictionFor(path, label, mode, Sizes[sizeIndex], content, &a, r)
			prediction.Baseline = response.Language
			if err := e.encoder.Encode(prediction); err != nil {
				return err
			}
		}
	}
	return nil
}

func predictionFor(path, label, mode string, size int, content []byte, a *languages.Analysis, r languages.Result) Prediction {
	hash := sha256.Sum256(content)
	expected := languages.Parse(label)
	if expected != languages.Unknown {
		label = expected.String()
	}
	p := Prediction{
		Path: path, PrefixSHA256: hex.EncodeToString(hash[:]), Expected: label,
		Supported: expected != languages.Unknown, Mode: mode, Bytes: size,
		Language: r.Language.String(), Candidates: r.Candidates, Confidence: r.Confidence,
		Conflict: r.Conflict, Binary: a.Binary, Statistical: r.Statistical,
	}
	if mode != pathMode {
		for _, match := range a.Signals[:a.Count] {
			p.Evidence = append(p.Evidence, match.Evidence())
		}
	}
	return p
}

func (e *evaluator) compare(expected languages.Language, mode, path string, content []byte) (Response, error) {
	if e.baseline == nil || expected == languages.Unknown {
		return Response{}, nil
	}
	req := Request{Mode: mode}
	if mode != pathMode {
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
		r.Candidates = languages.NewSet(l)
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

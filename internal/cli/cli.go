package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/git-pkgs/languages"
)

const (
	pathMode     = "path"
	combinedMode = "combined"
)

type options struct {
	mode, name, source string
	limit              int64
	depth              int
	prefix, json       bool
}

type Output struct {
	Language    string               `json:"language,omitempty"`
	Confidence  languages.Confidence `json:"confidence"`
	Candidates  []string             `json:"candidates,omitempty"`
	Conflict    bool                 `json:"conflict,omitempty"`
	Statistical bool                 `json:"statistical,omitempty"`
	Bytes       int64                `json:"bytes_examined"`
	Prefix      bool                 `json:"prefix"`
	Binary      bool                 `json:"binary,omitempty"`
	Content     []Evidence           `json:"content_evidence,omitempty"`
	Path        *PathEvidence        `json:"path_evidence,omitempty"`
}

type Evidence struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Languages   []string `json:"languages"`
	Offset      uint64   `json:"offset"`
}

type PathEvidence struct {
	Reason     string   `json:"reason"`
	Candidates []string `json:"candidates"`
}

func Names(s languages.Set) []string {
	var names []string
	for l := languages.Python; l < languages.LanguageCount; l++ {
		if s.Has(l) {
			names = append(names, l.String())
		}
	}
	return names
}

func Format(a *languages.Analysis, c languages.Context, result languages.Result) Output {
	o := Output{Language: result.Language.String(), Confidence: result.Confidence, Candidates: Names(result.Candidates), Conflict: result.Conflict, Statistical: result.Statistical, Bytes: a.Bytes, Prefix: a.Prefix, Binary: a.Binary}
	for _, m := range a.Signals[:a.Count] {
		e := m.Evidence()
		o.Content = append(o.Content, Evidence{e.ID, e.Description, Names(e.Languages), e.Offset})
	}
	if !c.Candidates.Empty() {
		o.Path = &PathEvidence{c.Reason, Names(c.Candidates)}
	}
	return o
}

// Run combines content with the source path or contextual filename by default.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	if info, err := os.Stat(opts.source); err == nil && info.IsDir() {
		if opts.name != "" || opts.prefix || opts.mode != combinedMode {
			return errors.New("directory scans require combined mode without -name or -prefix")
		}
		return runDirectory(opts, stdout)
	}
	return runFile(opts, stdin, stdout)
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("languages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.mode, "mode", combinedMode, "content, path, or combined")
	flags.StringVar(&opts.name, "name", "", "optional contextual filename")
	flags.Int64Var(&opts.limit, "bytes", languages.DefaultBytes, "maximum bytes to read per file; 0 reads the full file")
	flags.BoolVar(&opts.prefix, "prefix", false, "input is already truncated; EOF does not establish object completeness")
	flags.BoolVar(&opts.json, "json", false, "write a directory tree as JSON")
	flags.IntVar(&opts.depth, "depth", -1, "directory display depth; 0 shows the root, -1 shows all")
	if err := flags.Parse(args); err != nil {
		return opts, err
	}
	if opts.mode != "content" && opts.mode != pathMode && opts.mode != combinedMode {
		return opts, errors.New("mode must be content, path, or combined")
	}
	if opts.limit < 0 {
		return opts, errors.New("bytes must be zero or greater")
	}
	if opts.depth < -1 {
		return opts, errors.New("depth must be -1 or greater")
	}
	if flags.NArg() > 1 {
		return opts, errors.New("expected at most one file or directory, or - for stdin")
	}
	opts.source = flags.Arg(0)
	return opts, nil
}

func runFile(opts options, stdin io.Reader, stdout io.Writer) error {
	if opts.name == "" && opts.source != "-" {
		opts.name = opts.source
	}
	var a languages.Analysis
	var c languages.Context
	if opts.mode != "content" {
		c = languages.AnalyzePath(opts.name)
	}
	if opts.mode == pathMode {
		if opts.name == "" {
			return errors.New("path mode requires -name or a path argument")
		}
	} else {
		reader := stdin
		if opts.source != "" && opts.source != "-" {
			f, err := os.Open(opts.source)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			reader = f
		}
		err := languages.AnalyzeReader(context.Background(), reader, languages.ReadOptions{Bytes: opts.limit, Prefix: opts.prefix}, &a)
		if err != nil {
			return err
		}
	}
	var r languages.Result
	switch opts.mode {
	case pathMode:
		r = c.Result()
	case combinedMode:
		r = a.Detect(opts.name)
	default:
		r = a.Result()
	}
	return json.NewEncoder(stdout).Encode(Format(&a, c, r))
}

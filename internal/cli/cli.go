package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/git-pkgs/languages"
)

const pathMode = "path"

type Output struct {
	Language   string               `json:"language,omitempty"`
	Confidence languages.Confidence `json:"confidence"`
	Candidates []string             `json:"candidates,omitempty"`
	Conflict   bool                 `json:"conflict,omitempty"`
	Bytes      int                  `json:"bytes_examined"`
	Prefix     bool                 `json:"prefix"`
	Binary     bool                 `json:"binary,omitempty"`
	Content    []Evidence           `json:"content_evidence,omitempty"`
	Path       *PathEvidence        `json:"path_evidence,omitempty"`
}

type Evidence struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Languages   []string `json:"languages"`
	Offset      uint32   `json:"offset"`
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
	o := Output{Language: result.Language.String(), Confidence: result.Confidence, Candidates: Names(result.Candidates), Conflict: result.Conflict, Bytes: a.Bytes, Prefix: a.Prefix, Binary: a.Binary}
	for _, m := range a.Signals[:a.Count] {
		e := m.Evidence()
		o.Content = append(o.Content, Evidence{e.ID, e.Description, Names(e.Languages), e.Offset})
	}
	if c.Candidates != 0 {
		o.Path = &PathEvidence{c.Reason, Names(c.Candidates)}
	}
	return o
}

// Run combines content with the source path or contextual filename by default.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("languages", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mode := flags.String("mode", "combined", "content, path, or combined")
	name := flags.String("name", "", "optional contextual filename")
	limit := flags.Int("bytes", languages.DefaultBytes, "maximum bytes to read (1..65536)")
	prefix := flags.Bool("prefix", false, "input is already truncated; EOF does not establish object completeness")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *mode != "content" && *mode != pathMode && *mode != "combined" {
		return errors.New("mode must be content, path, or combined")
	}
	if *limit < 1 || *limit > languages.MaxBytes {
		return fmt.Errorf("bytes must be between 1 and %d", languages.MaxBytes)
	}
	if flags.NArg() > 1 {
		return errors.New("expected at most one input file, or - for stdin")
	}
	source := flags.Arg(0)
	if *name == "" && source != "-" {
		*name = source
	}
	var a languages.Analysis
	var c languages.Context
	if *mode != "content" {
		c = languages.AnalyzePath(*name)
	}
	if *mode == pathMode {
		if *name == "" {
			return errors.New("path mode requires -name or a path argument")
		}
	} else {
		reader := stdin
		if source != "" && source != "-" {
			f, err := os.Open(source)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			reader = f
		}
		buf := make([]byte, *limit)
		n, err := io.ReadFull(reader, buf)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return err
		}
		languages.Analyze(buf[:n], n < *limit && !*prefix, &a)
	}
	var r languages.Result
	switch *mode {
	case pathMode:
		r = c.Result()
	case "combined":
		r = a.Detect(*name)
	default:
		r = a.Result()
	}
	return json.NewEncoder(stdout).Encode(Format(&a, c, r))
}

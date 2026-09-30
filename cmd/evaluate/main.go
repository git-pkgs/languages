package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/git-pkgs/languages/internal/evaluate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	root := flag.String("corpus", "testdata/corpus", "Linguist samples directory")
	baseline := flag.String("baseline", "", "optional JSONL baseline executable")
	predictions := flag.String("predictions", "", "optional per-sample JSONL output")
	revision := flag.String("revision", "", "corpus commit or provenance label")
	flag.Parse()
	var compare evaluate.Baseline
	var cmd *exec.Cmd
	var input io.WriteCloser
	if *baseline != "" {
		cmd = exec.Command(*baseline)
		var err error
		input, err = cmd.StdinPipe()
		if err != nil {
			return err
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
		encoder := json.NewEncoder(input)
		decoder := json.NewDecoder(output)
		compare = func(req evaluate.Request) (evaluate.Response, error) {
			var resp evaluate.Response
			if err := encoder.Encode(req); err != nil {
				return resp, err
			}
			err := decoder.Decode(&resp)
			return resp, err
		}
	}
	var output io.Writer
	if *predictions != "" {
		f, err := os.Create(*predictions)
		if err != nil {
			return err
		}
		defer func() {
			if err := f.Close(); runErr == nil {
				runErr = err
			}
		}()
		output = f
	}
	report, err := evaluate.Run(*root, compare, output)
	if err != nil {
		return err
	}
	report.Revision = *revision
	if input != nil {
		if err := input.Close(); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/git-pkgs/languages/internal/localscan"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", "", "absolute directory containing repositories")
	exclude := flag.String("exclude", "", "repository to exclude from the scan")
	out := flag.String("out", "", "new directory outside root for sampled files")
	flag.Parse()
	if err := localscan.Validate(*root, *out); err != nil {
		return err
	}
	stats, err := localscan.Run(*root, *exclude, *out)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(stats)
}

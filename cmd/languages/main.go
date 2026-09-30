package main

import (
	"fmt"
	"github.com/git-pkgs/languages/internal/cli"
	"os"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/git-pkgs/languages/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		message := strconv.QuoteToGraphic(err.Error())
		fmt.Fprintln(os.Stderr, message[1:len(message)-1])
		os.Exit(1)
	}
}

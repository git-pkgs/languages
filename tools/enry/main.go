// enry is an optional evaluation adapter, isolated from the library's module.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/go-enry/go-enry/v2"
	"github.com/go-enry/go-enry/v2/data"
)

type request struct {
	Mode    string `json:"mode"`
	Name    string `json:"name"`
	Content []byte `json:"content"`
}
type response struct {
	Language string `json:"language"`
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input io.Reader, output io.Writer) error {
	all := make([]string, 0, len(data.LanguagesLogProbabilities))
	for language := range data.LanguagesLogProbabilities {
		all = append(all, language)
	}
	sort.Strings(all)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 128*1024), 256*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			return err
		}
		var language string
		if req.Mode == "content" {
			if !enry.IsBinary(req.Content) {
				language = enry.GetLanguage("", req.Content)
				if language == "" {
					language, _ = enry.GetLanguageByClassifier(req.Content, all)
				}
			}
		} else {
			language = enry.GetLanguage(req.Name, req.Content)
		}
		if err := encoder.Encode(response{language}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

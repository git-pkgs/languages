package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/git-pkgs/languages"
)

func runDirectory(opts options, output io.Writer) error {
	root, err := os.OpenRoot(opts.source)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	tree, err := languages.Scan(context.Background(), root.FS(), languages.ScanOptions{Bytes: opts.limit})
	if err != nil {
		return err
	}
	directory := tree.Root()
	limitDepth(&directory, opts.depth)
	if opts.json {
		return json.NewEncoder(output).Encode(directory)
	}
	label := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(opts.source)), "/") + "/"
	if _, err := fmt.Fprintf(output, "%s  %s\n", escapedPath(label), summaryText(directory.Summary)); err != nil {
		return err
	}
	return writeChildren(output, directory.Children, "")
}

func limitDepth(directory *languages.Directory, depth int) {
	if depth < 0 {
		return
	}
	if depth == 0 {
		directory.Children = nil
		return
	}
	for i := range directory.Children {
		limitDepth(&directory.Children[i], depth-1)
	}
}

func writeChildren(output io.Writer, children []languages.Directory, indent string) error {
	for i, child := range children {
		branch, next := "├── ", "│   "
		if i == len(children)-1 {
			branch, next = "└── ", "    "
		}
		if _, err := fmt.Fprintf(output, "%s%s%s/  %s\n", indent, branch, escapedPath(path.Base(child.Path)), summaryText(child.Summary)); err != nil {
			return err
		}
		if err := writeChildren(output, child.Children, indent+next); err != nil {
			return err
		}
	}
	return nil
}

func escapedPath(name string) string {
	quoted := strconv.QuoteToGraphic(name)
	return quoted[1 : len(quoted)-1]
}

func summaryText(summary languages.Summary) string {
	const percentage = 100
	var parts []string
	add := func(name string, count languages.FileTotals) {
		if count.Files == 0 {
			return
		}
		if summary.Bytes == 0 {
			parts = append(parts, name)
			return
		}
		parts = append(parts, fmt.Sprintf("%s %.1f%%", name, percentage*float64(count.Bytes)/float64(summary.Bytes)))
	}
	for _, language := range summary.Languages {
		add(language.Language, language.FileTotals)
	}
	add("unknown", summary.Unknown)
	add("ambiguous", summary.Ambiguous)
	add("conflicting", summary.Conflicts)
	add("binary", summary.Binary)
	if len(parts) == 0 {
		parts = append(parts, "empty")
	}
	unit := "files"
	if summary.Files == 1 {
		unit = "file"
	}
	text := fmt.Sprintf("%s (%d %s, %s", strings.Join(parts, ", "), summary.Files, unit, humanBytes(summary.Bytes))
	if summary.Incomplete > 0 {
		text += fmt.Sprintf(", %d partial", summary.Incomplete)
	}
	return text + ")"
}

func humanBytes(size int64) string {
	const kibibyte = 1024
	if size < kibibyte {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"} {
		value /= kibibyte
		if value < kibibyte {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d B", size)
}

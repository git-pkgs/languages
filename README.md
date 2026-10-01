# languages

Detect programming languages from a filename, source content, or both. The Go
library runs offline and supports WebAssembly. Results include candidate
languages, confidence, and conflicting evidence.

## Install

Add the module to your Go project:

```bash
go get github.com/git-pkgs/languages
```

## Detect a language

Pass a filename and source bytes to `Detect`:

```go
package main

import (
    "fmt"

    "github.com/git-pkgs/languages"
)

func main() {
    source := []byte("const count = 1;\n")
    result := languages.Detect("app.ts", source)
    fmt.Println(result.Language, result.Confidence)
    // Output: TypeScript low
}
```

Use an empty filename for content-only detection, or `nil` content to use the
filename alone:

```go
source := []byte("use strict;\nuse warnings;\n")
fmt.Println(languages.Detect("", source).Language)      // Perl
fmt.Println(languages.Detect("main.go", nil).Language) // Go
```

`Detect` performs no I/O and examines all supplied bytes. You can pass a prefix
when the rest of the file is unavailable. Only the final path component is used;
conventional filenames are case-sensitive and extensions are case-insensitive.
The exceptions are `.C`, which selects C++, and `.H`, which has C-family candidates.

For a file, use `AnalyzeReader` to read to EOF with bounded memory. Add `context`
and `os` to your imports:

```go
func detectFile(ctx context.Context, name string) (languages.Result, error) {
    file, err := os.Open(name)
    if err != nil {
        return languages.Result{}, err
    }
    defer file.Close()

    var content languages.Analysis
    if err := languages.AnalyzeReader(ctx, file, languages.ReadOptions{}, &content); err != nil {
        return languages.Result{}, err
    }
    return content.Detect(name), nil
}
```

Set `ReadOptions{Bytes: 1024}` to read at most 1 KiB; the default reads to EOF.
The reader never consumes an extra byte to check for EOF, so reaching a budget
without EOF leaves `content.Prefix` true. Use `ReadOptions{Prefix: true}` if the
reader contains truncated input.

Read errors and cancellation clear the result. Cancellation is checked between
reads and cannot interrupt a reader blocked inside `Read`.

## Handle the result

`Language` is `languages.Unknown` when no single language is selected. Check
`Candidates` to distinguish an ambiguous result from one with no selection.
Import `encoding/json` to display candidate names:

```go
result := languages.Detect("source.pl", nil)
switch {
case result.Conflict:
    fmt.Println("conflicting language evidence")
case result.Candidates.Empty():
    fmt.Println("no language detected")
case result.Language == languages.Unknown:
    names, err := json.Marshal(result.Candidates)
    if err != nil {
        panic(err)
    }
    fmt.Printf("ambiguous: %s\n", names)
default:
    fmt.Println(result.Language)
}
```

The example prints `ambiguous: ["Perl","Raku","Prolog"]`. Content such as
`use strict;` can resolve the `.pl` ambiguity to Perl. Check a candidate with
`result.Candidates.Has(languages.Perl)`, or count them with
`result.Candidates.Len()`.

`Language.String()` returns the name. `languages.Parse(name)` accepts canonical
names and aliases without regard to case, including `common-lisp`.

`Conflict` indicates disagreement between strong content evidence and the
filename, or between a declaration and strong syntax evidence. For example, a
Ruby shebang in `script.py` produces a conflict. Shared syntax, such as JavaScript
that is also valid TypeScript, can be narrowed by the filename.

Confidence describes the evidence supporting the result:

- `none`: no content or filename selection.
- `low`: a filename, weak syntax, statistical selection, or conflicting evidence.
- `medium`: stronger syntax evidence.
- `high`: a shebang or editor modeline, or multiple rules including a strong signal.

For a confidence threshold, compare
`result.Confidence.Rank() >= languages.Medium.Rank()`. High confidence can
have several candidates, so check `Language` as well when you need a single
selection. Confidence is not a probability. `Statistical` is true when the
selection comes from the token classifier; these results have low confidence.

## Reuse an analysis and inspect evidence

Use `Analyze` when you have the complete content, need matched rules, or want to
detect the same content under several filenames:

```go
source := []byte("use strict;\nuse warnings;\n")
var content languages.Analysis
languages.Analyze(source, true, &content)

fmt.Println(content.Result().Language)            // Perl
fmt.Println(content.Detect("source.pl").Language) // Perl
fmt.Println(content.Detect("source.py").Conflict) // true

for _, match := range content.Signals[:content.Count] {
    evidence := match.Evidence()
    fmt.Println(evidence.ID, evidence.Description, evidence.Offset)
}
```

Pass `true` only when the supplied bytes are the complete file. Use `false` for
a prefix with an unavailable suffix. `Detect` always treats content as a prefix.
`content.Prefix` records whether the input is incomplete, and `content.Bytes`
records the number of original bytes examined.

Emacs and Vim modelines are checked in the first five lines. The last five lines
are checked when `Analyze` or `AnalyzeReader` receives the complete file.

`Analyze` resets its destination on each call and does not retain the input
buffer. Reuse the buffer and analysis for successive files, with a separate
destination for each concurrent call. `Result` and `Detect` read the analysis
without changing it or rescanning the source.

Rule evidence contains an ID, description, candidate languages, and a byte
offset into the original input. Statistical scores have no source offsets and
do not appear in `Signals`. Keep cached analyses in memory: the default JSON
encoder omits their internal classifier state.

## Command line

Install the CLI with Go:

```bash
go install github.com/git-pkgs/languages/cmd/languages@latest
```

Make sure your Go binary directory (`GOBIN`, or `$(go env GOPATH)/bin` when unset)
is on your `PATH`. For a file, the CLI writes JSON and combines the filename with
content by default. Pass a file, or pipe content to stdin and supply its name
with `-name`:

```bash
languages source.pl
printf 'use strict;\n' | languages -name source.pl
```

Use `-mode content` to ignore the filename, or `-mode path` to inspect a filename
without opening the file:

```bash
languages -mode content source.pl
languages -mode path -name source.pl
```

The path-only example returns:

```json
{
  "confidence": "low",
  "candidates": ["Perl", "Raku", "Prolog"],
  "bytes_examined": 0,
  "prefix": false,
  "path_evidence": {
    "reason": "extension",
    "candidates": ["Perl", "Raku", "Prolog"]
  }
}
```

`language` is omitted when no single language is selected. Unknown, ambiguous,
and conflicting results exit successfully, so scripts must inspect the
JSON. Argument, file-reading, and output errors produce a nonzero exit status.

The CLI reads full files and stdin to EOF by default. Set a read budget with
`-bytes`, or use `-bytes 0` for full input. This command reads up to 1 KiB:

```bash
languages -bytes 1024 source.pl
```

Reaching the budget without EOF sets `prefix: true`. For input that was truncated
before reaching stdin, use `-prefix`:

```bash
languages -prefix -name source.pl < exported-prefix
```

## Directory breakdowns

Pass a directory to see language totals for the whole tree and each subdirectory:

```bash
languages ./project
```

For a project with Go and TypeScript files, output looks like:

```text
project/  Go 75.0%, TypeScript 25.0% (3 files, 4.0 KiB)
├── api/  Go 100.0% (2 files, 3.0 KiB)
│   └── lib/  Go 100.0% (1 file, 1.0 KiB)
└── web/  TypeScript 100.0% (1 file, 1.0 KiB)
```

Each directory includes all descendant files. Percentages use full file sizes,
even when detection reads only a prefix. Unknown, ambiguous, conflicting, and
binary files have separate totals and remain in the percentage denominator.
`partial` counts files whose content exceeds the read limit.

Scan a subdirectory on its own, limit the displayed depth, or request JSON:

```bash
languages ./project/api
languages -depth 1 ./project
languages -json ./project
languages -bytes 1024 ./project
```

`-depth 0` shows only the root; the default shows every directory containing
included files. Depth limits affect text and JSON output without changing the
totals. JSON contains `path`, `summary`, and `children`; each summary contains
file counts and byte totals, with language names under `languages`. JSON sizes
are integer byte counts so callers can calculate their own shares.

Scans work without Git metadata. They skip `.git`, symlinks, and non-regular
files, and include untracked, ignored, vendored, and generated files. Directory
scans use file sizes to identify complete input, including files exactly as
large as the read limit.

In Go, `Scan` accepts an `fs.FS`. Add `context` and `os` to your imports:

```go
tree, err := languages.Scan(context.Background(), os.DirFS("project"), languages.ScanOptions{})
if err != nil {
    panic(err)
}
fmt.Println(tree.Root().Summary)
if api, ok := tree.Subtree("api"); ok {
    fmt.Println(api.Summary)
}
```

Use `os.DirFS("project/api")` or `fs.Sub` to scan only a subdirectory. Set
`ScanOptions.Bytes` for a read budget. To exclude files or directories, supply
`ScanOptions.Exclude` with the `io/fs` import. It receives root-relative paths;
returning true for a directory skips its contents:

```go
options := languages.ScanOptions{
    Exclude: func(name string, entry fs.DirEntry) bool {
        return entry.IsDir() && entry.Name() == "vendor"
    },
}
```

Pass these options as the third argument to `Scan`. If a file cannot be read or
the context is cancelled, the scan returns an error without a partial tree.

If your application already traverses files, feed its analyses into a `Tree`
instead of scanning again:

```go
var tree languages.Tree
source := []byte("package main\n")
var analysis languages.Analysis
languages.Analyze(source, true, &analysis)
if err := tree.Add("api/main.go", int64(len(source)), &analysis); err != nil {
    panic(err)
}
fmt.Println(tree.Root().Summary)
```

For prefix analyses, pass the full file size to `Add`. Paths must be unique,
slash-separated, and relative to the tree root, without `.` or `..` components.
You can reuse the content buffer and analysis after each call. `Root` and
`Subtree` return independent snapshots with children sorted by path and languages
sorted by size, then file count and name. Subtree paths remain relative to the
original root; empty directories are omitted.

## Limits

Detection matches byte patterns and token frequencies without validating syntax.
Comment and string handling is partial, including for heredocs. Embedded
languages, minified code, and short fragments may be missed or misidentified.

Language metadata, extension heuristics, and classifier training samples come
from [Linguist]. Extension heuristics inspect the first 50 KiB through
[scan](https://github.com/git-pkgs/scan). Syntax rules and the token classifier
process all supplied content.

Binary classification and encoding recognition use
[magic](https://github.com/git-pkgs/magic). Binary content sets `Analysis.Binary`
and produces no language selection.

BOM-marked UTF-16 and UTF-32 are decoded before language analysis. Evidence
offsets refer to the original bytes, and a prefix may end inside a code
point. Malformed BOM-marked input is rejected. Other non-UTF-8 input is analyzed
as bytes when magic cannot classify it; detection quality may be lower.

## License

[MIT](LICENSE). Imported [Linguist] metadata, heuristics, and sample-derived model data are
covered by the upstream [MIT notice](LICENSE.linguist).

[Linguist]: https://github.com/github-linguist/linguist

# languages

Identify programming languages from file contents, including short prefixes,
with optional filename evidence. Results include candidate languages,
confidence categories, and the rules that matched. The library is pure Go,
runs offline, and has no third-party dependencies.

## Install

Build the command from this checkout:

```bash
CGO_ENABLED=0 go build -o /tmp/languages ./cmd/languages
```

The library's module path is `github.com/git-pkgs/languages`.

## Use

Analyze content once, then combine it with paths from individual occurrences:

```go
var content languages.Analysis
languages.Analyze(prefix, false, &content)

intrinsic := content.Result()
fmt.Println(intrinsic.Language, intrinsic.Confidence)

context := languages.AnalyzePath("src/example.pl")
combined := languages.Combine(&content, context)
fmt.Println(combined.Language, combined.Conflict)

for _, match := range content.Signals[:content.Count] {
    evidence := match.Evidence()
    fmt.Println(evidence.ID, evidence.Description, evidence.Offset)
}
```

Pass `true` as the second argument only when the supplied bytes are the complete
object. `Analyze` reads at most `MaxBytes` (64 KiB), even for a larger byte slice.
`Analysis.Prefix` records whether a suffix remains unavailable. Input is neither
copied nor retained, and callers can reuse both the input buffer and `Analysis`.
Each concurrent call needs its own destination.

`AnalyzePath(path).Result()` uses the path alone. `Combine` reads an existing
analysis without changing it. A Git blob or Software Heritage content object
can therefore be analyzed without walking trees or loading paths. Consumers
can cache that analysis by content identity and add occurrence evidence later.
Cache keys should also identify the byte limit, completeness, and detector
build. Numeric language and rule indices are internal to that build; use the
evidence IDs and language names for durable reports.

The API accepts the same prefix buffer a consumer uses for `magic` or obtains
through bounded input handling in `peek`. Consumers such as `brief`, `outline`,
and `proxy` can combine language evidence with physical format and the roles
reported by `roles`. Historical scans with `history` can reuse a blob's intrinsic
evidence across commits, then add the path at each occurrence to distinguish
renames from language changes supported by content.

## Results

`Language` is `Unknown` when there is no selection. `Candidates` preserves
ambiguity and supports `Has`, `Len`, and `Only`. It contains languages supported
by the current rules, not every language in which a fragment could be valid.
`Conflict` reports disagreement between a shebang and strong syntax evidence,
or between content and a path. A conflicting path does not override content.

Confidence categories describe the strength of the matched rules:

- `none`: no rule reached the minimum score.
- `low`: weak syntax or path evidence, or conflicting evidence.
- `medium`: a stronger syntax signal.
- `high`: an interpreter declaration, or multiple supporting rules.

Use `r.Confidence.Rank() >= languages.Medium.Rank()` to apply a confidence
threshold. A high-confidence result can still contain several candidates. Scores are
hand-assigned rather than calibrated probabilities, and each rule contributes
at most once regardless of repetition. Each content match has a rule ID,
description, language set, and byte offset; path evidence remains in `Context`.

The initial rules cover Python, Ruby, Go, Rust, Java, C, C++, Objective-C, MATLAB,
JavaScript, TypeScript, JSX, TSX, sh, Bash, Zsh, Fish, Perl, Raku, Prolog, Common
Lisp, Scheme, Clojure, Racket, PHP, Lua, C#, HTML, XML, Jinja, Twig, ERB, and SQL.
Coverage within each language is partial. Common declarations, imports,
directives, shebangs, and template markers supply the evidence.

A C header can match C, C++, and Objective-C; plain JavaScript can also match
TypeScript, JSX, and TSX. Scheme forms can match
Racket, and Jinja/Twig markers overlap. `.pl` permits Perl, Raku, and Prolog;
`use strict;` and `:- use_module(...)` provide different content evidence.
An isolated Prolog fact or `print(1)` may produce no selection.

The detector matches byte patterns without validating syntax. It skips comments
and common multiline strings, but handling of heredocs and other quoting
dialects is incomplete. Embedded languages, minified code, and fragments may
be missed or misidentified. A comment-only prefix often yields no evidence.
The binary flag checks for C0 control bytes except tab, newline, carriage return,
form feed, and ESC (used in ANSI colour codes). Use a format and encoding detector
before this package if you need file-format identification or UTF-16 decoding;
other non-UTF-8 bytes are scanned for ASCII signals.

## CLI

The default is content-only, even when the input has a filename. Output is JSON.
Omit the input file or use `-` to read stdin:

```bash
/tmp/languages source.pl
/tmp/languages -mode path -name source.pl
/tmp/languages -mode combined -name source.pl < source.pl
/tmp/languages -bytes 4096 < source.pl
/tmp/languages -prefix < exported-prefix
```

The default read limit is 1,024 bytes; the maximum is 65,536. No byte beyond the
limit is consumed. If the limit is filled exactly, the CLI reports a prefix
because it has not checked for EOF. Path mode does not open the supplied path.
Use `-prefix` for an already truncated export, where the stream's EOF does not
establish the end of the original object.

## Evaluation

The evaluator accepts Linguist's `samples/<language>/...` directory layout,
including its nested `filenames` directories. Adding corpus files or new language
directories requires no importer changes. Files for unimplemented languages are
counted separately and excluded from accuracy totals.

```bash
go run ./cmd/evaluate -corpus testdata/corpus
go run ./cmd/evaluate -corpus /path/to/linguist/samples -revision COMMIT \
  -predictions /tmp/languages-predictions.jsonl > /tmp/languages-results.json
```

Each supported sample is evaluated at 128, 256, 512, 1,024, and 4,096 bytes, plus
the complete file, in all three modes. Complete-file rows use `bytes: 0` and
exclude files over 64 KiB; `oversized_complete_skipped` records the count. Prefix
rows still include those files. No filename is passed to content-only detection.

The report separates exact correct labels, wrong labels, ambiguity, abstention,
candidate-set hits, high-confidence results, and extensionless files. Per-sample
JSONL includes prefix hashes and predictions for inspecting failures. The bundled
45 files are authored development fixtures, including deliberately ambiguous
and delayed-signal examples. They are not an independent accuracy benchmark.
Linguist's corpus is the intended larger evaluation input; preserve its source
revision and each sample's provenance when distributing samples.

An optional go-enry adapter is isolated in `tools/enry`, outside the library's
module. Build it separately and pass its executable to the evaluator:

```bash
(cd tools/enry && CGO_ENABLED=0 go build -o /tmp/languages-enry .)
go run ./cmd/evaluate -corpus /path/to/linguist/samples \
  -baseline /tmp/languages-enry
```

For content-only comparison, the adapter first tries go-enry's filename-free
strategies and then its classifier with all classifier languages as candidates.
Path and combined comparisons use its normal API. This is broader than this
package's initial language set. The JSONL protocol also allows other baseline
executables without adding dependencies to the library.

`go run ./cmd/localscan -root /absolute/path/to/repos` measures traversal, bounded
reads, and content detection across local Git repositories. Add `-exclude` for
this repository and `-out /tmp/new-corpus-directory` to export a bounded sample
with provenance. Sampling uses weak extension labels, excludes shared
extensions, and caps each language at 30 files and each repository/language at
three. Symlinks, Git metadata, dependency directories, and common build output
directories are skipped. No source checkout is modified or fetched.

An initial local sample contained 290 files across 22 language labels. At 1 KB,
content-only detection returned 174 exact labels, one wrong label, 56 ambiguous
results, and 59 unknowns. The go-enry adapter returned 139 exact labels. These
are weak extension labels from one development workspace, so they do not
establish general accuracy. The larger Linguist evaluation remains pending.

## Performance

The core has fixed-size result storage, no runtime initialization, and no mutable
shared state. Runtime work is bounded by 64 KiB and the fixed rule count. Corpus
evaluation reads one bounded sample at a time and at most 64 directory entries
per batch, with a maximum directory depth of 32. It does not retain all results
or deduplicate the corpus in memory.

On an Apple A18 Pro with Go 1.27.1, mixed padded 1 KiB fixtures took about
2.2 microseconds per call. A sample of real prefixes averaging 841 bytes took
about 5.9 microseconds. Both reported zero B/op and zero allocations/op. A serial
scan of 445,890 files across 123 local repositories took 43.5 seconds including
filesystem work, with 2.59 seconds in detection. A million-object in-memory scan
kept measured heap at about 142 KB with zero allocations in the scan loop.

```bash
CGO_ENABLED=0 go test ./...
go test -race ./...
go test -run '^$' -bench . -benchmem
make profile
LANGUAGES_BENCH_CORPUS=/tmp/new-corpus-directory go test -run '^$' \
  -bench BenchmarkCorpusPrefixes -benchmem
```

Benchmarks cover mixed 1 KiB prefixes, 1,024-object batches, concurrent calls,
and adversarial inputs at the byte limit. CPU and memory profiles go
to `/tmp`; inspect them before changing the hot path. CI builds and tests with
`CGO_ENABLED=0` and also builds the CLI for WASM.

## License

[MIT](LICENSE).

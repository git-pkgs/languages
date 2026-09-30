# languages

Detect programming languages from a filename and a bounded content prefix.
Either input can be omitted. The Go module accepts 1 KiB prefixes and larger
buffers up to 64 KiB, with candidate languages, confidence, and rule evidence.
It runs offline, has no third-party dependencies, and supports WebAssembly.

## Install

Add the module to a Go project:

```bash
go get github.com/git-pkgs/languages
```

The CLI is a consumer of the module. Build it with
`CGO_ENABLED=0 go build -o /tmp/languages ./cmd/languages`.

## Use

Pass the filename when available. An empty name uses content alone:

```go
result := languages.Detect("src/example.ts", prefix)
fmt.Println(result.Language, result.Confidence)

withoutName := languages.Detect("", prefix)
fmt.Println(withoutName.Candidates)
```

`Detect` performs no I/O and examines only the supplied bytes, up to `MaxBytes`
(64 KiB). Pass at most `DefaultBytes` (1,024 bytes) for a 1 KiB budget. It treats
the input as a prefix because a byte slice does not establish completeness.
Larger buffers can supply more evidence without changing the call.

For known completeness, rule evidence, or reuse across filenames, keep an
`Analysis` value:

```go
var content languages.Analysis
languages.Analyze(prefix, false, &content)

intrinsic := content.Result()
fmt.Println(intrinsic.Language, intrinsic.Confidence)

combined := content.Detect("src/example.pl")
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

`AnalyzePath(path).Result()` uses the path alone. `Analysis.Detect` and `Combine`
read an existing analysis without changing it. A Git blob or Software Heritage
content object can therefore be analyzed without walking trees or loading paths. Consumers
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

A consumer importing both `peek` and `languages` can share one buffer and its
completeness flag:

```go
var claims peek.Result
var content languages.Analysis
peek.InspectInto(&claims, peek.Input{Bytes: prefix, Complete: complete})
languages.Analyze(prefix, complete, &content)

intrinsic := content.Result()
combined := languages.Combine(&content, languages.AnalyzePath(filename))
fmt.Println(claims.Claims, intrinsic.Language, combined.Language)
```

Reuse both result values for successive inputs. The caller determines
`complete` once and passes it to both libraries. `Result` and `Combine` each
score the content evidence; `BenchmarkResultPath` measures extraction and the
cost of displaying both results. Neither module needs a dependency on the other.

## Results

`Language` is `Unknown` when there is no selection. `Candidates` preserves
ambiguity and supports `Has`, `Len`, and `Only`. It contains languages supported
by the current rules, not every language in which a fragment could be valid.
`Conflict` reports disagreement between a shebang and strong syntax evidence,
or between a path and strong content evidence. Compatible filename evidence
narrows the candidates. When only weak syntax disagrees, the filename takes
precedence at low confidence. Strong contradictions remain unresolved.

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

The default combines content with the input filename or `-name`. Without a name,
it uses content alone. Output is JSON. Omit the input file or use `-` to read stdin:

```bash
/tmp/languages source.pl
/tmp/languages -mode path -name source.pl
/tmp/languages -name source.pl < source.pl
/tmp/languages -mode content source.pl
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
candidate-set hits, high-confidence results, and extensionless files. Strict
scores require the canonical language label. `family_correct` also accepts
Shell, Bash, and Zsh as equivalent selections; other labels and ambiguous
results stay separate. Per-sample JSONL includes prefix hashes and predictions
for inspecting failures. The bundled
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
establish general accuracy.

The Linguist run at commit `5fbdfcb8133be2bed88bf3ce62b2335f50474525` scored 607
files across 29 supported labels. It skipped 2,797 unmapped files and two
symlinks; 22 files exceeded the complete-file byte limit. The supported set
includes Linguist's lowercase `fish` label. Results are per path, without
deduplication, and unsupported files are not analyzed for false positives.

Filename-plus-content results:

| Input | Files | Exact | Family correct | Wrong (strict) | Ambiguous | Unknown | go-enry exact |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 128 B | 607 | 408 | 442 | 37 | 122 | 40 | 563 |
| 256 B | 607 | 418 | 452 | 37 | 118 | 34 | 567 |
| 512 B | 607 | 425 | 459 | 37 | 117 | 28 | 573 |
| 1 KiB | 607 | 441 | 475 | 37 | 108 | 21 | 587 |
| 4 KiB | 607 | 449 | 483 | 37 | 102 | 19 | 600 |
| Complete | 585 | 429 | 463 | 36 | 102 | 18 | 577 |

At 1 KiB, strict exact identification increased from 353/607 (58.2%) to 441/607
(72.7%) after adding filename conventions and refining weak content evidence.
The Shell-family count increased from 371 to 475. Content-only detection now
returns 254 exact labels, 15 wrong labels, 146 ambiguities, and 192 unknowns;
go-enry returns 531 exact labels. Broader candidate sets for shared `use` and
`set` syntax reduced content-only wrong labels from 20 to 15, while exact
labels fell from 258 to 254.

These are development measurements on the corpus used to inspect failures.
go-enry's classifier is trained on Linguist samples, so its comparison is not
held out either. Its map-based tie ordering can also change tied predictions.
The aggregate results and provenance are in
[`testdata/evaluation/linguist.json`](testdata/evaluation/linguist.json).

## WebAssembly

The module has no filesystem or process requirements. The same Go API is used
by native consumers and WASM consumers. CI builds the CLI for `js/wasm` and
executes a separate module consumer in native Go and Node's WebAssembly runtime,
checking filenames, missing names, incomplete input, binary data, and byte limits.

```bash
GOOS=js GOARCH=wasm go build -o /tmp/languages-smoke.wasm ./internal/wasmcheck
"$(go env GOROOT)/lib/wasm/go_js_wasm_exec" /tmp/languages-smoke.wasm
```

## Performance

The core has fixed-size result storage, no runtime initialization, and no mutable
shared state. Runtime work is bounded by 64 KiB and the fixed rule count. Corpus
evaluation reads one bounded sample at a time and at most 64 directory entries
per batch, with a maximum directory depth of 32. It does not retain all results
or deduplicate the corpus in memory.

On an Apple A18 Pro with Go 1.27.1, `Detect` took 1.73-1.89 microseconds per call
on mixed padded 1 KiB fixtures. Filename-plus-content detection on the 607
supported Linguist files took 3.99-4.08 microseconds per prefix, capped at 1 KiB.
The same mixed fixtures took 5.23-5.56 microseconds in Go WASM under Node
26.10.0. All reported zero B/op and zero allocations/op across three runs.

The WASM module consumer built with default Go settings is 2,051,269 bytes,
about 614 KB with gzip. This includes the Go runtime and smoke checks. These
measurements come from one development machine and are not regression budgets.
Raw native and WASM benchmark outputs accompany the evaluation report.

A comment-heavy 1 KiB Python prefix took 5.95-6.09 microseconds for extraction
alone and 6.54-6.58 microseconds for extraction plus intrinsic and combined
results. Computing both results from cached evidence took 95-96 nanoseconds.
All paths reported zero allocations. A CPU profile attributed about 84% of
samples to `detectLine` and its callees, which match lines against rules.

```bash
CGO_ENABLED=0 go test ./...
go test -race ./...
go test -run '^$' -bench . -benchmem
go test -run '^$' -bench BenchmarkResultPath -benchmem
make profile
LANGUAGES_BENCH_CORPUS=/path/to/linguist/samples go test -run '^$' \
  -bench BenchmarkCorpusPrefixes -benchmem
GOOS=js GOARCH=wasm go test \
  -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" \
  -run '^$' -bench BenchmarkDetect1KB -benchmem
```

Benchmarks cover mixed 1 KiB prefixes, 1,024-object batches, concurrent calls,
and adversarial inputs at the byte limit. CPU and memory profiles go
to `/tmp`; inspect them before changing the hot path. CI builds and tests with
`CGO_ENABLED=0` and also builds the CLI for WASM.

## License

[MIT](LICENSE).

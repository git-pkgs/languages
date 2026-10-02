// Package languages identifies source languages from content and paths.
// Content analysis does not require or retain a filename or the input buffer.
package languages

import (
	"math/bits"
	"unicode"
	"unicode/utf8"

	"github.com/git-pkgs/languages/internal/classifier"
)

// Language identifies a language within a detector build. Store names, not indices.
type Language uint16

const (
	Unknown Language = iota
	Python
	Ruby
	Go
	Rust
	Java
	C
	CPP
	ObjectiveC
	MATLAB
	JavaScript
	TypeScript
	JSX
	TSX
	Shell
	Bash
	Zsh
	Fish
	Perl
	Raku
	CommonLisp
	Scheme
	Clojure
	Racket
	PHP
	Lua
	CSharp
	HTML
	XML
	Jinja
	Twig
	ERB
	SQL
	Prolog
)

func (l Language) String() string {
	if l >= LanguageCount {
		return ""
	}
	return names[l]
}

func Parse(name string) Language {
	lo, hi := 0, len(languageAliases)
	for lo < hi {
		mid := lo + (hi-lo)>>1
		entry := languageAliases[mid]
		comparison := compareAlias(name, entry.name)
		if comparison == 0 {
			return entry.language
		}
		if comparison < 0 {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return Unknown
}

func compareAlias(name, alias string) int {
	for len(name) > 0 && len(alias) > 0 {
		a, n := utf8.DecodeRuneInString(name)
		b, m := utf8.DecodeRuneInString(alias)
		a = unicode.ToLower(a)
		if a != b {
			return int(a - b)
		}
		name, alias = name[n:], alias[m:]
	}
	return len(name) - len(alias)
}

type Confidence string

const (
	None   Confidence = "none"
	Low    Confidence = "low"
	Medium Confidence = "medium"
	High   Confidence = "high"
)

// Rank orders confidence from none (0) to high (3). Unknown values rank as none.
func (c Confidence) Rank() int {
	for rank, value := range [...]Confidence{None, Low, Medium, High} {
		if c == value {
			return rank
		}
	}
	return 0
}

// Result leaves Language empty when the evidence supports multiple candidates.
// Confidence describes rule support, not an empirical probability.
type Result struct {
	Language    Language
	Confidence  Confidence
	Candidates  Set
	Conflict    bool
	Statistical bool
}

// DefaultBytes reads the entire file. Positive read budgets select a prefix.
const DefaultBytes = 0
const MaxSignals = 128

// Match records the first occurrence of a rule. Offsets refer to the original bytes.
// Rule indices are build-specific; use Evidence().ID for stored results.
type Match struct {
	Rule   uint16
	Offset uint64
}

type Evidence struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Languages   Set    `json:"languages"`
	Offset      uint64 `json:"offset"`
}

func (m Match) Evidence() Evidence {
	r := m.rule()
	if r == nil {
		return Evidence{}
	}
	return Evidence{ID: r.id, Description: r.description, Languages: r.languages, Offset: m.Offset}
}

func (m Match) rule() *rule {
	if int(m.Rule) < len(rules) {
		return &rules[m.Rule]
	}
	i := int(m.Rule) - len(rules)
	if i < len(interpreterRules) {
		return &interpreterRules[i]
	}
	i -= len(interpreterRules)
	if i < len(modelineRules) {
		return &modelineRules[i]
	}
	return nil
}

// Analysis is a reusable, value-copyable content result with no input references.
// Only Signals[:Count] is populated. It can be stored with a content object's ID.
// Prefix is true unless the caller supplied the complete object.
// JSON encoding omits classifier state.
type Analysis struct {
	Signals        [MaxSignals]Match
	Count          int
	Bytes          int64
	Prefix         bool
	Binary         bool
	classification classifier.Analysis
	heuristics     [len(heuristicGroups)]uint16
}

func (a *Analysis) Result() Result {
	r := a.ruleResult()
	if !r.Conflict && r.Candidates == jsFamily && a.hasDeclaration() {
		r.Language = JavaScript
		r.Candidates = NewSet(JavaScript)
	}
	if r.Conflict || r.Confidence == High || r.Language == GoTemplate || a.hasDeclaration() {
		return r
	}
	if r.Confidence == Medium && a.classification.Tokens < minimumOverrideTokens {
		return r
	}
	if result := a.classify(Set{}); !result.Candidates.Empty() {
		return result
	}
	return r
}

func (a *Analysis) ruleResult() Result {
	if a.Count == 0 {
		return Result{Confidence: None}
	}
	if a.Signals[a.Count-1].Rule == uint16(goTemplateRule) {
		return Result{Language: GoTemplate, Candidates: NewSet(GoTemplate), Confidence: Medium}
	}
	var scores [LanguageCount]uint16
	var counts [LanguageCount]uint8
	var explicit Set
	var strong Set
	var active Set
	var best uint16
	for _, m := range a.Signals[:a.Count] {
		r := m.rule()
		active = active.Union(r.languages)
		for i, word := range r.languages.words {
			for word != 0 {
				l := Language(i*64 + bits.TrailingZeros64(word))
				word &= word - 1
				scores[l] += uint16(r.weight)
				counts[l]++
				best = max(best, scores[l])
			}
		}
		if r.weight == declaredWeight {
			explicit = explicit.Union(r.languages)
		}
		if r.weight >= strongWeight {
			strong = strong.Union(r.languages)
		}
	}
	if best < minimumScore {
		return Result{Confidence: None}
	}
	var candidates Set
	for i, word := range active.words {
		for word != 0 {
			l := Language(i*wordBits + bits.TrailingZeros64(word))
			word &= word - 1
			if scores[l] >= minimumScore && scores[l]+scoreMargin >= best {
				candidates.add(l)
			}
		}
	}
	conf := Low
	if best >= mediumScore {
		conf = Medium
	}
	// The best-scoring language meets both candidate thresholds, so candidates is nonempty.
	if best >= highScore && (explicit.overlaps(candidates) || strong.overlaps(candidates) && counts[candidates.first()] >= 2) {
		conf = High
	}
	conflict := !explicit.Empty() && !explicit.overlaps(candidates)
	conflict = conflict || a.contradicts(explicit)
	if conflict {
		candidates = candidates.Union(explicit).Union(strong)
		conf = Low
	}
	return Result{Language: candidates.Only(), Confidence: conf, Candidates: candidates, Conflict: conflict}
}

func (a *Analysis) contradicts(explicit Set) bool {
	if explicit.Empty() {
		return false
	}
	if explicit.Has(Cython) {
		explicit.add(Python)
	}
	if explicit.overlaps(xmlLanguages) {
		explicit.add(XML)
	}
	for _, m := range a.Signals[:a.Count] {
		r := m.rule()
		if r.weight >= strongWeight && !r.languages.overlaps(explicit) {
			return true
		}
	}
	return false
}

const (
	minimumScore   = 3
	mediumScore    = 5
	highScore      = 9
	scoreMargin    = 1
	declaredWeight = 12
	strongWeight   = 6
)

// Context is occurrence-specific evidence. No path string is retained.
type Context struct {
	Candidates Set
	Reason     string
	heuristic  uint16
}

func (c Context) Result() Result {
	if c.Candidates.Empty() {
		return Result{Confidence: None}
	}
	return Result{Language: c.Candidates.Only(), Candidates: c.Candidates, Confidence: Low}
}

// Combine narrows content candidates with a path, preserving declaration conflicts.
func Combine(a *Analysis, c Context) Result {
	r := a.Result()
	if a.Binary {
		return Result{Confidence: None, Conflict: !c.Candidates.Empty()}
	}
	if c.Candidates.Empty() {
		return r
	}
	if c.Candidates.Has(Shell) && shellFamily.Has(r.Language) {
		c.Candidates.add(r.Language)
	}
	intrinsic := a.ruleResult()
	if result, ok := a.contentContext(intrinsic, c.Candidates); ok {
		return result
	}
	if intrinsic.Language != XML {
		if heuristic := a.heuristicResult(c); !heuristic.Candidates.Empty() {
			if heuristic.Language != Unknown {
				return heuristic
			}
			c.Candidates = heuristic.Candidates
		}
	}
	if !r.Conflict && !a.hasDeclaration() {
		if classified, ok := a.classifiedContext(c, intrinsic); ok {
			return classified
		}
	}
	if result, ok := xmlContext(intrinsic, c.Candidates); ok {
		return result
	}
	if !intrinsic.Conflict && intrinsic.Confidence.Rank() >= Medium.Rank() {
		shared := intrinsic.Candidates.Intersect(c.Candidates)
		if shared.Only() != Unknown {
			intrinsic.Candidates = shared
			intrinsic.Language = shared.Only()
			return intrinsic
		}
	}
	if a.contradicts(c.Candidates) {
		intrinsic.Candidates = intrinsic.Candidates.Union(c.Candidates)
		intrinsic.Language = Unknown
		intrinsic.Conflict = true
		intrinsic.Confidence = Low
		return intrinsic
	}
	if !r.Conflict && !a.hasDeclaration() && c.Candidates.Len() > 1 {
		if narrowed := a.classify(c.Candidates); !narrowed.Candidates.Empty() {
			return narrowed
		}
	}
	if r.Candidates.Empty() {
		return c.Result()
	}
	if shared := r.Candidates.Intersect(c.Candidates); !shared.Empty() {
		if r.Conflict {
			return r
		}
		r.Candidates = shared
		r.Language = shared.Only()
		return r
	}
	if !r.Conflict {
		return c.Result()
	}
	r.Candidates = r.Candidates.Union(c.Candidates)
	r.Language = Unknown
	r.Conflict = true
	r.Confidence = Low
	return r
}

func (a *Analysis) contentContext(intrinsic Result, candidates Set) (Result, bool) {
	if a.declarationConflict(candidates) {
		intrinsic.Candidates = intrinsic.Candidates.Union(candidates)
		intrinsic.Language = Unknown
		intrinsic.Conflict = true
		intrinsic.Confidence = Low
		return intrinsic, true
	}
	if intrinsic.Language == GoTemplate && candidates.overlaps(NewSet(GoTemplate, HTML, XML)) {
		return intrinsic, true
	}
	return Result{}, false
}

func xmlContext(result Result, candidates Set) (Result, bool) {
	if result.Language != XML {
		return Result{}, false
	}
	shared := candidates.Intersect(xmlLanguages)
	if shared.Empty() {
		return Result{}, false
	}
	if !shared.Has(XML) {
		result.Candidates = shared
		result.Language = shared.Only()
	}
	return result, true
}

func (a *Analysis) declarationConflict(candidates Set) bool {
	if candidates.Has(Pod) {
		candidates.add(Perl)
	}
	for _, match := range a.Signals[:a.Count] {
		r := match.rule()
		if (r.weight == declaredWeight || r.id == "ruby.frozen" || r.id == "perl.strict" || r.id == "perl.warnings") && !r.languages.overlaps(candidates) {
			return true
		}
	}
	return false
}

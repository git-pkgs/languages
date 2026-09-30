// Package languages identifies source languages from bounded byte prefixes and paths.
// Content analysis does not require or retain a filename or the input buffer.
package languages

import "math/bits"

type Language uint8

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
	LanguageCount
)

var names = [...]string{"", "Python", "Ruby", "Go", "Rust", "Java", "C", "C++", "Objective-C", "MATLAB", "JavaScript", "TypeScript", "JSX", "TSX", "Shell", "Bash", "Zsh", "Fish", "Perl", "Raku", "Common Lisp", "Scheme", "Clojure", "Racket", "PHP", "Lua", "C#", "HTML", "XML", "Jinja", "Twig", "ERB", "SQL", "Prolog"}

func (l Language) String() string {
	if l >= LanguageCount {
		return ""
	}
	return names[l]
}

func Parse(name string) Language {
	for l := Python; l < LanguageCount; l++ {
		if l.String() == name {
			return l
		}
	}
	switch name {
	case "Jinja2", "HTML+Jinja":
		return Jinja
	case "HTML+ERB":
		return ERB
	case "HTML+PHP":
		return PHP
	case "Matlab":
		return MATLAB
	}
	return Unknown
}

// Set is a bounded set of candidate languages. Its order is the Language order.
type Set uint64

func (s Set) Has(l Language) bool { return l > Unknown && l < LanguageCount && s&(1<<l) != 0 }
func (s Set) Len() int            { return bits.OnesCount64(uint64(s)) }
func (s Set) Only() Language {
	if s.Len() != 1 {
		return Unknown
	}
	return Language(bits.TrailingZeros64(uint64(s)))
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
	Language   Language
	Confidence Confidence
	Candidates Set
	Conflict   bool
}

// MaxBytes bounds every Analyze call, even when passed a complete large file.
const MaxBytes = 64 * 1024
const DefaultBytes = 1024
const MaxSignals = 128

// Match records the first occurrence of a rule. Offsets refer to the original bytes.
type Match struct {
	Rule   uint16
	Offset uint32
}

type Evidence struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Languages   Set    `json:"languages"`
	Offset      uint32 `json:"offset"`
}

func (m Match) Evidence() Evidence {
	if int(m.Rule) >= len(rules) {
		return Evidence{}
	}
	r := rules[m.Rule]
	return Evidence{ID: r.id, Description: r.description, Languages: r.languages, Offset: m.Offset}
}

// Analysis is a reusable, value-copyable content result with no input references.
// Only Signals[:Count] is populated. It can be stored with a content object's ID.
// Prefix is true unless the caller supplied the complete object within MaxBytes.
type Analysis struct {
	Signals [MaxSignals]Match
	Count   int
	Bytes   int
	Prefix  bool
	Binary  bool
}

func (a *Analysis) Result() Result {
	var scores [LanguageCount]uint16
	var counts [LanguageCount]uint8
	var explicit Set
	var strong Set
	for _, m := range a.Signals[:a.Count] {
		r := rules[m.Rule]
		for l := Python; l < LanguageCount; l++ {
			if r.languages.Has(l) {
				scores[l] += uint16(r.weight)
				counts[l]++
			}
		}
		if r.weight == declaredWeight {
			explicit |= r.languages
		}
		if r.weight >= strongWeight {
			strong |= r.languages
		}
	}
	var best uint16
	for _, score := range scores {
		if score > best {
			best = score
		}
	}
	if best < minimumScore {
		return Result{Confidence: None}
	}
	var candidates Set
	for l := Python; l < LanguageCount; l++ {
		if scores[l] >= minimumScore && scores[l]+scoreMargin >= best {
			candidates |= 1 << l
		}
	}
	conf := Low
	if best >= mediumScore {
		conf = Medium
	}
	// The best-scoring language meets both candidate thresholds, so candidates is nonempty.
	if best >= highScore && (explicit&candidates != 0 || counts[candidatesFirst(candidates)] >= 2) {
		conf = High
	}
	conflict := explicit != 0 && explicit&candidates == 0
	conflict = conflict || a.contradicts(explicit)
	if conflict {
		candidates |= explicit | strong
		conf = Low
	}
	return Result{Language: candidates.Only(), Confidence: conf, Candidates: candidates, Conflict: conflict}
}

func (a *Analysis) contradicts(explicit Set) bool {
	if explicit == 0 {
		return false
	}
	for _, m := range a.Signals[:a.Count] {
		r := rules[m.Rule]
		if r.weight >= strongWeight && r.languages&explicit == 0 {
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

func candidatesFirst(s Set) Language { return Language(bits.TrailingZeros64(uint64(s))) }

// Context is occurrence-specific evidence. No path string is retained.
type Context struct {
	Candidates Set
	Reason     string
}

func (c Context) Result() Result {
	if c.Candidates == 0 {
		return Result{Confidence: None}
	}
	return Result{Language: c.Candidates.Only(), Candidates: c.Candidates, Confidence: Low}
}

// Combine uses a path to narrow compatible content candidates. A conflicting
// path is retained as a conflict, never silently substituted for content evidence.
func Combine(a *Analysis, c Context) Result {
	r := a.Result()
	if a.Binary {
		return Result{Confidence: None, Conflict: c.Candidates != 0}
	}
	if c.Candidates == 0 {
		return r
	}
	if r.Candidates == 0 {
		return c.Result()
	}
	if shared := r.Candidates & c.Candidates; shared != 0 {
		if r.Conflict {
			return r
		}
		r.Candidates = shared
		r.Language = shared.Only()
		return r
	}
	r.Candidates |= c.Candidates
	r.Language = Unknown
	r.Conflict = true
	r.Confidence = Low
	return r
}

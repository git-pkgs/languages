package languages

import (
	"testing"

	"github.com/git-pkgs/languages/internal/classifier"
)

func TestRuleRegistry(t *testing.T) {
	if len(rules)+1 > MaxSignals {
		t.Fatal("fixed capacity exceeded")
	}
	ids := make(map[string]bool)
	for _, r := range rules {
		if ids[r.id] || r.id == "" || r.description == "" || r.prefix == "" || r.languages.Empty() || r.weight == 0 {
			t.Fatalf("invalid rule: %+v", r)
		}
		ids[r.id] = true
	}
	for l := Python; l < LanguageCount; l++ {
		if Parse(l.String()) != l {
			t.Fatal(l)
		}
	}
}

func TestParseAliases(t *testing.T) {
	for name, want := range map[string]Language{
		"Jinja2": Jinja, "HTML+Jinja": Jinja, "HTML+ERB": ERB,
		"HTML+PHP": PHP, "Matlab": MATLAB, "fish": Fish, "": Unknown, "unrecognized": Unknown,
	} {
		if got := Parse(name); got != want {
			t.Errorf("Parse(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestModelLanguages(t *testing.T) {
	for _, name := range classifier.Names {
		if Parse(name) == Unknown {
			t.Fatalf("unmapped classifier language %q", name)
		}
	}
}

func TestIndexedRulesThroughAnalyze(t *testing.T) {
	for source, id := range map[string]string{
		"SeLeCt id FrOm users;":          "sql.select",
		"<HTML lang=\"en\">":             "html.root",
		"const title: string = 'hello';": "ts.binding",
		"ancestor(X,Y) :- parent(X,Y).":  "prolog.rule",
	} {
		var a Analysis
		Analyze([]byte(source+"\n"+source+"\n"), true, &a)
		matches := 0
		for _, match := range a.Signals[:a.Count] {
			if match.Evidence().ID == id {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("%q: %d matches for %s, want one", source, matches, id)
		}
	}
}

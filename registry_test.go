package languages

import "testing"

func TestRuleRegistry(t *testing.T) {
	if len(rules) > MaxSignals || LanguageCount > 64 {
		t.Fatal("fixed capacity exceeded")
	}
	ids := make(map[string]bool)
	for _, r := range rules {
		if ids[r.id] || r.id == "" || r.description == "" || r.prefix == "" || r.languages == 0 || r.weight == 0 {
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

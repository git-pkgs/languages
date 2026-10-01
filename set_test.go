package languages_test

import (
	"encoding/json"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestExpandedLanguageSet(t *testing.T) {
	want := languages.NewSet(languages.Go, languages.Swift, languages.Zig)
	if want.Len() != 3 || !want.Has(languages.Zig) || languages.Zig < 64 {
		t.Fatal(want)
	}
	if got := want.Intersect(languages.NewSet(languages.Zig)); got.Only() != languages.Zig {
		t.Fatal(got)
	}
	if got := want.Union(languages.NewSet(languages.Go, languages.Unknown, languages.LanguageCount)); got != want {
		t.Fatal(got)
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got languages.Set
	if err := json.Unmarshal(data, &got); err != nil || got != want {
		t.Fatal(string(data), got, err)
	}
	if err := json.Unmarshal([]byte(`["Go","not a language"]`), &got); err == nil || got != want {
		t.Fatal(got, err)
	}
}

func TestRegistryCoverage(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"main.swift", "", languages.Swift},
		{"main.kt", "", languages.Kotlin},
		{"main.zig", "", languages.Zig},
		{"", "#!/usr/bin/env julia\n", languages.Julia},
		{"", "#!/usr/bin/env elixir\n", languages.Elixir},
	} {
		var a languages.Analysis
		languages.Analyze([]byte(tt.source), true, &a)
		if got := a.Detect(tt.name); got.Language != tt.want || got.Conflict {
			t.Fatal(tt.name, got)
		}
		for _, match := range a.Signals[:a.Count] {
			if evidence := match.Evidence(); !evidence.Languages.Has(tt.want) || evidence.ID == "" {
				t.Fatal(evidence)
			}
		}
	}
}

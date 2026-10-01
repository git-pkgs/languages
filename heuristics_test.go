package languages_test

import (
	"testing"

	"github.com/git-pkgs/languages"
)

func TestExtensionHeuristics(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         languages.Language
	}{
		{"person.json", `{"name":"Ada","description":"API client","version":"3.0.0"}`, languages.JSON},
		{"api.json", `{"openapi":"3.1.0","info":{"title":"Example","version":"1"},"paths":{}}`, languages.OASv3Json},
		{"api.json", `{"swagger":"2.0","info":{"title":"Example","version":"1"},"paths":{}}`, languages.OASv2Json},
		{"settings.yaml", "name: Ada\nretries: 3\n", languages.YAML},
		{"project/.releaserc", "branches:\n  - main\nplugins:\n  - '@semantic-release/commit-analyzer'\n", languages.YAML},
		{"project/.releaserc", "{\n  \"branches\": [\"main\"],\n  \"plugins\": []\n}\n", languages.JSON},
		{"api.yml", "openapi: 3.1.0\ninfo:\n  title: Example\npaths: {}\n", languages.OASv3Yaml},
		{"entities.yaml", "Tank:\n\tHealth:\n\t\tHP: 100\n", languages.MiniYAML},
		{"readme.md", "# Example\n\nRun the command.\n", languages.Markdown},
		{"notes.txt", "A short description of the project.\n", languages.Text},
		{"counter.v", "module counter(input clk, output reg [7:0] value);\nalways @(posedge clk) value <= value + 1;\nendmodule\n", languages.Verilog},
	} {
		t.Run(tt.name+"/"+tt.want.String(), func(t *testing.T) {
			data := []byte(tt.source)
			if got := languages.Detect(tt.name, data); got.Language != tt.want || got.Conflict {
				t.Fatalf("got %+v, want %s", got, tt.want)
			}
			var a languages.Analysis
			languages.Analyze(data, true, &a)
			before := a
			clear(data)
			if got := a.Detect(tt.name); got.Language != tt.want || got.Conflict || a != before {
				t.Fatalf("cached detection: %+v", got)
			}
		})
	}
}

func TestHeuristicsPreserveDeclarations(t *testing.T) {
	data := []byte("# -*- ruby -*-\n{\"openapi\":\"3.1.0\"}\n")
	if got := languages.Detect("api.json", data); !got.Conflict || got.Language != languages.Unknown {
		t.Fatal(got)
	}
}

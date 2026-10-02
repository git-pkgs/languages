package languages_test

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/git-pkgs/languages"
)

func TestStartupAllocations(t *testing.T) {
	if os.Getenv("LANGUAGES_TEST_STARTUP") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStartupAllocations$")
		cmd.Env = append(os.Environ(), "LANGUAGES_TEST_STARTUP=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("startup: %v\n%s", err, output)
		}
		return
	}
	for _, source := range [][]byte{nil, []byte("package main\n")} {
		if got := languages.Detect("main.go", source); got.Language != languages.Go {
			t.Fatal(got)
		}
	}
	const startupBudget = 16 << 20
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	if memory.TotalAlloc > startupBudget {
		t.Fatalf("startup and unambiguous detection allocated %d bytes, budget %d", memory.TotalAlloc, startupBudget)
	}
	var analysis languages.Analysis
	languages.Analyze([]byte(`{"openapi":"3.1.0"}`), true, &analysis)
	if got := analysis.Detect("api.json"); got.Language != languages.OASv3Json {
		t.Fatal(got)
	}
}

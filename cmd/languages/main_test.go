package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainProcess(t *testing.T) {
	if os.Getenv("LANGUAGES_TEST_MAIN") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append(os.Args[:1], os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	t.Fatal("missing command arguments")
}

func TestCLIErrorEscaping(t *testing.T) {
	for _, args := range [][]string{
		{filepath.Join(t.TempDir(), "\x1b[31mmissing\x1b[0m\n.go")},
		{"-\x1b[31minvalid\x1b[0m\n"},
		{"-bytes", "\x1b[31minvalid\x1b[0m\n"},
	} {
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestMainProcess$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "LANGUAGES_TEST_MAIN=1")
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("accepted invalid arguments %q", args)
		}
		text := strings.TrimSuffix(string(output), "\n")
		if strings.ContainsAny(text, "\x1b\n\r") || !strings.Contains(text, `\x1b`) {
			t.Fatalf("unsafe diagnostic: %q", output)
		}
	}
}

func TestCLIHelp(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainProcess$", "--", "-help")
	cmd.Env = append(os.Environ(), "LANGUAGES_TEST_MAIN=1")
	output, _ := cmd.CombinedOutput()
	if !strings.Contains(string(output), "Usage of languages:\n") || !strings.Contains(string(output), "-bytes") {
		t.Fatalf("missing help: %q", output)
	}
}

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{
		"Go/main.go":     "package main\nfunc main() {}\n",
		"Python/main.py": "def main():\n    print(1)\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), fileMode); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCommandGeneratesModel(t *testing.T) {
	root := fixture(t)
	prefix := bytes.Repeat([]byte("package main\n"), trainingBytes/len("package main\n")+1)[:trainingBytes]
	source := append(bytes.Clone(prefix), []byte("\nBeyondTrainingPrefix\n")...)
	if err := os.WriteFile(filepath.Join(root, "Go", "large.go"), source, fileMode); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "model")
	command := exec.Command("go", "run", ".", "-corpus", root, "-revision-file", "../../tools/registrygen/revision", "-out", output)
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	for _, name := range []string{"index.bin", "tokens.bin", "weights.bin", "model_generated.go", "training.json"} {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(output, "training.json"))
	if err != nil || !bytes.Contains(manifest, []byte(`"prefix_sha256"`)) {
		t.Fatal(string(manifest), err)
	}
	var metadata struct {
		ByteLimit int      `json:"byte_limit"`
		Samples   []sample `json:"samples"`
	}
	if err := json.Unmarshal(manifest, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.ByteLimit != trainingBytes {
		t.Fatal(metadata.ByteLimit)
	}
	found := false
	for _, item := range metadata.Samples {
		if item.Path == "Go/large.go" {
			found = true
			if item.Bytes != trainingBytes {
				t.Fatal(item)
			}
		}
	}
	if !found {
		t.Fatal("large sample missing from manifest")
	}
	tokens, err := os.ReadFile(filepath.Join(output, "tokens.bin"))
	if err != nil || bytes.Contains(tokens, []byte("BeyondTrainingPrefix")) {
		t.Fatal("training consumed bytes beyond its budget", err)
	}
}

func TestReproducibleModel(t *testing.T) {
	trained, err := train(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := trained.generate("fixture")
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		got, err := trained.generate("fixture")
		if err != nil {
			t.Fatal(err)
		}
		for name, want := range first {
			if !bytes.Equal(got[name], want) {
				t.Fatalf("nondeterministic %s", name)
			}
		}
	}
	index := first["index.bin"]
	weights := first["weights.bin"]
	for i := 0; i < len(index); i += 16 {
		offset := binary.LittleEndian.Uint32(index[i+8:])
		count := binary.LittleEndian.Uint32(index[i+12:])
		if int(offset+count*4) > len(weights) {
			t.Fatal("posting extends beyond weights")
		}
	}
}

func TestRejectsUnknownLabels(t *testing.T) {
	root := fixture(t)
	if err := os.Mkdir(filepath.Join(root, "Unmapped"), directoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Unmapped", "sample"), []byte("content"), fileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := train(root); err == nil {
		t.Fatal("accepted an unmapped label")
	}
}

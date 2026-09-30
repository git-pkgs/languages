package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestProtocol(t *testing.T) {
	var input, output bytes.Buffer
	for _, req := range []request{{Mode: "content", Content: []byte("#!/usr/bin/env ruby\n")}, {Mode: "path", Name: "test.go"}} {
		if err := json.NewEncoder(&input).Encode(req); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(&input, &output); err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(&output)
	for _, want := range []string{"Ruby", "Go"} {
		var resp response
		if err := d.Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if resp.Language != want {
			t.Fatal(resp)
		}
	}
}

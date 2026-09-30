package main

import "testing"

func TestMeasure(t *testing.T) {
	m := measure(100)
	if m.Objects != 100 || m.Detected != 84 || m.ElapsedNS <= 0 {
		t.Fatal(m)
	}
}

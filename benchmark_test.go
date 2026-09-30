package languages_test

import (
	"bytes"
	"github.com/git-pkgs/languages"
	"testing"
)

func benchmarkInputs() [][]byte {
	var result [][]byte
	for _, s := range []string{"package main\nimport (\"fmt\")\nfunc main() {}\n", "#!/usr/bin/env ruby\n# frozen_string_literal: true\nrequire 'json'\n", "/* license */\n#include <stdio.h>\nint main() {}\n", "const count = 1;\n", ":- module(family, [ancestor/2]).\nancestor(X,Y) :- parent(X,Y).\n", "The quick brown fox.\n"} {
		b := bytes.Repeat([]byte(" "), 1024)
		copy(b, s)
		result = append(result, b)
	}
	return result
}

func BenchmarkAnalyze1KB(b *testing.B) {
	inputs := benchmarkInputs()
	var a languages.Analysis
	b.ReportAllocs()
	b.SetBytes(1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		languages.Analyze(inputs[i%len(inputs)], false, &a)
		_ = a.Result()
	}
}

func BenchmarkDetect1KB(b *testing.B) {
	inputs := benchmarkInputs()
	names := [...]string{"main.go", "script.rb", "main.c", "app.js", "family.pl", ""}
	b.ReportAllocs()
	b.SetBytes(languages.DefaultBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index := i % len(inputs)
		_ = languages.Detect(names[index], inputs[index])
	}
}

func BenchmarkResultPath(b *testing.B) {
	data := bytes.Repeat([]byte("# ordinary source line\n"), 64)[:languages.DefaultBytes]
	copy(data, "#!/usr/bin/python3\n# SPDX-License-Identifier: MIT\nprint(1)\n")
	context := languages.AnalyzePath("script.py")
	for _, mode := range []string{"extract", "intrinsic", "combined", "intrinsic-and-combined", "cached-results"} {
		b.Run(mode, func(b *testing.B) {
			var a languages.Analysis
			languages.Analyze(data, false, &a)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if mode != "cached-results" {
					languages.Analyze(data, false, &a)
				}
				if mode == "intrinsic" || mode == "intrinsic-and-combined" || mode == "cached-results" {
					_ = a.Result()
				}
				if mode == "combined" || mode == "intrinsic-and-combined" || mode == "cached-results" {
					_ = languages.Combine(&a, context)
				}
			}
		})
	}
}

func BenchmarkBatch1KB(b *testing.B) {
	inputs := benchmarkInputs()
	var a languages.Analysis
	const batch = 1024
	b.ReportAllocs()
	b.SetBytes(batch * 1024)
	b.ResetTimer()
	for range b.N {
		for i := range batch {
			languages.Analyze(inputs[i%len(inputs)], false, &a)
			_ = a.Result()
		}
	}
}

func BenchmarkParallel1KB(b *testing.B) {
	inputs := benchmarkInputs()
	b.ReportAllocs()
	b.SetBytes(1024)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var a languages.Analysis
		i := 0
		for pb.Next() {
			languages.Analyze(inputs[i%len(inputs)], false, &a)
			_ = a.Result()
			i++
		}
	})
}

func BenchmarkAdversarial(b *testing.B) {
	for _, pattern := range []string{"f\n", "\\\"", "a", "/*"} {
		b.Run(pattern, func(b *testing.B) {
			input := bytes.Repeat([]byte(pattern), languages.MaxBytes/len(pattern))
			var a languages.Analysis
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for range b.N {
				languages.Analyze(input, false, &a)
				_ = a.Result()
			}
		})
	}
}

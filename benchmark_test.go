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

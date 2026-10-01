// benchbatch measures retained heap while scanning without retaining results.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/git-pkgs/languages"
)

type measurement struct {
	Objects        int    `json:"objects"`
	ElapsedNS      int64  `json:"elapsed_ns"`
	HeapBefore     uint64 `json:"heap_before"`
	HeapAfter      uint64 `json:"heap_after"`
	MaxSampledHeap uint64 `json:"max_sampled_heap"`
	Allocated      uint64 `json:"allocated_bytes"`
	Mallocs        uint64 `json:"allocations"`
	Detected       int    `json:"detected"`
}

func measure(n int) measurement {
	sources := [...]string{"package main\nfunc main() {}\n", "#!/usr/bin/ruby\nrequire 'json'\n", "#include <stdio.h>\nint main() {}\n", "const count = 1;\n", ":- module(family, [ancestor/2]).\n", "A plain text example.\n"}
	var buffers [len(sources)][1024]byte
	for i := range buffers {
		for j := range buffers[i] {
			buffers[i][j] = ' '
		}
		copy(buffers[i][:], sources[i])
	}
	var a languages.Analysis
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	m := measurement{Objects: n, HeapBefore: before.HeapAlloc, MaxSampledHeap: before.HeapAlloc}
	for i := range n {
		languages.Analyze(buffers[i%len(buffers)][:], false, &a)
		if !a.Result().Candidates.Empty() {
			m.Detected++
		}
		if i%10000 == 0 {
			runtime.ReadMemStats(&after)
			m.MaxSampledHeap = max(m.MaxSampledHeap, after.HeapAlloc)
		}
	}
	m.ElapsedNS = time.Since(start).Nanoseconds()
	runtime.ReadMemStats(&after)
	m.HeapAfter = after.HeapAlloc
	m.Allocated = after.TotalAlloc - before.TotalAlloc
	m.Mallocs = after.Mallocs - before.Mallocs
	return m
}

func main() {
	const defaultObjects = 1000000
	n := flag.Int("n", defaultObjects, "number of 1 KB objects to scan")
	flag.Parse()
	if *n < 1 {
		fmt.Fprintln(os.Stderr, "n must be positive")
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(measure(*n)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

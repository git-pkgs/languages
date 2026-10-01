package languages

import (
	"runtime"
	"testing"

	"github.com/git-pkgs/scan"
)

func BenchmarkHeuristicCompilation(b *testing.B) {
	for _, name := range []string{"shared", "all-groups", "typescript"} {
		b.Run(name, func(b *testing.B) {
			compile := func() []*heuristicScanner {
				switch name {
				case "shared":
					return []*heuristicScanner{compileHeuristics(nil)}
				case "typescript":
					return []*heuristicScanner{compileHeuristics(&heuristicGroups[pathHeuristic("app.ts")-1])}
				default:
					scanners := make([]*heuristicScanner, len(heuristicGroups))
					for i := range scanners {
						scanners[i] = compileHeuristics(&heuristicGroups[i])
					}
					return scanners
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				runtime.KeepAlive(compile())
			}
			runtime.GC()
			var before, databases, scratch runtime.MemStats
			runtime.ReadMemStats(&before)
			scanners := compile()
			runtime.GC()
			runtime.ReadMemStats(&databases)
			scratches := make([]*scan.Scratch, len(scanners))
			for i, scanner := range scanners {
				scratches[i] = scan.NewScratch(scanner.database)
			}
			runtime.GC()
			runtime.ReadMemStats(&scratch)
			runtime.KeepAlive(scanners)
			runtime.KeepAlive(scratches)
			b.ReportMetric(float64(databases.HeapAlloc)-float64(before.HeapAlloc), "database-B")
			b.ReportMetric(float64(scratch.HeapAlloc)-float64(databases.HeapAlloc), "scratch-B")
		})
	}
}

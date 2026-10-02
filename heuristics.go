package languages

import (
	"strings"
	"sync"

	"github.com/git-pkgs/scan"
)

const (
	heuristicBytes = 50 * 1024
	allHeuristics  = uint16(len(heuristicGroups) + 1)
)

type heuristicGroup struct {
	extensions []string
	rules      []heuristicRule
}

type heuristicRule struct {
	languages  Set
	conditions []heuristicCondition
}

type heuristicCondition struct {
	pattern  uint16
	negative bool
}

type heuristicScanner struct {
	database *scan.Database
	scratch  sync.Pool
}

var sharedHeuristics = sync.OnceValue(func() *heuristicScanner { return compileHeuristics(nil) })

var groupHeuristics = func() [len(heuristicGroups)]func() *heuristicScanner {
	var scanners [len(heuristicGroups)]func() *heuristicScanner
	for i := range scanners {
		scanners[i] = sync.OnceValue(func() *heuristicScanner {
			return compileHeuristics(&heuristicGroups[i])
		})
	}
	return scanners
}()

func compileHeuristics(group *heuristicGroup) *heuristicScanner {
	var selected [len(heuristicPatterns)]bool
	if group != nil {
		for _, rule := range group.rules {
			for _, condition := range rule.conditions {
				selected[condition.pattern] = true
			}
		}
	}
	var patterns []*scan.Pattern
	for i, expression := range heuristicPatterns {
		if group == nil || selected[i] {
			patterns = append(patterns, &scan.Pattern{Expression: expression, ID: uint(i), Flags: scan.SingleMatch | scan.AllowEmpty})
		}
	}
	database, err := scan.Compile(patterns...)
	if err != nil {
		panic(err)
	}
	return &heuristicScanner{database: database, scratch: sync.Pool{New: func() any { return scan.NewScratch(database) }}}
}

func analyzeHeuristics(data []byte, dst *Analysis, selection uint16) {
	if len(data) == 0 || selection == 0 {
		return
	}
	data = data[:min(len(data), heuristicBytes)]
	var scanner *heuristicScanner
	first, end := 0, len(heuristicGroups)
	if selection == allHeuristics {
		scanner = sharedHeuristics()
	} else {
		first, end = int(selection)-1, int(selection)
		scanner = groupHeuristics[first]()
	}
	scratch := scanner.scratch.Get().(*scan.Scratch)
	defer scanner.scratch.Put(scratch)
	var matches [len(heuristicPatterns)]bool
	if err := scanner.database.Scan(data, scratch, func(match scan.Match) error {
		matches[match.ID] = true
		return nil
	}); err != nil {
		panic(err)
	}
	for i := first; i < end; i++ {
		group := heuristicGroups[i]
		for j, rule := range group.rules {
			if matchesHeuristic(rule, &matches) {
				dst.heuristics[i] = uint16(j + 1)
				break
			}
		}
	}
}

func matchesHeuristic(rule heuristicRule, matches *[len(heuristicPatterns)]bool) bool {
	for _, condition := range rule.conditions {
		if matches[condition.pattern] == condition.negative {
			return false
		}
	}
	return true
}

func pathHeuristic(path string) uint16 {
	path = strings.ToLower(path)
	for i, group := range heuristicGroups {
		for _, extension := range group.extensions {
			if strings.HasSuffix(path, extension) {
				return uint16(i + 1)
			}
		}
	}
	return 0
}

func (a *Analysis) heuristicResult(context Context) Result {
	if context.heuristic == 0 {
		return Result{Confidence: None}
	}
	index := context.heuristic - 1
	match := a.heuristics[index]
	if match == 0 {
		return Result{Confidence: None}
	}
	candidates := heuristicGroups[index].rules[match-1].languages
	return Result{Language: candidates.Only(), Candidates: candidates, Confidence: Medium}
}

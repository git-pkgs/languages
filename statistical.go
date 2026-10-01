package languages

import "github.com/git-pkgs/languages/internal/classifier"

const minimumClassifierTokens = 12
const minimumOverrideTokens = 32
const classifierMargin = 256

var classifierLanguages = func() (result [len(classifier.Names)]Language) {
	for i, name := range classifier.Names {
		result[i] = Parse(name)
	}
	return result
}()

func (a *Analysis) classifiedContext(context Context, intrinsic Result) (Result, bool) {
	classified := a.classify(Set{})
	if classified.Language == Unknown || !context.Candidates.Has(classified.Language) {
		return Result{}, false
	}
	if intrinsic.Language == classified.Language && intrinsic.Confidence.Rank() > classified.Confidence.Rank() {
		return intrinsic, true
	}
	return classified, true
}

func (a *Analysis) hasDeclaration() bool {
	for _, match := range a.Signals[:a.Count] {
		if match.rule().weight == declaredWeight {
			return true
		}
	}
	return false
}

func (a *Analysis) classify(candidates Set) Result {
	if candidates.Empty() && a.classification.Tokens < minimumClassifierTokens {
		return Result{Confidence: None}
	}
	first, _ := a.classification.Best(func(i int) bool {
		return candidates.Empty() || candidates.Has(classifierLanguages[i])
	})
	if first < 0 {
		return Result{Confidence: None}
	}
	best := a.classification.Scores[first]
	var selected Set
	for i, score := range a.classification.Scores {
		language := classifierLanguages[i]
		if score+classifierMargin >= best && (candidates.Empty() || candidates.Has(language)) {
			selected.add(language)
		}
	}
	return Result{Language: selected.Only(), Candidates: selected, Confidence: Low, Statistical: true}
}

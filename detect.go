package languages

// Detect identifies source from a filename and the supplied content.
// Either input may be empty. Content is treated as a prefix; use Analyze with
// complete=true and Analysis.Detect when the supplied content is complete.
func Detect(filename string, content []byte) Result {
	context := AnalyzePath(filename)
	var a Analysis
	analyze(content, false, &a, context.heuristic)
	return Combine(&a, context)
}

// Detect combines reusable content evidence with an optional filename.
// It does not change the analysis or retain the filename.
func (a *Analysis) Detect(filename string) Result {
	return Combine(a, AnalyzePath(filename))
}

package languages

// Detect identifies source from a filename and up to MaxBytes of content.
// Either input may be empty. Content is treated as a prefix; use Analyze with
// complete=true and Analysis.Detect when the supplied content is complete.
func Detect(filename string, content []byte) Result {
	var a Analysis
	Analyze(content, false, &a)
	return a.Detect(filename)
}

// Detect combines reusable content evidence with an optional filename.
// It does not change the analysis or retain the filename.
func (a *Analysis) Detect(filename string) Result {
	return Combine(a, AnalyzePath(filename))
}

package languages

import (
	"bytes"
	"math/bits"
	"sync"
	"unicode/utf8"

	"github.com/git-pkgs/languages/internal/classifier"
)

const utf8BOMBytes = len("\xef\xbb\xbf")

var textStreams = sync.Pool{New: func() any { return new(textStream) }}

type modelineMatch struct {
	language Language
	offset   uint64
}

type textStream struct {
	analysis               Analysis
	classifier             classifier.Stream
	declared               bool
	heuristics             [heuristicBytes]byte
	heuristicSize          int
	prefix                 [utf8BOMBytes]byte
	prefixOffsets          [utf8BOMBytes + 1]uint64
	prefixSize, skipBOM    int
	ready                  bool
	header                 headerStream
	headerOffset           uint64
	lexical                lexicalState
	line                   lineSummary
	lineNumber, lineSize   int64
	lineStart, codeOffset  uint64
	linePresent, codeFound bool
	codeTail               [len(`"""`)]byte
	codeSources            [len(`"""`)]uint64
	codeTailSize           int
	firstCode              [len("#!")]byte
	firstCodeSize          int
	seen                   [2]uint64
	emacs                  emacsStream
	vim                    vimStream
	lineMode               Language
	modeComplete           bool
	vimball                lineTail
	modelinesSuppressed    bool
	firstModeline          modelineMatch
	lastModelines          [modelineLines]modelineMatch
	tailCount, tailNext    int
}

func (s *textStream) reset() {
	stream := s.classifier
	*s = textStream{classifier: stream}
	s.classifier.Reset(&s.analysis.classification)
	s.lexical.startLine()
}

func (s *textStream) write(data []byte, sourceAt func(int) uint64) {
	if !s.declared {
		s.classifier.Write(data)
	}
	s.heuristicSize += copy(s.heuristics[s.heuristicSize:], data)
	if s.ready {
		s.content(data, sourceAt)
		return
	}
	n := min(len(data), len(s.prefix)-s.prefixSize)
	for i := 0; i <= n; i++ {
		s.prefixOffsets[s.prefixSize+i] = sourceAt(i)
	}
	s.prefixSize += copy(s.prefix[s.prefixSize:], data[:n])
	if s.prefixSize < len(s.prefix) {
		return
	}
	s.begin()
	s.content(data[n:], func(i int) uint64 { return sourceAt(n + i) })
}

func (s *textStream) begin() {
	s.ready = true
	s.lineStart, s.headerOffset = s.prefixOffsets[0], s.prefixOffsets[0]
	if string(s.prefix[:s.prefixSize]) == "\xef\xbb\xbf" {
		s.skipBOM = utf8BOMBytes
		s.headerOffset = s.prefixOffsets[utf8BOMBytes]
	}
	s.content(s.prefix[:s.prefixSize], func(i int) uint64 { return s.prefixOffsets[i] })
}

func (s *textStream) content(data []byte, sourceAt func(int) uint64) {
	position := 0
	for len(data) > 0 {
		line, rest, ended := bytes.Cut(data, []byte("\n"))
		if len(line) > 0 {
			s.modeline(line, ended && !s.linePresent)
			s.linePresent = true
		}
		skip := min(s.skipBOM, len(line))
		s.skipBOM -= skip
		s.code(line[skip:], func(i int) uint64 { return sourceAt(position + skip + i) })
		if s.header.lines < modelineLines {
			s.header.write(line[skip:])
			if ended {
				s.header.finishLine(true)
			}
		}
		if !ended {
			return
		}
		s.endLine(true)
		position += len(line) + 1
		s.lineStart = sourceAt(position)
		s.lineNumber++
		if s.lineNumber == modelineLines {
			// Wrappers and Vimball suppression can change declarations within the header.
			s.declared = s.header.finish(false) >= 0 || s.modelineResult(false).language != Unknown
		}
		s.resetLine()
		data = rest
	}
}

func (s *textStream) modeline(data []byte, complete bool) {
	if complete {
		s.lineMode = emacsLineMode(data)
		if s.lineMode == Unknown {
			s.lineMode = vimLineMode(data)
		}
		s.modeComplete = true
	} else {
		s.emacs.write(data)
		s.vim.write(data)
	}
	if s.lineNumber >= modelineLines || s.modelinesSuppressed {
		return
	}
	const marker = "UseVimball"
	var boundary [2 * ruleWindow]byte
	n := copy(boundary[:], s.vimball.data[:s.vimball.size])
	n += copy(boundary[n:], data[:min(len(data), ruleWindow)])
	s.modelinesSuppressed = bytes.Contains(boundary[:n], []byte(marker)) || bytes.Contains(data, []byte(marker))
	s.vimball.append(data)
}

func (s *textStream) code(data []byte, sourceAt func(int) uint64) {
	if s.lineNumber == 0 {
		s.firstCodeSize += copy(s.firstCode[s.firstCodeSize:], data)
	}
	s.lexical.write(data)
	switch {
	case s.codeFound:
		s.line.write(data)
	case s.lexical.start >= 0:
		s.codeFound = true
		start := s.lexical.start - s.lineSize
		if start < 0 {
			s.startFromTail(int(-start))
			start = 0
		} else {
			s.codeOffset = sourceAt(int(start))
		}
		s.line.write(data[start:])
	}
	s.retainTail(data, sourceAt)
	s.lineSize += int64(len(data))
}

func (s *textStream) startFromTail(n int) {
	start := s.codeTailSize - n
	s.codeOffset = s.codeSources[start]
	s.line.write(s.codeTail[start:s.codeTailSize])
}

func (s *textStream) retainTail(data []byte, sourceAt func(int) uint64) {
	if len(data) >= len(s.codeTail) {
		start := len(data) - len(s.codeTail)
		s.codeTailSize = copy(s.codeTail[:], data[start:])
		for i := range s.codeSources {
			s.codeSources[i] = sourceAt(start + i)
		}
		return
	}
	keep := min(s.codeTailSize, len(s.codeTail)-len(data))
	copy(s.codeTail[:], s.codeTail[s.codeTailSize-keep:s.codeTailSize])
	copy(s.codeSources[:], s.codeSources[s.codeTailSize-keep:s.codeTailSize])
	s.codeTailSize = keep + copy(s.codeTail[keep:], data)
	for i := range data {
		s.codeSources[keep+i] = sourceAt(i)
	}
}

func (s *textStream) endLine(terminated bool) {
	start := s.lexical.finishLine()
	if !s.codeFound && start >= 0 {
		s.codeFound = true
		s.startFromTail(int(s.lineSize - start))
	}
	if s.codeFound && (s.lineNumber != 0 || string(s.firstCode[:s.firstCodeSize]) != "#!") {
		for word, candidates := range s.line.finish(terminated) {
			candidates &^= s.seen[word]
			for candidates != 0 {
				i := word*wordBits + bits.TrailingZeros64(candidates)
				candidates &= candidates - 1
				s.seen[word] |= 1 << uint(i%wordBits)
				s.analysis.Signals[s.analysis.Count] = Match{Rule: uint16(i), Offset: s.codeOffset}
				s.analysis.Count++
			}
		}
	}
	if !terminated {
		return
	}
	mode := s.lineMode
	if !s.modeComplete {
		mode = s.emacs.result()
		if mode == Unknown {
			mode = s.vim.finish()
		}
	}
	match := modelineMatch{language: mode, offset: s.lineStart}
	if s.lineNumber < modelineLines {
		if s.firstModeline.language == Unknown {
			s.firstModeline = match
		}
	} else {
		s.lastModelines[s.tailNext] = match
		s.tailNext = (s.tailNext + 1) % modelineLines
		s.tailCount = min(s.tailCount+1, modelineLines)
	}
}

func (s *textStream) resetLine() {
	s.line, s.vimball = lineSummary{}, lineTail{}
	if !s.modeComplete {
		s.emacs, s.vim = emacsStream{}, vimStream{}
	}
	s.lineMode, s.modeComplete = Unknown, false
	s.lexical.startLine()
	s.lineSize, s.codeTailSize = 0, 0
	s.linePresent, s.codeFound = false, false
}

func (s *textStream) finish(complete bool, heuristic uint16) Analysis {
	if !s.ready {
		s.begin()
	}
	if s.linePresent {
		s.endLine(complete)
	}
	if rule := s.header.finish(complete); rule >= 0 {
		s.prepend(Match{Rule: uint16(rule), Offset: s.headerOffset})
	}
	if mode := s.modelineResult(complete); mode.language != Unknown {
		s.prepend(Match{Rule: uint16(len(rules) + len(interpreterRules) + int(mode.language) - 1), Offset: mode.offset})
	}
	s.analysis.Prefix = !complete
	if s.analysis.hasDeclaration() {
		s.analysis.classification = classifier.Analysis{}
	} else {
		s.classifier.Finish()
		analyzeHeuristics(s.heuristics[:s.heuristicSize], &s.analysis, heuristic)
	}
	return s.analysis
}

func (s *textStream) prepend(match Match) {
	copy(s.analysis.Signals[1:], s.analysis.Signals[:s.analysis.Count])
	s.analysis.Signals[0] = match
	s.analysis.Count++
}

func (s *textStream) modelineResult(complete bool) modelineMatch {
	if s.modelinesSuppressed {
		return modelineMatch{}
	}
	if s.firstModeline.language != Unknown {
		return s.firstModeline
	}
	if complete {
		for i := 0; i < s.tailCount; i++ {
			index := (s.tailNext - s.tailCount + i + modelineLines) % modelineLines
			if match := s.lastModelines[index]; match.language != Unknown {
				return match
			}
		}
	}
	return modelineMatch{}
}

type sourceMap struct {
	data                                 []byte
	encoding                             string
	base                                 uint64
	text, source, textStart, sourceStart int
}

func (m *sourceMap) at(offset int) uint64 {
	if m.encoding == "" {
		return m.base + uint64(offset)
	}
	if offset < m.textStart {
		m.text, m.source, m.textStart, m.sourceStart = 0, 0, 0, 0
	}
	for m.text < offset {
		r, width, _ := encodedRune(m.data[m.source:], m.encoding)
		m.textStart, m.sourceStart = m.text, m.source
		m.text += utf8.RuneLen(r)
		m.source += width
	}
	if offset < m.text {
		return m.base + uint64(m.sourceStart)
	}
	return m.base + uint64(m.source)
}

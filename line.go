package languages

import (
	"bytes"
	"math/bits"
	"unicode"
	"unicode/utf8"
)

const ruleWindow = 32

type lineTail struct {
	data [ruleWindow]byte
	size int
}

func (t *lineTail) append(data []byte) {
	if len(data) >= len(t.data) {
		t.size = copy(t.data[:], data[len(data)-len(t.data):])
		return
	}
	keep := min(t.size, len(t.data)-len(data))
	copy(t.data[:], t.data[t.size-keep:t.size])
	t.size = keep + copy(t.data[keep:], data)
}

type lineSummary struct {
	head          lineTail
	tail, rawTail lineTail
	size, trimmed int64
	candidates    [2]uint64
	containsEnd   [len(rules)]int64
	present       [4]uint64
	initialized   bool
	previous      byte
	prologOpen    bool
	prologBad     bool
	prologFound   bool
	prologMatch   bool
	special       lineConstraint
}

func (s *lineSummary) write(data []byte) {
	if !s.initialized {
		n := min(len(data), ruleWindow-s.head.size)
		s.head.size += copy(s.head.data[s.head.size:], data[:n])
		data = data[n:]
		if s.head.size < ruleWindow {
			return
		}
		s.initialize()
	}
	s.consume(data)
}

func (s *lineSummary) initialize() {
	s.initialized = true
	if s.head.size == 0 {
		return
	}
	head := s.head.data[:s.head.size]
	s.candidates = ruleStarts[head[0]]
	for word, candidates := range s.candidates {
		for candidates != 0 {
			i := word*wordBits + bits.TrailingZeros64(candidates)
			candidates &= candidates - 1
			r := &rules[i]
			if r.weight == declaredWeight || r.prefix[0] != '~' && !rulePrefix(head, r) {
				s.candidates[word] &^= 1 << uint(i%wordBits)
			}
		}
	}
	s.special.initialize(head)
	s.consume(head)
}

func rulePrefix(data []byte, r *rule) bool {
	if len(data) < len(r.prefix) {
		return false
	}
	if r.languages == NewSet(HTML) || r.languages == NewSet(SQL) {
		return bytes.EqualFold(data[:len(r.prefix)], []byte(r.prefix))
	}
	return bytes.HasPrefix(data, []byte(r.prefix))
}

func (s *lineSummary) consume(data []byte) {
	if len(data) == 0 {
		return
	}
	s.findContents(data)
	trimmed := bytes.TrimRight(data, " \t\r")
	if len(trimmed) > 0 {
		s.trimmed = s.size + int64(len(trimmed))
		s.tail = s.rawTail
		s.tail.append(trimmed)
	}
	s.rawTail.append(data)
	for _, b := range data {
		s.present[b/wordBits] |= 1 << uint(b%wordBits)
		s.prologByte(b)
	}
	s.special.write(data, false)
	s.size += int64(len(data))
}

func (s *lineSummary) findContents(data []byte) {
	var boundary [2 * ruleWindow]byte
	n := copy(boundary[:], s.rawTail.data[:s.rawTail.size])
	n += copy(boundary[n:], data[:min(len(data), ruleWindow)])
	for word, candidates := range s.candidates {
		for candidates != 0 {
			i := word*wordBits + bits.TrailingZeros64(candidates)
			candidates &= candidates - 1
			r := &rules[i]
			if r.contains == "" || s.containsEnd[i] > 0 {
				continue
			}
			fold := r.languages == NewSet(HTML) || r.languages == NewSet(SQL)
			if at := contentIndex(boundary[:n], r.contains, fold); at >= 0 {
				s.containsEnd[i] = s.size - int64(s.rawTail.size) + int64(at+len(r.contains))
			} else if at := contentIndex(data, r.contains, fold); at >= 0 {
				s.containsEnd[i] = s.size + int64(at+len(r.contains))
			}
		}
	}
}

func contentIndex(data []byte, pattern string, fold bool) int {
	if !fold {
		return bytes.Index(data, []byte(pattern))
	}
	for i := 0; i+len(pattern) <= len(data); i++ {
		if bytes.EqualFold(data[i:i+len(pattern)], []byte(pattern)) {
			return i
		}
	}
	return -1
}

func (s *lineSummary) prologByte(b byte) {
	if !s.prologFound {
		if s.previous == ':' && b == '-' {
			s.prologFound = true
			s.prologMatch = s.prologOpen && !s.prologBad
		}
		s.prologOpen = s.prologOpen || b == '('
		s.prologBad = s.prologBad || b == '\'' || b == '"' || b == '='
	}
	s.previous = b
}

func (s *lineSummary) finish(terminated bool) [2]uint64 {
	if !s.initialized {
		s.initialize()
	}
	s.special.write(nil, true)
	var matched [2]uint64
	if s.trimmed < minSignalBytes {
		return matched
	}
	for word, candidates := range s.candidates {
		for candidates != 0 {
			i := word*wordBits + bits.TrailingZeros64(candidates)
			candidates &= candidates - 1
			if s.matches(i, terminated) {
				matched[word] |= 1 << uint(i%wordBits)
			}
		}
	}
	return matched
}

func (s *lineSummary) matches(index int, terminated bool) bool {
	r := &rules[index]
	if r.prefix == prologPrefix {
		return s.head.data[0] >= 'a' && s.head.data[0] <= 'z' && s.prologMatch
	}
	if r.prefix == typedPrefix {
		return s.special.kind == constraintTyped && s.special.match
	}
	if s.trimmed < int64(len(r.prefix)) || r.contains != "" && (s.containsEnd[index] == 0 || s.containsEnd[index] > s.trimmed) {
		return false
	}
	if !bytes.HasSuffix(s.tail.data[:s.tail.size], []byte(r.suffix)) {
		return false
	}
	return s.constraint(r, terminated)
}

func (s *lineSummary) hasAny(chars string) bool {
	for i := range chars {
		b := chars[i]
		if s.present[b/wordBits]&(1<<uint(b%wordBits)) != 0 {
			return true
		}
	}
	return false
}

func (s *lineSummary) constraint(r *rule, terminated bool) bool {
	switch r.id {
	case rubyRequireID, matlabFunctionID:
		return s.special.match && !s.special.invalid
	case rubyDefID:
		return !s.hasAny(":{")
	case rubyModuleID:
		b := s.head.data[len(r.prefix)]
		return s.trimmed > int64(len(r.prefix)) && b >= 'A' && b <= 'Z' && !s.hasAny(";{=./")
	case pythonImportID:
		return terminated && !s.hasAny(";\"'(){}") && (s.special.fromEnd == 0 || s.special.fromEnd > s.trimmed)
	case goFuncID, swiftFuncID, goSwiftFuncID:
		return s.special.function(terminated) == r.languages
	case javaPackageID, javaImportID:
		return !s.hasAny(":\"'{}()=")
	case racketLangID:
		return s.special.racket()
	case goPackageID:
		return !s.hasAny(";{")
	case htmlRootID, htmlDoctypeID, phpOpenID, hackOpenID:
		b := s.head.data[len(r.prefix)]
		return s.trimmed == int64(len(r.prefix)) && terminated || s.trimmed > int64(len(r.prefix)) && (space(b) || b == '>')
	case cIncludeID:
		b := s.head.data[len(r.prefix)]
		return s.trimmed > int64(len(r.prefix)) && (space(b) || b == '<' || b == '"')
	}
	return true
}

type constraintKind uint8

const (
	constraintNone constraintKind = iota
	constraintRequire
	constraintFunction
	constraintMatlab
	constraintTyped
	constraintRacket
	constraintImport
)

const (
	typedLeading uint8 = iota
	typedName
	typedColon
	typedValue
)

type lineConstraint struct {
	kind             constraintKind
	prefix, position int
	state            uint8
	match, invalid   bool
	body, close      bool
	open, swift      bool
	previous         rune
	fromEnd, read    int64
	window           lineTail
	pending          [utf8.UTFMax]byte
	pendingLen       int
	word             lineTail
	wordSize         int64
	wordTrimmed      int64
	wordEnd          bool
	afterWord        bool
}

func (c *lineConstraint) initialize(head []byte) {
	for _, entry := range [...]struct {
		prefix string
		kind   constraintKind
	}{
		{"require ", constraintRequire}, {funcPrefix, constraintFunction},
		{functionPrefix, constraintMatlab}, {constPrefix, constraintTyped},
		{"let ", constraintTyped}, {"var ", constraintTyped},
		{"#lang ", constraintRacket}, {importPrefix, constraintImport},
	} {
		if bytes.HasPrefix(head, []byte(entry.prefix)) {
			c.kind, c.prefix = entry.kind, len(entry.prefix)
			return
		}
	}
}

func (c *lineConstraint) write(data []byte, final bool) {
	if c.kind == constraintNone {
		return
	}
	if c.kind == constraintImport {
		c.imports(data)
		return
	}
	for c.pendingLen > 0 && (len(data) > 0 || final) {
		n := copy(c.pending[c.pendingLen:], data)
		c.pendingLen += n
		data = data[n:]
		if !final && !utf8.FullRune(c.pending[:c.pendingLen]) {
			return
		}
		r, width := utf8.DecodeRune(c.pending[:c.pendingLen])
		c.rune(r, c.pending[:width])
		c.pendingLen = copy(c.pending[:], c.pending[width:c.pendingLen])
	}
	for len(data) > 0 {
		if !final && !utf8.FullRune(data) {
			c.pendingLen = copy(c.pending[:], data)
			return
		}
		r, width := utf8.DecodeRune(data)
		c.rune(r, data[:width])
		data = data[width:]
	}
}

func (c *lineConstraint) imports(data []byte) {
	var boundary [2 * ruleWindow]byte
	n := copy(boundary[:], c.window.data[:c.window.size])
	n += copy(boundary[n:], data[:min(len(data), ruleWindow)])
	if c.fromEnd == 0 {
		if at := bytes.Index(boundary[:n], []byte(" from ")); at >= 0 {
			c.fromEnd = c.read - int64(c.window.size) + int64(at+len(" from "))
		} else if at := bytes.Index(data, []byte(" from ")); at >= 0 {
			c.fromEnd = c.read + int64(at+len(" from "))
		}
	}
	c.window.append(data)
	c.read += int64(len(data))
}

func (c *lineConstraint) rune(r rune, data []byte) {
	if c.position < c.prefix {
		c.position += len(data)
		return
	}
	switch c.kind {
	case constraintRequire:
		c.require(r)
	case constraintMatlab:
		c.matlab(r)
	case constraintTyped:
		c.typed(r)
	case constraintFunction:
		c.functionRune(r)
	case constraintRacket:
		c.racketRune(r, data)
	}
}

func (c *lineConstraint) require(r rune) {
	if c.state > 1 || unicode.IsSpace(r) {
		return
	}
	if c.state == 0 && r == '(' {
		c.state = 1
		return
	}
	c.state = 2
	c.match = r == '\'' || r == '"' || r >= 'A' && r <= 'Z'
}

func (c *lineConstraint) matlab(r rune) {
	if !c.open {
		c.open = r == '='
		return
	}
	c.invalid = c.invalid || r == '|' || r == '{'
	if c.state == 0 && !unicode.IsSpace(r) {
		c.state = 1
		c.match = r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}
}

func (c *lineConstraint) typed(r rune) {
	if c.invalid || c.match {
		return
	}
	letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || r == '$'
	switch c.state {
	case typedLeading:
		if !unicode.IsSpace(r) {
			c.invalid, c.state = !letter, typedName
		}
	case typedName:
		if letter || r >= '0' && r <= '9' {
			return
		}
		c.state = typedColon
		c.typed(r)
	case typedColon:
		if !unicode.IsSpace(r) {
			c.invalid, c.state = r != ':', typedValue
		}
	case typedValue:
		c.match = r == '='
	}
}

func (c *lineConstraint) functionRune(r rune) {
	if c.body {
		return
	}
	if r == '{' {
		c.body = true
		return
	}
	c.close = c.close || r == ')'
	c.swift = c.swift || r == ':' || c.previous == '-' && r == '>'
	c.previous = r
	if !c.open {
		c.open = r == '('
		return
	}
	if !unicode.IsSpace(r) {
		if c.state == 0 && r == ')' {
			c.state = 1
		} else {
			c.state = 2
		}
	}
}

func (c *lineConstraint) function(terminated bool) Set {
	if !c.close || !terminated && !c.body {
		return Set{}
	}
	if c.swift {
		return NewSet(Swift)
	}
	if c.state == 1 {
		return NewSet(Go, Swift)
	}
	return NewSet(Go)
}

func (c *lineConstraint) racketRune(r rune, data []byte) {
	if c.wordSize == 0 && unicode.IsSpace(r) {
		return
	}
	if space(byte(r)) && r < utf8.RuneSelf {
		c.wordEnd = true
	}
	if c.wordEnd {
		c.afterWord = c.afterWord || !unicode.IsSpace(r)
		return
	}
	if c.word.size < len(c.word.data) {
		c.word.size += copy(c.word.data[c.word.size:], data)
	}
	c.wordSize += int64(len(data))
	if !unicode.IsSpace(r) {
		c.wordTrimmed = c.wordSize
	}
}

func (c *lineConstraint) racket() bool {
	data := c.word.data[:c.word.size]
	for _, prefix := range [...]string{"racket/", "typed/racket/", "scheme/", "scribble/"} {
		if bytes.HasPrefix(data, []byte(prefix)) {
			return true
		}
	}
	size := c.wordSize
	if !c.afterWord {
		size = c.wordTrimmed
	}
	return size <= int64(len(data)) && racketLanguage(data[:size])
}

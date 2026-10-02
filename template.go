package languages

import "bytes"

const goTemplateRule = len(rules) - 1

type templateStream struct {
	active, brace, close, found bool
	quote                       byte
	escaped, comment, dot       bool
	previous                    byte
	head                        [len("template")]byte
	headSize                    int
	headDone, named, signature  bool
	offset                      uint64
}

func (s *templateStream) write(data []byte, sourceAt func(int) uint64) {
	for i := 0; i < len(data) && !s.found; i++ {
		b := data[i]
		if s.active {
			s.action(b)
			continue
		}
		if s.brace && b == '{' {
			s.active, s.brace = true, false
			continue
		}
		s.brace = false
		at := bytes.IndexByte(data[i:], '{')
		if at < 0 {
			return
		}
		i += at
		*s = templateStream{brace: true, offset: sourceAt(i)}
	}
}

func (s *templateStream) action(b byte) {
	defer func() { s.previous = b }()
	switch {
	case s.comment:
		if s.previous == '*' && b == '/' {
			s.comment = false
		}
	case s.quote != 0:
		s.quoted(b)
	case s.close:
		s.found, s.active = b == '}' && s.signature, false
	case b == '{':
		s.active = false
	case b == '}':
		s.close = true
	default:
		s.close = false
		s.syntax(b)
	}
}

func (s *templateStream) quoted(b byte) {
	switch {
	case s.escaped:
		s.escaped = false
	case b == '\\' && s.quote != '`':
		s.escaped = true
	case b == s.quote:
		s.quote = 0
		s.signature = s.signature || s.named
	}
}

func (s *templateStream) syntax(b byte) {
	if s.previous == '/' && b == '*' {
		s.comment = true
		return
	}
	if s.dot && (b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == '_') {
		s.signature = true
	}
	s.dot = b == '.' && (s.previous == 0 || space(s.previous) || s.previous == '(')
	if b == '"' || b == '`' || b == '\'' {
		s.quote = b
		s.named = s.headDone && (b == '"' || b == '`') && namedTemplateAction(s.head[:s.headSize])
		return
	}
	if s.headDone {
		return
	}
	if space(b) || b == '-' && s.headSize == 0 {
		s.headDone = s.headSize > 0
		return
	}
	if s.headSize == len(s.head) {
		s.headDone = true
		s.head[0] = 0
		return
	}
	s.head[s.headSize] = b
	s.headSize++
}

func namedTemplateAction(word []byte) bool {
	return bytes.Equal(word, []byte("define")) || bytes.Equal(word, []byte("template")) || bytes.Equal(word, []byte("block"))
}

func (s *templateStream) finish(a *Analysis) {
	if !s.found {
		return
	}
	markup := false
	var source Set
	for _, match := range a.Signals[:a.Count] {
		r := match.rule()
		if r.weight == declaredWeight {
			return
		}
		if r.languages == NewSet(HTML) || r.languages == NewSet(XML) {
			markup = true
		} else {
			source = source.Union(r.languages)
		}
	}
	if !source.Empty() && (!markup || source.Intersect(jsFamily) != source) {
		return
	}
	a.Signals[a.Count] = Match{Rule: uint16(goTemplateRule), Offset: s.offset}
	a.Count++
}

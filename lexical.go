package languages

import "bytes"

const textChunkBytes = 32 * 1024

// This shield handles common multiline comments and strings, without parsing.
type lexicalState struct {
	block  bool
	quote  byte
	triple bool

	offset, start int64
	stopped       bool
	pending       [len(`"""`)]byte
	pendingLen    int
}

func (s *lexicalState) codeStart(line []byte) int {
	s.startLine()
	for len(line) > 0 {
		n := min(len(line), textChunkBytes)
		s.write(line[:n])
		line = line[n:]
	}
	return int(s.finishLine())
}

func (s *lexicalState) startLine() {
	s.offset, s.start = 0, -1
	s.stopped, s.pendingLen = false, 0
}

func (s *lexicalState) write(data []byte) {
	for s.pendingLen > 0 && len(data) > 0 {
		n := copy(s.pending[s.pendingLen:], data)
		s.pendingLen += n
		data = data[n:]
		used := s.scan(s.pending[:s.pendingLen], false)
		s.offset += int64(used)
		s.pendingLen = copy(s.pending[:], s.pending[used:s.pendingLen])
	}
	if len(data) > 0 {
		used := s.scan(data, false)
		s.offset += int64(used)
		s.pendingLen = copy(s.pending[:], data[used:])
	}
}

func (s *lexicalState) finishLine() int64 {
	s.offset += int64(s.scan(s.pending[:s.pendingLen], true))
	s.pendingLen = 0
	if s.quote != '`' && !s.triple {
		s.quote = 0
	}
	return s.start
}

func (s *lexicalState) scan(data []byte, final bool) int {
	for i := 0; i < len(data); {
		if s.stopped {
			return len(data)
		}
		var n int
		switch {
		case s.block:
			n = s.skipBlock(data[i:], final)
		case s.quote != 0:
			n = s.skipQuoted(data[i:], final)
		default:
			n = s.code(data[i:], s.offset+int64(i), final)
		}
		if n == 0 {
			return i
		}
		i += n
	}
	return len(data)
}

func (s *lexicalState) skipBlock(data []byte, final bool) int {
	if end := bytes.Index(data, []byte("*/")); end >= 0 {
		s.block = false
		return end + len("*/")
	}
	if !final && data[len(data)-1] == '*' {
		return len(data) - 1
	}
	return len(data)
}

func (s *lexicalState) skipQuoted(data []byte, final bool) int {
	if data[0] == '\\' {
		if len(data) < len(`\x`) && !final {
			return 0
		}
		return min(len(data), len(`\x`))
	}
	if data[0] != s.quote {
		return 1
	}
	if !s.triple {
		s.quote = 0
		return 1
	}
	if len(data) < len(`"""`) {
		if !final {
			return 0
		}
		return 1
	}
	if data[1] == s.quote && data[2] == s.quote {
		s.quote, s.triple = 0, false
		return len(`"""`)
	}
	return 1
}

func (s *lexicalState) code(data []byte, offset int64, final bool) int {
	b := data[0]
	if space(b) {
		return 1
	}
	if (b == '/' || b == '-') && len(data) < len("/*") && !final {
		return 0
	}
	if bytes.HasPrefix(data, []byte("//")) {
		s.stopped = true
		return len(data)
	}
	if bytes.HasPrefix(data, []byte("/*")) {
		s.block = true
		return len("/*")
	}
	if s.start < 0 {
		if commentLine(data) {
			s.stopped = true
			return len(data)
		}
		s.start = offset
		if b == '#' || b == '%' {
			s.stopped = true
			return len(data)
		}
	}
	if b == '\'' || b == '"' || b == '`' {
		return s.openQuoted(data, final)
	}
	return 1
}

func (s *lexicalState) openQuoted(data []byte, final bool) int {
	if len(data) < len(`"""`) && !final {
		return 0
	}
	s.quote = data[0]
	if len(data) >= len(`"""`) && data[1] == s.quote && data[2] == s.quote {
		s.triple = true
		return len(`"""`)
	}
	return 1
}

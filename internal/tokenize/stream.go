package tokenize

import (
	"bytes"
	"unicode/utf8"
)

type streamMode uint8

const (
	streamNormal streamMode = iota
	streamWord
	streamNumber
	streamString
	streamBlock
	streamComment
)

// Stream retains tokens up to the supplied buffer length. Longer tokens are omitted.
// The callback must copy tokens it retains. Finish emits the last unterminated word.
type Stream struct {
	buffer        []byte
	word          int
	mode          streamMode
	quote         byte
	escaped, star bool
	pending       [utf8.UTFMax]byte
	pendingLen    int
}

func NewStream(buffer []byte) Stream { return Stream{buffer: buffer} }

func (s *Stream) Write(data []byte, visit func([]byte)) {
	for s.pendingLen > 0 && len(data) > 0 {
		n := copy(s.pending[s.pendingLen:], data)
		s.pendingLen += n
		data = data[n:]
		used := s.scan(s.pending[:s.pendingLen], false, visit)
		s.pendingLen = copy(s.pending[:], s.pending[used:s.pendingLen])
	}
	if len(data) > 0 {
		used := s.scan(data, false, visit)
		s.pendingLen = copy(s.pending[:], data[used:])
	}
}

func (s *Stream) Finish(visit func([]byte)) {
	s.scan(s.pending[:s.pendingLen], true, visit)
	s.pendingLen = 0
	if s.mode == streamWord {
		s.emitWord(visit)
	}
	s.mode = streamNormal
}

func (s *Stream) scan(data []byte, final bool, visit func([]byte)) int {
	for i := 0; i < len(data); {
		switch s.mode {
		case streamWord:
			end := i
			for end < len(data) && (letter(data[end]) || digit(data[end])) {
				end++
			}
			s.appendWord(data[i:end])
			i = end
			if i < len(data) {
				s.emitWord(visit)
				s.mode = streamNormal
			}
		case streamNumber:
			for i < len(data) && (letter(data[i]) || digit(data[i]) || data[i] == '.') {
				i++
			}
			if i < len(data) {
				s.mode = streamNormal
			}
		case streamComment:
			end := bytes.IndexByte(data[i:], '\n')
			if end < 0 {
				return len(data)
			}
			i += end + 1
			s.mode = streamNormal
		case streamBlock:
			i += s.block(data[i:])
		case streamString:
			s.quoted(data[i])
			i++
		default:
			n := s.normal(data[i:], final, visit)
			if n == 0 {
				return i
			}
			i += n
		}
	}
	return len(data)
}

func (s *Stream) block(data []byte) int {
	if s.star && data[0] == '/' {
		s.star = false
		s.mode = streamNormal
		return 1
	}
	end := bytes.Index(data, []byte("*/"))
	if end < 0 {
		s.star = data[len(data)-1] == '*'
		return len(data)
	}
	s.star = false
	s.mode = streamNormal
	return end + len("*/")
}

func (s *Stream) quoted(c byte) {
	if s.escaped {
		s.escaped = false
		return
	}
	switch c {
	case '\\':
		s.escaped = true
	case s.quote:
		s.mode = streamNormal
	}
}

func (s *Stream) normal(data []byte, final bool, visit func([]byte)) int {
	c := data[0]
	if c >= utf8.RuneSelf {
		if !final && !utf8.FullRune(data) {
			return 0
		}
		_, width := utf8.DecodeRune(data)
		if width > 1 {
			visit(data[:width])
		}
		return width
	}
	if c <= ' ' {
		return 1
	}
	if c == '/' || c == '-' || c == '#' {
		return s.commentStart(data, final, visit)
	}
	if c == '\'' || c == '"' || c == '`' {
		s.mode = streamString
		s.quote = c
		return 1
	}
	if letter(c) {
		end := wordEnd(data, 0, false)
		if end < len(data) || final {
			if end <= len(s.buffer) {
				visit(data[:end])
			}
		} else {
			s.mode = streamWord
			s.appendWord(data[:end])
		}
		return end
	}
	if digit(c) {
		end := wordEnd(data, 0, true)
		if end == len(data) && !final {
			s.mode = streamNumber
		}
		return end
	}
	visit(data[:1])
	return 1
}

func (s *Stream) commentStart(data []byte, final bool, visit func([]byte)) int {
	if len(data) < len("/*") && !final {
		return 0
	}
	if bytes.HasPrefix(data, []byte("/*")) {
		s.mode = streamBlock
		return len("/*")
	}
	if lineComment(data) {
		if data[0] == '/' && len(data) < len("//!") && !final {
			return 0
		}
		s.mode = streamComment
		if bytes.HasPrefix(data, []byte("//!")) || bytes.HasPrefix(data, []byte("///")) {
			visit(data[:len("//!")])
			return len("//!")
		}
		return len("//")
	}
	visit(data[:1])
	return 1
}

func (s *Stream) appendWord(data []byte) {
	if s.word <= len(s.buffer) {
		copy(s.buffer[s.word:], data)
		s.word += min(len(data), len(s.buffer)+1-s.word)
	}
}

func (s *Stream) emitWord(visit func([]byte)) {
	if s.word > 0 && s.word <= len(s.buffer) {
		visit(s.buffer[:s.word])
	}
	s.word = 0
}

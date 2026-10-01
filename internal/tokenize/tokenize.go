// Package tokenize extracts lexical features for training and classification.
package tokenize

import (
	"bytes"
	"unicode/utf8"
)

// Scan visits tokens backed by data. The callback must copy tokens it retains.
func Scan(data []byte, visit func([]byte)) {
	scanner := NewScanner(data)
	for token := scanner.Next(); token != nil; token = scanner.Next() {
		visit(token)
	}
}

type Scanner struct {
	data   []byte
	offset int
}

func NewScanner(data []byte) Scanner { return Scanner{data: data} }

func (s *Scanner) Next() []byte {
	data := s.data
	for i := s.offset; i < len(data); {
		c := data[i]
		if c >= utf8.RuneSelf {
			_, width := utf8.DecodeRune(data[i:])
			if width > 1 {
				s.offset = i + width
				return data[i:s.offset]
			}
			i += width
			continue
		}
		if c <= ' ' {
			i++
			continue
		}
		if bytes.HasPrefix(data[i:], []byte("/*")) {
			i = skipBlock(data, i)
			continue
		}
		if lineComment(data[i:]) {
			start := i
			end := bytes.IndexByte(data[i:], '\n')
			if end < 0 {
				i = len(data)
			} else {
				i += end + 1
			}
			if bytes.HasPrefix(data[start:], []byte("//!")) || bytes.HasPrefix(data[start:], []byte("///")) {
				s.offset = i
				return data[start : start+len("//!")]
			}
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			i = skipString(data, i)
			continue
		}
		start := i
		i++
		if letter(c) {
			i = wordEnd(data, start, false)
		} else if digit(c) {
			i = wordEnd(data, start, true)
			continue
		}
		s.offset = i
		return data[start:i]
	}
	s.offset = len(data)
	return nil
}

func wordEnd(data []byte, start int, number bool) int {
	i := start + 1
	for i < len(data) && (letter(data[i]) || digit(data[i]) || number && data[i] == '.') {
		i++
	}
	return i
}

func skipBlock(data []byte, start int) int {
	end := bytes.Index(data[start+len("/*"):], []byte("*/"))
	if end < 0 {
		return len(data)
	}
	return start + end + len("/**/")
}

func skipString(data []byte, start int) int {
	quote := data[start]
	for i := start + 1; i < len(data); i++ {
		if data[i] == '\\' {
			i++
			continue
		}
		if data[i] == quote {
			return i + 1
		}
	}
	return len(data)
}

func lineComment(data []byte) bool {
	return bytes.HasPrefix(data, []byte("//")) || bytes.HasPrefix(data, []byte("--")) || bytes.HasPrefix(data, []byte("# "))
}

func letter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' }
func digit(c byte) bool  { return c >= '0' && c <= '9' }

package languages

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

const (
	vimName        = "vim"
	vimCapitalName = "Vim"
)

type vimMarkerState struct {
	head                        [len(vimName)]byte
	size                        int
	whitespace, invalid, digits bool
}

func (s *vimMarkerState) write(r rune) bool {
	if vimSpace(r) {
		*s = vimMarkerState{whitespace: true}
		return false
	}
	if r == ':' {
		valid := s.valid()
		s.invalid = true
		return valid
	}
	if r >= utf8.RuneSelf {
		s.invalid = true
	}
	switch {
	case s.size < len(s.head):
		s.head[s.size] = byte(r)
	case r >= '0' && r <= '9':
		s.digits = true
	case s.size != len(s.head) || r != '<' && r != '=' && r != '>':
		s.invalid = true
	}
	s.size = min(s.size+1, len(s.head)+1)
	return false
}

func (s *vimMarkerState) valid() bool {
	if s.invalid {
		return false
	}
	if s.size == len("vi") {
		return string(s.head[:s.size]) == "vi" || string(s.head[:s.size]) == "ex" && s.whitespace
	}
	if string(s.head[:]) != vimName && string(s.head[:]) != vimCapitalName {
		return false
	}
	return s.size == len(s.head) || s.digits
}

func vimSpace(r rune) bool { return r == ' ' || r == '\t' }

type vimStream struct {
	decoder                                     runeStream
	marker                                      vimMarkerState
	found, leading, first                       bool
	set, closed                                 bool
	key, value                                  modeField
	equal, mode, escaped, separated, pendingKey bool
	selected, afterValue                        bool
	raw, trimmed                                Language
}

func (s *vimStream) write(data []byte) { s.decoder.write(data, false, s.rune) }

func (s *vimStream) finish() Language {
	s.decoder.write(nil, true, s.rune)
	if !s.selected {
		s.option(0, true)
	}
	if !s.found || s.set && !s.closed {
		return Unknown
	}
	if s.afterValue {
		return s.raw
	}
	return s.trimmed
}

func (s *vimStream) rune(r rune, data []byte) {
	if !s.found {
		if s.marker.write(r) {
			s.found, s.leading, s.first = true, true, true
		}
		return
	}
	if s.closed {
		return
	}
	if s.set && r == ':' {
		if !s.selected {
			s.option(r, false)
		}
		s.afterValue, s.closed = true, true
		return
	}
	if s.selected {
		s.afterValue = s.afterValue || !unicode.IsSpace(r)
		return
	}
	if s.leading && unicode.IsSpace(r) {
		return
	}
	s.leading = false
	if s.pendingKey {
		if vimSpace(r) {
			return
		}
		s.pendingKey = false
		if r == '=' {
			s.equal, s.mode, s.separated = true, true, true
			return
		}
	}
	if !s.escaped && (vimSpace(r) || r == ':') {
		s.option(r, false)
		return
	}
	s.wordRune(r, data)
}

func (s *vimStream) wordRune(r rune, data []byte) {
	switch {
	case !s.equal && r == '=':
		s.equal = true
		s.mode = modeOption(s.key.raw())
	case s.equal:
		s.value.append(r, data)
	default:
		s.key.append(r, data)
	}
	s.escaped = !s.separated && !s.escaped && r == '\\'
}

func (s *vimStream) option(separator rune, final bool) {
	key := s.key.raw()
	switch {
	case s.first && !s.equal && !final && vimSpace(separator) && (bytes.Equal(key, []byte("set")) || bytes.Equal(key, []byte("se"))):
		s.set = true
	case s.equal && s.mode:
		s.selected = true
		s.raw, s.trimmed = parseMode(s.value.raw()), parseMode(s.value.trimmed())
		s.afterValue = !final && !unicode.IsSpace(separator)
	case !s.equal && !final && vimSpace(separator) && modeOption(key):
		s.pendingKey = true
	}
	s.key.size, s.key.end, s.value.size, s.value.end = 0, 0, 0, 0
	s.first, s.equal, s.mode, s.escaped, s.separated = false, false, false, false, false
}

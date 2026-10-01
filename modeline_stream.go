package languages

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

const modeFieldBytes = 512

type modeField struct {
	data      [modeFieldBytes]byte
	size, end int
}

func (f *modeField) add(r rune, data []byte) {
	if f.size == 0 && unicode.IsSpace(r) {
		return
	}
	f.append(r, data)
}

func (f *modeField) append(r rune, data []byte) {
	if f.size < len(f.data) {
		copy(f.data[f.size:], data)
	}
	f.size = min(len(f.data)+1, f.size+len(data))
	if !unicode.IsSpace(r) {
		f.end = f.size
	}
}

func (f *modeField) raw() []byte {
	if f.size > len(f.data) {
		return nil
	}
	return f.data[:f.size]
}

func (f *modeField) trimmed() []byte {
	if f.end > len(f.data) {
		return nil
	}
	return f.data[:f.end]
}

type emacsStream struct {
	marker                     [len(emacsMarker)]byte
	markerLen                  int
	started, closed            bool
	delimited, colon, selected bool
	simple, key, value         modeField
	pending                    [utf8.UTFMax]byte
	pendingLen                 int
	language                   Language
}

func (s *emacsStream) write(data []byte) {
	for _, b := range data {
		if s.closed {
			return
		}
		s.marker[s.markerLen] = b
		s.markerLen++
		if s.markerLen < len(s.marker) {
			continue
		}
		if string(s.marker[:]) == emacsMarker {
			if s.started {
				s.finishBody()
				s.closed = true
			}
			s.started = true
			s.markerLen = 0
			continue
		}
		if s.started {
			s.bodyByte(s.marker[0])
		}
		s.markerLen = copy(s.marker[:], s.marker[1:])
	}
}

func (s *emacsStream) bodyByte(b byte) {
	s.pending[s.pendingLen] = b
	s.pendingLen++
	for s.pendingLen > 0 && utf8.FullRune(s.pending[:s.pendingLen]) {
		r, width := utf8.DecodeRune(s.pending[:s.pendingLen])
		s.bodyRune(r, s.pending[:width])
		s.pendingLen = copy(s.pending[:], s.pending[width:s.pendingLen])
	}
}

func (s *emacsStream) bodyRune(r rune, data []byte) {
	s.simple.add(r, data)
	if r == ';' {
		s.delimited = true
		s.option()
		s.key, s.value = modeField{}, modeField{}
		s.colon = false
		return
	}
	if r == ':' && !s.colon {
		s.delimited, s.colon = true, true
		return
	}
	if s.colon {
		s.value.add(r, data)
	} else {
		s.key.add(r, data)
	}
}

func (s *emacsStream) option() {
	if !s.selected && s.colon && bytes.EqualFold(s.key.trimmed(), []byte("mode")) {
		s.language = parseMode(s.value.trimmed())
		s.selected = true
	}
}

func (s *emacsStream) finishBody() {
	for s.pendingLen > 0 {
		r, width := utf8.DecodeRune(s.pending[:s.pendingLen])
		s.bodyRune(r, s.pending[:width])
		s.pendingLen = copy(s.pending[:], s.pending[width:s.pendingLen])
	}
	if s.delimited {
		s.option()
	} else {
		s.language = parseMode(s.simple.trimmed())
	}
}

func (s *emacsStream) result() Language {
	if !s.closed {
		return Unknown
	}
	return s.language
}

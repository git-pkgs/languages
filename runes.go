package languages

import "unicode/utf8"

type runeStream struct {
	pending [utf8.UTFMax]byte
	size    int
}

func (s *runeStream) write(data []byte, final bool, visit func(rune, []byte)) {
	for s.size > 0 && (len(data) > 0 || final) {
		n := copy(s.pending[s.size:], data)
		s.size += n
		data = data[n:]
		if !final && !utf8.FullRune(s.pending[:s.size]) {
			return
		}
		r, width := utf8.DecodeRune(s.pending[:s.size])
		visit(r, s.pending[:width])
		s.size = copy(s.pending[:], s.pending[width:s.size])
	}
	for len(data) > 0 {
		if !final && !utf8.FullRune(data) {
			s.size = copy(s.pending[:], data)
			return
		}
		r, width := utf8.DecodeRune(data)
		visit(r, data[:width])
		data = data[width:]
	}
}

package languages

import (
	"unicode/utf8"

	"github.com/git-pkgs/magic"
)

type textDecoder struct {
	encoding    string
	offset      uint64
	skip        int
	pending     [utf8.UTFMax]byte
	pendingSize int
	buffer      [textChunkBytes]byte
	invalid     bool
}

func (d *textDecoder) start(encoding string) {
	switch encoding {
	case magic.EncodingUTF16LE, magic.EncodingUTF16BE:
		d.encoding, d.skip = encoding, utf16Width
	case magic.EncodingUTF32LE, magic.EncodingUTF32BE:
		d.encoding, d.skip = encoding, utf32Width
	}
}

func (d *textDecoder) write(data []byte, text *textStream) {
	if d.invalid {
		return
	}
	if d.encoding == "" {
		location := sourceMap{base: d.offset}
		text.write(data, location.at)
		d.offset += uint64(len(data))
		return
	}
	skip := min(d.skip, len(data))
	d.skip -= skip
	d.offset += uint64(skip)
	data = data[skip:]
	for d.pendingSize > 0 && len(data) > 0 {
		d.pending[d.pendingSize] = data[0]
		d.pendingSize++
		data = data[1:]
		r, width, ok := encodedRune(d.pending[:d.pendingSize], d.encoding)
		if !ok {
			if width > 0 {
				d.invalid = true
				return
			}
			continue
		}
		n := utf8.EncodeRune(d.buffer[:], r)
		d.emit(d.pending[:width], d.buffer[:n], text)
		d.pendingSize = 0
	}
	for len(data) > 0 {
		written, consumed, valid := decodeText(data, d.buffer[:], d.encoding, false)
		if !valid {
			d.invalid = true
			return
		}
		if consumed == 0 {
			d.pendingSize = copy(d.pending[:], data)
			return
		}
		d.emit(data[:consumed], d.buffer[:written], text)
		data = data[consumed:]
	}
}

func (d *textDecoder) emit(raw, decoded []byte, text *textStream) {
	location := sourceMap{data: raw, encoding: d.encoding, base: d.offset}
	text.write(decoded, location.at)
	d.offset += uint64(len(raw))
}

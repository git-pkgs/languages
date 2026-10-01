package languages

import (
	"encoding/binary"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/git-pkgs/magic"
)

const encodingBufferBytes = 64 * 1024

var encodingBuffers = sync.Pool{New: func() any { return new([encodingBufferBytes]byte) }}

const (
	utf16Width     = 2
	utf32Width     = 4
	lowSurrogate   = 0xdc00
	surrogateWidth = 2 * utf16Width
)

func analyzeEncoded(data []byte, complete bool, dst *Analysis, encoding string, heuristic uint16) {
	bom := utf16Width
	if encoding == magic.EncodingUTF32LE || encoding == magic.EncodingUTF32BE {
		bom = utf32Width
	}
	buffer := encodingBuffers.Get().(*[encodingBufferBytes]byte)
	defer encodingBuffers.Put(buffer)
	text := buffer[:]
	decoded, consumed, valid := decodeText(data[bom:], text, encoding, complete)
	if !valid {
		dst.Binary = true
		return
	}
	consumed += bom
	prefix := !complete || consumed < len(data)
	analyzeText(text[:decoded], !prefix, dst, heuristic)
	dst.Bytes = int64(len(data))
	dst.Prefix = prefix
	for i := range dst.Signals[:dst.Count] {
		offset := originalOffset(data[bom:consumed], encoding, int(dst.Signals[i].Offset))
		dst.Signals[i].Offset = uint64(offset + bom)
	}
}

func decodeText(data, buffer []byte, encoding string, complete bool) (written, consumed int, valid bool) {
	for consumed < len(data) {
		r, width, ok := encodedRune(data[consumed:], encoding)
		if !ok {
			return written, consumed, !complete && width == 0
		}
		if utf8.RuneLen(r) > len(buffer)-written {
			break
		}
		written += utf8.EncodeRune(buffer[written:], r)
		consumed += width
	}
	return written, consumed, true
}

func encodedRune(data []byte, encoding string) (rune, int, bool) {
	if encoding == magic.EncodingUTF32LE || encoding == magic.EncodingUTF32BE {
		if len(data) < utf32Width {
			return 0, 0, false
		}
		value := binary.BigEndian.Uint32(data)
		if encoding == magic.EncodingUTF32LE {
			value = binary.LittleEndian.Uint32(data)
		}
		r := rune(value)
		return r, utf32Width, utf8.ValidRune(r)
	}
	if len(data) < utf16Width {
		return 0, 0, false
	}
	r := codeUnit(data, encoding)
	if !utf16.IsSurrogate(r) {
		return r, utf16Width, true
	}
	if r >= lowSurrogate {
		return 0, utf16Width, false
	}
	if len(data) < surrogateWidth {
		return 0, 0, false
	}
	r = utf16.DecodeRune(r, codeUnit(data[utf16Width:], encoding))
	return r, surrogateWidth, r != utf8.RuneError
}

func codeUnit(data []byte, encoding string) rune {
	if encoding == magic.EncodingUTF16LE {
		return rune(binary.LittleEndian.Uint16(data))
	}
	return rune(binary.BigEndian.Uint16(data))
}

func originalOffset(data []byte, encoding string, offset int) int {
	consumed := 0
	for offset > 0 {
		r, width, _ := encodedRune(data[consumed:], encoding)
		offset -= utf8.RuneLen(r)
		consumed += width
	}
	return consumed
}

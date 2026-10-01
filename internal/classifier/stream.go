package classifier

import (
	"unicode/utf8"

	"github.com/git-pkgs/languages/internal/tokenize"
)

var maxTokenBytes = func() int {
	length := utf8.UTFMax
	for offset := 0; offset < len(indexData); offset += indexWidth {
		length = max(length, indexUint(offset+fieldWidth))
	}
	return length
}()

type Stream struct {
	buffer  []byte
	tokens  tokenize.Stream
	counter tokenCounter
	dst     *Analysis
}

func (s *Stream) Reset(dst *Analysis) {
	buffer := s.buffer
	if len(buffer) != maxTokenBytes {
		buffer = make([]byte, maxTokenBytes)
	}
	*s = Stream{buffer: buffer, tokens: tokenize.NewStream(buffer), dst: dst}
	reset(dst)
}

func (s *Stream) Write(data []byte) { s.tokens.Write(data, s.add) }

func (s *Stream) Finish() {
	s.tokens.Finish(s.add)
	s.counter.flush(s.dst)
}

func (s *Stream) add(token []byte) { s.counter.add(token, s.dst) }

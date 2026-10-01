package languages

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/git-pkgs/magic"
)

const (
	formatBytes     = 512
	maxEmptyReads   = 100
	allTextControls = "\x01\x02\x03\x04\x05\x06\x07\b\t\n\v\f\r\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f"
)

type ReadOptions struct {
	Bytes  int64 // Zero reads to EOF; positive values set an exact read budget.
	Prefix bool  // The reader contains a prefix even if it reaches EOF.
}

// AnalyzeReader reads content with bounded memory. Reaching a byte budget without
// EOF leaves Prefix true. Errors clear dst. Cancellation is checked between reads;
// it cannot interrupt an io.Reader blocked inside Read.
func AnalyzeReader(ctx context.Context, reader io.Reader, options ReadOptions, dst *Analysis) error {
	return analyzeReader(ctx, reader, options, dst, allHeuristics, -1)
}

var readerStreams = sync.Pool{New: func() any { return new(readerStream) }}

type readerStream struct {
	text            textStream
	decoder         textDecoder
	validator       magic.TextValidator
	buffer          [textChunkBytes]byte
	head            [formatBytes]byte
	headSize        int
	started, binary bool
}

func analyzeReader(ctx context.Context, reader io.Reader, options ReadOptions, dst *Analysis, heuristic uint16, size int64) error {
	*dst = Analysis{}
	if options.Bytes < 0 {
		return errors.New("bytes must be zero or greater")
	}
	s := readerStreams.Get().(*readerStream)
	defer readerStreams.Put(s)
	s.reset()
	count, complete, err := s.read(ctx, reader, options.Bytes)
	if err != nil {
		return err
	}
	complete = (complete || size >= 0 && count == size) && !options.Prefix
	*dst = s.finish(complete, heuristic)
	dst.Bytes = count
	return nil
}

func (s *readerStream) reset() {
	s.text.reset()
	s.decoder = textDecoder{}
	s.validator = magic.TextValidator{}
	s.headSize, s.started, s.binary = 0, false, false
}

func (s *readerStream) read(ctx context.Context, reader io.Reader, limit int64) (int64, bool, error) {
	var count int64
	empty := 0
	for {
		if err := ctx.Err(); err != nil {
			return count, false, err
		}
		next := int64(len(s.buffer))
		if limit > 0 {
			next = min(next, limit-count)
			if next == 0 {
				return count, false, nil
			}
		}
		n, err := reader.Read(s.buffer[:next])
		if n > 0 {
			s.write(s.buffer[:n])
			count += int64(n)
			empty = 0
		} else {
			empty++
		}
		if cancelled := ctx.Err(); cancelled != nil {
			return count, false, cancelled
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return count, true, nil
			}
			return count, false, err
		}
		if empty >= maxEmptyReads {
			return count, false, io.ErrNoProgress
		}
	}
}

func (s *readerStream) write(data []byte) {
	_, _ = s.validator.Write(data)
	if !s.started {
		n := copy(s.head[s.headSize:], data)
		s.headSize += n
		data = data[n:]
		if s.headSize < len(s.head) {
			return
		}
		s.start()
	}
	if !s.binary {
		s.decoder.write(data, &s.text)
	}
}

func (s *readerStream) start() {
	s.started = true
	// Controls are validated over the full stream, independently of signatures.
	format := magic.DetectWithOptions(s.head[:s.headSize], magic.Options{Prefix: true, TextControls: allTextControls})
	s.binary = format.Kind == magic.KindBinary
	s.decoder.start(format.Encoding)
	if !s.binary {
		s.decoder.write(s.head[:s.headSize], &s.text)
	}
}

func (s *readerStream) finish(complete bool, heuristic uint16) Analysis {
	if !s.started {
		s.start()
	}
	format := s.validator.Result(magic.Options{Prefix: !complete, TextControls: sourceTextControls})
	if s.binary || format.Kind == magic.KindBinary || format.Kind == magic.KindUnknown && format.Encoding != "" {
		return Analysis{Binary: true, Prefix: !complete}
	}
	return s.text.finish(complete, heuristic)
}

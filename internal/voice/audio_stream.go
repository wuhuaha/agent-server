package voice

import (
	"context"
	"io"
)

type staticAudioStream struct {
	chunks [][]byte
	index  int
}

func NewStaticAudioStream(chunks [][]byte) AudioStream {
	cloned := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		cloned = append(cloned, append([]byte(nil), chunk...))
	}
	return &staticAudioStream{chunks: cloned}
}

func (s *staticAudioStream) Next(context.Context) ([]byte, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := append([]byte(nil), s.chunks[s.index]...)
	s.index++
	return chunk, nil
}

func (s *staticAudioStream) Close() error {
	return nil
}

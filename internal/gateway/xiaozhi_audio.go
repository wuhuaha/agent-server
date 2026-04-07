package gateway

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"agent-server/internal/voice"

	"github.com/pion/opus/pkg/oggreader"
)

type xiaozhiOutputEncoder interface {
	EncodePCM16(context.Context, voice.AudioStream, int, int, int, int, int) (voice.AudioStream, error)
}

type ffmpegXiaozhiOutputEncoder struct {
	Binary string
}

func newDefaultXiaozhiOutputEncoder() xiaozhiOutputEncoder {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ffmpegXiaozhiOutputEncoder{}
	}
	return ffmpegXiaozhiOutputEncoder{Binary: path}
}

func (e ffmpegXiaozhiOutputEncoder) EncodePCM16(
	ctx context.Context,
	source voice.AudioStream,
	inputSampleRateHz, inputChannels int,
	targetSampleRateHz, targetChannels int,
	frameDurationMs int,
) (voice.AudioStream, error) {
	if strings.TrimSpace(e.Binary) == "" {
		return nil, fmt.Errorf("ffmpeg is required for xiaozhi opus downlink encoding")
	}
	if source == nil {
		return nil, fmt.Errorf("pcm16 source audio stream is required")
	}
	if inputSampleRateHz <= 0 || inputChannels <= 0 {
		return nil, fmt.Errorf("invalid source pcm16 profile %d/%d", inputSampleRateHz, inputChannels)
	}
	if targetSampleRateHz <= 0 || targetChannels <= 0 {
		return nil, fmt.Errorf("invalid target opus profile %d/%d", targetSampleRateHz, targetChannels)
	}
	if frameDurationMs <= 0 {
		frameDurationMs = 60
	}

	cmd := exec.CommandContext(
		ctx,
		e.Binary,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "s16le",
		"-ar", strconv.Itoa(inputSampleRateHz),
		"-ac", strconv.Itoa(inputChannels),
		"-i", "pipe:0",
		"-vn",
		"-c:a", "libopus",
		"-application", "voip",
		"-frame_duration", strconv.Itoa(frameDurationMs),
		"-ar", strconv.Itoa(targetSampleRateHz),
		"-ac", strconv.Itoa(targetChannels),
		"-f", "ogg",
		"pipe:1",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}

	stream := &ffmpegOggOpusPacketStream{
		source: source,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan struct{}),
	}
	go stream.feedInput(ctx)

	reader, _, err := oggreader.NewWith(bufio.NewReader(stdout))
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		stderrBytes, _ := io.ReadAll(stderr)
		_ = cmd.Wait()
		return nil, fmt.Errorf("initialize opus ogg reader: %w: %s", err, strings.TrimSpace(string(stderrBytes)))
	}
	stream.reader = reader
	return stream, nil
}

type ffmpegOggOpusPacketStream struct {
	source voice.AudioStream
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	reader *oggreader.OggReader

	pending [][]byte
	done    chan struct{}

	waitOnce sync.Once
	waitErr  error
	stderrMu sync.Mutex
	stderrTx string

	closeOnce sync.Once
}

func (s *ffmpegOggOpusPacketStream) feedInput(ctx context.Context) {
	defer close(s.done)
	defer func() { _ = s.stdin.Close() }()
	defer func() { _ = s.source.Close() }()

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		chunk, err := s.source.Next(ctx)
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return
			}
			s.waitErr = fmt.Errorf("read pcm16 source stream: %w", err)
			return
		}
		if len(chunk) == 0 {
			continue
		}
		if _, err := s.stdin.Write(chunk); err != nil {
			s.waitErr = fmt.Errorf("write pcm16 to ffmpeg stdin: %w", err)
			return
		}
	}
}

func (s *ffmpegOggOpusPacketStream) Next(ctx context.Context) ([]byte, error) {
	for len(s.pending) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		segments, _, err := s.reader.ParseNextPage()
		if err != nil {
			if err == io.EOF {
				if waitErr := s.wait(); waitErr != nil {
					return nil, waitErr
				}
				return nil, io.EOF
			}
			return nil, err
		}
		if len(segments) == 0 {
			continue
		}
		if len(segments) == 1 {
			payload := segments[0]
			if bytes.HasPrefix(payload, []byte("OpusTags")) || bytes.HasPrefix(payload, []byte("OpusHead")) {
				continue
			}
		}
		for _, segment := range segments {
			if len(segment) == 0 {
				continue
			}
			if bytes.HasPrefix(segment, []byte("OpusTags")) || bytes.HasPrefix(segment, []byte("OpusHead")) {
				continue
			}
			s.pending = append(s.pending, append([]byte(nil), segment...))
		}
	}

	chunk := s.pending[0]
	s.pending = s.pending[1:]
	return chunk, nil
}

func (s *ffmpegOggOpusPacketStream) Close() error {
	s.closeOnce.Do(func() {
		_ = s.stdin.Close()
		_ = s.stdout.Close()
		_ = s.stderr.Close()
		_ = s.source.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		_ = s.wait()
	})
	return nil
}

func (s *ffmpegOggOpusPacketStream) wait() error {
	s.waitOnce.Do(func() {
		stderrBytes, _ := io.ReadAll(s.stderr)
		s.stderrMu.Lock()
		s.stderrTx = strings.TrimSpace(string(stderrBytes))
		s.stderrMu.Unlock()
		if err := s.cmd.Wait(); err != nil {
			if s.waitErr != nil {
				s.waitErr = fmt.Errorf("%v; ffmpeg wait: %w", s.waitErr, err)
			} else {
				s.waitErr = fmt.Errorf("ffmpeg wait: %w", err)
			}
		}
		if s.waitErr == nil && s.stderrText() != "" {
			return
		}
		if s.waitErr != nil && s.stderrText() != "" {
			s.waitErr = fmt.Errorf("%v: %s", s.waitErr, s.stderrText())
		}
	})
	return s.waitErr
}

func (s *ffmpegOggOpusPacketStream) stderrText() string {
	s.stderrMu.Lock()
	defer s.stderrMu.Unlock()
	return s.stderrTx
}

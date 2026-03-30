package voice

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

func chunkPCM16(audioPCM []byte, sampleRateHz, channels int, frameMs int) [][]byte {
	if len(audioPCM) == 0 || sampleRateHz <= 0 || channels <= 0 {
		return nil
	}
	if frameMs <= 0 {
		frameMs = 20
	}
	bytesPerFrame := channels * 2
	samplesPerChunk := sampleRateHz * frameMs / 1000
	if samplesPerChunk <= 0 {
		samplesPerChunk = sampleRateHz / 50
	}
	chunkBytes := samplesPerChunk * bytesPerFrame
	if chunkBytes <= 0 {
		return nil
	}

	var chunks [][]byte
	for offset := 0; offset < len(audioPCM); offset += chunkBytes {
		end := offset + chunkBytes
		if end > len(audioPCM) {
			end = len(audioPCM)
		}
		chunks = append(chunks, append([]byte(nil), audioPCM[offset:end]...))
	}
	return chunks
}

func decodeWAVPCM16(payload []byte) ([]byte, int, int, error) {
	if len(payload) < 12 {
		return nil, 0, 0, errors.New("wav payload is too short")
	}
	if string(payload[0:4]) != "RIFF" || string(payload[8:12]) != "WAVE" {
		return nil, 0, 0, errors.New("wav header is invalid")
	}

	var (
		offset        = 12
		sampleRateHz  int
		channels      int
		audioFormat   uint16
		bitsPerSample uint16
		dataChunk     []byte
	)

	for offset+8 <= len(payload) {
		chunkID := string(payload[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(payload[offset+4 : offset+8]))
		offset += 8
		if offset+chunkSize > len(payload) {
			return nil, 0, 0, errors.New("wav chunk exceeds payload length")
		}
		chunkData := payload[offset : offset+chunkSize]

		switch chunkID {
		case "fmt ":
			if len(chunkData) < 16 {
				return nil, 0, 0, errors.New("wav fmt chunk is too short")
			}
			audioFormat = binary.LittleEndian.Uint16(chunkData[0:2])
			channels = int(binary.LittleEndian.Uint16(chunkData[2:4]))
			sampleRateHz = int(binary.LittleEndian.Uint32(chunkData[4:8]))
			bitsPerSample = binary.LittleEndian.Uint16(chunkData[14:16])
		case "data":
			dataChunk = append([]byte(nil), chunkData...)
		}

		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}

	if audioFormat != 1 {
		return nil, 0, 0, fmt.Errorf("unsupported wav audio format %d", audioFormat)
	}
	if bitsPerSample != 16 {
		return nil, 0, 0, fmt.Errorf("unsupported wav bit depth %d", bitsPerSample)
	}
	if channels <= 0 || sampleRateHz <= 0 {
		return nil, 0, 0, errors.New("wav metadata is incomplete")
	}
	if len(dataChunk) == 0 {
		return nil, 0, 0, errors.New("wav data chunk is empty")
	}
	return dataChunk, sampleRateHz, channels, nil
}

func adaptPCM16(audioPCM []byte, sourceRateHz, sourceChannels, targetRateHz, targetChannels int) ([]byte, error) {
	if targetChannels != sourceChannels {
		return nil, fmt.Errorf("channel adaptation is not implemented: %d -> %d", sourceChannels, targetChannels)
	}
	if sourceChannels != 1 {
		return nil, fmt.Errorf("only mono pcm is currently supported, got %d channels", sourceChannels)
	}
	if targetRateHz <= 0 || sourceRateHz <= 0 {
		return nil, errors.New("invalid sample rate")
	}
	if sourceRateHz == targetRateHz {
		return append([]byte(nil), audioPCM...), nil
	}
	if len(audioPCM)%2 != 0 {
		return nil, errors.New("pcm payload length must be even")
	}

	sourceSamples := len(audioPCM) / 2
	if sourceSamples == 0 {
		return nil, nil
	}
	targetSamples := int(math.Round(float64(sourceSamples) * float64(targetRateHz) / float64(sourceRateHz)))
	if targetSamples <= 0 {
		return nil, errors.New("target sample count is invalid")
	}

	values := make([]int16, sourceSamples)
	for index := 0; index < sourceSamples; index++ {
		values[index] = int16(binary.LittleEndian.Uint16(audioPCM[index*2 : index*2+2]))
	}

	out := make([]byte, targetSamples*2)
	if targetSamples == 1 {
		binary.LittleEndian.PutUint16(out[0:2], uint16(values[0]))
		return out, nil
	}

	ratio := float64(sourceSamples-1) / float64(targetSamples-1)
	for index := 0; index < targetSamples; index++ {
		position := float64(index) * ratio
		base := int(math.Floor(position))
		if base >= sourceSamples-1 {
			binary.LittleEndian.PutUint16(out[index*2:index*2+2], uint16(values[sourceSamples-1]))
			continue
		}
		fraction := position - float64(base)
		left := float64(values[base])
		right := float64(values[base+1])
		sample := int16(math.Round(left + (right-left)*fraction))
		binary.LittleEndian.PutUint16(out[index*2:index*2+2], uint16(sample))
	}
	return out, nil
}

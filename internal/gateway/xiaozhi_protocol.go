package gateway

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

const (
	xiaozhiBinaryProtocolV1 = 1
	xiaozhiBinaryProtocolV2 = 2
	xiaozhiBinaryProtocolV3 = 3

	xiaozhiBinaryTypeAudio = 0

	xiaozhiBinaryProtocol2HeaderSize = 16
	xiaozhiBinaryProtocol3HeaderSize = 4
)

func normalizeXiaozhiProtocolVersion(version int) int {
	switch version {
	case xiaozhiBinaryProtocolV2, xiaozhiBinaryProtocolV3:
		return version
	default:
		return xiaozhiBinaryProtocolV1
	}
}

func parseXiaozhiProtocolVersion(raw string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return xiaozhiBinaryProtocolV1
	}
	return normalizeXiaozhiProtocolVersion(parsed)
}

func resolveXiaozhiProtocolVersion(headerValue string, helloVersion, fallback int) int {
	version := normalizeXiaozhiProtocolVersion(fallback)
	if strings.TrimSpace(headerValue) != "" {
		version = parseXiaozhiProtocolVersion(headerValue)
	}
	if helloVersion > 0 {
		version = normalizeXiaozhiProtocolVersion(helloVersion)
	}
	return version
}

func unwrapXiaozhiBinaryFrame(payload []byte, version int) ([]byte, error) {
	switch normalizeXiaozhiProtocolVersion(version) {
	case xiaozhiBinaryProtocolV1:
		return append([]byte(nil), payload...), nil
	case xiaozhiBinaryProtocolV2:
		if len(payload) < xiaozhiBinaryProtocol2HeaderSize {
			return nil, fmt.Errorf("xiaozhi protocol v2 frame shorter than %d-byte header", xiaozhiBinaryProtocol2HeaderSize)
		}
		frameVersion := int(binary.BigEndian.Uint16(payload[0:2]))
		if normalizeXiaozhiProtocolVersion(frameVersion) != xiaozhiBinaryProtocolV2 {
			return nil, fmt.Errorf("xiaozhi protocol v2 header declared version %d", frameVersion)
		}
		frameType := binary.BigEndian.Uint16(payload[2:4])
		if frameType != xiaozhiBinaryTypeAudio {
			return nil, fmt.Errorf("unsupported xiaozhi protocol v2 frame type %d", frameType)
		}
		payloadSize := int(binary.BigEndian.Uint32(payload[12:16]))
		if payloadSize < 0 || payloadSize > len(payload)-xiaozhiBinaryProtocol2HeaderSize {
			return nil, fmt.Errorf("xiaozhi protocol v2 payload_size %d exceeds frame body", payloadSize)
		}
		start := xiaozhiBinaryProtocol2HeaderSize
		end := start + payloadSize
		return append([]byte(nil), payload[start:end]...), nil
	case xiaozhiBinaryProtocolV3:
		if len(payload) < xiaozhiBinaryProtocol3HeaderSize {
			return nil, fmt.Errorf("xiaozhi protocol v3 frame shorter than %d-byte header", xiaozhiBinaryProtocol3HeaderSize)
		}
		frameType := int(payload[0])
		if frameType != xiaozhiBinaryTypeAudio {
			return nil, fmt.Errorf("unsupported xiaozhi protocol v3 frame type %d", frameType)
		}
		payloadSize := int(binary.BigEndian.Uint16(payload[2:4]))
		if payloadSize < 0 || payloadSize > len(payload)-xiaozhiBinaryProtocol3HeaderSize {
			return nil, fmt.Errorf("xiaozhi protocol v3 payload_size %d exceeds frame body", payloadSize)
		}
		start := xiaozhiBinaryProtocol3HeaderSize
		end := start + payloadSize
		return append([]byte(nil), payload[start:end]...), nil
	default:
		return nil, fmt.Errorf("unsupported xiaozhi protocol version %d", version)
	}
}

func wrapXiaozhiBinaryFrame(payload []byte, version int) ([]byte, error) {
	payload = append([]byte(nil), payload...)
	switch normalizeXiaozhiProtocolVersion(version) {
	case xiaozhiBinaryProtocolV1:
		return payload, nil
	case xiaozhiBinaryProtocolV2:
		frame := make([]byte, xiaozhiBinaryProtocol2HeaderSize+len(payload))
		binary.BigEndian.PutUint16(frame[0:2], uint16(xiaozhiBinaryProtocolV2))
		binary.BigEndian.PutUint16(frame[2:4], uint16(xiaozhiBinaryTypeAudio))
		binary.BigEndian.PutUint32(frame[12:16], uint32(len(payload)))
		copy(frame[xiaozhiBinaryProtocol2HeaderSize:], payload)
		return frame, nil
	case xiaozhiBinaryProtocolV3:
		if len(payload) > 0xFFFF {
			return nil, fmt.Errorf("xiaozhi protocol v3 payload exceeds 65535 bytes")
		}
		frame := make([]byte, xiaozhiBinaryProtocol3HeaderSize+len(payload))
		frame[0] = xiaozhiBinaryTypeAudio
		binary.BigEndian.PutUint16(frame[2:4], uint16(len(payload)))
		copy(frame[xiaozhiBinaryProtocol3HeaderSize:], payload)
		return frame, nil
	default:
		return nil, fmt.Errorf("unsupported xiaozhi protocol version %d", version)
	}
}

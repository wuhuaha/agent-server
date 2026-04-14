package session

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSessionAlreadyActive = errors.New("session already active")
	ErrNoActiveSession      = errors.New("no active session")
)

type StartRequest struct {
	RequestedSessionID string
	DeviceID           string
	ClientType         string
	FirmwareVersion    string
	Mode               string
	InputCodec         string
	InputSampleRate    int
	InputChannels      int
	ClientCanEnd       bool
	ServerCanEnd       bool
}

type Snapshot struct {
	SessionID       string
	State           State
	CloseReason     CloseReason
	DeviceID        string
	ClientType      string
	FirmwareVersion string
	Mode            string
	InputCodec      string
	InputSampleRate int
	InputChannels   int
	InputFrames     int
	AudioBytes      int
	Turns           int
	StartedAt       time.Time
	LastActivityAt  time.Time
	ClientCanEnd    bool
	ServerCanEnd    bool
}

type CommittedTurn struct {
	Snapshot Snapshot
	AudioPCM []byte
}

type RealtimeSession struct {
	mu              sync.Mutex
	active          bool
	current         Snapshot
	turnAudioChunks [][]byte
}

func NewRealtimeSession() *RealtimeSession {
	return &RealtimeSession{
		current: Snapshot{State: StateIdle},
	}
}

func (s *RealtimeSession) Start(req StartRequest) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return Snapshot{}, ErrSessionAlreadyActive
	}

	now := time.Now().UTC()
	sessionID := req.RequestedSessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess_%d", now.UnixNano())
	}

	s.current = Snapshot{
		SessionID:       sessionID,
		State:           StateActive,
		DeviceID:        req.DeviceID,
		ClientType:      req.ClientType,
		FirmwareVersion: req.FirmwareVersion,
		Mode:            req.Mode,
		InputCodec:      req.InputCodec,
		InputSampleRate: req.InputSampleRate,
		InputChannels:   req.InputChannels,
		StartedAt:       now,
		LastActivityAt:  now,
		ClientCanEnd:    req.ClientCanEnd,
		ServerCanEnd:    req.ServerCanEnd,
	}
	s.turnAudioChunks = nil
	s.active = true

	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) IngestAudioFrame(payload []byte) (Snapshot, error) {
	owned := append([]byte(nil), payload...)
	return s.IngestOwnedAudioFrame(owned)
}

func (s *RealtimeSession) IngestOwnedAudioFrame(payload []byte) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.State = StateActive
	s.current.InputFrames++
	s.current.AudioBytes += len(payload)
	if len(payload) > 0 {
		s.turnAudioChunks = append(s.turnAudioChunks, payload)
	}
	s.current.LastActivityAt = time.Now().UTC()
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) AcceptText() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.State = StateActive
	s.current.LastActivityAt = time.Now().UTC()
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) CommitTurn() (CommittedTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return CommittedTurn{}, ErrNoActiveSession
	}

	s.current.State = StateThinking
	s.current.Turns++
	s.current.LastActivityAt = time.Now().UTC()
	audioPCM := flattenTurnAudioChunks(s.turnAudioChunks, s.current.AudioBytes)
	s.turnAudioChunks = nil
	return CommittedTurn{
		Snapshot: cloneSnapshot(s.current),
		AudioPCM: audioPCM,
	}, nil
}

func (s *RealtimeSession) SetState(state State) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.State = state
	s.current.LastActivityAt = time.Now().UTC()
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) End(reason CloseReason) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.State = StateClosing
	s.current.CloseReason = reason
	s.current.LastActivityAt = time.Now().UTC()
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) ClearTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.current.InputFrames = 0
	s.current.AudioBytes = 0
	s.turnAudioChunks = nil
	s.current.LastActivityAt = time.Now().UTC()
}

func (s *RealtimeSession) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.active = false
	s.current = Snapshot{State: StateIdle}
	s.turnAudioChunks = nil
}

func (s *RealtimeSession) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return cloneSnapshot(s.current)
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	return snapshot
}

func flattenTurnAudioChunks(chunks [][]byte, totalBytes int) []byte {
	switch len(chunks) {
	case 0:
		return nil
	case 1:
		return chunks[0]
	}
	if totalBytes <= 0 {
		for _, chunk := range chunks {
			totalBytes += len(chunk)
		}
	}
	audio := make([]byte, 0, totalBytes)
	for _, chunk := range chunks {
		audio = append(audio, chunk...)
	}
	return audio
}

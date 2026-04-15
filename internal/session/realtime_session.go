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
	InputState      InputState
	OutputState     OutputState
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
		current: idleSnapshot(),
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
		InputState:      InputStateActive,
		OutputState:     OutputStateIdle,
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
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, true)
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

	s.current.InputState = InputStateActive
	s.current.InputFrames++
	s.current.AudioBytes += len(payload)
	if len(payload) > 0 {
		s.turnAudioChunks = append(s.turnAudioChunks, payload)
	}
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) AcceptText() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.InputState = InputStateActive
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) CommitTurn() (CommittedTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return CommittedTurn{}, ErrNoActiveSession
	}

	s.current.InputState = InputStateCommitted
	switch s.current.OutputState {
	case OutputStateSpeaking, OutputStateClosing:
		// Keep the output lane unchanged so accepted follow-up input can coexist
		// with ongoing playback or shutdown handling.
	default:
		s.current.OutputState = OutputStateThinking
	}
	s.current.Turns++
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
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

	applyCompatState(&s.current, state)
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) SetInputState(state InputState) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.InputState = normalizeInputState(state)
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) SetOutputState(state OutputState) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.OutputState = normalizeOutputState(state)
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
	return cloneSnapshot(s.current), nil
}

func (s *RealtimeSession) End(reason CloseReason) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return Snapshot{}, ErrNoActiveSession
	}

	s.current.InputState = InputStateClosing
	s.current.OutputState = OutputStateClosing
	s.current.CloseReason = reason
	s.current.LastActivityAt = time.Now().UTC()
	s.current.State = deriveCompatState(s.current.InputState, s.current.OutputState, s.active)
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
	s.current = idleSnapshot()
	s.turnAudioChunks = nil
}

func (s *RealtimeSession) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return cloneSnapshot(s.current)
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.State = deriveCompatState(snapshot.InputState, snapshot.OutputState, snapshot.SessionID != "")
	return snapshot
}

func idleSnapshot() Snapshot {
	return Snapshot{
		State:       StateIdle,
		InputState:  InputStateIdle,
		OutputState: OutputStateIdle,
	}
}

func deriveCompatState(input InputState, output OutputState, active bool) State {
	if !active {
		return StateIdle
	}
	switch output {
	case OutputStateClosing:
		return StateClosing
	case OutputStateSpeaking:
		return StateSpeaking
	case OutputStateThinking:
		return StateThinking
	default:
		return StateActive
	}
}

func applyCompatState(snapshot *Snapshot, state State) {
	switch state {
	case StateIdle:
		snapshot.InputState = InputStateIdle
		snapshot.OutputState = OutputStateIdle
	case StateThinking:
		if snapshot.InputState == InputStateIdle || snapshot.InputState == InputStateClosing {
			snapshot.InputState = InputStateCommitted
		}
		snapshot.OutputState = OutputStateThinking
	case StateSpeaking:
		if snapshot.InputState == InputStateIdle || snapshot.InputState == InputStateClosing {
			snapshot.InputState = InputStateCommitted
		}
		snapshot.OutputState = OutputStateSpeaking
	case StateClosing:
		snapshot.InputState = InputStateClosing
		snapshot.OutputState = OutputStateClosing
	case StateActive:
		fallthrough
	default:
		snapshot.InputState = InputStateActive
		snapshot.OutputState = OutputStateIdle
	}
}

func normalizeInputState(state InputState) InputState {
	switch state {
	case InputStateIdle, InputStateActive, InputStatePreviewing, InputStateCommitted, InputStateClosing:
		return state
	default:
		return InputStateActive
	}
}

func normalizeOutputState(state OutputState) OutputState {
	switch state {
	case OutputStateIdle, OutputStateThinking, OutputStateSpeaking, OutputStateClosing:
		return state
	default:
		return OutputStateIdle
	}
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

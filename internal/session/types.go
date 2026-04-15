package session

type State string
type InputState string
type OutputState string

type CloseReason string

const (
	StateIdle     State = "idle"
	StateActive   State = "active"
	StateThinking State = "thinking"
	StateSpeaking State = "speaking"
	StateClosing  State = "closing"
)

const (
	InputStateIdle      InputState = "idle"
	InputStateActive    InputState = "active"
	InputStatePreviewing InputState = "previewing"
	InputStateCommitted InputState = "committed"
	InputStateClosing   InputState = "closing"
)

const (
	OutputStateIdle     OutputState = "idle"
	OutputStateThinking OutputState = "thinking"
	OutputStateSpeaking OutputState = "speaking"
	OutputStateClosing  OutputState = "closing"
)

const (
	CloseReasonClientStop    CloseReason = "client_stop"
	CloseReasonServerStop    CloseReason = "server_stop"
	CloseReasonCompleted     CloseReason = "completed"
	CloseReasonWakeCancelled CloseReason = "wake_cancelled"
	CloseReasonDeviceSleep   CloseReason = "device_sleep"
	CloseReasonNetworkStop   CloseReason = "network_shutdown"
	CloseReasonIdle          CloseReason = "idle_timeout"
	CloseReasonMaxDuration   CloseReason = "max_duration"
	CloseReasonError         CloseReason = "error"
)

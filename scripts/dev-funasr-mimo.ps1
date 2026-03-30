param(
  [string]$AsrUrl = "http://127.0.0.1:8091/v1/asr/transcribe",
  [string]$Language = "auto",
  [string]$Voice = "mimo_default",
  [string]$Style = ""
)

if (-not $env:AGENT_SERVER_ADDR) {
  $env:AGENT_SERVER_ADDR = ":8080"
}

if (-not $env:MIMO_API_KEY) {
  $userKey = [System.Environment]::GetEnvironmentVariable("MIMO_API_KEY", "User")
  if ($userKey) {
    $env:MIMO_API_KEY = $userKey
  }
}

if (-not $env:MIMO_API_KEY) {
  throw "MIMO_API_KEY is not configured in the current process or the current Windows user environment."
}

$env:AGENT_SERVER_VOICE_PROVIDER = "funasr_http"
$env:AGENT_SERVER_VOICE_ASR_URL = $AsrUrl
$env:AGENT_SERVER_VOICE_ASR_LANGUAGE = $Language
$env:AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO = "true"

$env:AGENT_SERVER_TTS_PROVIDER = "mimo_v2_tts"
$env:AGENT_SERVER_TTS_MIMO_VOICE = $Voice
$env:AGENT_SERVER_TTS_MIMO_STYLE = $Style

go run ./cmd/agentd

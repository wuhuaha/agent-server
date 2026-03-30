param(
  [string]$AsrUrl = "http://127.0.0.1:8091/v1/asr/transcribe",
  [string]$Language = "auto"
)

if (-not $env:AGENT_SERVER_ADDR) {
  $env:AGENT_SERVER_ADDR = ":8080"
}

$env:AGENT_SERVER_VOICE_PROVIDER = "funasr_http"
$env:AGENT_SERVER_VOICE_ASR_URL = $AsrUrl
$env:AGENT_SERVER_VOICE_ASR_LANGUAGE = $Language
$env:AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO = "true"

go run ./cmd/agentd

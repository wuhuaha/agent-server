param(
  [string]$CondaEnv = "xiaozhi-esp32-server",
  [string]$BindHost = "127.0.0.1",
  [int]$Port = 8091,
  [string]$Model = "iic/SenseVoiceSmall",
  [string]$Device = "cpu",
  [string]$Language = "auto"
)

$repoRoot = Split-Path -Parent $PSScriptRoot
$env:PYTHONPATH = Join-Path $repoRoot "workers\python\src"
$env:AGENT_SERVER_FUNASR_HOST = $BindHost
$env:AGENT_SERVER_FUNASR_PORT = "$Port"
$env:AGENT_SERVER_FUNASR_MODEL = $Model
$env:AGENT_SERVER_FUNASR_DEVICE = $Device
$env:AGENT_SERVER_FUNASR_LANGUAGE = $Language
$env:AGENT_SERVER_FUNASR_DISABLE_UPDATE = "true"
$env:AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE = "true"
$env:AGENT_SERVER_FUNASR_USE_ITN = "true"

conda run -n $CondaEnv python -m agent_server_workers.funasr_service --host $BindHost --port $Port --model $Model --device $Device --language $Language --trust-remote-code --disable-update --use-itn

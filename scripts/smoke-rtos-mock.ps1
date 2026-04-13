param(
  [string]$OutputDir = "",
  [string]$SpeechText = "hello from agent server",
  [int]$ServerPort = 18080,
  [int]$WorkerPort = 18091,
  [switch]$EnableBargeIn
)

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $OutputDir) {
  $OutputDir = Join-Path $repoRoot ("artifacts\live-smoke\{0}\rtos-mock" -f (Get-Date).ToString("yyyyMMdd"))
}

$wavPath = Join-Path $OutputDir "input.wav"
$reportPath = Join-Path $OutputDir "report.json"
$rxPath = Join-Path $OutputDir "received-audio.wav"
$workerLog = Join-Path $OutputDir "worker.log"
$workerErr = Join-Path $OutputDir "worker.err.log"
$serverLog = Join-Path $OutputDir "agentd.log"
$serverErr = Join-Path $OutputDir "agentd.err.log"
$previousAgentServerAddr = $env:AGENT_SERVER_ADDR

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

Add-Type -AssemblyName System.Speech
$fmt = New-Object System.Speech.AudioFormat.SpeechAudioFormatInfo(
  16000,
  [System.Speech.AudioFormat.AudioBitsPerSample]::Sixteen,
  [System.Speech.AudioFormat.AudioChannel]::Mono
)
$synth = New-Object System.Speech.Synthesis.SpeechSynthesizer
$synth.SetOutputToWaveFile($wavPath, $fmt)
$synth.Speak($SpeechText)
$synth.Dispose()

$workerProc = Start-Process -FilePath "powershell" -ArgumentList "-ExecutionPolicy","Bypass","-File",(Join-Path $repoRoot "scripts\start-funasr-worker.ps1"),"-BindHost","127.0.0.1","-Port","$WorkerPort","-Device","cpu" -WorkingDirectory $repoRoot -RedirectStandardOutput $workerLog -RedirectStandardError $workerErr -PassThru
$serverProc = $null

try {
  $deadline = (Get-Date).AddSeconds(20)
  do {
    Start-Sleep -Milliseconds 500
    try {
      $workerHealth = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$WorkerPort/healthz"
      if ($workerHealth.StatusCode -eq 200) { break }
    } catch {}
  } while ((Get-Date) -lt $deadline)

  if (-not $workerHealth) {
    throw "FunASR worker failed to become healthy."
  }

  $env:AGENT_SERVER_ADDR = ":$ServerPort"
  $serverProc = Start-Process -FilePath "powershell" -ArgumentList "-ExecutionPolicy","Bypass","-File",(Join-Path $repoRoot "scripts\dev-funasr-mimo.ps1"),"-AsrUrl","http://127.0.0.1:$WorkerPort/v1/asr/transcribe" -WorkingDirectory $repoRoot -RedirectStandardOutput $serverLog -RedirectStandardError $serverErr -PassThru

  $deadline = (Get-Date).AddSeconds(20)
  do {
    Start-Sleep -Milliseconds 500
    try {
      $serverHealth = Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$ServerPort/healthz"
      if ($serverHealth.StatusCode -eq 200) { break }
    } catch {}
  } while ((Get-Date) -lt $deadline)

  if (-not $serverHealth) {
    throw "agentd failed to become healthy."
  }

  Push-Location (Join-Path $repoRoot "clients\python-desktop-client")
  try {
    $env:PYTHONPATH = "src"
    $args = @(
      "-m", "agent_server_desktop_client.rtos_mock",
      "--http-base", "http://127.0.0.1:$ServerPort",
      "--wav", $wavPath,
      "--save-rx", $rxPath,
      "--save-rx-dir", $OutputDir,
      "--output", $reportPath,
      "--timeout-sec", "60"
    )
    if ($EnableBargeIn) {
      $args += @("--interrupt-wav", $wavPath)
    }
    python @args
  } finally {
    Pop-Location
  }
}
finally {
  $env:AGENT_SERVER_ADDR = $previousAgentServerAddr
  if ($serverProc -and -not $serverProc.HasExited) {
    Stop-Process -Id $serverProc.Id -Force -ErrorAction SilentlyContinue
  }
  if ($workerProc -and -not $workerProc.HasExited) {
    Stop-Process -Id $workerProc.Id -Force -ErrorAction SilentlyContinue
  }
  Get-NetTCPConnection -LocalPort $ServerPort,$WorkerPort -State Listen -ErrorAction SilentlyContinue |
    Select-Object -ExpandProperty OwningProcess -Unique |
    ForEach-Object { Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue }
}

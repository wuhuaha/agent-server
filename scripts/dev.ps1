if (-not $env:AGENT_SERVER_ADDR) {
  $env:AGENT_SERVER_ADDR = ':8080'
}

go run ./cmd/agentd

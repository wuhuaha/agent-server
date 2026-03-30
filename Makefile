run:
	go run ./cmd/agentd

test:
	go test ./...

fmt:
	gofmt -w cmd internal pkg

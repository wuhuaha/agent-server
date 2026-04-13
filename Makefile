.PHONY: run test test-go test-py fmt doctor docker-config verify-fast bootstrap-linux

run:
	go run ./cmd/agentd

test: verify-fast

test-go:
	go test ./...

test-py:
	PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v

fmt:
	gofmt -w cmd internal pkg

doctor:
	./scripts/codex-doctor.sh

docker-config:
	./scripts/docker-config-check.sh

verify-fast:
	./scripts/verify-fast.sh

bootstrap-linux:
	./scripts/install-linux-stack.sh

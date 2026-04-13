.PHONY: run test test-go test-py test-py-workers fmt doctor docker-config verify-fast bootstrap-linux

PYTHON ?= python3

run:
	go run ./cmd/agentd

test: verify-fast

test-go:
	go test ./...

test-py:
	PYTHON_BIN="$(PYTHON)" bash scripts/test-python-desktop.sh

test-py-workers:
	PYTHON_BIN="$(PYTHON)" bash scripts/test-python-workers.sh

fmt:
	gofmt -w cmd internal pkg

doctor:
	PYTHON_BIN="$(PYTHON)" bash scripts/codex-doctor.sh

docker-config:
	bash scripts/docker-config-check.sh

verify-fast:
	PYTHON_BIN="$(PYTHON)" bash scripts/verify-fast.sh

bootstrap-linux:
	bash scripts/install-linux-stack.sh

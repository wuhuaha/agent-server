.PHONY: run test test-go test-go-unit test-go-integration test-go-system test-py test-py-workers fmt doctor docker-config verify-fast bootstrap-linux

PYTHON ?= python3

run:
	go run ./cmd/agentd

test: verify-fast

test-go:
	bash scripts/test-go-unit.sh

test-go-unit:
	bash scripts/test-go-unit.sh

test-go-integration:
	bash scripts/test-go-integration.sh

test-go-system:
	bash scripts/test-go-system.sh

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

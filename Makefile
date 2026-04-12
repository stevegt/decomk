# Intent: Provide one top-level entrypoint for stage-0 generation/drift checks
# so developers and CI can enforce template/example sync consistently.
# Source: DI-001-20260312-141200 (TODO/001)

.PHONY: generate check-generated test verify selftest-devpod selftest-codespaces selftest-codespaces-clean

all: verify

generate:
	go generate ./...

check-generated:
	go run ./cmd/stage0gen -check

test:
	go test ./...

verify: generate check-generated test

# Intent: Provide stable top-level wrappers for both local DevPod and
# Codespaces parity selftests so operators can run the same harness flows via
# `make` without memorizing script paths or cleanup flags. Source:
# DI-007-20260412-043200 (TODO/007)
selftest-devpod:
	examples/decomk-selftest/devpod-local/run.sh

selftest-codespaces:
	examples/decomk-selftest/codespaces/run.sh

selftest-codespaces-clean:
	examples/decomk-selftest/codespaces/run.sh --cleanup

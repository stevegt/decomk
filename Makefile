# Intent: Provide one top-level entrypoint for stage-0 generation/drift checks
# so developers and CI can enforce template/example sync consistently.
# Source: DI-001-20260312-141200 (TODO/001)

.PHONY: generate check-generated check-no-shell-swallow check-no-go-blank-assign check-errcheck test verify selftest-devpod selftest-codespaces selftest-codespaces-clean

all: verify

generate:
	go generate ./...

check-generated:
	go run ./cmd/stage0gen -check

test:
	go test ./...

# Intent: Enforce fail-fast error-handling policy in both shell and Go code so
# CI and local verify runs reject silent failures before runtime.
# Source: DI-008-20260412-122157 (TODO/008)
check-no-shell-swallow:
	@if rg -n '\|\|[[:space:]]*true' --glob 'Makefile' --glob '*.mk' --glob '*.sh' --glob '*.tmpl' . | rg -v "disallowed '\\|\\| true'"; then \
		echo "disallowed '|| true' found; handle command exit codes explicitly"; \
		exit 1; \
	fi

check-no-go-blank-assign:
	@if rg -n --glob '*.go' '^\s*_\s*=' .; then \
		echo "disallowed '_ = ...' found in Go code; handle errors explicitly"; \
		exit 1; \
	fi

check-errcheck:
	@if ! command -v errcheck >/dev/null 2>&1; then \
		echo "missing required tool: errcheck"; \
		echo "install with: go install github.com/kisielk/errcheck@latest"; \
		exit 1; \
	fi
	errcheck ./...

verify: generate check-generated check-no-shell-swallow check-no-go-blank-assign check-errcheck test

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

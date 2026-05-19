# Intent: Provide one top-level entrypoint for stage-0 generation/drift checks
# so developers and CI can enforce template/example sync consistently.
# Source: DI-tikub (TODO-jirin)

.PHONY: generate check-generated check-no-shell-swallow check-no-go-blank-assign check-errcheck test verify selftest-devpod selftest-codespaces selftest-codespaces-clean release-minor promote-testing promote-stable

all: verify

ship: test commit-push selftest-codespaces release-minor install-latest

commit-push:
	git add -A
	# 'grok' is stevegt/grokker, not the xAI thing
	grok commit | git commit -F-
	git push

generate:
	go generate ./...

check-generated:
	# Intent: Keep both stage-0 and conf-repo generated examples in sync with
	# embedded templates so `decomk init` (consumer and `-conf` producer mode)
	# outputs cannot drift from checked-in references. Source:
	# DI-migil (TODO-rufiz)
	go run ./cmd/stage0gen -check
	go run ./cmd/confrepogen -check
	go run ./cmd/versiongen -check

test:
	go test ./...

# Intent: Enforce fail-fast error-handling policy in both shell and Go code so
# CI and local verify runs reject silent failures before runtime.
# Source: DI-golak (TODO-gamuz)
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
# DI-topir (TODO-fuviv)
selftest-devpod:
	examples/decomk-selftest/devpod-local/run.sh

selftest-codespaces:
	examples/decomk-selftest/codespaces/run.sh

selftest-codespaces-clean:
	examples/decomk-selftest/codespaces/run.sh --cleanup

# Intent: Provide a one-command release operator entrypoint that bumps VERSION,
# commits, tags, and pushes through an explicit scripted workflow.
# Source: DI-gavaj (TODO-jirin)
release-minor:
	bash scripts/release.sh minor

# Intent: Keep testing/stable branch promotion explicit and scripted so channel
# movement is fast-forward-only and repeatable across operators.
# Source: DI-vikid (TODO-jirin)
promote-testing:
	bash scripts/release.sh promote-testing

promote-stable:
	bash scripts/release.sh promote-stable

install-latest:
	latest_version=$$(git tag | sort -n -t '.' -k 2 | tail -1); \
	go install github.com/stevegt/decomk/cmd/decomk@$${latest_version}; \
	echo "Installed decomk version $${latest_version}"

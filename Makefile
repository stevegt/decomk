# Intent: Provide one top-level entrypoint for stage-0 generation/drift checks
# so developers and CI can enforce template/example sync consistently.
# Source: DI-001-20260312-141200 (TODO/001)

.PHONY: generate check-generated test verify

all: verify

generate:
	go generate ./...

check-generated:
	go run ./cmd/stage0gen -check

test:
	go test ./...

verify: generate check-generated test

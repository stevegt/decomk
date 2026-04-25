SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c
.RECIPEPREFIX := >

# Intent: Keep self-test pass/fail logic in make targets and called scripts so
# run.sh can act only as a harness that reads emitted log markers.
# Source: DI-007-20260311-145221 (TODO/007)
SELFTEST_SCRIPT_DIR := $(DECOMK_HOME)/conf/scripts

# Intent: Ensure fixture scripts see decomk bootstrap/runtime env variables even
# when make is invoked with tuple-heavy argv and context-expanded targets.
# Source: DI-007-20260412-171000 (TODO/007)
export DECOMK_HOME
export DECOMK_LOG_DIR
export DECOMK_TOOL_URI
export DECOMK_CONF_URI

# Intent: Lock selftest identity expectations to root for both stage-0 and make
# so harness runs verify the root-only execution contract across providers.
# Source: DI-007-20260424-200248 (TODO/007)
SELFTEST_EXPECTED_USER ?= root
SELFTEST_EXPECTED_UID ?= 0

# Intent: Keep idempotent fixture targets as real file targets (not .PHONY), and
# keep stamp-regression state inside DECOMK_STAMPDIR so all make-run artifacts
# live under decomk's stamp working directory.
# Source: DI-007-20260313-101500 (TODO/007)
SELFTEST_STAMP_COUNT := $(DECOMK_STAMPDIR)/selftest-stamp-count

# Intent: `all` is an orchestration alias for grouped checks; keep it phony and
# never stamp it so aggregate runs are not skipped by stale stamp files.
# Source: DI-007-20260313-181500 (TODO/007)
.PHONY: all
all: selftest-verify-tool-repo selftest-verify-conf-repo selftest-context-repo selftest-context-shared
>@echo "SELFTEST PASS all-target"

selftest-verify-tool-repo:
>bash "$(SELFTEST_SCRIPT_DIR)/verify_tool_repo.sh"
>@touch $@

selftest-verify-conf-repo:
>bash "$(SELFTEST_SCRIPT_DIR)/verify_conf_repo.sh"
>@touch $@

selftest-context-default:
>@echo "SELFTEST FAIL context-default-target-executed"
>@exit 42

selftest-context-repo:
>@echo "SELFTEST PASS context-override"
>@touch $@

selftest-context-shared:
>@echo "SELFTEST PASS default-tuple-available"
>@touch $@

selftest-stamp-probe:
>@if [[ "$$PWD" != "$$DECOMK_STAMPDIR" ]]; then \
>  echo "SELFTEST FAIL stamp-working-dir-mismatch expected=$$DECOMK_STAMPDIR actual=$$PWD"; \
>  exit 1; \
>fi
>@count=0
>@if [[ -f "$(SELFTEST_STAMP_COUNT)" ]]; then count="$$(cat "$(SELFTEST_STAMP_COUNT)")"; fi
>@count=$$((count + 1))
>@echo "$$count" > "$(SELFTEST_STAMP_COUNT)"
>@echo "SELFTEST PASS stamp-dir-working-dir"
>@echo "SELFTEST PASS stamp-probe-ran"
>@touch $@

selftest-stamp-verify:
>@if [[ ! -f "$(SELFTEST_STAMP_COUNT)" ]]; then \
>  echo "SELFTEST FAIL stamp-count-missing"; \
>  exit 1; \
>fi
>@count="$$(cat "$(SELFTEST_STAMP_COUNT)")"
>@if [[ "$$count" != "1" ]]; then \
>  echo "SELFTEST FAIL stamp-count-expected-1 actual=$$count"; \
>  exit 1; \
>fi
>@echo "SELFTEST PASS stamp-idempotent"
>@touch $@

# Intent: Validate that stage-0 phase routing is explicit and that user identity
# checks can distinguish prebuild/common (`updateContent`) from runtime/user
# (`postCreate`) invocations.
# Source: DI-001-20260416-223600 (TODO/001)
#
# Intent: Emit make-side identity markers with phase labels so selftests can
# correlate runtime user identity between stage-0 logs and make logs.
# Source: DI-007-20260423-180202 (TODO/007)
selftest-phase-update-content:
>@if [[ "$${DECOMK_STAGE0_PHASE:-}" != "updateContent" ]]; then \
>  echo "SELFTEST FAIL phase-expected-updateContent actual=$${DECOMK_STAGE0_PHASE:-<unset>}"; \
>  exit 1; \
>fi
>@if [[ -n "$${GITHUB_USER:-}" ]]; then \
>  echo "SELFTEST FAIL github-user-expected-empty-in-updateContent actual=$${GITHUB_USER}"; \
>  exit 1; \
>fi
>@if [[ "$$(id -un)" != "$(SELFTEST_EXPECTED_USER)" ]] || [[ "$$(id -u)" != "$(SELFTEST_EXPECTED_UID)" ]]; then \
>  echo "SELFTEST FAIL make-identity-mismatch phase=updateContent expected_user=$(SELFTEST_EXPECTED_USER) expected_uid=$(SELFTEST_EXPECTED_UID) actual_user=$$(id -un) actual_uid=$$(id -u)"; \
>  exit 1; \
>fi
>@if [[ "$${DECOMK_STAGE0_USER:-}" != "$(SELFTEST_EXPECTED_USER)" ]] || [[ "$${DECOMK_STAGE0_UID:-}" != "$(SELFTEST_EXPECTED_UID)" ]]; then \
>  echo "SELFTEST FAIL stage0-identity-mismatch phase=updateContent expected_user=$(SELFTEST_EXPECTED_USER) expected_uid=$(SELFTEST_EXPECTED_UID) stage0_user=$${DECOMK_STAGE0_USER:-<unset>} stage0_uid=$${DECOMK_STAGE0_UID:-<unset>}"; \
>  exit 1; \
>fi
>@echo "SELFTEST PASS phase-updateContent"
>@echo "SELFTEST PASS github-user-empty-in-updateContent"
>@echo "SELFTEST PASS identity-match phase=updateContent user=$(SELFTEST_EXPECTED_USER) uid=$(SELFTEST_EXPECTED_UID)"
>@echo "SELFTEST PASS make-id phase=updateContent uid=$$(id -u) user=$$(id -un) stage0_uid=$${DECOMK_STAGE0_UID:-<unset>} stage0_user=$${DECOMK_STAGE0_USER:-<unset>}"
>@touch $@

selftest-phase-post-create:
>@if [[ "$${DECOMK_STAGE0_PHASE:-}" != "postCreate" ]]; then \
>  echo "SELFTEST FAIL phase-expected-postCreate actual=$${DECOMK_STAGE0_PHASE:-<unset>}"; \
>  exit 1; \
>fi
>@if [[ -z "$${GITHUB_USER:-}" ]]; then \
>  echo "SELFTEST FAIL github-user-expected-nonempty-in-postCreate"; \
>  exit 1; \
>fi
>@if [[ "$$(id -un)" != "$(SELFTEST_EXPECTED_USER)" ]] || [[ "$$(id -u)" != "$(SELFTEST_EXPECTED_UID)" ]]; then \
>  echo "SELFTEST FAIL make-identity-mismatch phase=postCreate expected_user=$(SELFTEST_EXPECTED_USER) expected_uid=$(SELFTEST_EXPECTED_UID) actual_user=$$(id -un) actual_uid=$$(id -u)"; \
>  exit 1; \
>fi
>@if [[ "$${DECOMK_STAGE0_USER:-}" != "$(SELFTEST_EXPECTED_USER)" ]] || [[ "$${DECOMK_STAGE0_UID:-}" != "$(SELFTEST_EXPECTED_UID)" ]]; then \
>  echo "SELFTEST FAIL stage0-identity-mismatch phase=postCreate expected_user=$(SELFTEST_EXPECTED_USER) expected_uid=$(SELFTEST_EXPECTED_UID) stage0_user=$${DECOMK_STAGE0_USER:-<unset>} stage0_uid=$${DECOMK_STAGE0_UID:-<unset>}"; \
>  exit 1; \
>fi
>@echo "SELFTEST PASS phase-postCreate"
>@echo "SELFTEST PASS github-user-present-in-postCreate"
>@echo "SELFTEST PASS identity-match phase=postCreate user=$(SELFTEST_EXPECTED_USER) uid=$(SELFTEST_EXPECTED_UID)"
>@echo "SELFTEST PASS make-id phase=postCreate uid=$$(id -u) user=$$(id -un) stage0_uid=$${DECOMK_STAGE0_UID:-<unset>} stage0_user=$${DECOMK_STAGE0_USER:-<unset>}"
>@touch $@

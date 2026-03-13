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
# Source: DI-007-20260311-145221 (TODO/007)
export DECOMK_HOME
export DECOMK_LOG_DIR
export DECOMK_TOOL_REPO
export DECOMK_CONF_REPO

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

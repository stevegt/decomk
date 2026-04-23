SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c
.RECIPEPREFIX := >

# - decomk expands tuples from decomk.conf, then calls `make` in a stamp
#   directory (`DECOMK_STAMPDIR`), not in this repo root.
# - Because of that, scripts in this repo should be referenced with an absolute
#   path derived from `DECOMK_HOME` (which points at the decomk state root).
# - Targets should normally end with `touch $@` so repeated runs are
#   idempotent.  Exceptions include those cases where you definitely want
#   to run the stanza every time, e.g. to update content, or any
#   "parent" targets that just call other targets.

CONF_BIN_DIR := $(DECOMK_HOME)/conf/bin
HELLO_SCRIPT := $(CONF_BIN_DIR)/hello-world.sh
HELLO_TEXT ?= Hello from Makefile
DEVCONTAINER_GUI ?= 0

# Decomk stage-0 sets DECOMK_STAGE0_PHASE to either updateContent or postCreate.
DECOMK_STAGE0_PHASE ?= postCreate
PHASE_TARGET := phase-$(DECOMK_STAGE0_PHASE)

.PHONY: all
all: hello-common $(PHASE_TARGET)
>@echo "decomk conf repo all-target completed (phase=$(DECOMK_STAGE0_PHASE))"

hello-common:
>bash "$(HELLO_SCRIPT)" "hello-common" "$(HELLO_TEXT)"
>@touch $@

hello-repo:
>bash "$(HELLO_SCRIPT)" "hello-repo" "$(HELLO_TEXT)"
>@touch $@

phase-updateContent:
>@if [[ "$${DECOMK_STAGE0_PHASE:-}" != "updateContent" ]]; then \
>  echo "Expected updateContent phase, got '$${DECOMK_STAGE0_PHASE:-<unset>}'"; \
>  exit 1; \
>fi
>@echo "Running updateContent phase actions"
>@touch $@

phase-postCreate:
>@if [[ "$${DECOMK_STAGE0_PHASE:-}" != "postCreate" ]]; then \
>  echo "Expected postCreate phase, got '$${DECOMK_STAGE0_PHASE:-<unset>}'"; \
>  exit 1; \
>fi
>@echo "Running postCreate phase actions"
>@if [[ "$(DEVCONTAINER_GUI)" == "1" ]]; then \
>  echo "GUI mode is enabled by tuple/env policy"; \
>fi
>@touch $@

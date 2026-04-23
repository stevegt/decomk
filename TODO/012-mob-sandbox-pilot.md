# TODO 012 - mob-sandbox pilot for decomk stage-0

## Decision Intent Log

ID: DI-012-20260421-221834
Date: 2026-04-21 22:18:34
Status: active
Decision: Use this TODO as the authoritative cross-repo execution checklist for TODO `001.8`, with each task labeled by the repository where it must be performed.
Intent: Make pilot execution decision-complete across `decomk`, `mob-sandbox`, and `decomk-conf-cswg`, and ensure closure of TODO `001.8` is evidence-driven.
Constraints: Keep `decomk` templates/docs/test generalization in this repo; keep pilot configuration/migration work in `mob-sandbox` and `decomk-conf-cswg`; close `001.8` only after Codespaces evidence is captured.
Affects: `TODO/012-mob-sandbox-pilot.md`, `TODO/001-decomk-devcontainer-tool-bootstrap.md`, `TODO/TODO.md`.

ID: DI-012-20260422-102409
Date: 2026-04-22 10:24:09
Status: active
Decision: Expand TODO 012 from a high-level checklist into a command-level runbook with exact shell commands and renumbered steps, including explicit `gh codespace` create/ssh/stop/delete commands.
Intent: Remove ambiguity during pilot execution by making each step directly runnable in its target repo.
Constraints: Keep task ownership labeled by repository, keep TODO 012 as the authoritative 001.8 execution plan, and keep closure gated on captured Codespaces evidence.
Affects: `TODO/012-mob-sandbox-pilot.md`.
Supersedes: DI-012-20260421-221834

ID: DI-012-20260423-045339
Date: 2026-04-23 04:53:39
Status: active
Decision: Add a first-class stage-0 failure policy (`DECOMK_FAIL_NOBOOT`) that defaults to continue-boot behavior, records deterministic marker/log artifacts under `DECOMK_HOME`, and emits MOTD hints (or a fallback hint file) when stage-0 fails.
Intent: Keep container boot non-blocking by default while making bootstrap failures visible, diagnosable, and testable with one consistent artifact contract across templates, examples, and selftests.
Constraints: `DECOMK_FAIL_NOBOOT=true` must fail fast with non-zero exit; unset/false must return success after recording artifacts; invalid values must fail explicitly; stage-0 generated files must remain in sync through template regeneration.
Affects: `cmd/decomk/templates/decomk-stage0.sh.tmpl`, `cmd/decomk/templates/devcontainer.json.tmpl`, `cmd/decomk/templates/confrepo.devcontainer.json.tmpl`, `stage0/stage0.go`, `cmd/decomk/init*.go`, generated `examples/**/.devcontainer/*`, `README.md`, `doc/decomk-design.md`, `examples/decomk-selftest/README.md`, `cmd/decomk/*_test.go`.

## Goal

Pilot decomk stage-0 lifecycle integration in `mob-sandbox` against the
real shared config repo `decomk-conf-cswg`, then generalize the resulting
failure-policy and documentation contracts in the decomk repo.

## Execution repos

- `decomk` (this repo): templates, selftests, docs, TODO closure.
- `mob-sandbox`: devcontainer lifecycle hook migration and pilot runs.
- `decomk-conf-cswg`: `decomk.conf` + Makefile policy/target definitions.

## Subtasks

- [ ] 012.1 [repo: decomk-conf-cswg] Sync local checkout and verify remote.
  ```bash
  cd /home/stevegt/lab/decomk-conf-cswg
  git checkout main
  git pull --ff-only origin main
  git remote get-url origin
  ```
- [ ] 012.2 [repo: decomk-conf-cswg] Edit `decomk.conf` to add/confirm `mob-sandbox` context and phase targets.
  ```bash
  cd /home/stevegt/lab/decomk-conf-cswg
  ${EDITOR:-vi} decomk.conf
  ```
- [ ] 012.3 [repo: decomk-conf-cswg] Edit `Makefile` to add/confirm phase-aware behavior keyed on `DECOMK_STAGE0_PHASE`.
  ```bash
  cd /home/stevegt/lab/decomk-conf-cswg
  ${EDITOR:-vi} Makefile
  ```
- [ ] 012.4 [repo: decomk-conf-cswg] Enforce explicit error handling (no silent `|| true`) and preserve GUI tuple/env passthrough behavior.
  ```bash
  cd /home/stevegt/lab/decomk-conf-cswg
  rg -n "\\|\\| true|DECOMK_STAGE0_PHASE|MOB_SANDBOX_GUI|\\b\\$\\b" decomk.conf Makefile
  ```
- [ ] 012.5 [repo: decomk-conf-cswg] Commit and push pilot config changes.
  ```bash
  cd /home/stevegt/lab/decomk-conf-cswg
  git status --short
  git add decomk.conf Makefile
  git commit -m "Add mob-sandbox decomk pilot contexts"
  git push origin main
  ```
- [ ] 012.6 [repo: mob-sandbox] Sync local checkout and verify remote.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  git checkout main
  git pull --ff-only origin main
  git remote get-url origin
  ```
- [ ] 012.7 [repo: mob-sandbox] Checkpoint existing `.devcontainer` state before regeneration.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  git add .devcontainer
  git commit -m "Checkpoint devcontainer before decomk pilot"
  ```
- [ ] 012.8 [repo: mob-sandbox] Regenerate stage-0 scaffolding with `decomk init` using the real conf repo URI.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  decomk init -repo-root . -f -no-prompt \
    -name "mob-sandbox" \
    -conf-uri "git:https://github.com/ciwg/decomk-conf-cswg.git?ref=main" \
    -tool-uri "go:github.com/stevegt/decomk/cmd/decomk@latest" \
    -home "/var/decomk" \
    -log-dir "/var/log/decomk"
  ```
- [ ] 012.9 [repo: mob-sandbox] Merge required repo-specific settings back into regenerated files.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  git diff -- .devcontainer/devcontainer.json .devcontainer/decomk-stage0.sh
  ${EDITOR:-vi} .devcontainer/devcontainer.json
  if [[ -f .devcontainer/gui/devcontainer.json ]]; then
    ${EDITOR:-vi} .devcontainer/gui/devcontainer.json
  fi
  ```
- [ ] 012.10 [repo: mob-sandbox] Ensure both lifecycle hooks call decomk stage-0 with explicit phase args.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  rg -n "updateContentCommand|postCreateCommand|decomk-stage0\\.sh" .devcontainer/devcontainer.json .devcontainer/gui/devcontainer.json
  ```
- [ ] 012.11 [repo: mob-sandbox] Set container env values (`DECOMK_CONF_URI`, `DECOMK_TOOL_URI`, `DECOMK_FAIL_NOBOOT`) and remove old active bootstrap path.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  rg -n "DECOMK_CONF_URI|DECOMK_TOOL_URI|DECOMK_FAIL_NOBOOT|postCreateCommand\\.sh" .devcontainer/devcontainer.json .devcontainer/gui/devcontainer.json .devcontainer/postCreateCommand.sh
  ```
- [ ] 012.12 [repo: mob-sandbox] Commit and push pilot devcontainer changes.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  git status --short
  git add .devcontainer
  git commit -m "Migrate mob-sandbox to decomk stage0 pilot hooks"
  git push origin main
  ```
- [ ] 012.13 [repo: mob-sandbox] Start a fresh Codespace on the pilot branch.
  ```bash
  cd /home/stevegt/lab/mob-sandbox
  PILOT_TS="$(date -u +%Y%m%dT%H%M%SZ)"
  PILOT_DISPLAY="mob-pilot-${PILOT_TS}"
  gh codespace create -R ciwg/mob-sandbox -b main \
    --devcontainer-path .devcontainer/devcontainer.json \
    --machine basicLinux32gb \
    --idle-timeout 30m \
    --retention-period 1h \
    --default-permissions \
    -d "${PILOT_DISPLAY}"
  CODESPACE_NAME="$(gh codespace list -R ciwg/mob-sandbox --json name,displayName --jq ".[] | select(.displayName==\"${PILOT_DISPLAY}\") | .name" | head -n1)"
  echo "CODESPACE_NAME=${CODESPACE_NAME}"
  ```
- [ ] 012.14 [repo: mob-sandbox] Capture lifecycle logs and baseline in-container environment.
  ```bash
  gh codespace logs -c "${CODESPACE_NAME}" > "/tmp/mob-sandbox-${CODESPACE_NAME}-codespace.log"
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'set -euo pipefail; cd /workspaces/mob-sandbox; env | sort | grep -E "^(DECOMK_|GITHUB_|USER=|LOGNAME=)"'
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'set -euo pipefail; ls -la /var/decomk /var/log/decomk'
  ```
- [ ] 012.15 [repo: mob-sandbox] Run explicit hook probes for success path.
  ```bash
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'set -euo pipefail; cd /workspaces/mob-sandbox; bash .devcontainer/decomk-stage0.sh updateContent'
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'set -euo pipefail; cd /workspaces/mob-sandbox; bash .devcontainer/decomk-stage0.sh postCreate'
  ```
- [ ] 012.16 [repo: mob-sandbox] Inject failure with `DECOMK_FAIL_NOBOOT=false` and capture rc + marker evidence.
  ```bash
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'cd /workspaces/mob-sandbox; set +e; DECOMK_CONF_URI="git:https://github.com/ciwg/does-not-exist.git" DECOMK_FAIL_NOBOOT=false bash .devcontainer/decomk-stage0.sh postCreate; rc=$?; echo "continue-mode rc=${rc}"; exit 0'
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'set -euo pipefail; find /var/decomk -maxdepth 5 -type f | sort'
  ```
- [ ] 012.17 [repo: mob-sandbox] Inject failure with `DECOMK_FAIL_NOBOOT=true` and verify non-zero behavior.
  ```bash
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'cd /workspaces/mob-sandbox; set +e; DECOMK_CONF_URI="git:https://github.com/ciwg/does-not-exist.git" DECOMK_FAIL_NOBOOT=true bash .devcontainer/decomk-stage0.sh postCreate; rc=$?; echo "fail-noboot rc=${rc}"; if [[ "${rc}" -eq 0 ]]; then echo "expected non-zero rc"; exit 1; fi'
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'if [[ -d /etc/motd.d ]]; then ls -la /etc/motd.d; else echo "/etc/motd.d missing"; fi'
  ```
- [ ] 012.18 [repo: mob-sandbox] Archive evidence artifacts locally.
  ```bash
  EVID_DIR="/tmp/mob-sandbox-pilot-${CODESPACE_NAME}"
  mkdir -p "${EVID_DIR}"
  gh codespace logs -c "${CODESPACE_NAME}" > "${EVID_DIR}/codespace.log"
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'set -euo pipefail; find /var/decomk /var/log/decomk -maxdepth 5 -type f -print | sort' > "${EVID_DIR}/decomk-files.txt"
  gh codespace ssh -c "${CODESPACE_NAME}" -- bash -lc 'if [[ -d /etc/motd.d ]]; then find /etc/motd.d -maxdepth 2 -type f -print | sort; else echo "/etc/motd.d missing"; fi' > "${EVID_DIR}/motd-files.txt"
  echo "${EVID_DIR}"
  ```
- [ ] 012.19 [repo: mob-sandbox] Stop/delete the pilot Codespace after artifacts are collected.
  ```bash
  gh codespace stop -c "${CODESPACE_NAME}"
  gh codespace delete -c "${CODESPACE_NAME}" -f
  ```
- [ ] 012.20 [repo: decomk] Sync local checkout and create the generalization patch for `DECOMK_FAIL_NOBOOT` + marker behavior.
  ```bash
  cd /home/stevegt/lab/decomk
  git checkout main
  git pull --ff-only origin main
  ```
- [ ] 012.21 [repo: decomk] Implement template/runtime changes and regenerate stage-0 outputs.
  ```bash
  cd /home/stevegt/lab/decomk
  go generate ./...
  ```
- [ ] 012.22 [repo: decomk] Add/adjust selftests for continue-boot vs fail-no-boot contracts.
  ```bash
  cd /home/stevegt/lab/decomk
  go test ./...
  ```
- [ ] 012.23 [repo: decomk] Update docs for `DECOMK_FAIL_NOBOOT` and marker/MOTD behavior.
  ```bash
  cd /home/stevegt/lab/decomk
  rg -n "DECOMK_FAIL_NOBOOT|motd|stage-0|decomk-stage0" README.md doc/decomk-design.md examples/decomk-selftest/README.md
  ```
- [ ] 012.24 [repo: decomk] Record evidence paths from `mob-sandbox` and mark `012.*` + `001.8` complete.
  ```bash
  cd /home/stevegt/lab/decomk
  ${EDITOR:-vi} TODO/012-mob-sandbox-pilot.md TODO/001-decomk-devcontainer-tool-bootstrap.md
  ```
- [ ] 012.25 [repo: decomk] Commit and push TODO/docs/template/test changes after validation.
  ```bash
  cd /home/stevegt/lab/decomk
  git status --short
  git add TODO/012-mob-sandbox-pilot.md TODO/001-decomk-devcontainer-tool-bootstrap.md TODO/TODO.md README.md doc/decomk-design.md examples/decomk-selftest/README.md cmd/decomk/templates
  git commit -m "Implement mob-sandbox decomk pilot runbook and fail-noboot contract"
  git push origin main
  ```

## Evidence requirements

Minimum evidence set from the `mob-sandbox` Codespaces pilot:

- lifecycle evidence that both hooks ran (`updateContent`, `postCreate`),
- success-path decomk run evidence,
- injected-failure evidence with `DECOMK_FAIL_NOBOOT` unset/false showing:
  - boot completion,
  - marker creation,
  - visible warning/hint behavior,
- injected-failure evidence with `DECOMK_FAIL_NOBOOT=true` showing non-zero failure behavior,
- final pass/fail summary with artifact paths.

Recommended artifact directory naming:

- `/tmp/mob-sandbox-pilot-<codespace-name>/codespace.log`
- `/tmp/mob-sandbox-pilot-<codespace-name>/decomk-files.txt`
- `/tmp/mob-sandbox-pilot-<codespace-name>/motd-files.txt`

## Completion criteria

TODO `001.8` may be checked only when all are true:

- all `012.*` subtasks are complete,
- Codespaces evidence is linked in this file,
- decomk template/test/doc updates are complete and consistent with the pilot contract.

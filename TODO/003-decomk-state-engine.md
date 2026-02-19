# TODO 003 - decomk: state engine (make vs alternatives) + lunamake review

Goal: decide whether `decomk` should use GNU `make` as its **state
engine** (stamps + dependency graph), and if so, constrain how we use
it. If not, identify a better engine/library and a better language for
expressing actions.

This TODO is prompted by the real tradeoff:
- `make` is ubiquitous and “good enough” for a lot of stamp-driven
  bootstraps, but
- its language and stamp idioms are awkward for provisioning-style
  work, and a custom engine may be simpler *for this domain*.

## What “state engine” means here

The state engine answers:
- What work is already done?
- What work needs to run now (and in what order)?
- How do we record success/failure so re-runs converge?

The main contenders:
- **Make-as-engine**: use file targets in a persistent stamp dir.
- **Journal-as-engine**: append-only “migration” stanzas, tracked by a
  journal with hashes.
- **Custom DAG engine**: explicit tasks + deps + state DB/hashes.

## Option A: Use `make` as the state engine (current MVP direction)

### Pros

- Ubiquitous: already installed in most devcontainer images.
- Dependency graph: expresses partial ordering and fan-out naturally.
- Stamps are “just files”: easy to inspect, delete, and cache.
- Parallelism: `make -j` can help when tasks are independent.
- Tooling familiarity: many people already know how to read/modify it.

### Cons (where `make` fights the domain)

- **The language is hostile to “provisioning scripts”**:
  - multi-line shell blocks are awkward (tabs, escaping, line
    continuations, per-line shells unless using `.ONESHELL`)
  - quoting rules are easy to get wrong (Make expansion + shell
    expansion + env expansion)
  - error reporting can be opaque (especially with `@` and nested
    shells)
- **Timestamp-based rebuild logic is often the wrong default**:
  - provisioning tasks usually want “run once unless explicitly
    invalidated”, not “re-run because a prereq timestamp changed”
- **State is implicit**:
  - the “truth” is scattered across stamp files, logs, and whatever a
    recipe touched; there’s no structured journal of “what ran and why”.

### The `touch $@` problem (idiom drawbacks)

`touch $@` works, but it’s fragile and aesthetically unpleasant:
- It’s easy to forget, leading to perpetual re-runs.
- It’s easy to do too early, leading to false success (partial work
  still writes the stamp).
- It teaches “recipes must remember to manage state”, rather than
  making state management automatic.
- It encourages targets that are “named after the action” rather than
  “named after a real artifact”.

Mitigations if we keep `make`:
- Prefer “real artifacts” as targets when possible (a file that is
  actually produced by the step).
- When a stamp is unavoidable, centralize stamp creation:
  - wrap recipes in a helper script that only `touch`es on success
  - or standardize a make macro/snippet used by every stamped target
- Consider “versioned stamps” for upgrades (encode tool/version in the
  stamp filename) to avoid manual timestamp games.

### The “touch all stamps” hack (isconf/lunamake behavior)

Both isconf and lunamake include a pattern equivalent to:
1. `cd <stampdir>`
2. `touch *`
3. run `make`

This makes stamps an explicit *invalidation mechanism*:
- If you want a target to run again, you delete its stamp file.
- Mere prerequisite timestamp changes won’t cause surprise rebuilds.

Pros:
- Better fits provisioning/bootstrapping expectations.
- Makes rerun behavior explicit and deterministic.

Cons:
- It intentionally disables a core `make` feature (timestamp-driven
  rebuilds via prereqs).
- It can hide genuine dependency drift (a prereq changed, but the
  stamped target doesn’t re-run).

If we adopt this in `decomk`, it should be an explicit mode/flag (and
documented as “make is used primarily as an ordered stamp executor”).

## Option B: Write our own engine (Go)

### Pros

- Domain-aligned semantics:
  - run-once steps, force rebuild, upgrade steps, and audit logs can be
    first-class
  - content-hash state (not timestamps) becomes straightforward
- Better UX:
  - structured errors, better “plan” output, deterministic logs
- Easier to test: state transitions can be unit-tested without shell
  quoting edge cases.

### Cons

- High scope / re-implementing decades of build-engine behavior:
  - DAG execution, concurrency limits, retries, failure modes
  - dependency scanning, partial rebuild correctness
- You still need a **language** to describe work:
  - if it’s “shell strings”, you haven’t escaped quoting issues, you
    just moved them
  - if it’s a DSL, you need to maintain the parser + semantics

For `decomk`, a custom engine is only justified if we keep tripping on
make’s ergonomics or correctness.

## Option C: Use an existing non-make tool/library

Candidates (roughly):
- “task runners” (`just`, Taskfile/go-task, mage): nicer UX, but often
  not designed around persistent stamp state.
- “real build systems” (ninja, bazel): great for compilation; usually
  awkward/heavy for provisioning steps and not preinstalled.
- “config mgmt” tools (ansible/chef/puppet): powerful, but heavy, and
  often overkill for devcontainers.

Risk here is adding a new dependency that devcontainers must install
before they can bootstrap anything.

## Lunamake review (what it suggests for decomk)

Path examined: `/home/stevegt/gohack/github.com/stevegt/lunamake`.

What exists there (high level):
- A Python “modular” runner that:
  - expands contexts/macros from `.conf` files and writes an env file
    (`.local/lunamake`)
  - runs numbered scripts, one of which runs `make` in a stamp dir and
    pre-touches all stamps (`.local/20-make`)
- A Go codebase that experiments with a **Dockerfile-like** `.lm`
  stanza syntax (`testdata/simple.lm`, `lunamake.go`), with notes about:
  - hashing each stanza (chained with the previous hash)
  - recording execution history in a journal
  - aborting if an already-executed stanza’s body changes (unless
    forced)

### Pros of “finish lunamake” (vs build decomk)

- The Python implementation demonstrates a proven pattern we want:
  - context expansion → env snapshot → numbered hooks → make-in-stampdir
- The Dockerfile-like stanza idea is a promising **better language**
  than Makefile recipes for multi-line commands and explicit ordering.
  - XXX but the advantage of make is the re-use and partial ordering of targets
  - XXX make is actually a functional language, I think

### Cons / risks

- The Go rewrite appears incomplete/inconsistent; finishing it likely
  means a significant redesign effort.
  - XXX but codex can help with this
- Lunamake’s scope trends toward full host provisioning (and even
  gdo-backed state ideas), which is larger than decomk’s devcontainer
  bootstrap goal.
  - XXX gdo is now called promisegrid; some principles carry over
- Mixing “journaled stanzas” with a DAG engine is non-trivial:
  journaling is naturally linear; make is naturally a graph.
  - XXX show examples from lunamake to clarify this tension

### What we can steal for decomk (low risk, high value)

- Use a persistent stamp dir outside the repo and run `make -C <stampdir> -f <makefile>`.
- Offer an optional “touch all stamps first” mode (explicit invalidation).
- Keep config syntax close to isconf/hosts.conf (continuations +
  shlex-like quoting) and generate an env snapshot for audit.
  - XXX stamps, or a journal?

### A possible “better language” path for decomk

If Makefile recipes become the pain point, consider a future `.lm`
format inspired by lunamake’s Dockerfile-like stanzas:
- Each stanza is one op (RUN/COPY/ENV/…)
- Multi-line bodies are natural (indentation)
- State can be journaled by stanza hash (no `touch $@`)
  - XXX does lunamake support make-like stanzas with deps? if so, show
    an example.

This could start as a **separate mode**:
- `decomk run` (make engine)
- `decomk apply` (lm/journal engine)
- XXX no

Keeping both might be worthwhile if they serve different audiences:
- Make engine: people already living in Makefiles
- Journal engine: people who want “migration-like” provisioning steps

## Subtasks

- [ ] 003.1 Clarify the domain: do we want DAG semantics or “ordered run-once” semantics?
- [ ] 003.2 Decide whether `decomk` should default to “touch all stamps first”.
- [ ] 003.3 Document a recommended Makefile style for decomk (scripts for complex steps; stamp helpers).
- [ ] 003.4 Prototype a minimal journal/stanza runner (no deps) to validate the `.lm` idea.
- [ ] 003.5 Decide whether lunamake is a predecessor we port from, or a separate project we leave alone.
- [ ] 003.6 If adopting `.lm`, define a tiny grammar + state journal format (JSONL or sqlite).
- [ ] 003.7 Update TODO 002 if this decision changes the architecture.

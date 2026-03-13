# isconf Design: How `bin/isconf` Translates Runtime Context + CLI Args into `make` Args

This document reverse-engineers the `isconf2i` execution model from:

- `~/lab/isconf2/isconf2i-git/bin/isconf`
- `~/lab/isconf2/isconf2i-git/bin/parseargs.pl`
- `~/lab/isconf2/isconf2i-git/bin/expandmacro.pl`
- `~/lab/isconf2/isconf2i-git/conf/hosts.conf`
- `~/lab/isconf2/isconf2i-git/conf/main.mk`
- `~/lab/isconf2/isconf2i-git/conf/aix.mk`
- `~/lab/isconf2/isconf2i-git/conf/tsm.mk`

The focus is the algorithm that turns:

- runtime environment (`HOSTNAME`, `DOMAIN`, `ISCONFDIR`, etc.)
- `isconf` command arguments (`INSTALL`, `BOOT`, `CRON`, literal targets)

into the final `make` command-line arguments.

---

## 1) Big Picture

At a high level, `isconf` does four things:

1. **Select context macros** from `hosts.conf`:
   - always start with `DEFAULT`
   - optionally append host-specific macro (`FQHOSTNAME` preferred, else short `HOSTNAME`)
2. **Expand macros recursively** into a flat token stream (`expandmacro.pl`).
3. **Translate CLI args to action targets** (`parseargs.pl`):
   - if arg matches a variable in expanded tuples, use its value
   - if nothing matches, fallback to literal CLI args
4. **Run make with both tuples and targets**:
   - expanded tuples (and any literal tokens from macro expansion)
   - plus resolved `PACKAGES` targets.

This yields a two-layer model:

- `hosts.conf`: policy composition (which vars/actions apply on this host)
- `*.mk`: implementation/dependency graph (how selected targets are applied)

---

## 2) Inputs and Runtime State

From `bin/isconf`, the effective inputs are:

- **CLI args** (`$*`): usually action variable names (`INSTALL`, `BOOT`, etc.), but can be literal targets.
- **Environment**:
  - `ISCONFDIR` (default: `dirname(dirname($0))`)
  - `DOMAIN` (used to build `FQHOSTNAME`)
  - `NOOP` (enables `make -n`)
  - `DEBUG` (shell tracing)
- **Runtime host info**:
  - `HOSTNAME=$(hostname)`
  - `FQHOSTNAME="$HOSTNAME.$DOMAIN"`
- **Config files**:
  - `conf/hosts.conf` (macro + tuple definitions)
  - `conf/main.mk` and included `conf/$(OS).mk` (target graph/recipes)

It also sets and exports environment consumed by makefiles, including:

- `HOSTNAME`, `PLATFORM`, `OS`, `OSVERSION`, `HARDWARE`, `MAKEFILE`

---

## 3) `hosts.conf` Data Model

`hosts.conf` lines map keys to token lists:

```text
KEY: token token token ...
```

Tokens can be:

- macro names (`HQFT`, `TOS`, `TNG_FRONT`, etc.)
- tuples (`BOOT='...'`, `CFCLUSTER=configure_cluster`, `NTP_MASTER=y`)
- literals

Important default stanza:

```text
DEFAULT: BOOT=Block12 CRON=cron INSTALL=Block00_install ... CFCLUSTER=configure_cluster ...
```

`hosts.conf` includes:

- reusable policy macros (`HQFT`, `HQPS`, `INHS`, `TOS`, `TNG_FRONT`, etc.)
- concrete hosts (`hqftms01`, `mulder`, `kirk`, `hqenms01`, `hqftwb01`, ...)

---

## 4) Component Algorithms

## 4.1 `expandmacro.pl`

### Parse phase

It reads `hosts.conf` from stdin and builds a map:

- key line: `^(\S+):(.*)` => start/replace key
- continuation line: appended to the most recent key
- comment line: `^\s*#` ignored

### Expand phase

Given input tokens (argv), recursively expand each token:

- if token is a known macro key: split its expansion on whitespace and recurse
- if token is unknown: keep as literal
- if token directly references itself while expanding itself: emit literal token and stop direct self-recursion

### Pseudocode

```pseudo
function load_conf(lines):
  conf = {}
  current_key = null
  for line in lines:
    line = chomp(line)
    if line starts with optional-space '#':
      continue
    if matches /^(\S+):(.*)/:
      current_key = group1
      conf[current_key] = group2
      continue
    if current_key exists:
      conf[current_key] = conf[current_key] + " " + line
  return conf

function expand(tokens, conf):
  out = []
  for token in tokens:
    if token not in conf:
      out.append(token)        # literal
      continue
    for part in split_whitespace(conf[token]):
      if part == token:
        out.append(token)      # direct self reference guard
      else:
        out.extend(expand([part], conf))
  return out
```

---

## 4.2 `parseargs.pl`

This script maps CLI action names to values from expanded tuples.

Input:

- arg1: a **single string** containing expanded makeargs
- remaining args: action selectors from `isconf` CLI

Behavior:

1. Tokenize tuple string, recognizing:
   - `\w+='...'`
   - `\w+=\S+`
2. Build hash `vars[key] = value` (later duplicates overwrite earlier values).
3. For each CLI arg:
   - print `vars[arg]` if exists, else empty string.

`bin/isconf` then checks word count:

- if output has zero words => fallback to literal CLI args

### Pseudocode

```pseudo
function parse_tuple_string(makeargs_str):
  tuple_tokens = []
  while makeargs_str not empty:
    if next token matches WORD='...':
      tuple_tokens.append(token)
      consume
    else if next token matches WORD=NONSPACE:
      tuple_tokens.append(token)
      consume
    else if only whitespace remains:
      break
    else:
      error

  vars = {}
  for tok in tuple_tokens:
    key, value = split_first(tok, '=')
    value = strip_single_quotes(value)
    vars[key] = value          # last wins
  return vars

function resolve_actions(makeargs_str, cli_args):
  vars = parse_tuple_string(makeargs_str)
  out_words = []
  for arg in cli_args:
    if arg in vars:
      out_words.append(vars[arg])
  return join_with_spaces(out_words)
```

---

## 4.3 `bin/isconf` Orchestration

Core flow inside `main()`:

1. Initialize:
   - `makeargs="DEFAULT"`
   - `HOSTNAME`, `FQHOSTNAME`, `MAKEFILE`, platform info, etc.
2. Host stanza probe:
   - `fqhostargs = expandmacro(FQHOSTNAME)`
   - `hostargs = expandmacro(HOSTNAME)`
   - if `fqhostargs != FQHOSTNAME`, append `FQHOSTNAME` to seed
   - else if `hostargs != HOSTNAME`, append `HOSTNAME`
3. Expand for real:
   - `makeargs = expandmacro(makeargs_seed)`
4. Resolve action args:
   - `PACKAGES = parseargs(makeargs, cli_args...)`
   - if empty => `PACKAGES = cli_args...` (literal fallback)
5. Append packages:
   - `makeargs = makeargs + " " + PACKAGES`
6. Execute:
   - `cd $stampdir`
   - `touch *`
   - `eval set -- "$makeargs"`
   - `mk_env "$@"`
   - `make -f conf/main.mk [maybe -n] "$@"`

### Integrated pseudocode

```pseudo
function isconf_translate_and_run(cli_args, env):
  isconfdir = env.ISCONFDIR or dirname(dirname(argv0))
  hostname = system_hostname()
  fqhostname = hostname + "." + env.DOMAIN
  hosts_conf = isconfdir + "/conf/hosts.conf"
  makefile = isconfdir + "/conf/main.mk"

  seed = ["DEFAULT"]

  if expandmacro([fqhostname], hosts_conf) != [fqhostname]:
    seed.append(fqhostname)
  else if expandmacro([hostname], hosts_conf) != [hostname]:
    seed.append(hostname)

  expanded_tokens = expandmacro(seed, hosts_conf)
  expanded_str = join_with_spaces(expanded_tokens)

  packages = parseargs(expanded_str, cli_args)
  if word_count(packages) == 0:
    packages = join_with_spaces(cli_args)    # literal-target fallback

  final_str = expanded_str + " " + packages
  argv = shell_split_via_eval_set(final_str)

  cd(isconfdir + "/stamps")
  touch_existing_stamps()
  mk_env(argv)

  flags = []
  if env.NOOP set:
    flags.append("-n")

  exec make -f makefile flags... argv...
```

---

## 5) Worked Examples from `hosts.conf`

Below examples were replayed against the scripts and `hosts.conf`.

### Example A — `hqftms01 INSTALL`

- `DEFAULT` always selected.
- Host key `hqftms01` exists, so it is appended.
- `INSTALL` not overridden in `hqftms01`, so from `DEFAULT`:
  - `INSTALL=Block00_install`
- Result: `PACKAGES=Block00_install`

### Example B — `hqftms01 BOOT`

- `hqftms01` overrides `BOOT` with a long host-specific list.
- `parseargs` maps BOOT -> that overridden value.
- Result: host-specific package list (not `DEFAULT` `Block12`).

### Example C — `hqenms01 BOOT CRON`

- Chain: `hqenms01 -> HQEN -> HQFT` + `DEFAULT`.
- `BOOT` resolved from `HQEN` override.
- `CRON` resolved from `DEFAULT` (`cron`).
- Result: concatenated BOOT list + `cron`.

### Example D — `mulder INSTALL`

- Chain: `mulder -> HQPS` + `DEFAULT`.
- `HQPS` overrides `BOOT` but not `INSTALL`.
- Result: `PACKAGES=Block00_install`.

### Example E — `unknownhost INSTALL`

- No host macro selected; only `DEFAULT`.
- `INSTALL` still resolved from `DEFAULT`.
- Result: `PACKAGES=Block00_install`.

### Example F — `mulder custom_target`

- `parseargs` finds no tuple named `custom_target`.
- Empty packages from parser triggers fallback.
- Result: `PACKAGES=custom_target` (literal target).

### Example G — `hqftwb01 BOOT`

- Chain: `hqftwb01 -> TNG_FRONT -> HQFT` + `DEFAULT`.
- BOOT comes from `TNG_FRONT`:
  - `Block12 nfs_home mkusers monitor etherchannel_failover_en2 isinit_on`

### Example H — `kirk CFCLUSTER`

- Chain: `kirk -> TOS -> INHS` + `DEFAULT`.
- `CFCLUSTER` defined in `DEFAULT`:
  - `CFCLUSTER=configure_cluster`
- Result: `PACKAGES=configure_cluster`.

---

## 6) What `conf/*.mk` Reveals About Design Intent

`main.mk` and `aix.mk` show why `hosts.conf` parsing exists as a front-end instead of encoding everything purely as make prerequisites.

## 6.1 Separation of concerns

- `hosts.conf` = **host/site policy composition** (`BOOT`, `CRON`, `INSTALL`, host params).
- `aix.mk` / `tsm.mk` = **execution graph** (dependencies + shell recipes + idempotence stamps).

This allows policy to vary by host without duplicating full target graphs.

## 6.2 Stable action interface

`rc.isconf` invokes actions like:

- `isconf INSTALL`
- `isconf BOOT`
- `isconf CRON`
- `isconf CFCLUSTER`

Action names stay stable, while each host maps them to different target sets via tuples.

## 6.3 Runtime selection before Make graph evaluation

Host selection depends on runtime `hostname` (+ optional `DOMAIN`) before make runs.
Using `hosts.conf` as a preprocessor lets isconf compute host-specific variables/targets first, then run a common makefile graph.

## 6.4 Override model via command-line tuple ordering

Because make command-line variable assignments are ordered, later assignments win.
Macro expansion order (`DEFAULT` first, then host macros) provides a simple override mechanism without deeply nested conditional make logic.

## 6.5 Why not only Make prerequisites for BOOT lists?

You *could* model some BOOT groupings as make phony target dependencies, but isconf chose a hybrid for practical reasons:

1. **Host policy data externalized from recipe code**  
   `hosts.conf` acts as inventory/policy; `aix.mk` remains implementation.

2. **Action-to-package mapping is data-driven**  
   BOOT/INSTALL/CRON values are tuples that can vary per host without rewriting makefile targets.

3. **Lower makefile complexity**  
   Without tuple indirection, makefiles would need many host/action conditional branches or many host-specific wrapper targets.

4. **Literal fallback flexibility**  
   Unknown action args can still run literal targets directly (`isconf custom_target`).

5. **Operational ergonomics**  
   Ops can tune host policy in one inventory-like file while keeping procedural logic in OS-specific makefiles.

In short: prerequisites solve ordering/dependency inside selected goals; `hosts.conf` + parseargs solves *which goals and vars to request* per host/action.

---

## 7) Makefile Integration Observations

From `conf/main.mk`:

- includes OS-specific implementation (`include $(ISCONFDIR)/conf/$(OS).mk`)
- default `all` prints usage/error (so explicit context/action is expected)

From `conf/aix.mk`:

- defines hierarchical blocks (`Block00`, `Block10`, `Block12`, `Cluster00`, `Isconf00`)
- maps operational actions to concrete targets (`cron`, `configure_cluster`, etc.)
- delegates TSM targets to GNU make on `tsm.mk` (`/usr/local/bin/make -f .../tsm.mk $@`)

From `conf/tsm.mk`:

- specialized subsystem graph (TSM setup), parameterized by command-line variables
- indicates deliberate modular split by domain/stack.

---

## 8) Quirks and Edge Cases

1. **`hosts.conf` formatting sensitivity**
   - `expandmacro.pl` expects continuation lines to follow a previously parsed key.
   - In this specific file, line 22 is an uncommented continuation-style line, which causes Perl warnings during parsing; behavior still proceeds.

2. **Limited parser grammar**
   - `parseargs.pl` keys are `\w+` only.
   - single-quote handling is simplistic (quotes stripped globally in captured value).

3. **Macro cycle handling is partial**
   - direct self-reference protected (`A` containing `A`).
   - no general cycle detection for longer loops (`A->B->A`).

4. **`eval set -- "$makeargs"`**
   - shell parsing is intentionally leveraged to preserve single-quoted tuple values with spaces as single argv items.
   - this is powerful but sensitive to malformed quoting in upstream data.

---

## 9) Condensed Mental Model

`isconf` is a **host-aware action compiler** in front of make:

1. compile host policy macros -> tuple/value space
2. compile action names -> concrete package targets
3. execute make with both variable context and explicit goals in stampdir

That design explains both:

- why host/action parsing exists outside make, and
- why makefiles remain focused on dependency/recipe mechanics.


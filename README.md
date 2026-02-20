# decomk

decomk is an isconf-inspired bootstrap for development containers.
For more background on isconf, see the [infrastructures.org](https://infrastructures.org/).


It resolves a **context** (e.g., `owner/repo`, `repo`, `DEFAULT`) into:
- Make target groups to run (shared + repo-specific), and
- a resolved environment snapshot for auditing/debugging.

Primary use case: keep devcontainers portable across hosts (Codespaces
now; self-host later), while making it easy to roll forward local tools
(e.g., `mob-consensus`) in a controlled, repeatable way.

- Design/work plan: `TODO/001-decomk-devcontainer-tool-bootstrap.md`

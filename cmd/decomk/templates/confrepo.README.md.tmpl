# Shared decomk config repo starter

This repository was bootstrapped by `decomk init-conf`.

It contains:

- `decomk.conf` — context and tuple policy
- `Makefile` — executable target graph
- `bin/hello-world.sh` — tiny example script called from Makefile
- `.devcontainer/` — optional producer workspace used to build a genesis image

## How to customize

1. Edit `decomk.conf`:
   - keep `DEFAULT` for shared policy,
   - replace `owner/repo` with real repo keys,
   - update tuple values and target composition.
2. Edit `Makefile`:
   - replace hello-world targets with your real setup targets,
   - keep idempotent file targets (`touch $@`) for repeatable runs.
3. Edit `.devcontainer/devcontainer.json`:
   - set real `DECOMK_CONF_URI`,
   - set real `DECOMK_TOOL_URI`,
   - tune run args and lifecycle options.

## Genesis image workflow (important)

The generated `.devcontainer/Dockerfile` and `build` stanza in
`.devcontainer/devcontainer.json` are intended for the first image ("genesis")
bootstrap only.

After the genesis image is stable:

1. remove the `build` stanza from `.devcontainer/devcontainer.json`,
2. replace it with an `image` stanza that points to your stable channel tag, for example:

```json
"image": "ghcr.io/<org>/<repo>:stable"
```

3. remove `.devcontainer/Dockerfile` from active use in this repo.

The long-term shared setup should live in `decomk.conf` and `Makefile`; the
Dockerfile should stay minimal.

# Thought Experiment: Consumer init image-source UX

- **TE ID:** `TE-20260428-020715`
- **Decision under test:** How image-consumer `decomk init` should source the final `image` value.
- **Related TODO:** `TODO/016-consumer-init-minimal-template-and-image-source.md`

## Assumptions

1. Consumer repos should keep `.devcontainer/devcontainer.json` minimal and stable.
2. Operators may know either:
   - an image tag, or
   - an image producer/conf repo URL.
3. `decomk` is pre-production, so migration compatibility text is optional.

## Alternatives

1. **Image-only input**
   - Consumer must always provide `-image` (or interactive image prompt).
2. **Conf-URL-only input**
   - Consumer must always derive image from producer/conf repo metadata.
3. **Dual-source input (chosen)**
   - Accept direct `-image` and `-conf-url` derivation; keep explicit precedence.

## Scenario analysis

### Scenario A: Fast non-interactive automation

- **Image-only:** Simple, but requires every caller to already know canonical tag.
- **Conf-URL-only:** Adds clone/parsing dependency and network sensitivity.
- **Dual-source:** Caller picks what they already have; `-image` path remains lightweight.

### Scenario B: Human interactive onboarding

- **Image-only:** Friction when user only knows producer/conf repo location.
- **Conf-URL-only:** Friction when user already knows exact image.
- **Dual-source:** Menu supports both workflows and reduces trial/error.

### Scenario C: Derivation failure (repo down / bad URL / missing image field)

- **Image-only:** No derivation path, but no discovery support.
- **Conf-URL-only:** Hard stop in all paths.
- **Dual-source:** In interactive mode, warn and continue to manual image input; in `-no-prompt`, fail fast.

### Scenario D: Long-term maintenance clarity

- **Image-only:** Clear output contract, but weaker discovery ergonomics.
- **Conf-URL-only:** Couples consumer init strongly to producer repo availability.
- **Dual-source:** Keeps output minimal while preserving user choice and explicit precedence.

## Conclusion

Surviving alternative: **Dual-source input**.

Locked implications:

1. Consumer output remains minimal (`name` + `image`).
2. `-image` short-circuits `-conf-url` derivation.
3. `-conf-url` uses HTTP(S) with optional `?ref=...`.
4. Interactive derivation failures warn then fall back to manual image prompt.
5. Non-interactive derivation failures remain hard-fail.

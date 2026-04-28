# Thought Experiment: Init template architecture split

- **TE ID:** `TE-20260428-023011`
- **Decision under test:** How many devcontainer templates should be maintained for init + examples.
- **Related TODO:** `TODO/016-consumer-init-minimal-template-and-image-source.md`

## Assumptions

1. Image-consumer output now needs a minimal `devcontainer.json` contract.
2. Producer scaffolding (`init -conf`) still needs full stage-0 env and lifecycle hooks.
3. Examples/selftests still need full template parity with stage-0 behavior.

## Alternatives

1. **Single template**
   - One template with conditional branches for consumer/producer/examples.
2. **Two templates**
   - Minimal consumer template + one full template shared by producer and examples.
3. **Three templates**
   - Producer full template + consumer minimal template + examples/selftest full template (chosen architecture record for TODO 016).

## Scenario analysis

### Scenario A: Consumer contract evolution

- **Single template:** High branch complexity; easy to accidentally leak producer keys into consumer output.
- **Two templates:** Better isolation for consumer.
- **Three templates:** Strongest isolation; explicit ownership per output family.

### Scenario B: Producer-only fields and lifecycle keys

- **Single template:** Condition combinatorics are hard to reason about and test.
- **Two templates:** Producer/full behavior is centralized.
- **Three templates:** Producer behavior can diverge from example behavior only when intentional.

### Scenario C: Drift prevention and generated parity

- **Single template:** Fewer files, but larger branching surface to test.
- **Two templates:** Lower risk for consumer leakage.
- **Three templates:** More files, but each file has a narrow contract and clearer tests.

### Scenario D: Maintenance readability

- **Single template:** Lowest file count, highest cognitive load.
- **Two templates:** Balanced readability.
- **Three templates:** Most explicit contracts by artifact type; easiest review diffs.

## Conclusion

Surviving alternative for TODO 016 lock: **Three-template model**.

Implementation implications:

1. Add dedicated consumer template (`consumer.devcontainer.json.tmpl`) for minimal output.
2. Keep full template(s) for producer and example/selftest generation paths.
3. Cover each contract with focused tests to prevent cross-mode drift.

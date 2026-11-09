---
inclusion: always
---
# Specter: product steering

Specter links each requirement to the tests that prove it, anchored to the requirement's exact
words. When the words or the tests change after sign-off, the proof is marked stale and the
change is shown as a ghost diff. Sign-offs are Ed25519 attestations on a hash-chained log.

## Say exactly what is known

- Statuses, worst first: `failing`, `stale`, `orphaned`, `uncovered`, `unsealed`, `verified`.
  Never add a status without adding it to the precedence list and the UI badge map.
- A change to a number, negation, modal verb or quantifier is `material` whatever the
  similarity score says. Case and punctuation alone are not a change.
- A seal proves a key signed these words, these tests and these results at a log position. It
  does not prove the tests are good, who holds the key (pin it), or that the clock is right.
- Never write "compliant", "certified" or "guaranteed" in code, UI or docs.

## Audience

Teams doing spec-driven development, including with Kiro, where the requirements file is the
contract and an agent keeps editing code. The demo moment: change "within 2 seconds" to
"within 5 seconds" and watch the green test turn stale.

## Honest prior art

Doorstop, DOORS and Jama trace requirements to tests by id. Specter adds word-level anchoring
(drift is detected in the words, not just the id), a severity for the change, and signed,
independently verifiable evidence.

## Tone

Plain, short, concrete. White, quiet UI; status is never carried by colour alone.

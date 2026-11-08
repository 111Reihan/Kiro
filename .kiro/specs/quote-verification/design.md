# Quote verification: design

## Pipeline

`text.Normalize` folds a string (NFC, case, smart quotes to straight, dash and space runs to one
space) and keeps a map from each folded rune back to its original offset. Both quote and source
are folded, then tokenised into words. `align.Locate` runs semi-global word alignment of the
quote against the source, so the quote may start and end anywhere in the source but is scored
over all its words. `verify.Check` turns the alignment into a verdict.

## Grading

1. Exact word match over a contiguous run, and the run starts and ends on word boundaries: `verbatim`.
2. Otherwise score = word similarity in permille. At or above `SimThreshold` (600): `drifted`. Below: `unsupported`.
3. Severity is decided by *which* words changed, not by the score. A word is material when it is in
   in the sensitive list (negations, spelled-out numbers, modals, quantifiers), ends in `n't`, contains a
   digit, or is one capitalised word swapped for another (a name). A case-only change is `cosmetic`.
   Any other changed word is `minor`.
4. Quotes under `MinFuzzyTokens` (4) words are exact-or-nothing: fuzzy matching a three-word phrase finds noise.

## Why severity is separate from similarity

"did" to "did not" is one inserted word in twenty, a 95% match. A similarity threshold alone would
wave it through. This is the central design decision and R3, R14 exist to pin it down.

## Known limits (shown to the user, not hidden)

- It finds quotations, not paraphrases or unquoted claims.
- A passage that is not in the loaded sources is `unsupported`, which is not proof it is false.
- The sensitive-word list is English only, and a changed word outside it is graded `minor` even if it matters ("Paris" to "Berlin" is caught as a name swap; "cat" to "dog" is not).

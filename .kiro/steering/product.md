---
inclusion: always
---
# Revenant: product steering

Revenant checks quotations against versioned sources and keeps the evidence. It brings back
two unfinished ideas: Xanadu's quotes-by-reference and Memex trails.

## Say exactly what is known

- Verdicts are `verbatim`, `drifted`, `unsupported`, `no-source`. Never write "fake", "false"
  or "fabricated" in code, UI or docs. `unsupported` means "not found in the loaded sources".
- `drifted` carries a severity: `cosmetic`, `minor`, `material`. A change to a negation,
  number, name or modal verb is always `material`, whatever the similarity score says.
- A receipt proves that a key saw a source version contain a quote at a claimed time. It does
  not prove the source is authentic or the clock is right. Every receipt lists what it did
  not check, and the UI shows that list.

## Audience

Writers, students, journalists, researchers and anyone publishing text an AI helped draft.
The demo moment is a quote that looks right and is subtly wrong.

## Tone

Plain, short, concrete. No marketing words. Halloween theme lives in the visual design and in
a few names (séance, ledger, haunts); error messages stay literal.

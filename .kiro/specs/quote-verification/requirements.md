# Quote verification: requirements

Introduction: given a quotation and one or more source texts, say whether the source supports it, and how.

**R1** WHEN a quotation appears word for word in a source, after folding case-insensitive punctuation, quote marks, dashes and whitespace, THE SYSTEM SHALL grade it `verbatim` and report the source span.
**R2** WHEN a quotation differs from the closest source passage by one or more words, THE SYSTEM SHALL grade it `drifted`, report the span, and return a word-level diff.
**R3** IF the changed words include a negation, a number (digits or spelled out), a name swapped for another name, a modal verb, or a quantifier, THEN THE SYSTEM SHALL set severity `material` regardless of the similarity score.
**R4** WHEN only case, punctuation or spacing differs, THE SYSTEM SHALL set severity `cosmetic`, and SHALL still call the quote `verbatim` if the words are identical.
**R5** IF no source passage is close enough, THEN THE SYSTEM SHALL grade `unsupported` and SHALL NOT name a closest source.
**R6** IF no source document is loaded, THEN THE SYSTEM SHALL grade `no-source`.
**R7** THE SYSTEM SHALL NOT fuzzy-match quotations shorter than four words.
**R8** WHEN a quotation starts or ends mid-word relative to the source, THE SYSTEM SHALL NOT grade it `verbatim`.
**R9** WHEN a quotation uses an ellipsis, THE SYSTEM SHALL match each piece in order and flag the elision.
**R10** WHEN a quotation matches more than one place, THE SYSTEM SHALL pick one deterministically and add a note saying the passage repeats.
**R11** WHEN given a draft, THE SYSTEM SHALL find every double-quoted passage of four or more words and grade each one.
**R12** FOR ALL source passages P and any contiguous word span S of P, checking S against P SHALL grade `verbatim`. (property)
**R13** FOR ALL spans with one word replaced by a different word, THE SYSTEM SHALL NOT grade `verbatim`. (property)
**R14** FOR ALL spans with a negation inserted, THE SYSTEM SHALL grade at least `material`. (property)
**R15** All offsets SHALL be Unicode code points, and mapping a folded span back to the original text SHALL round-trip. (property)

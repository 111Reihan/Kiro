# Ghost citations: requirements

Introduction: a citation points at a span of a source version, not at a copy of its text, so it can
tell you what happened to that text when the source was edited. (Xanadu's transclusion, minus the
network.)

**G1** THE SYSTEM SHALL keep every version of a source; saving an edit SHALL add a version and SHALL NOT change an earlier one.
**G2** WHEN the new text is identical to the latest version, THE SYSTEM SHALL refuse with `ErrNoChange` and add nothing.
**G3** WHEN a citation is recorded, THE SYSTEM SHALL store the doc, version, rune range, the exact source text, and a prefix and suffix of context.
**G4** IF a quote is not found in the source it is cited against, THEN THE SYSTEM SHALL refuse to record the citation.
**G5** WHILE a citation's mode is `follow`, THE SYSTEM SHALL re-locate it in the latest version and report `intact`, `moved`, `drifted` or `orphaned`.
**G6** WHILE a citation's mode is `pinned`, THE SYSTEM SHALL keep reporting against the pinned version.
**G7** WHEN a cited passage occurs more than once, THE SYSTEM SHALL pick the occurrence whose surrounding context matches.
**G8** WHEN a citation is `drifted`, THE SYSTEM SHALL return the old text, the new text and a word diff.
**G9** WHEN asked "who haunts" a range of a source, THE SYSTEM SHALL list every citation whose range overlaps it.
**G10** FOR ALL edits that do not touch a cited span, a `follow` citation SHALL stay `intact` or `moved`, never `drifted`. (property)
**G11** WHEN a trail is replayed, THE SYSTEM SHALL verify each step's receipt signature and compare the quote's grade then and now.

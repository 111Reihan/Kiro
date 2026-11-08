# Quote verification: tasks

| # | task | covers | test |
|---|---|---|---|
| 1 | fold text, keep offset map | R15 | `text.TestPropSpanRoundTrip`, `text.TestNormalizeCollapsesSpace` |
| 2 | extract double-quoted passages | R11 | `text.TestExtractQuotes`, `engine.TestCheckTextGradesEachQuote` |
| 3 | word alignment and diff | R2 | `verify.TestSynonymIsMinor` |
| 4 | verbatim detection | R1, R4 | `verify.TestVerbatim`, `verify.TestFoldingIsNotDrift`, `verify.TestClosingPunctuationIsNotDrift`, `verify.TestCaseOnlyIsCosmetic` |
| 5 | material change rules | R3 | `verify.TestNegationInsertedIsMaterial`, `verify.TestSpelledNumberChangeIsMaterial` |
| 6 | unsupported and no-source | R5, R6 | `verify.TestUnsupported`, `engine.TestNoSource` |
| 7 | short quotes never fuzzy | R7 | `verify.TestShortQuoteNeverFuzzy` |
| 8 | mid-word starts | R8 | `verify.TestMidWordIsNotVerbatim` |
| 9 | ellipsis | R9 | `verify.TestEllipsis` |
| 10 | repeated passages | R10 | `verify.TestRepeatedPassageIsNoted` |
| 11 | properties | R12, R13, R14 | `verify.TestPropSpanOfSourceIsVerbatim`, `verify.TestPropChangedWordIsNeverVerbatim`, `verify.TestPropInsertedNegationIsMaterial` |

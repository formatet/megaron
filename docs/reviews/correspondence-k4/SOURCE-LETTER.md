# K4 exact source correction

A citation's case link could open only the latest30 letters and leave an older exact source unreachable. `GET /worlds/{worldID}/correspondence/cases/{caseID}/letters/{letterID}` now returns the same case envelope containing exactly the requested visible letter, with an empty continuation cursor.

The route reuses Case's existing historical participant authorization, delivered-letter visibility, reply/offer knowledge masks and snapshot handling. Its only query addition is a nullable exact-letter predicate. Normal paginated Case reads retain their original behavior. Exact reads reject an invalid/zero UUID and any supplied cursor; a missing, wrong-case or invisible source returns404, never an empty misleading case or a different letter.

`TestCorrespondenceExactSourceLetterIsReachableBeyondFirstPage` sends32 letters in one case, proves the original source is absent from the default first30, and retrieves that exact source. It additionally rejects wrong-case/stranger/missing letters and malformed/cursor requests, prevents an undelivered recipient reading the source, and prevents an otherwise-visible case from exposing a different undelivered letter.

Fresh209 combined K2–K4 `TestCorrespondence` tests GREEN4.411s. The strengthened exact-source test then passes alone on another fresh209 DB0.256s. Logs: server-evidence/exact-source-*.log. Parent owns final native/CLI/full-suite verification against this correction. No new domain rule, migration or source mutation was needed; the existing visibility authority is reused.

# K4 server evidence

Memory separates engine-accepted transactions from a person's exact private citations. Opening one case recalls the same historical counterpart across that world's visible cases. Neither a quoted assertion nor an absent transport outcome becomes a shared promise, debt or accusation.

## API and lifecycle

`GET /worlds/{worldID}/correspondence/cases/{caseID}/memory?limit=&cursor=` returns `{counterpart_id,transactions,citations,next_cursor}`. Both streams have their own immutable key and exhaustion position inside one bound cursor. Default30, max100 rows per stream. No memory fulltext search is promised; nonempty q requests are rejected and the case archive remains searchable.

Transactions carry stable source letter/case IDs, case subject, historical parties/cities, exact raw accepted terms, send time, recorded acceptance time where known, and `outcome: unavailable`. A recipient must have actually received the letter; a sender must have physically received the bearer back. No live remote transport/status join is made. Existing accepted rows migrated from before209 retain `accepted_at: null`, never send/migration time. Migration209 records acceptance/return transaction membership so a later acceptance or physical return cannot slip into a previous pagination snapshot.

`POST .../cases/{caseID}/citations` requires `{letter_id,source,quote}`. Source is explicitly message or reply; quote is an exact nonempty substring up to1000bytes. The source may be another visible case with this same counterpart, but never a different person/world. Hidden replies cannot be quoted. Response includes citation ID, actual source case/letter, source type, exact quote, creation time and immutable source author/city attribution. Reply author is the historical recipient, never the original sender. Exact retries return the same citation ID.

`DELETE .../cases/{caseID}/citations/{citationID}` deletes only the caller's citation for this same historical counterpart. Repeated/foreign-owner IDs are successful no-ops (204), exposing no private existence. An inaccessible context case remains404.

Private citations reference retained archive cases/letters, not live cities. Name/ownership/contact changes and settlement/messenger deletion cannot transfer or erase the private citation. World/case deletion removes its memory deliberately. No new event type, notification, money movement, transport rule, or existing event interpretation changes. Quotes remain explicitly a person's note, not registered bilateral obligations.

## Proof

- Unchanged K3 `c6d35631`, fresh208: `TestCorrespondenceMemoryHTTPRouteContract` fails with HTTP404, expected empty memory200. Conditional production-method binding lets the identical chi HTTP test compile before the feature; it is not a claim of a native baseline process. Parent native proof tests executable routing.
- Fresh209: nine memory tests cover accepted visibility, cross-case same-person scope, private/deduplicated/deletable exact citations, source attribution/escaping, hidden reply, stranger/inexact/different-person rejection, retention, transaction-only/citation-only/mixed pagination, equal timestamps, backdated late insertion, acceptance/return after first page, cursor context/epoch/query binding, and explicit unknown historic acceptance time.
- Six physical production-source mutants each give the named behavioral failure: premature remote acceptance; reading another person's private citation; accepting invented quote; quoting hidden reply; later physical return entering old snapshot; later acceptance entering old snapshot. Runner: tools/correspondence_k4_server_mutations.py.
- All source bytes restored with matching SHA256 at2026-10-24T06:54:52.831121+02:00; timestamps/hashes in server-evidence/mutation-intervals.log. Subsequent fresh209 combined K2/K3/K4 targeted handler tests GREEN3.059s. Logs retained under server-evidence.

## Handover limits

Server projection tests explicitly fixture accepted trade outcomes; parent's native scenario performs the actual HTTP offer/accept flow and browser/Keryx actions. Parent owns the combined full Go/JS/vet and native proof. No full-suite/native-success claim is made by this server report. No merge, push or deploy.


Scope clarification: return-gating proves only the new archive/memory reads. Existing immediate OfferAccepted/OfferDeclined notifications in messenger.go remain unchanged and use current originOwner. No global blackout of pre-return acceptance knowledge is claimed; notification/ownership timing is a separate recorded contract/reproduction issue.

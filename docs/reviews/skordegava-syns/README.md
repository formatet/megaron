# Skördegåvan syns — proof package

## SAMMANFATTNING

An ongoing harvest blessing was invisible alongside city food. The Food section,
`keryx status` and `keryx city` now show its yield bonus, affected goods and days
left. The row disappears at the end tick. Codex kharis/rites explain where to find it.

Current example: **Blessed harvest: +25% grain, livestock, wine, oil yield · 12 days left**.
The API reads `harvest_blessing_until_tick` and `current_world_tick()` together;
bonus and goods come from the existing economy owner. Fish is excluded by that
owner. No rate, duration, balance, migration or granting mechanic changed.

Base `6b8b69ea`; production `5df4aa7db0993361bb3d63f63aaa206aad811dea`;
branch `codex/skordegava-syns`. Not deployed by Codex. Claude owns integration;
**BILD goes to Timothy via Claude**.

**Evidence:** 25 fresh-DB Go test packages + 568 JS tests, no skips; vet and both
binary builds green. Five physical mutations named red, exact-byte restore.
Firefox1280/390: before/active/end/repeat, no errors/overflow. Repeated active
and before-versus-ended images each pixel diff0. Built Keryx status/city active
and ended output captured. API boundary rows in [api-harvest-status.json](api-harvest-status.json).

## SLICE-KONTRAKT

| Field | Contract |
|---|---|
| Problem | An ongoing yield blessing cannot be read beside the city's food |
| Player truth | Bonus, affected goods and days left visible on all requested surfaces |
| Invariant | No row at/before end; authoritative tuning/column only; fish excluded |
| Scope | Province settlement response, city Food paragraph, two CLI commands, Codex articles, tests/proof |
| Non-scope | Granting/rate/balance changes, overview expansion, notifications work |
| Acceptance | Active/last-tick show, end/expired/never hide, values not hardcoded, both viewport widths fit |
| Stop condition | Canon/mechanic change would need Timothy; none required |
| Evidence | Real handler + fresh DB, actual commands + explicit HTTP, actual drawer + explicit HTTP, screenshots, mutations |

This proves food-supply legibility in geography→shortage→bronze→elite.

## BASLINJE

Fresh PostgreSQL16, migration171: affected Go packages green before implementation
([baseline-go.log](baseline-go.log)). New contract tests are named red on the
unmodified implementation: missing API blessing, CLI row and web paragraph
([baseline-go-contract.log](baseline-go-contract.log), [baseline-js-contract.log](baseline-js-contract.log)).
Web before images load only the Food module from locked base `6b8b69ea`; the
same current authoritative-style HTTP blessing is supplied, and the old module
cannot show it. Mycenae/map surroundings are explicit deterministic fixtures.

## EXPERIMENT

| Hypothesis | Change/check | Outcome |
|---|---|---|
| Existing response lacks blessing | Read expiry together with existing current tick; add nullable `settlement.harvest_blessing` | Active object includes until_tick, remaining_ticks, yield_bonus, goods; inactive null |
| Clients could hardcode the bonus | Use response values in shared CLI formatter and Food builder; test bonus37%, goods grain/wine | Both commands and web show supplied values |
| Expiry boundary can leak an active status | Real DB current_tick100; until112/101/100/99/NULL | Remaining12/1; null/null/null at end, expired, never |
| Removing any surface should fail | Disable API expiry/bonus, web paragraph, status call, city call physically | Five named red mutations, exact-byte restores; [mutations.json](mutations.json) |
| Paragraph might overflow or alter inactive UI | Actual city drawer, real CSS/map renderer, 1280/390, repeated captures | Text wraps within viewport; active repeat0 pixels; before/end0 pixels |

The combined authoritative tick read returns HTTP500 if it fails, instead of
inventing a remaining duration from a default current tick. The surrounding
response fields and successful request behavior remain intact.

The first full Go run caught `tick` in the new player-facing error string. It was
changed to `could not read settlement status`; the complete suite was rerun on
another fresh database and passed. The initial fixture harness also lacked the
MusicPlayer bridge and read-only catalog/actions responses; those fixture
omissions were supplied, then all browser scenarios rerun clean.

## GRINDAR

| Gate | Result |
|---|---|
| Code | [full-go.log](full-go.log):25 test packages; [full-js.log](full-js.log):568/568; [targeted-go.log](targeted-go.log):real handler boundary JSON; vet + Temenos/Keryx builds green |
| Visual | Firefox actual city controller,1280×900 and390×844 at1:1; viewed both active images; [browser/results.json](browser/results.json), [browser/pixeldiff.json](browser/pixeldiff.json); BILD awaits Timothy |
| Semantic | Ticks internal/days at surface; exact expiry; owner-derived bonus/goods, never fish; no empty paragraph when inactive |
| User | Built Keryx status/city and actual web drawer render supplied authoritative-style response; active/end checked; explicit fixtures, not a gameplay acceptance claim |
| Drift | Not deployed; acceptance rig untouched. Provenance here is the source commit/hashes plus fresh test DB migration171, not another live-app healthz measurement |

New pictures:
[1280 active](browser/firefox-1280-active.png) ·
[390 active](browser/firefox-390-active.png) ·
[1280 baseline](browser/firefox-1280-before.png) ·
[390 baseline](browser/firefox-390-before.png).

## METODISK LÄRDOM

Carry the affected goods along with the bonus, so the player sentence explains
what the observed production increase applies to without duplicating tuning.

## KÄNDA AVGRÄNSNINGAR

Browser and built CLI use explicit Mycenae HTTP fixtures, while persistence,
actual response shape and expiry are independently proven through the real
ProvinceHandler.Get against fresh PostgreSQL. This slice does not claim a
normal-player rite cast or a new acceptance-world walkthrough. The displayed
countdown refreshes with the existing city/status fetch behavior; no new timer
or polling behavior is introduced.

Reproduce: `tools/gotest.sh`; `node --test $(rg --files web/static/js | rg '\.test\.mjs$')`;
`python3 tools/harvest_status_mutation.py`; `python3 tools/harvest_status_browser.py`
(Playwright Firefox + Pillow); build Keryx, then
`python3 tools/harvest_status_keryx.py /path/to/keryx`. The browser tool records
fixture data and source hashes; the CLI tool captures actual executable output.
Log trailing whitespace is normalized for version control. No token, secret or
live-world write is involved.

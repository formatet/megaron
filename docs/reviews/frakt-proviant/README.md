# ORDER20 — freight ship provisions gauge

Contract/base bad61a32/migration202. Proves freight supply visible in main chain.
Server already computes provision_days/provisions_aboard/provisions_by_good for
naval units including freighting. Keryx formatSize already shows them while
freighting; web card explicitly excludes that status using an obsolete comment.
Change only visibility: use existing server values, warning colours, manifest
and last-ration distinction. No ration, loading, consumption, return, orders or
freight timing change. Four surfaces: server/CLI verified, web corrected, Codex
clarifies freight gauge. No migration. ORDER21 waits for new order (203 noted).

Proofs: named JS old-code RED, real fresh API/CLI parity, physical visibility
mutation, two real HTTP/Firefox390/1000 runs with tap; final JS/vet/fresh Go.


## Result / evidence

Frozen checkpoint `ec3a6988`. Runtime diff: one visibility condition in war.js
plus removal of obsolete comment, and Codex sea explanation. No server or CLI
runtime changes; new actual API/CLI tests prove their existing parity.

- baseline-red.log: named old-code real renderUnitCard test fails to find food
  on a freighting ship. Targeted JS/API/CLI logs green after visibility fix.
- mutate.py physically hides freighting again: named card test RED, byte-exact
  restored source hash in mutation.json, restored test GREEN.
- player-scenario.log: two clean synthetic three-ship states through actual
  authenticated ListUnits HTTP, built Keryx unit list and real Firefox Army
  drawer. Stocks .75/.05/0 food correspond to7days/last part/empty for10rowers.
  Browser tap War and Recruit→Army; all three gauges persist. No new Mission
  button; both runs zero page errors. Actual DB stores remain unchanged.
- index.html A1/A2 native390/1000 BILD, inspected1:1. Existing flex wrap handles
  extra manifest text; no clipping or new controls. Typography/layout unchanged.
- full-fresh.log/js-final.log/vet.log final verification against migration202.

Initial fixture-attempt log preserves .7food/.1ration flooring to6 because of
existing floating representation. The parity test uses .75, unambiguously7
whole days, and does not change server numeric rules. Existing browser manifest
format rounds .75 to0.8 while CLI shows0.75; browser harness initially expected
CLI precision and failed. Rounding-attempt log preserved; corrected harness
expects the established web precision, no production change.

## Limits / gates

Ships/starting stock are seeded; no claim of a newly negotiated physical trade
or a voyage executed end-to-end. API and Army renderer are real, bootstrap/map
markers and unrelated endpoints scaffolding. Browser intercepts only bootstrap
and static shell, never game data APIs. This proves presentation of the current
projection, not cargo allocation/consumption/return mechanics. Those are existing
ORDER12 substrate, unchanged here. Human comprehension remains TEXT in playtest.
No migration, live writes, merge, push or deployment. ORDER21 not started.

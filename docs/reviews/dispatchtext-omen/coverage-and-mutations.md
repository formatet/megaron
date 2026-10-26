# Coverage and mutation review

Final production base: `c4a66c74`, rebased onto fetched `origin/master fc2741e2` immediately before the final fresh suite. No integration is claimed by this file alone; see deploy proof.

## New test reach and limits

| Test / proof | Calls and branches covered | Not covered by this new proof |
| --- | --- | --- |
| TestCompletionNoticesCarryCityAndPlayerNames | actual combat BuildComplete (hex workplace, city label and human building name), legacy TrainComplete merchantman notification | other legacy training types, every building/god permutation |
| TestShipyardCompletionPreservesNotificationIdentityAndSeverity | actual forming merchantman Emporos and repairing Galley notifications, city, level, exactly once | every naval type and failure path |
| TestBuildQueueCarriesCityPlayerNameAndHex + Keryx queue test | real queue API city/site/label plus CLI formatting | concurrent cancellation while reading |
| TestCompletionDispatchCityAndPlayerNames + format.test | historical payload without optional names, Emporos, War Galley, Elite Infantry; building city/hex | arbitrary future types |
| TestRecruitWarChariotCedarCancellationLandAndSpill | real Recruit HTTP partial 80 + top-up20/spill80, actual16cedar debit, refunds3.2/12.8, timber0 | every land type, other substitution ratios |
| TestOmenNoFactsMeansSilence | real neutral food/no deposit/no storm world | all possible production recipes |
| TestOmenTemplatesRequireTruePredicatesAndBoundPrecision | all four templates true/false, all bands; actual L3 cap100 reaches90, L2 cap75 cannot; critical-only exact hex | simulation of every temple-upgrade path |
| TestOmenObservedOutcomeCooldownAndSpecificity | real sea storm, event/gossip/cooldown atomic outcome, same-tick retry, second temple same realm, ordinary vs rare top roll, low radius excludes distant storm | forced serialization failure, every map geometry |
| TestMaintainedTickProducesObservedOmenOnce | actual processMaintenance caller and processed claim retry, real positive food candidate | full worker scheduling race |
| TestTransportGossipDoesNotReturnToItsOwner | Broadcast owner exclusion, foreign witness; PropagateOnContact excludes returning owner and preserves marker to another learner | all contact mechanisms individually |
| TestDeliveryTransportGossipKeepsOriginalOwner | actual DeliveryHandler physical transport owner and legacy origin owner branches | owner lookup DB failure (best effort skips rumor), all transport emitters |
| TestGossipShowsTrueOmensAndOthersTradeOnly | API filters own metadata, legacy own trade and unfounded old omen; true omen/foreign trade retained | historical ownership cannot be reconstructed for legacy rows |
| dispatch_actions.test + inventory | measured current/archive kinds have no invented empty target; coordinate fallback per reader kind; explicit staff/unit/letter/cult/goods/cargo/standing route; close covering drawer before target | payload permutations of all95 kinds are not individually end-to-end tested; dynamic emitter trace is explicit manual inventory |
| dispatch_window.test | delayed route response cannot reopen closed dispatch or overwrite newer dispatch/world | every other async drawer request |
| citygrid/kult JS | exact hex ordinal including0; explicit own cult city, deleted/foreign explicit city has no capital fallback | submitting another worker or rite after city ownership changes |
| native Firefox BILD | actual worker completion in noncapital Petras, exact staffing selection, real Emporos focused card, real RiteAnswered Cult in Petras, real standing-delivery route highlighted, maintained-temple true omen and own/foreign gossip,1280/390; only intended drawer remains open | Goods/cargo navigation has focused JS and shared existing rendering tests, no new browser scenario; no extra placement submitted merely for screenshots |

## Executed mutations

20 physical assertion-red mutations, exact source restore. Go mutations use a fresh migrated DB. The first train-name attempt failed compilation due to an unused import and is explicitly **not** evidence; the corrected mutant preserves import usage and fails the real completion assertion. `mutations/verified.json` has the corrected20; every sed command below was applied separately and byte-compared with the intended mutation, then restored. Run `python3 tools/dispatch_omen_mutants_20261026.py /path/to/separate-clean-worktree` to reproduce. Logs use test-only31b65419 or49f4bd4a; production is identical except the separately verified drawer-close fix and two food-description strings (town food production includes fishing).

### false-deposit

Expected red: `TestOmenNoFactsMeansSilence`. Actual: assertion red.

```sh
sed -i 's|copper_deposit = true|copper_deposit IS NOT NULL|g' server/internal/kharis/omen.go
```

### tick-caller

Expected red: `TestMaintainedTickProducesObservedOmenOnce`. Actual: assertion red.

```sh
sed -i 's|if err := h\.generateOmen(ctx, w\.settlementID, worldID);|if err := error(nil);|g' server/internal/kharis/tick.go
```

### own-broadcast

Expected red: `TestTransportGossipDoesNotReturnToItsOwner`. Actual: assertion red.

```sh
sed -i 's|transportOwnerID != nil && recipID == \*transportOwnerID|false \&\& transportOwnerID != nil \&\& recipID == *transportOwnerID|g' server/internal/gossip/gossip.go
```

### trade-owner

Expected red: `TestDeliveryTransportGossipKeepsOriginalOwner`. Actual: assertion red.

```sh
sed -i 's|p\.GoodKey, transportOwner);|p.GoodKey);|g' server/internal/economy/trade.go
```

### api-owner

Expected red: `TestGossipShowsTrueOmensAndOthersTradeOnly`. Actual: assertion red.

```sh
sed -i 's|transport_owner_id<>\$2|true|g' server/api/handlers/settlement.go
```

### queue-city

Expected red: `TestBuildQueueCarriesCityPlayerNameAndHex`. Actual: assertion red.

```sh
sed -i 's|bi\.SettlementName = sett\.Name|bi.SettlementName = ""|g' server/api/handlers/province.go
```

### build-city

Expected red: `TestCompletionNoticesCarryCityAndPlayerNames`. Actual: assertion red.

```sh
sed -i 's|"settlement_name": settlementName|"settlement_name": ""|g' server/internal/combat/build.go
```

### shipyard-name

Expected red: `TestShipyardCompletionPreservesNotificationIdentityAndSeverity`. Actual: assertion red.

```sh
sed -i 's|body\["unit_name"\] = unit\.DisplayName(typ)|body["unit_name"] = typ|g' server/internal/combat/shipyard_work.go
```

### train-name

Expected red: `TestCompletionNoticesCarryCityAndPlayerNames`. Actual: assertion red.

```sh
sed -i 's|unit\.DisplayName(p\.UnitType)|(func() string { _ = unit.DisplayName(p.UnitType); return p.UnitType })()|g' server/internal/combat/train.go
```

### keryx-name

Expected red: `TestCompletionDispatchCityAndPlayerNames`. Actual: assertion red.

```sh
sed -i 's|unit\.DisplayName(b\.UnitType)|b.UnitType|g' server/cmd/keryx/notification_text.go
```

### propagate-owner

Expected red: `TestTransportGossipDoesNotReturnToItsOwner`. Actual: assertion red.

```sh
sed -i 's|rm\.transportOwner != nil && \*rm\.transportOwner == learnerID|false \&\& rm.transportOwner != nil \&\& *rm.transportOwner == learnerID|g' server/internal/gossip/gossip.go
```

### realm-cooldown

Expected red: `TestOmenObservedOutcomeCooldownAndSpecificity`. Actual: assertion red.

```sh
sed -i 's|state != "empty" && current-previous < band\.Gap|false \&\& state != "empty" \&\& current-previous < band.Gap|g' server/internal/kharis/omen.go
```

### land-paid

Expected red: `TestRecruitWarChariotCedarCancellationLandAndSpill`. Actual: assertion red.

```sh
sed -i 's|unitID, paidCosts, totalKharis, unitSize-existingSize|unitID, totalCosts, totalKharis, unitSize-existingSize|g' server/api/handlers/province.go
```

### spill-paid

Expected red: `TestRecruitWarChariotCedarCancellationLandAndSpill`. Actual: assertion red.

```sh
sed -i 's|spillID, paidCosts, totalKharis, newSize-100|spillID, totalCosts, totalKharis, newSize-100|g' server/api/handlers/province.go
```

### drawer-close

Expected red: `ui/dispatch_actions.test.mjs`. Actual: assertion red.

```sh
sed -i 's|if(State\.activeDrawer)window\.closeDrawer?\.(State\.activeDrawer);||g' web/static/js/megaron/ui/dispatch_actions.js
```

### staff-action

Expected red: `ui/dispatch_actions.test.mjs`. Actual: assertion red.

```sh
sed -i 's|kind==='"'"'BuildComplete'"'"'&&STAFFABLE\.has(p\.building_type)|false\&\&kind==='"'"'BuildComplete'"'"'\&\&STAFFABLE.has(p.building_type)|g' web/static/js/megaron/ui/dispatch_actions.js
```

### exact-hex

Expected red: `ui/citygrid.test.mjs`. Actual: assertion red.

```sh
sed -i 's|return hex?`hex:\${hex\.hex_ordinal}`:null;|return "city";|g' web/static/js/megaron/ui/citygrid.js
```

### kult-city

Expected red: `ui/drawers/kult.test.mjs`. Actual: assertion red.

```sh
sed -i 's|return id?provinces\.find(p=>p\.own&&p\.id===id)\|\|null:fallback;|return fallback;|g' web/static/js/megaron/ui/drawers/kult.js
```

### unit-text

Expected red: `ui/format.test.mjs`. Actual: assertion red.

```sh
sed -i 's|body\.unit_name \|\| unitTypeLabel(body\.unit_type)|body.unit_name \|\| body.unit_type|g' web/static/js/megaron/ui/format.js
```

### stale-dispatch

Expected red: `ui/dispatch_window.test.mjs`. Actual: assertion red.

```sh
sed -i 's|if(version!==dispatchOpenVersion\|\|world!==State\.WORLD_ID)return;|if(false)return;|g' web/static/js/megaron/ui/dispatch_window.js
```

## Remaining consumer review

`province/building_names.go` shares the canonical catalogue consumed by the real build and CLI proofs. CLI `cmd_build.go`/`cmd_status.go` share queue formatting; `cmd_gossip.go` only labels truthful metadata already API-tested. Browser `drawers/city.js`, `drawers/economy.js` and `render/map.js` wire to existing surfaces; the native scenarios above cover city and route wiring, map fallback has resolver/selection tests but no fresh browser scenario. `drawers/gossip.js` has explicit truthful-metadata rendering tests. These limits are retained instead of claiming every branch of every changed file was mutated.

Schema215 up/down/up passed on fresh PostgreSQL with a retained legacy row; no production data migration or production-rules recompute is needed.

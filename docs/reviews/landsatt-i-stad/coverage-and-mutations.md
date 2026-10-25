# Täckning och korta granskningsmutanter

Slutkoden är rebasad på `1747b395` innan slutbeviset. `city_landing_mutants_20261025.py` kör varje servermutant i egen återställd kod och på ny DB. Kör bara EN mutation åt gången, återställ exakt original efter varje prov.

| Nytt prov | Täcker | Täcker inte |
| --- | --- | --- |
| `TestCityLandingAllPassengersStayAndSupportUnchanged` | StartMarch direkt: intent land med tre kohorter, vanlig segling med tre, vanlig segling tom; ny dock-gren i resolve; alla CargoIDs; dubbel resolve; ingen automatisk retur; fysisk målstad; support för alla; nästa avgång tar mat ur gamla stödstaden; låg mat nära målstad överger inte dock-mission | HTTP-intag eller fientlig amfibies stridsutfall; andra båttyper; varje möjlig kustanslutning |
| `TestCityLandingRechecksOwnershipAndKeepsPassengers` | Målet blir främmande under resan: återvalidering, retur med passagerare, ingen automatisk ny strid | Erövring mitt i själva transaktionen; upphävd hemstad (befintliga returprov) |
| `TestCitiesOwnPortMetadataKeepsForeignHarbourPrivate` | loadCities: egen kust/hamn och känd främmande kust utan privat byggnadsuppgift | Nytt FOW-system; ryktenas befintliga övriga data (gamla cities-testsviten täcker det) |
| marchctx/city_targets/map-metadata JS | tomt/lastat eget hamnmål från War och kartmeny, kust/hamn/inland, allied≠own, befintlig fientlig amfibie, forwarding från faktisk karttile och citydirectory | serverns sjöruttsökning |
| War dock-status + Keryx dock-location | läsytor lovar kvarliggande skepp för dock; gammal auto-return-text kvar för övriga missioner | mänsklig textplaytest |
| native Firefox | verklig register/join/found, tre Load-API-anrop, War→March→stad, riktig land-order202, deadline/worker-ankomst, API/Keryx/DB; alla fyra i Petras med stödLato, inga returjobb | tomt skepp verkligen avsänt från UI (eligibilitet+ETA i bild, själva tomresan i DB-provet); andra frontendmotorer |

Befintliga `TestLandMission_*` täcker bar marks landsättning+hemresa och kolonisering inklusive purse; de ingår i både berört prov och fullsvit. Persistenta gamla land-missioner omtolkas inte: ny stadsväg sparas som ny intern mission `dock`.

## En servermutant per kärnfil

```sh
sed -i 's/if dockMission {/if false \&\& dockMission {/' server/internal/combat/march_start.go
# tools/gotest.sh ./internal/combat/ -run TestCityLandingAllPassengers

sed -i 's/for _, id := range ids {/if len(ids)>1 { ids=ids[:1] }; for _, id := range ids {/' server/internal/combat/city_disembark.go
# tools/gotest.sh ./internal/combat/ -run TestCityLandingAllPassengers

sed -i 's/settlement_id = \$5,/settlement_id = $5, support_settlement_id = $5,/' server/internal/combat/unit_arrival.go
# tools/gotest.sh ./internal/combat/ -run TestCityLandingAllPassengers

sed -i 's/s\.owner_id=\$2 AND s\.state/s.state/' server/internal/combat/ship_port_call.go
# tools/gotest.sh ./internal/combat/ -run TestCityLandingRechecksOwnership

sed -i 's/ || \*intent == "dock"//' server/internal/combat/return_food.go
# tools/gotest.sh ./internal/combat/ -run TestCityLandingAllPassengers

sed -i 's/(s\.owner_id=\$2 AND EXISTS/(true AND EXISTS/' server/api/handlers/world.go
# tools/gotest.sh ./api/handlers/ -run TestCitiesOwnPortMetadata
```

Alla sex kördes fysiskt och gav riktiga assertion-röda prov på slutkoden, utan byggfel/panik. Loggarna och `mutations.json` anger utfallet; `mutants-restored-green.log` är ny färsk DB efter exakt restore.

## Korta klientmutanter för granskning

Dessa är föreslagna granskningsrader, inte extra påstådda körningar utöver klienternas rött→grönt-loggar.

```sh
sed -i 's/dest\.own === true/false/' web/static/js/megaron/ui/marchctx.js
# node --test web/static/js/megaron/ui/marchctx.test.mjs
# Rött: own coastal city offers empty and laden ships in the map menu and War target picker

sed -i 's/coastal:city\.coastal/coastal:false/' web/static/js/megaron/ui/city_targets.js
# node --test web/static/js/megaron/ui/city_targets.test.mjs
# Rött: city destination retains ownership and harbour metadata for the ship target gate

sed -i 's/coastal: target\.coastal ?? tile\.coastal/coastal: false/' web/static/js/megaron/render/map.js
# node --test web/static/js/megaron/ui/marchctx.test.mjs
# Rött: map city target forwards actual ownership and port metadata instead of equating allied with own

sed -i "s/u.march_intent === 'dock'/false/" web/static/js/megaron/ui/drawers/war.js
# node --test web/static/js/megaron/ui/war_cards.test.mjs
# Rött: a ship bound for an own city promises to dock and stay, and land everyone aboard

sed -i 's/if u.MarchIntent != nil \&\& \*u.MarchIntent == "dock" {/if false {/' server/cmd/keryx/cmd_unit.go
# tools/gotest.sh ./cmd/keryx/ -run TestLocationStr_DockingShipStaysAndLandsPassengers
```

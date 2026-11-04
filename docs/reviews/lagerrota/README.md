# Lagerrötan bort (grain + timber −1 %/dygn) — bevis

Order: `.agents/order-ta-bort-lagerrota-20261102.md` (Timothy 2026-11-02). Gren `sonnet/ekonomins-sanning`, commit `f20bfcc2`.

## Ändring
`server/internal/kharis/tick.go` `applyDecay`: SQL-satsen `amount = GREATEST(0, settled(...) * 0.99)` för grain/timber, dess
`WITH claim`-mot-`processed_tick_claims` (scope `uuid.NewSHA1(worldID, "goods_decay")`) och rötkommentarerna är borttagna.
Allt annat i `applyDecay` (tillväxt, svält, per-settlement-claim, befolkning) är orört. Ingen migration, tabellen och övriga scope orörda.

## Inventering (grep `0.99|goods_decay|spoil|ruttn|Store rot`)
Produktionskod: bara `tick.go` (borta). Övriga 0.99-träffar är orelaterade: `cmd/keryx/cmd_goods.go:137` (cap-visning ≥99 %),
`combat/rower_ration_tick_test.go`, `combat/ship_hull_db_test.go` (retreat_at_loss). `tools/lagersvangning_matning.py:210` modellerar
rötan i ett mätskript (rörd ej; historiskt). Codex: ingen text om röta av grain/timber (`kharis.md` "food rots" = gudastraffet, nedan).
Vault `temenos_balans_spakar.md` raden "Store rot" var redan markerad ABOLISHED 2026-11-02.
Berörda tester: 3 (ingen över ~10; alla bevisade bara rötan eller hade rötan i sin spegel).

## Baslinje vs efter (färsk DB per körning, `tools/gotest.sh`)
| Paket | Baslinje (b8a5c8b5) | Efter ändring + testfix |
|---|---|---|
| internal/kharis, combat, economy, api/handlers | alla gröna | alla gröna |
| Hela sviten + webbtester (en gång, färsk DB) | – | allt grönt, inga röda |
Direkt efter koständringen, före testfix: röda `TestApplyDecay_ReplayIsIdempotent` och `TestApplyDecay_GoodsDecayReplayIsIdempotent` (bara dessa).

## Ändrade tester och skäl
- `kharis/applydecay_idempotent_test.go`
  - `TestApplyDecay_ReplayIsIdempotent` (bevisar tillväxtclaimens idempotens, ej rötan): sanity-kravet "grain ska ha rört sig (×0.99)" och
    kommentaren om ×0.99-replay är borttagna; replay-kravet "grain oförändrad" och populationskraven är kvar oförändrade.
  - `TestApplyDecay_GoodsDecayReplayIsIdempotent` (bevisade ENBART rötans claim): borttaget, ersatt av nya testet nedan. Hjälpfunktionen `readTimber` (dött) och två oanvända importer borta.
  - NYTT `TestApplyDecay_StoresDoNotRot`: grain, timber och cedar med rate 10, lager 500, calc_tick en dag bakåt; efter `applyDecay` är värdet exakt `settled()` (510).
- `kharis/grain_growth_test.go`: Go-spegeln `expectedDayResult` tappar `* 0.99` (och kommentaren). Testet var grönt även med fel spegel (tolerans 5.0) — nu exakt.
Inget tests avsikt ändrad.

## Mutant
Temporär `* 0.99`-sats tillbaka i början av `applyDecay` (loggen `/home/tk/.cache/sjok1-mutant.log`): `TestApplyDecay_StoresDoNotRot` RÖTT, dödad:
`applydecay_idempotent_test.go:221: grain after applyDecay = 504.9000, want exactly the settled 510.0000` (samma för timber; cedar orörd). Återställd via `git checkout HEAD -- tick.go`.

## Taksbeteende (flagga till Claude, ej löst)
- `settled()` (mig 048) klipper inte vid cap: lagret kan i läsningar överstiga taket tills nästa skrivning. Skrivningarna (`RecomputeProduction`, `credit_settlement_good`, upkeep) går via SQL `storage_capped` (mig 191/235): överskottet försvinner då,
  händelsen `StorageOverflow` skrivs varje gång, och en notis `StorageOverflow` skickas när förlusten *börjar* (tyst i 24 speldagar därefter). Taket är 2000 per livsmedel utan silo (`food_store_base()`, mig 198), 4 000/10 000/20 000 med Granary 1/2/3.
  Dvs. vid taket slösas produktion, med notis, som för alla andra varor. Det är ny exponering för grain/timber: förr planade rötan ut stocken, nu fylls den till taket.
- Inget test eller balansantagande i sviten förutsatte rötan utöver de tre ovan. `tools/lagersvangning_matning.py` har rötan inbyggd i sin modell (mätskript).
- Kvarvarande förruttnelse som INTE är lagerrötan: gudastraffet "fields lie fallow — half your grain stores rotted" (`tick.go` ~rad 1280, `kharis` punishment, `cmd/keryx/cmd_notifications.go:1126`). Orört; det är straff, ej daglig röta.

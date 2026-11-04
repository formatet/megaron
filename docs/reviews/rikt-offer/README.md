# Rikt offer ovanpå tiondet — inventering (2026-11-04)

Order: `.agents/order-rikt-offer-20261101.md`. Kanon: vault `temenos_kharis.md` §Rikt offer ovanpå tiondet.

## Inventering (kodverifierad på master dbea29f7)

- Grundkravet: `internal/kharis/temple_offering.go` `titheParts`, `planGrainPart`, `newTempleOfferForLevel`, `planCityOffers` (templen planeras i gud-ordning mot ett delat lager). `grainOfferingGoods` = grain, fish, timber. `livestock` saknas.
- Pris: `religion.FixedDivineValues` (livestock=2, grain=1, fish=2, timber=2, silver=2, incense=12 ...).
- Lagerladdning: `templeOfferingGoods` (grain, fish, timber, silver, incense) används både vid läsning (`loadTempleOffers`, `chargeTempleOffering`) och debitering; måste utvidgas för rika varor.
- Konsumenter av planen: tick (`chargeTempleOffering`, `tick.go` summerar `EffectiveWeight`), prognos (`project.go`, `ForecastTithe`), status/Kult (`handlers/cult.go`, `province.go`), keryx `cmd_status.go`, webb `kult.js`/`kult_realm.js`.
- Gudens smak: **finns bara per bön** (`PrayerSpec.Favours`, `religion.FavoursFor(spec)` -> arketyp via `favoursByEffect`). Ingen smak per tempel/gud finns i koden. `OfferingWorth` använder den för böner.

## Smak per tempel: ingen befintlig källa passar

Smaken är nyckad på effekt/bön, inte gud. Ett tempel har flera böner med olika smaker (Potnia: oracle-smak vs battle-frenzy-smak).

Förslag, väntar på Timothys beslut:
- **A. Gudssmak:** ny tabell `TempleTaste[god][good]` (en rad per gud, t.ex. havsdaimonerna fisk 2.2). Ny kanon, enkel, ett ställe att läsa.
- **B. Härled ur gudens böner:** medelvärdet (eller max) av `FavoursFor` över gudens böner. Ingen ny tabell men implicit och svårläst för spelaren.

Tills dess: `templeTaste(god, good) float64` returnerar 1 (en enda funktion, enda ändringspunkt).

## Designval att bekräfta
- Rika varan dras i en andra pass efter ALLA tempels grundkrav i staden (gud-ordning), så ett rikt offer aldrig kan svälta ett grundkrav.
- Rikt offer dras bara om templets grundkrav betalas till någon del (annars är bonusen ×0 ändå).
- Mängd = 0,5 × 0,25 × nivå² × värde(korn) / (värde(vara) × smak).

# Kharis-golv för värden

Inventering före kod, bas062276e6, gren codex/kharis-golv-vard, eget worktree.

Kontrakt: ny och återuppstånden Host startar på samma golv som tickens Kharis-clamp. Grundningens seed förblir oförändrad. Befintliga aktiva Hosts under golvet lyfts additivt; stadsrealmer/avslutade Hosts påverkas inte. Ingen ny yta, BILD undantagen av ordern. Kedjegrind: visar en korrekt egen relation före grundning, ingen ny progression/balans.

## Inventering före implementation

- Produktionssökning `INSERT INTO player_world_records` hittar bara join och create_metropolis. Rise gör statusUPDATE och återanvänder sin befintliga PWR-rad. SeedNomadicHost→seedHost används av join; rise anropar seedHost direkt. Inga andra produktionsanrop.
- Join INSERT statusactive använder DEFAULT kharis_amount0, rate0, calc_tick0. Rise återställer bara status och founder_phase; gamla amount/rate/calc följer med.
- Delad seedHost är samma transaktion som Host+eskort+founder_phase+events. Dit flyttas PWR active-upsert med golvet, rate0 och världens egen current_tick. last_settlement_id/founded_tick bevaras. Idempotenta återbesök join/rise svarar före seed och får inte återställa en befintlig Host.
- CreateMetropolis seeder starterKharis25, kapitalens geografiska rate, calc_tick/founded_tick=current_world_tick(), både INSERT/UPDATE. Ingen produktionsändring där.
- Golvets ägare: internal/kharis/tick.go kharisFloor1.0 används av clampKharis. Exportera alias Floor för seeden, ingen andra Go-siffra.
- Read: loadPlayerKharis läser GREATEST(0,settled(...)); world/me använder helpern och profiler/Keryx me visar resultatet före grundning. Status har Host-läsning utan egen Kharisrad; grundade städers status/Kult använder realm-poolen. Ingen klientmaskering eller ny statusrad behövs.
- Tick: tempelunderhåll behandlar stadsrealmer; Host når inte clamping via en aktiv stad. Därför behövs mig174 för befintliga aktiva founder_phase utan aktiv egen stad. Begränsa också PWRstatusactive; grundade/avslutade/dispossessed lämnas. Migrationens1.0 är fryst historisk golvkalibrering; Go-seed använder konstantens ägare. Liftade rader får rate0 och egen världstick för att inte ackumulera förra livets geografiska takt.

## Provplan
Färsk Go-baslinje join/rise/grundning före kod. Verklig join→me visar Floor, delad rise-seed och riktig rise visar Floor/rate0/calc_tick, återbesök resetter inte. Grundningens amount/calc oförändrade. Mig174 inkluderar låg aktiv Host men utesluter aktiv stad, avslutad/dispossessed och redan hög Host; omkörning ändrar inget. Fysisk seedmutation ger namngivet rött, byteåterställt grönt. Full färsk Go/JS/vet. Claude integrerar efter hash+STOPP.

Implementation: PWR active-upsert har EN ägare i seedHost; gamla statusupdateringar i join/rise är borttagna. Seeden håller last_settlement_id/founded_tick orörda. Floor är exporterat alias till tickens kharisFloor. Mig174:s down är avsiktligt en no-op: additiv reparation kan inte säkert återskapa gamla nollvärden. Codex kharis beskriver Host/rise och rättar den nu felaktiga uppgiften att inget tal syns (Account visar redan mängden).


## Slutbevis

- Full färsk `tools/gotest.sh ./...` grön på871d6388, tom DB/migration174. Full `go vet ./...` grön och602JS gröna. [Full Go](full-go.log), [JS](full-js.log), [vet](vet.log).
- Fysisk seedmutation: seedHost argument Floor ersattes med0.0 (import bort så giltig kompilering). Namngivet rött både `new Host missing shared kharis floor` och `risen Host did not reset kharis to shared floor`; byteåterställt färskt grönt. [Rött](seed-mutation.log), [återställt](seed-restored.log).
- Färsk DB testar mig174 efter en verklig joinmiljö: låg aktiv Host repareras, redan hög Host, aktiv stad, avslutad fas, dispossessed och ingen fas lämnas exakt orörda. Omkörning är idempotent. Grundning visar fortsatt25 och aktuell calc_tick, produktionsdiff0 i create_metropolis.go.
- Isolerad runtime871d638/mig174: NORMAL auth/register→join för ny Wanax **Deukalas**, world/me och nybyggd Keryx me/JSON visar delade Floor1.0 före grundning. **Eumedes**, tidigare0.0 i spelarprofilens bevis, läses nu1.0 efter174. Ingen SQL-skrivning eller payloadoverride i provhandlingen. [CLI me](new-host-cli.txt), [status](new-host-status.txt), [verification](verification.json). Status behåller sin befintliga Hostinformation och har ingen numerisk Kharisrad; det är API/me och keryx me som bevisar talet. Ingen onödig presentationsändring.
- Rigg byggd CGO0 lokalt på cachad acceptansruntime, nuvarande migreringar + eget webmount; ingen ren basimagebuild. [Runtime](rig/runtime.json). Tre labbvärldar: script väljer uttryckligen aktiv3b5fb9cd, acceptance provenance väljer automatiskt nyaste arkiverad2e335492. Inga dev-containerändringar.

Repro: `PLACE_PROOF_OUTPUT="$PWD/docs/reviews/kharis-golv-vard/rig" python3 tools/place_names_rig.py`, sedan `python3 tools/host_kharis_proof.py`. CLI-token ligger bara i tillfällig0600-fil och sparas inte i rapporten. Värdens join är verklig; tidigare labbvärldsdata kommer från föregående acceptansslicers, inget påstående om naturligt ekonomiförlopp.

Grindar: kod/färska prov och faktiskt läsbeteende gröna; samma golvsemantik på delad seed och migration; ingen BILD enligt uttrycklig order (tal). Claude granskar/integrerar efter hash+STOPP. Ingen merge/push/deploy från Codex.

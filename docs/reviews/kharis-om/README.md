> Uppdatering: Claude förtydligade beslutet 2026-10-18 13:15: taket utan tempel ska vara40, L1/L2/L3 oförändrade. Cap40-kurvan är därmed beslutad och simuleringsgrinden PASS. Nedanstående ursprungliga STOPP beskriver den förkastade cap25-varianten; implementation/bevis läggs till separat.

# Slice19 — simulering: STOPP före implementation

**Grinden i orderns punkt1 slog till.** Befintligt tempellöst tak25 klipper den beslutade starten40
vid första tick. Med ingen frist och nya bandregeln börjar en Wanax med en stad utan tempel direkt
på25/Suspicious, med10% straffrollrisk per tick. Det uppfyller inte avsikten med start över straffbandet.
**Ingen spelkod ändrad.** Bas590d47ac, gren `codex/kharis-om`; endast denna analys/simulator och artefakter.

Faktiska produktionshelpers bekräftar problemet, inte bara en kopierad modell:
[cap-probe.log](cap-probe.log) kör `EffectiveKharisCap(100,0)` och `clampKharis(40−0,1,25)` på oförändrad bas.
Resultat25/25. [Recept](cap_probe.py) skapar en tillfällig DB-fri testharness och tar alltid bort den.

Rekommendation till Timothy via Claude: **godkänn tempellöst tak40** så start40 kan gälla, med
L1/L2/L3-taken50/75/100 oförändrade. Det är analyserat som ett uttryckligt **diagnostiskt alternativ**,
inte beslutat eller implementerat. Att behålla tak25 kräver i stället ett nytt samlat beslut om
start/band/risk. Jag ändrar inte de godkända talen på egen hand.

## Slice-kontrakt

| Del | Kontrakt |
|---|---|
| Problem | Frist/expansionskostnad/tempelimmunitet ersätts enligt Timothy13:00. |
| Invariant | Tempelvinst, offer, knapphet och L1–L3-tak ändras inte; befintliga värden lyfts inte; gamla event omtolkas inte. |
| Scope först | 300ticks,1/3/6/10städer×0/1/ettL1perstad, kurvor, bandpassager och straffrisk mot dagens regel. |
| Grind | Vid orimligt utfall: STOPP och rapport innan spelkod (orderns uttryckliga punkt1). |
| Därefter | Endast efter hanterad grind: implementering, färsk läsande livepåverkan, fyra ytor/regression/mutation. Lojalitet/notiser hör till senare slicer. |

## Metod och proveniens

Ren Go [simulate.go](simulate.go), ingen DB eller körande värld. Basmotorns tre lästa filers SHA256 finns i
[simulation.json](simulation.json); modellen kontrollerar att de använda gamla konstanterna och takformeln
fortfarande finns i basen. Första tick har ålder1: gamla `inFoundingGrace(age<144)` ger därför skydd vid1–143,
inte vid144. Golv1, recordcap100, tempellöst effektivt tak25 och L1-tak50 enligt dagens kod.

Scenarierna har fast stadsantal; första templet står i huvudstaden. Fullmatade L1-tempel från start,
15%devotion pertempel, offerfraction1, scarcity1. Gain är SUM(devotion)×14:2,1 pertempel ochtick,
inte medel över städer. Passiv geografisk kharis_rate satt0 i alla jämförda armar. Faktisk geografisk
rate kan förskjuta sena bandpassager men kan inte rädda40 från tak25 första tick.
Ingen byggtid, resursbrist, rit, välsignelse eller återkoppling från straff simuleras.

Fyra armar finns med alla301tickpunkter:

- `current_actual_start25`: dagens riktiga start25, frist och decay/immunitet.
- `current_controlled_start40`: samma gamla regel med40 i starten som kontrollerad jämförelse.
- `proposed_existing_caps`: den beställda nya regeln/start40 med befintliga tak.
- `diagnostic_only_no_temple_cap40`: endast tempellöst tak40 ändrat i analysen; **inte en byggd regel**.

`first_suspicious_tick` betyder första tick medvärde<30 (även direkt till Wrathful);
`first_wrathful_tick` första<5. `-1` betyder ingen sådan passage under300ticks.
Tärningsrisk beräknas analytiskt, inte med slumpade resultat. Summa p är förväntat antal positiva
straffrollar; 1−produkt(1−p) är risk för minst en, givet att det finns ett möjligt mål.
Det är **inte** antal garanterat utförda straff eller prognos med straffens följdskador.
Strikta floatjämförelser speglar kodens form; en exakt decimalgräns kan passeras en tick tidigare på grund av avrundning.

## Resultat

| Städer | Total ny decay | 0 tempel: Suspicious / Wrathful | 1 matatL1: netto / utfall | EttL1perstad: utfall |
|---|---:|---|---|---|
| 1 | 0,1 | 1 /201 | +2,0 →50 vidtick5 | 50 vid5 |
| 3 | 0,4 | 1 /52 | +1,7 →50 vid6 | 50 vid2 |
| 6 | 1,6 | 1 /14 | +0,5 →50 vid20 | 50 vid1 |
| 10 | 4,6 | 1 /6 | −2,5; Suspicious5, Wrathful15 | 50 vid1 |

En stad med matat tempel klättrar; sex städer med ett matat tempel klarar den uttryckliga kontrollen.
Problemet är den tempellösa starten. Vid en stad utan tempel är nya första värdet **25**, inte39,9.
Under16ticks utan tempel blir risk för minst en positiv straffroll **81,47%**, förväntat1,6rollträffar.
Ingen av dessa kurvor antar att ett tempel som tar16ticks att bygga redan står före tick16.
Huvudmatrisen antar däremot att de angivna templen står och är matade redan vidtick0.

### Risk över300ticks och jämförelse med dagens regel

| Städer | Tempel | Nu: Wrathful / förväntade rollträffar | Förslag: Wrathful / förväntade rollträffar |
|---|---|---|---|
| 1 | none | 155 /47.1 | 201 /50.0 |
| 1 | one | — /0.0 | — /0.0 |
| 1 | each_city | — /0.0 | — /0.0 |
| 3 | none | 155 /47.1 | 52 /79.8 |
| 3 | one | — /0.0 | — /0.0 |
| 3 | each_city | — /0.0 | — /0.0 |
| 6 | none | 151 /47.1 | 14 /87.4 |
| 6 | one | 208 /0.0 | — /0.0 |
| 6 | each_city | — /0.0 | — /0.0 |
| 10 | none | 146 /47.1 | 6 /89.0 |
| 10 | one | 153 /0.0 | 15 /86.8 |
| 10 | each_city | — /0.0 | — /0.0 |

Dagens enstaka tempel ger ingen straffrisk även när gamla rikeskostnaden drar kharis till Wrathful;
det är den tempelimmunitet ordern tar bort. Nolltempelfall har idag30% risk efterfristen,
nya regeln10% Suspicious/30% Wrathful från början. Med nya regeln och minst ett matat tempel perstad
hamnar alla dessa L1-scenarier på50; Favorable60 är fortsatt utanför L1:s tak.

Diagnostiskt tempellöst tak40 ger vid en stad första Suspicious kringtick100 och inget Wrathful under300ticks
(slutvärde10 vidpassivrate0), i stället för omedelbar risk. Vid6städer/1temple och alla L1-scenarier
förändras ingenting, eftersom L1-taket redan är50. Detta jämför bara beslutsalternativet, det är inte en ny kanon.

[Kurvor som SVG](curves.svg) · [PNG](curves.png) · [samtliga tickpunkter CSV](curves.csv) ·
[JSON med antaganden/filhashar](simulation.json) · [sammanfattad körlogg](simulation.log).

## Verifiering och avgränsning

Go-simulatorn har interna konsistensassertioner för capklippet,16tickrisk och6städers tempeluppgång.
Faktiska bashelpers är dessutom körda DB-fritt enligt ovan. Simuleringen kördes med ren HOME/PATH-miljö.
Kurvorna produceras med [plot.py](plot.py) och matplotlib i separat temporär venv; ingen global installation.

Ingen produktionskod, migration, API, CLI, webb eller Codex ändrad. Ingen merge/push/deploy/liveändring.
Ingen full DB-svit eller runtime-acceptans behövs för en analysgren som inte ändrar spelkod.
Inga live-Wanax räknade ännu: orderns steg3 och implementation/fyraytor väntar att stoppvillkoret är löst.
Faktisk livepåverkan ska därför inte utläsas ur denna slutna 300ticksmatris.

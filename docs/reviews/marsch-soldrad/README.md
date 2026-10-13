# Marschens soldrad

## Slice-kontrakt

Problem: marschens preview visar ankomst men inte hur solden ändras.
Spelarsanning: samma enda soldrad i webb och Keryx visar urvalets aktuella
silveravdrag, avdrag i fält och betalarstadens soldreserv i ticks (days på ytan).
Invariant: en preview är read-only, använder samma upkeep/cirkulation som tick,
och avslöjar aldrig någon annans enheter eller kassa.
Scope: server-preview, delad soldcirkulation, CLI/webb/Codex och prov.
Non-scope: nya soldnivåer, balansändringar, orderregler, migrations- eller auditkod.
Acceptans: faktisk tick jämförs, spjut 0→0, nollavdrag ger null,
färsk Go/JS/vet och Firefox 1280/390 innan integration.
Stopvillkor: kanonkonflikt parkeras i vault BESLUT; oberoende köarbete fortsätter.
Bevisplan: auth-HTTP/PG, tickdebitering, fysisk mutation byteåterställd,
två rena native körningar och healthz efter deploy.

Baslinje: master eb6d57c1, migration 178. Färsk
TestMarchPreview_ReadOnlyAndTiming grön före serverändringar. Klientändringar
fanns då redan i den egna worktreen, men påverkar inte HTTP-baslinjen.

## Implementationsval

Webburvalet kan omfatta flera betalande städer. Alla preview-GET får samma
unit_ids; enhets-ID och medföljande skeppslast dedupliceras. Svarens soldtal
avser hela urvalet, och webb visar en enda gemensam soldrad.
För flera kassor visas den kortaste runway (silver_payer_count märker texten),
utan att summera pengar som inte kan flyttas omedelbart mellan städer.
Runway använder stadens totala nettosold efter avmarsch, även ej valda enheter,
inklusive reparerande/fraktande skepp. Den är en soldreserv, inte en prognos
för framtida produktion/handel/rekrytering. Mine income subtraheras därför inte
från orderns uttryckliga soldavdrag. Saknad betalare för soldkrävande enhet ger
runway 0; gratis spjut och aktiv founder phase ger inget soldavdrag.
Redan marscherande enheter visar fortsatt fältsold medan ankomst förblir okänd.

## Grindar och reproduktion

Produktion: 67d79cb8 + 099a66b5 + 4354c387. Egen kärndiffgranskning klar:
auth/world före finansdata; faktisk supportstad utan kapitalfallback;
sex debiterade statusar och founder-exemption; read-only SQL; gamla event
orörda, inga balansvärden/migrationer ändrade.

| Grind | Bevis |
|---|---|
| Kod | go-final.log: hel färsk PG16/mig178 Go-svit grön (world 91,918 s); vet.log grön; js-full.log 616/616 utan skip |
| Funktion/semantik | TestMarchSilverForecastMatchesActualTick: elit .6→2, automatiskt cargo/dedup, faktisk tick 6,25/2, lazy 110, kvarvarande repair/freight, flera städer min6, spjut 0→0/null och spjut med betalande syskon, saknad betalare0, founder0; auth-HTTP read-only och otillåtet urval |
| Mutation | mutate.py: field-circulates och cargo-missing båda fysisk named RED → identiska återställda bytes → färsk GREEN; fyra loggar |
| BILD/användare | live.py: två rena natural register/join/coastal found, verklig galär .45→1.5 och kassa1278/1.5=852; Keryx + Firefox1280/390 samma enda soldrad; ingen SQL-fixtur eller HTTP/CSS-override; units/goods identiska före/efter preview |
| Visuell granskning | Fyra slutbilder live/ och live-repeat/ granskade 1:1 av Codex enligt rolländringen. Text13px/ordinarie textfärg, inga overflow eller JS-fel, March-knappen nåbar. En rad i betydelsen ett finansiellt meddelande, omslag på smal panel. |

Repro: `python3 docs/reviews/marsch-soldrad/mutate.py`,
`python3 docs/reviews/marsch-soldrad/live.py` (andra körning
`MARCH_PROOF_OUT=live-repeat`), `tools/gotest.sh`, `go -C server vet ./...`,
`node --test $(rg --files web | rg '\.test\.mjs$')`.
Native bygger endast committad produktion och sparar faktisk healthz-provenance.

Även misslyckade försök redovisas: go-full.log/JS fann förbjuden /day-form,
nu silver per day enligt oförändrad Day guard. Första tickfixturen saknade
skeppsproviant och svalt före sold; rättad till provisionerade skepp.
Första nativeriggen letade efter galär före grundningen; Poseidons gåva
kommer vid kustgrundning. Riggen väljer därför verklig synlig kuststart.
Before-contrast-bilderna visar funktionell första rad före färgförbättringen.
Ingen sådan riggbrist har normaliserats som skip.

## Kända avgränsningar

Runway är kontinuerlig soldreserv, inte en garanti: dagens nettoavdrag
förutsätter mat och full payroll liquidity; tick kräver bruttosold före
återcirkulation. Framtida ändringar av styrkor/kassor eller inkomster är
inte bokade av preview. Nytt språk silver per day följer befintlig språkgrind.
Ingen kanonfråga blockerar denna slice. Integration/deploy verifieras separat.


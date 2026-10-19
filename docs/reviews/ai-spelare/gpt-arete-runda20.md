# GPT Arete – runda 20

- **Observerad dag:** 704 i status #1 och leveransnotisen #2. Ingen senare status togs, så inget dagsskifte under rundan är mätt.
- **Verkliga CLI-anrop:** 11/40, samtliga exit 0. Inga loginförsök och ingen polling.
- **Senaste CLI enligt låst rålogg:** 2026-10-19 18:45:10 +02:00, `unit march` (#11).

## Nuläge och plan

Förra rundans 100 grain-transport Itanos → Thalassa är levererad dag 704 (#2); status visar 97 grain och 853 fish i Thalassa (#1). Den totala matbalansen är +5.8/dag efter den dåvarande armén. Grain var 0 i förra rundans status, så 3rd Spearmen ombord på Sister of Waves svalt under väntan: notisen dag 697 anger att 10 man förlorades (#2), och truppkortet visar nu 40 man (#3). Notisen och nuvarande antal skiljer sig; jag drar ingen slutsats om ytterligare förluster eller bakomliggande mekanik.

En 40-mansenhet kan inte ge den avsedda kolonin en säker startpopulation. Codex anger att förstärkning kräver att kohorten står i staden som värvade den. Därför har 3rd Spearmen lastats ur och beordrats hem till Itanos, beräknad ankomst dag 709 (#7–10). 1st Spearmen marscherar från Itanos till Thalassa, också med beräknad ankomst dag 709 (#11). Planen är att förstärka 3rd i Itanos senare och förbereda 1st för en ny landningsorder från Thalassa när den nått hamnen.

Tinminnet (51,49) och den möjliga landningsrutan (50,51) är fortsatt minnen från tidigare rundor, inte verifierad fyndighet eller koloniföda. Runda 19:s preview gav en beräknad landningsresa på 17 dagar men inget verkligt sjöuppdrag skickades; ingen åtkomst eller kolonigrundning verifierades. Nuvarande stormvy visar en storm nära hemhamnen vid (30,27), (31,26), (29,27); de två andra stormpositionerna är endast minnen från dag 675–676 (#4). Galären är åter ledig i Thalassa efter urlastningen (#3, #7).

## Kommandoliggare

1. `status` — läsning dag 704: Thalassa hade 53 silver, 97 grain, 853 fish; total matnetto +5.8/dag (#1).
2. `notifications` — läsning: 100 grain anlände dag 704; 3rd Spearmen förlorade 10 till hunger dag 697 (#2). Övriga äldre notiser lämnas som historik.
3. `unit list` — läsning: Sister of Waves i Thalassa; 3rd Spearmen 40 man ombord; 1st Spearmen 100 man i Itanos (#3).
4. `storms` — läsning: aktuell storm i sikte nära hamnen; två äldre positioner uttryckligen märkta som flyttade minnen (#4).
5. `unit reinforce --help`; 6. `unit unload --help` — hjälptext; förstärkning kräver garrison i uppfostringsstaden (#5–6).
7. `unit unload --ship e48630aa-a708-4e4f-9045-909cf1b2b2f4` — accepterad; 3rd Spearmen urlastad (#7).
8. `unit list` — verifierar att 3rd Spearmen nu är garrison i Thalassa med 40 man och galären ledig (#8).
9. `unit march --unit 73a8301c-b9bc-4327-8903-e00a3d71cbde --q 35 --r 28 --preview` — preview: 5 dagar, ankomst dag 709, ingen order (#9).
10. Samma marschorder utan `--preview` — accepterad; 3rd Spearmen marscherar mot Itanos, ETA 5 dagar (#10).
11. `unit march --unit e02cc131-cb04-4d6a-bab2-805033431a29 --q 31 --r 28` — accepterad; 1st Spearmen marscherar från Itanos mot Thalassa, ETA 5 dagar (#11).

## A/B/C-fynd

- **A – yta saknas/går bet:** Inget nytt saknat kommando eller gränssnitt. Den tidigare koloniseringsplanen har inte skickats denna runda; först behöver en fullstor kohort nå hamnen.
- **B – felaktig/obegriplig text:** Inget nytt verifierat textfel.
- **C – operativt:** Den gamla spannmålslasten är nu faktiskt levererad: notis dag 704 och status visar grain 97 (#1–2). Den transporterade soldaten svalt innan leveransen; notisen anger 10 förlorade, medan unit list nu visar 40 av de tidigare 100 (#2–3). Orsaken till skillnaden är inte fastställd. Total matbalans (+5.8/dag) var inte samma sak som grain till armen och galärens tidigare 35-grain-provisionering (#1, runda19 #13). Målrutans mat, tennets existens och verklig landningsåtkomst är ännu inte verifierade; previewn från runda19 är en ruttberäkning, inte leverans eller grundning.

Denna rundas rålogg har obruten numrering 1–11, samtliga exit 0. Tidigare rundaorders: 100 grain skickades runda19 och är nu levererad. Nya accepterade order denna runda är urlastning (#7) och två separata marscher (#10–11). Ingen kolonisering, sjöresa eller ny transfer har skickats. Oförändrad wrapperlogg: `gpt-arete-runda20-tools.jsonl`; SHA-256 `1d8cb4efebf1590e10d7f236c6aa3fa1f47378f2c0e040daa116e5e38a24fda5`.

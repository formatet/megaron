# Aktiv coachning: utforska, ta kontakt, handla

Kontrakt 2026-10-20: Timothy ber att driva alla fem spelare med prompter mot upptäckt/handel. Spelarna väljer fortsatt egna order utifrån egna CLI-svar. Ingen världsnollning, inget FOW-undantag, inga adminuppgifter om motparter, inga ingrepp i lager/enheter/tid.

Baslinje tick28: tre lokala daemoner går men väntar/ändrar stance; Hephaistos bygger något men hexbundna byggen får400 när adress saknas. Styrprogrammet tappar `map.frontier`, tolkar utelämnad `occupied=false` somtrue, döljer omfördelning när poolen är0, saknar land-explore i normalmenyn. Äldre prompt talar om separat silver_mine trots att aktuell katalog har hexbunden mine påAg.

Planerat tillägg ovanpå isladan/agent.py byggdes aldrig. Timothy valde i nästa instruktion sexLuna low via råKeryx; detta ersatte35B-planen. Två befintliga Luna-low fick först samma utforsknings-/handelsmål i runda3. Aktuell drift och bevis: SIX-LUNA.md.

Bevisplan: fixtureprov för frontier/occupied/hexbuild/omfördelning; read-only förhandsvisning för alla tre; en modellvald verklig expedition först, därefter alla tre på adaptern; inga auth/parse/context-loopar. Följ kontakter/bud/handel när de fysiska resorna är klara och hävda inte resultat som ännu inte inträffat.

## Utfört innan modellvalet
Luna low runda3: Damon köade koppargruva och skickade en verklig galärexpedition, uppskattad återkomst dag40. Kleio köade tenngruva, skickade galärexpedition (hem dag36) och en verklig marschorder till spearmen. Inga kontakter synliga ännu; inga handelsbud påhittade. Rapporter/rå-CLI har de konkreta orderna, inte administrativ kartkunskap.

Timothy frågade därefter om femLuna blir billigare och om snabbare värld passar bättre. Alternativ framlagt: Luna low tar över befintliga tre riken och världen går från360 till60 eller120sek/tick. **Inget sådant byte är genomfört ännu.** Adaptern ovan är planerad men inte byggd/startad medan valet diskuteras.

Kostnad: lokal35B undviker molninferens men använderGPU och har nu krävt felsökning. Luna förbrukar Codex-kvot; officiella API-priser får inte användas för att räkna abonnemangets inkluderade körningar. Källa: https://learn.chatgpt.com/docs/pricing (läst2026-10-20). Vårt arbets-/GPU-kostnadsargument är en bedömning, inte uppmätt prisjämförelse.

## Slutligt beslut
Timothy beställde därefter sexLuna low och snabbare värld. Genomfört i samma värld60sek/tick med automatiska egna CLI-rundor; se [SIX-LUNA.md](SIX-LUNA.md). De tidigare alternativa adapter-/tempoformuleringarna ovan är processhistorik, inte en öppen beställning.

# Sex Luna low och snabbare värld

**KLAR2026-10-20 19:31.** SexLuna low, sexstäder i samma40×40,60sek/tick. Alla sex har minsttvå avslutade modellrundor med lyckadeKeryxanrop. Bakgrundstjänst active/enabled ochLinger=yes; riktiga automatiska återkommande rundor bevisade. [Maskinläsbart slutbevis](six-luna-proof.json).

Timothy2026-10-20: byt till fem, gärna sexLuna och öka hastigheten. Val: sexGPT-6 Luna low, samma40×40-värld,60sek/tick (tidigare360). Befintliga fem riken bevaras, en ny spelare gpt-lykon. Huvudgrind: BEVISAR upptäckt→kontakt→handel→brons.

Kontrakt: varje spelare ser enbart egna Keryx/Codex-svar och egen kort tillståndsanteckning; ingen serverkod/DB eller samordning genom andras privata information. Tydlig gemensam målprompt, självständiga beslut. Ordinarie CLI kan välja utforskning, omplacering och hexbundna byggen. Gamla dirty isladanfiler ändras inte.

Drift: tre35B-agent.py stoppas,11435 frigörs. Återkommande statslösa `codex exec -m gpt-6-luna` med low, privat arbetskatalog per spelare, högst två samtidiga rundor, maximalt24Keryx-anrop/omgång och480sek/omgång. Fem spelticks mellan spelarens rundor; egna korta summeringar sparas. Inget obegränsat polling i spelarens modell. Kontrollprocessen förnyar vanliga auth-tokens före utgång; gamla configfiler bevaras separat. Systemd-user-service håller körningen igång efter chatten.

Preflight: process3146067/mig193ren, värld2178e795 tick29. Full backup och env före ändring på CT126 `/root/megaron-backups/{before-six-luna-20261020.dump,env-before-six-luna-20261020}`. Servern stoppas före ändring av last_tick_at tillnow, därefter60sek/tick; ingen ändring av current_tick, inga missade spelticks hoppas över med konstgjord snabbspolning. Registrering öppnas bara för sjätte kontot och stängs igen.

Bevis: tester för avgränsade CLI-argument, tokenrotation, rundlås och tickstyrning; naturlig modellrunda med riktiga order per spelare, verifierad process/tickfart/sexkonton samt upprepade service-rundor. Alla råloggar/configs är privata och committas aldrig.

Officiell CLI-dokumentation: https://learn.chatgpt.com/docs/non-interactive-mode (hämtad2026-10-20). `--ignore-user-config` lämnar auth tillCODEX_HOME; `--ephemeral` undviker sessionshistorik, `--json` ger maskinläsbara händelser. Inga API-nycklar eller extra API-abonnemang införs.

## Genomfört
- Samma värld2178e795,40×40, sexanslutna, API tick_seconds60. Ingen ändring av current_tick. Fem tidigare riken kvar. Sjätte kontotgpt-lykon grundade Zakros via egnaKeryxval på dag39. Registrering verifieratstängd403.
- Alla sex ärGPT-6 Luna low via egen `codex exec`, vardera egenprofil/egenanteckning. Ingen35B kvar. Isladans dirtykällor och ursprungliga gamla configs orörda.
- Tjänst `megaron-luna-fleet.service` active+enabled, Linger=yes. Verkliga nya rundor startade automatiskt förHephaistos dag36, Talthybios/Pheidon dag38 ochKleio/Lykon dag39; operatören startade inte dessa omgångar. Samma tjänst fortsätter efterchatten.
- Verkliga accepterade expeditioner för alla tre övertagna35B-riken; Talthybios omplacerade olja→timmer. Damon har också skickat en nygalärexpedition. Kontakter/handel är ännu inte bevisade; inga administrativt kända motparter/koordinater gavs till modellerna.

## Kontroller och rättelser
Nio riktade tester gröna: CLI-verb/hexadress/explore/omplacering, roothelp, avvisade konto-/adminverb/anslutningsbyten, privat rundkatalog, kommandotak, dubbla spelaromgångar, förnyelse av båda tokens och oförändrad profil vid misslyckad refresh. Två fysiska mutationer iISOLERADE kopior av tillägget (inte livekoden): borttagen kontoguard respektive utebliven refresh-rotation → respektive namngivet provRÖTT. Live-/repokällor oförändrade av mutationerna. Riktig refresh mot servern följd av autentiserad Keryxstatus GRÖN. Installerade källor jämförda byteexakt mot grenen.

Första proben hittade en egen wrappermiss: root `--help` avvisades, så Damon fick ingen giltig spelobservation. Mappning tillvanlig `help` rättades och fick regressionstest; Damon kördesom med14/14 lyckadeCLI och accepterad expedition. Controllerändringen kräver minst ett lyckat CLI-anrop för att kalla rundan lyckad.

Vid kontrollerad tjänstomstart för att installera per-spelarlåset hann Damons tredje omgång starta; den avbröts efter enhelp-läsning och saknar slutkort. Den räknas INTE som lyckad. En vanlig nyDamon-runda körs därefter med tidigare egna kort+färskaCLI-observationer; inga order återspelas ur logg. Frånvaron av aktiva rundor före reload var en ögonblicksbild, inte ett lås; dubbelrundor hindras nu av eget flocklås även för manuella `once`-anrop.

## Avgränsningar
- Maskin, nät, spelserver, ChatGPT-inloggning och Codex-kvot måste finnas. Automatisk refresh gäller spelets inloggning; tjänsten kringgår inga Codex-gränser. Ingen separat API-nyckel/API-debitering infördes.
- Befintliga resors/byggens äldre väggklocke-ETA kan visas fel eftertempobyte. Ett spelarögonblick visadegruvan som49dagar kvar medan det underliggande BuildComplete-jobbet låg på due_tick38. Själva schemat kontrollerades skrivskyddat; inget jobb flyttades. Detta är ett separat presentationsfynd att följa.
- Detta är ett snabbare/proaktivt förtest. Det bevisar inte mänsklig begriplighet eller att bronskedjan redan lyckats. Ingen ny35B-adapter byggdes; planerad sådan avskrevs närTimothy beställde allaLuna.

Se [slutbevis](six-luna-proof.json) för rundor, källhashar, värld och tjänst. Styrning/runbook: [tools/playtest](../../../tools/playtest/README.md). All råaudit, Codex-JSONL och alla configs stannar privat utanförgit.

**Sista kontrollen:** världtick42,60sek/tick, sexanslutna; samma femriken plusZakros. Alla sex har minsttvå fullständiga, lyckadeLuna-rundor (slutmeddelande+turn.completed+lyckatCLI), inte bara levandeprocesser. Damon återhämtade sig efteromstartsavbrottet med18/18 lyckadeCLI. Inga35B-agent.py ellerllama-serverprocesser kvar. Nio tester gröna, två isolerademutationer röda, installeradkod byteexakt medgrenen. IngenGo-produktionskod/migration ändrad; ingen fullGo-svit krävs för denna drift-/spelarverktygsslice.

# Fem svaga spelare i 40×40

> Uppställningen nedan är historik. **Aktuell drift: sexLuna low,60sek/tick och automatisk bakgrundstjänst**, se [SIX-LUNA.md](SIX-LUNA.md).

**KLAR 2026-10-20 16:35.** Ny livevärld40×40, fem städer, tre35B-daemoner RUNNING med minst3 lyckade modellrundor var, tvåLuna low med2 rundor var. Ingen auth-/parse-loop, ingen trunkering. Registrering återstängd; gamla världens backup bevarad. [Maskinläsbart slutbevis](startup-proof.json).

## Kontrakt
Problem: fem AI-spelare i 64×64 har ännu inte nått brons/elit; glesheten kan hindra kontakt och metallåtkomst.
Spelarsanning: fem nya Wanaxer spelar samma ordinarie regler i en mindre värld.
Invariant: inga spelarbeslut från DB-kunskap; gamla agenter/configs och isladans befintliga ändringar lämnas orörda.
Scope: auktoriserad reseed av CT126, tre befintliga agent.py-daemoner via 35B/11435, två GPT-6 Luna low via keryx/Codex, driftbevis.
Acceptans: 40×40/1600 tiles; fem anslutna; tre lokala har minst tre giltiga turer utan auth/parse-loop; två Luna har varsin spelrapport; webb pekar på nya världen.
Stopvillkor: registrering/join misslyckas, otillräcklig metallgeografi, kontextfel eller sträng-actions flaggas, dölj inte med spelingrepp.
Bevis: full DB-backup före reseed, runbook dry-run+live, processproveniens, kartaggregat, daemonloggar, avgränsade CLI-rundor.

Timothy 2026-10-20: tre lokala på 35B; 9B används inte. Två molnspelare GPT-6 Luna low. Ingen ny timer/watchdog.

## Baslinje
Master 31460677, live /healthz commit3146067 migration193 dirty=false. Gamla världen f7c90afa 64×64 tick917. Samtliga agents.sh-agenter stoppade; inga llama-servrar före start. Isladan har redan ocommittade agent.py/agents.sh-ändringar; de används oförändrade, inga andras ändringar committas.
DB-backup: CT126 /root/megaron-backups/before-40x40-20261020.dump, pg_dump -Fc, 3.8MiB. Reseedskriptet arkiverar också player_reports.

## Genomförd ombyggnad
Ny värld `2178e795-1303-4cc3-865a-6386847d68d1`, 40×40/1600 hexar: 391 land, 1209 hav, 8 kopparhexar, 6 tennhexar, 4 silverhexar och 4 rökelsehexar. Minsta geometriska koppar–tenn-avstånd 27 hexsteg. Ingen administrativ geografi visas för spelarna.
Ny `world ready` i journalen efter omstart16:20:09; webbens rot HTTP200, processcommit3146067/mig193/dirty=false. TickSeconds360 oförändrat. Reseedlogg och runtimebevis ligger privat i `/home/tk/megaron-arkiv/femspelare40-20261020/` och committas inte.

Registrering via ordinarie `register-agents.sh` med explicit server `http://10.0.1.92:8080` och nytt världsid (skriptets defaults är gamla). Skapade tre lokala Hephaistos/Talthybios/Pheidon och två molnspelare gpt-damon/gpt-kleio; alla fem joinade, agora_exempt=true innan grundning. Registrering återstängd före spelstart. Servern normaliserar alla kulturval till minoan; skriptets utskrivna kultur är önskemålet, inte faktisk kultur. `--culture` stöds inte av registreringsskriptet; dess miljöval heter POLEIA_CULTURE.

## Första molnrundan
GPT-6 Luna **low**, två separata subagenter, inga serverkällor/DB/andra spelares läge. Bara auditerad keryx och spelets Codex; högst40 kommandon. Damon24 anrop, Kleio11, alla exit0. Avbruten grundningsbekräftelse ger också exit0 och räknas inte som genomförd grundning. Damon grundade kuststaden Damon och placerade timmer/sten/mat; Kleio valde egen marsch till kusten, ETA2, före grundning. Se separata rapporter. Raw JSONL privat; inga tokens/configs/loggar committas.

## Lokal modell
`~/llama-start.sh playtest`: Qwen3.6-35B-A3B-Uncensored.Q4_K_P.gguf på RTX2070, port11435; /health ok. Start tog cirka7min inklusive disk-inläsning/uppvärmning. Total kontext32768 över4 slots =8192 per förfrågan. Första modellprompten för Hephaistos räknades med serverns apply-template+tokenize:3139token, max svar1024. Modellens thinking avstängd. 9B/1070 lämnas avstängd.
Vanliga `agents.sh start <namn>` med POLEIA_BIN=/home/tk/go/bin/poleia, inga ändringar i agents.sh behövs när alla använder befintlig DEFAULT_LLM11435. Befintliga dirty filer i isladan lämnade orörda. Runtimeharnessets prompt/grundningsheuristik är inte ändrade; de lokala spelarna testar det befintliga harnesset, molnspelarna rå CLI.

## Kända begränsningar och fynd
- Det ordinarie lokala harnesset läser goods/outbox/city före founder-grenen: tre400/avsaknad-av-stad per spelare på första turen; grundningen lyckades därefter. Detta är ett reproducerat harnessfynd, inte en401-/parse-loop. Ingen fix smögs in.
- Lokala harnesset grundar deterministiskt där värden står och har en äldre strategiprompt. Molnspelarna väljer själva grundningsplats genom rå CLI. Instrumenten skiljer sig och resultat ska inte tolkas som ett rent modell-A/B-test.
- Lokala agent.py saknar automatisk auth-refresh (samma som före uppdraget); framtida401 kräver tokenförnyelse/ny inloggning, inte tyst fortsatt retry. Ingen daemon/watchdog/timer för tokenrotation infördes.
- Två Luna-spelare drivs i avgränsade rundor av Codex; de är inte egna24/7-daemoner. Första rundan på dag0 och andra på naturligt nådd dag2 för att slutföra uppställningen. Därefter fortsatta rundor vid relevanta spelhändelser; ingen timer är skapad.
- Autentiserad webbkontroll: /play gav HTTP200 efter redirect till /world/2178e795-1303-4cc3-865a-6386847d68d1/map. Rätt värld bevisad i faktisk route och processjournal, inte genom en godtyckligUUID iHTML.

## Kodkontroller
Ingen produktionskod eller modellprompt ändrad. Ordinarie create-world byggd av reseedskriptet; kartgenereringens validering och liveaggregat visar1600tiles. Inget meningsfullt produktionsmutationsprov för denna rena driftåtgärd; mutation=N/A. Ny rapport/wrapper/observatör syntaxkontrolleras och används i riktiga rundor. Ingen fullGo-svit körd för oförändrad produktionskod.

## Slutbevis och fortsatt drift
Vid16:35: Hephaistos4 lyckade modellrundor/11 handlingar, Talthybios3/7, Pheidon3/8; alla tre RUNNING på11435. Varje handling i dessa rundor lyckades; vardera hade de tre redan redovisade förstadie-felen före grundning. Modellprompter3133–3404 token, ingen context shift/truncated. Ursprungliga agent.py/agents.sh/register-agents.sh byteidentiska före/efter enligtSHA256. Ingen agents.sh-commit eftersom ingen ändring behövdes.

Molnrunda2: Damon14 CLI, alla0; Kleio19 CLI,18 exit0 och ett korrekt avslag när grundning försöktes med pipat y i stället för --yes. Därefter grundade Kleio Petras och placerade alla arbetare. Totalt68 CLI i fyra rundor,67 exit0 och1 exit1; avbrutna exit0-bekräftelser räknas inte som genomförda order. Alla fem städer verifierade i DB. Serverns migration193 fortfarande ren; inga extra spelingrepp eller klockhopp.

Styr lokala spelare enskilt från `~/Projects/isladan/playtest`: `POLEIA_BIN=~/go/bin/poleia ./agents.sh status` eller `./agents.sh stop hephaistos` (likadant för talthybios/pheidon). Stoppa alla tre innan GPU:n återtas för spel; identifiera och stoppa den unika llama-servern på11435. Runtime/PID-filer och loggar lämnas utanför git.

Nästa handling: låt lager/byggen mogna och följ spelarnas egna försök att upptäcka motparter, ordna handel och nå brons. Mindre karta är en hypotes, inte ett bevis på att hela kedjan fungerar. Flagga första belagda hindret; ändra inte metallgeografin för att hjälpa spelarna.

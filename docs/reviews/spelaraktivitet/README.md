# Senaste spelaraktivitet och Pompejus tickfråga

Kontrakt före kod2026-10-26, beställt direkt av Timothy före bud/minneskön.
Problem: en förnyad session säger inte om ett konto gör autentiserade anrop; Pompejus bud sent_tick380 har jämförts med världens senare1458 utan kontrollerad tidslinje.
Spelarsanning: drift kan se senast observerad autentiserad kontakt, utan att kräva ny inloggning. Det är kontakt med servern, inte bevis på mänsklig uppmärksamhet (öppen flik pollar).
Invariant: inga offentliga exakta närvarotider; ogiltigt/anonkonto registrerar ingen aktivitet; lyckad auth får inte bli beroende av telemetri; högst en faktisk players-uppdatering per fem minuter, även samtidiga anrop.
Scope: nullable players.last_seen_at, authmiddleware för API/web/optional, driftläsverktyg+DB/integrationprov; read-only Pompejus-/världsdiagnostik.
Non-scope: ny publik status eller onlineindikator, AIövertagande, spelbalans, budets fysiskförföljning/synminne. Inga historiska besök hittas på; gamla konton förblir NULL tills observerade.
Acceptans: giltigt bearer/cookie/API/web/optional registreras; invalid/anon skriver inget; inom fem minuter ingen UPDATE; efter gränsen ny UPDATE; auth fungerar vid telemetrifel; Pompejus kopplas till rätt värld/tidslinje.
Stopvillkor: nytt informations-/mekanikbeslut eller belägg för annan budbugg utanför scope tas till Timothy.
Bevis: äkta rött→grönt DBprov, samtidig spärr/mutant, migration216upp/ned/upp, fullGo/vet/JS, faktisk HTTP/Keryx isolerat samt livehash/grundtillstånd.
Gate: proves driftens möjlighet att skilja saknad aktivitet från en bestående session; ingen ny spelmekanik.

## Pompejus: löst utan kodändring

`pompejus-baseline.json` kopplar samtliga sex bud till samma nu aktiva värld. De senaste två skickades2026-10-24T22:47:20Z =25okt00:47:20svensksommar, sent_tick380. Driftloggen (`pompejus-tick-journal.log`) visar tick380 vid00:46:44 och381 vid00:47:44. Alltså korrekt380 vid sändning.1458var ett senare observationsläge, inte budets sändtid. Ingen tick-/budfix eller datakorrektion behövs. Befintlig sent_tick-default sparar sändningens tick; den ska aldrig följa världens senare tick.

## Aktivitetsstämpelns avgränsning

NULL betyder ännu inte observerad sedan införandet; historiska login-/budtider kopieras inte in. Öppenflikspollning räknas som serverkontakt. Cachen begränsar observationsförsök perprocess till femminuter; SQLvillkoret begränsar faktiska rowupdates även mellan processer. Mätningen är grov: om en ny process träffar nära slutet på en tidigare observationsperiod kan nästa observation dröja ytterligare en cacheperiod. Ingen offentlig online-/närvaroyta tillkommer.

## Resultat och grindar

Implementerat i `0d5bc55f`: nullable stämpel, alla tre authmiddleware, perprocesscache och villkoradUPDATE, ettsekundsgräns och retry efter telemetrifel. Stämpeln används bara av driftläsverktyget. [Täckning och nio egna mutanter](coverage-and-mutations.md) anger exakt vad nya prov täcker/inte täcker och exakta GNU sed-rader.

Riktig isolerad process: [health och HTTP/cookie/nativeKeryx/schema](acceptance/proof.json). Ren216 och216→215→216prov bevarar konton; rollback släpper avsiktligt den nya observationskolumnen. Inga visuella ändringar, därför inget BILDbehov. Hel färskGo26paket är grön (world145.305s), auth-race grön,794JS/vet/fyradayguards gröna. Råloggar finns bredvid. Inga livekonton eller bud skapades för acceptansen.

Första baslinjekörningen saknade kolumnen och räknas bara som schemaunderlag. Den riktiga röda baslinjen kördes med216kolumnen men utan middlewareimplementation (`auth-baseline-red.log`), där faktiska assertions nekar utebliven aktivitet på API/web/optional och0uppdateringar vid samtidighet. Mutationernas första portkollision är ett riggfel, inte bevis; den ersattes av komplett renkörning med explicita lediga portar.

Drift: LIVE `c2ef4b9d`, processhash korrekt/migration216ren, samma värld15spelare/5städer/18enheter/85varurader och inga nya felloggar. [Deploybevis](deploy-proof.json) jämför ground-before.json. Read-only operatörsaudit verifierade att ett verkligt konto redan fått stämpel; Hajen/Pompejus ännuNULL vid kontrollen. Den privata rålistan ligger utanför git, inga exakta aktuella närvarotider publiceras. Ingen spelmekanik ändrades för Pompejus eftersom tidslinjen bevisade korrekt sändtick.

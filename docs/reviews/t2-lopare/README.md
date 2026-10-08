# T2 — löparens öde till sjöss

Kontrakt före kod, bas daf740ddd0efa051adc78c945b92d3c0c5d94d48, codex/t2-lopare.
Problem: stormförlist transport lämnar sin löpare aboard; sänkt/kapad bärare förseglar felaktigt löparen.
Spelarsanning: storm/ingen räddare innebär förlust med omedelbar fullständig dispatch till avsändaren; strid/kapning räddar löparen till ett verkligt skepp, som den lämnar i nästa hamn och fortsätter från.
Invariant: inga andra nyheter färdas omedelbart; fienden kan inte läsa eller hålla löparen; inga dubbelutfall eller gammal terminaltimer som levererar efter förlust/räddning.
Scope: carrier-death/capture TX och nytt hållbart vittnesmål, messenger-projektion/landstigning, events/migration/main, webbdispatch/Keryx/Codex och prov/rigg. Non-scope: T1-risk/tärningar/andra stridsutfall, gudar/riter, Umami/U.
Acceptans: tre röda verkliga livscykelprov → grönt; full fresh Go/vet/JS; fyra begärda mutationer; riktig privat payload och webb/Keryx; Firefox/Chromium/WebKit desktop/mobil om webben ändras.
Stopvillkor: kanonlucka tas till Claude. Arkitektur: utfall en gång där skeppet dör/kapas, i samma TX, uppåt som ny händelse enligt G1; ingen direkt messenger-SQL från combat/transport.
Kedjegrind: bevisar fysiska order-/transportkontrakt; bud över havet ska inte fastna på en obefintlig återkomst.

Implementation klar inför slutgrind. Utökad kanon från Claude 23:05: upplösning av svält/desertion skriver också utfall i döds-TX; vid/intill aktiv bosättning går löparen i land, till sjöss förloras den. Gäller egna och redan räddade löpare.

`internal/carrier` äger fysiska vittnesmål och har bara `events` som intern kant. G1-listan och arkitekturprovet innehåller paketet och dess konsumenter i samma slice. Migration 162 är additiv: tillämpat vittnesmåls-id och partiellt eventindex. Gamla eventtyper ändras inte.

Fyra nya privata typer på messenger-streamen: `CarrierPassengerLostV1`, `CarrierPassengerRescuedV1`, `CarrierPassengerLandedV1`, `CarrierPassengerRedirectedV1`. Storm, strid, transportkapning, upplösning och faktisk hamnankomst skriver i sin egen TX; messenger äger projektion/notis. Pending-vittnesmål följs även före projektion, så en räddare kan hinna dö eller nå hamn mellan scans. Gammal terminaltimer spärras, generation uppdateras och tillämpat id ger en enda arkiverad dispatch vid dubbelscan. Landstigning finns kvar när skeppet redan vänt och lämnat hamnen.

Förlust ger `MessengerLostAtSea` på första passage-scan, med fryst HELT kuvert (brev, handelsvillkor eller order), avsändarens Wanax-id, avsändningstid och ursprungligt från/till. Privata skepps-/räddar-id och position publiceras inte i undantagsnotisen. Räddning ger ingen fjärrnotis vid strid eller landstigning: API/utkorg visar `no word`, kartan och eyes döljer löparen, även väntnotiser spärras. `MessengerRescuedAtSea` arkiveras först vid fysisk hemkomst med svar; envägsorder ger ingen magisk hemrapport. Limped är omdirigering på samma levande skepp, inte räddning/förlust.

Webbdispatch, Keryx och Codex visar hela kuvertet/hemrapporten. De fyra JSON-fixturerna kommer från verkliga arkiverade DB-notiser i livscykelproven, inte handbyggda förväntningar.

Bevis före slutgrind:

- `baseline.log`, `red-transport-storm.log`, `red-three.log`: grön bas och de tre ursprungliga faktiska röda livscyklerna.
- `red-upkeep.log`: hamn/hav röda; `red-boarding-stall.log`: död bärare och fjärrnyheter röda före sina spärrar.
- `green-final-focus.log` och `mutations/restored-*.log`: berörda livscykler och 20 upkeep-fall för egna/räddade löpare gröna.
- `mutations.log`: åtta riktiga mutationer, en i taget med färsk DB. Kuvert borttaget, landstigning borttagen, dubbelscan duplicerad notis, storm som räddning, carrier läckt till sender, hamn som förlust, hav som hemhamn, gammal terminaltimer tillåten. Var och en ger namngivet rött prov; alla källor återställda och gröna.
- Browserriggen är uttryckligen renderingsbevis med verkliga arkiverade DB-payloads och produktionsmoduler/CSS. Preferences-GET är en märkt read-only HTTP-fixtur. Go-proven bevisar den verkliga DB-livscykeln; riggen påstår inte en livebackend eller iOS-paritet. Firefox först, sedan Chromium/WebKit, desktop och 390×844. Bilder och maskinresultat i `browser/`.

Ingen merge, push eller deploy. Äldre rader utan nya vittnesmål har kvar det avgränsade legacy-skyddsnätet; ny fysisk död/kapning är kontrakterad via vittnesmål.

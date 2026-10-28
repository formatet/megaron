# Proviant från faktisk avgångshamn

Kontrakt före kod, Timothy2026-10-28: alla båtar tar avgångens mat från hamnen där de ligger. Detta ersätter25okt-beslutets proviantdel; stöd-/underhållsankare ändras inte.

Problem: en vanlig seglingsorder efter dockning i en annan egen stad drar mat ur den gamla försörjningsstaden. Frakt använder redan fysisk hamn.

Spelarsanning: fyll lagren där båten ligger; ett välfyllt avlägset hem kan inte betala nästa avgångs mat. Befintlig ombordmat och ransonregler behålls.

Scope: combat-avgångens proviantkälla, befintlig ekonomiägare, regressioner, fyra ytors text/semantik och verkligt API/Keryx/Firefox-bevis. Ingen migration, ändrat upkeep, ändrad besättning eller omskrivning av redan frysta resor/event.

Acceptans: faktisk hamn minskar, gammal support oförändrad; lokal matbrist avvisar atomiskt trots välfyllt hem; dockat skepp utan support kan proviantera lokalt; ingen påfyllning till sjöss; befintlig frakt/gästfrihet/ranson oförändrad.

Bevisplan: assertion-röd baslinje på färsk218, riktiga DB-prov, fysisk kärnmutant med exakt återställning, full färsk Go/vet/JS/tidsvakter, native flytt till egen stad och nästa avgång med DB-lagerdelta, ny process/ren218/bevarat grundläge och assets efter deploy. Bevisar kedjegrinden genom fysisk läsbar försörjning.

## Verifierat beteende

LIVE `d09a74af`: rätt ny process, ren218, samma värld/grundantal och två byte-exakta ändrade statiska filer verifierade (`deploy-proof.json`).

Slutlig hel färsk Go,820JS,vet och fyra tidsvakter gröna. Native4bilder/Keryx5verkliga avgångar och5mutanter med färsk återkontroll gröna.

Kod `c6769bb9`, serverprov `0f13616d`, sista passagefixtur `97f926cb`. Native/Keryx på samma produktionsbytes: lokal matbrist avvisas utan lager-/skepps-/jobbdeltan; därefter fem riktiga avgångar tar enbart fisk i Petras, Latos mat och supportankaret orörda. Fyra Firefoxbilder från två1280/390rundor, stabila produktionshashar.820JS/vet/tidsvakter gröna. Fem fysiska servermutanter med exakt återställning och färsk grön återkontroll: `mutations-server/summary.json`.

Första helsviten fångade en ytterligare äldre APIpickupfixtur utan mat i avgångshamnen. Fixturen rättades; färskt riktat prov grönt0.171s, och hela färska Go-sviten är nu grön (world146.992s; `full-go-green.log`). Den första röda helsviten ligger kvar som underkänt försök, inte som slutbevis.

Kärngranskning: economy äger fysisk hamnåtkomst; samma ShipDeparturePort används för förhandsvisning och faktisk avgång. Garrison kräver en aktiv egen stad i samma värld och, om koordinater finns, samma faktiska position. Legacy NULL-koordinater behåller settlement-dockningsbeviset. Frakt använder sin befintliga fysiska benägare. Ingen ny hamnbyggnad krävs. Både lokalt lageruttag och ombordmat ändras i befintlig avgångstransaktion; inget fjärrlager används när support saknas eller pekar annanstans. Silver/underhåll och stödankare är oförändrade. Se `server-audit.md` för samtliga skrivvägar.

Reproduktion: `tools/gotest.sh`; `python3 tools/port_provisions_bild_20261028.py OUTPUT`; `python3 tools/port_provisions_mutations_20261028.py CHECKOUT OUTPUT` i separat arbetskopia. Native har riktig registrering/join/founding, uttrycklig lokal Petras/skepps-/lagerfixtur och normala CLI-/UI-order. Dockningsresans föregående ankomst täcks av domänprovet, inte av nativefixturen. `python3 tools/port_provisions_deploy_verify_20261028.py --before` fångar driftsläge; utan flagga verifierar den ny process innan migrationsläsning och kontrollerar grundläge/assets.

Avgränsningar: ingen datamigrering eller retroaktiv påfyllning av pågående resor. Redan lastad mat, gästfrihet och returmat följer tidigare regler. Same-world/active-filter granskat och testat i helperns positiva läsning, men har ingen egen enskild mutant. Native provar tom hamn och fem tomma Emporosavgångar; lastade kohorter/ranson prövas i domänsviten, inte som nytt lastscenario i Firefox.

Metodisk lärdom: fysisk plats och ekonomiskt ankare är skilda fält. Preview och avgång måste läsa samma plats. Äldre testfixturer som fick gratis avgång via nil support måste få verklig lokal mat.

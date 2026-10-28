# Proviant från faktisk avgångshamn

Kontrakt före kod, Timothy2026-10-28: alla båtar tar avgångens mat från hamnen där de ligger. Detta ersätter25okt-beslutets proviantdel; stöd-/underhållsankare ändras inte.

Problem: en vanlig seglingsorder efter dockning i en annan egen stad drar mat ur den gamla försörjningsstaden. Frakt använder redan fysisk hamn.

Spelarsanning: fyll lagren där båten ligger; ett välfyllt avlägset hem kan inte betala nästa avgångs mat. Befintlig ombordmat och ransonregler behålls.

Scope: combat-avgångens proviantkälla, befintlig ekonomiägare, regressioner, fyra ytors text/semantik och verkligt API/Keryx/Firefox-bevis. Ingen migration, ändrat upkeep, ändrad besättning eller omskrivning av redan frysta resor/event.

Acceptans: faktisk hamn minskar, gammal support oförändrad; lokal matbrist avvisar atomiskt trots välfyllt hem; dockat skepp utan support kan proviantera lokalt; ingen påfyllning till sjöss; befintlig frakt/gästfrihet/ranson oförändrad.

Bevisplan: assertion-röd baslinje på färsk218, riktiga DB-prov, fysisk kärnmutant med exakt återställning, full färsk Go/vet/JS/tidsvakter, native flytt till egen stad och nästa avgång med DB-lagerdelta, ny process/ren218/bevarat grundläge och assets efter deploy. Bevisar kedjegrinden genom fysisk läsbar försörjning.

## Checkpoint efter compute-gräns

Kod `c6769bb9`, serverprov `0f13616d`. Native/Keryx: lokal matbrist avvisad utan lager-/skepps-/jobbdeltan; därefter fem riktiga avgångar tar enbart fisk i Petras, Latos mat och supportankaret orörda. Fyra Firefoxbilder, två1280/390rundor, stabila produktionshashar.820JS/vet/tidsvakter gröna. Fem fysiska servermutanter med exakt återställning och färsk grön återkontroll: `mutations-server/summary.json`.

Hela färska Go-sviten hittade EN ytterligare äldre fixtur utan mat i fysisk avgångshamn: `TestArrangePassage_Pickup_ShipFetchesFromDifferentOwnPort`. Endast fixturen har fyllts med lokal mat; riktat färskt prov är grönt (`passage-target-green.log`,0.171s). Slutlig fullsvit måste köras om efter denna teständring. Ingen merge/push/deploy ännu.

Nästa: riktat prov grönt → full färsk Go → granska diff/manifest → merge/push/deploy och hash/grundläge → återuppta inventerade wikin6a774405 med ny proviantkanon. Därefter handelsbud24 → flodfortsättning → K6 → notiskategorier enligt kön.

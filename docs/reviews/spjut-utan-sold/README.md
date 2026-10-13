# Spjut utan silversold — B

| Kontrakt | Innehåll |
|---|---|
| Problem | Vanligt fotfolks löpande sold kan hindra spelaren från att använda stridsenheter. |
| Spelarsanning | Spjut äter korn men kostar0 silver i löpande upkeep; professionella betalas som förut. |
| Invariant | Korn0,5/1, rekrytering12korn+20silver/100man, alla startbelopp480/60/730/1278 och typkolonipung30/60/90 bevaras. Gamla events behåller betydelse. |
| Scope | Centralt upkeep, fasta host-/kolonipengar, övergång av sparad founder-rate/unpaid, enhetsrosterDTO samt keryx/webb/Codex. |
| Non-scope | Marsch-soldrad, komplettSilverAudit, AI-guvernör, mapgen, havsvägar och ändrade startbelopp. |
| Acceptans | 0silverstad matar spjut utan desertion; elit betalar2; fasta Rise60/koloni30 och nyvärd480; migrering bevarar nuvarande balances/events; fyra ytor visar0. |
| Stopvillkor | Kvarvarande soldkoppling skulle ändra något startbelopp eller en gammal eventsemantik. |
| Bevisplan | Färsk baslinje; namngivna DB-tick/backfill/purse/eventprov och fysiska mutationer; fullfreshGo+JS+vet; tvårena Firefox1280/390/keryx scenarier i isolerad miljö. |

Bas532608d9. Blockerar ekonomigrinden till stridsenheter; naturlig gruv-/brons-/elitkedja är separat bevis.
Timothys riktlinje: vid tvekan välj det generösare. Inga extra slutliga silverkranar införs.

Startbelopp är reserver, inte längre årssold för spjut. Gamla hostbalanser bevaras vid den aktuella
världens tick innan deras sparade rate blir0; gamla carried-pengar flyttas som tidigare.
Spjutens gamla unpaid_periods ska inte fortsätta som livevarning efter övergången.

## Resultat och kärngranskning

Produktion **b2fd9f11** från532608d9. Spjut kostar0silver; .5grain i garnison och1 i
fält/embarked består. Professionella soldtabellen och spjutrekrytering12grain+20silver+100man
är oförändrade. MetropolisSilver730/1278 och caps1460/2556 är orörda, bara kommentaren om
årssold blir reserver. Nyvärd480/Rise60 har namngivna fasta reserver och rate0. Kolonister
bär fasta30/60/90 per100spjut/elit/vagn, proportionellt för delkohort; samma dispatchclamp,
debit/credit/return och capregler. Inga nya pengar vid koloni eller återgrundning.

Kärndiffen är granskad av Codex enligt Timothys rolländring07:09: ranson kör före zero-silver
continue, gammal unpaid-räknare rensas, professionellas fulla afford-gate/återcirkulation och
desertion är orörda. Inga solddivisioner tillkommer. Handlern behåller idempotensclaimen även
för ration-only. Migrationsbackfill använder **respektivevärlds tick**, inte den globala
current_world_tick, och realiserar både grain/silver som delar calc_tick innan ratefrysningen.
Manpower, status, befintlig carried_silver och gamla eventpayloads uppdateras aldrig av backfill.
Live spjutens unpaid_periods blir0, professionella och historiska döda förbandsräknare lämnas.
Down återställer framtida hostrate−2/−1 utan att ta bort valuta eller återskapa gamla varningar.

Serverns roster visar nominell ranson/bruttosold med explicit0; kontext active/inactive/subsistence
följer tickens status och dess verkliga founder_phase-undantag. None döljer raden för civilhostens
markör, som upplöses vid grundning. Keryx och War→Army konsumerar dessa fält och anger aktiv
upkeep/when active/after settling; de har inga egna soldtabeller. Sex Codexartiklar beskriver
fotfolk på korn och fasta reserver. Gamla UpkeepUnpaid/UnitDeserted-texter återges historiskt
som förut; inga events har omtolkats. Verblista och två beställda vaulthem uppdaterade.

## Prov och mutationer

| Prov | Utfall |
|---|---|
| Baslinje före central ändring | FärskPG16/mig177, combat/API relevanta äldre prov gröna. Klientagent arbetade parallellt; baselineheader har +ändringar men serverns kostnadtabell var ännu master. |
| Faktisk tick80 gånger,0silver och gammalunpaid71 | `TestSpearmanZeroSilverStillEatsAndNeverDeserts`:100man kvar, unpaid0, grain100→60, silver0, inga unpaid/deserted-events. |
| Fält och professionell | `TestSpearmanFieldRationAndElitePayRemain`:spjut1grain+elit1,2grain, elit2silver, samma betalare. |
| Fasta reserver | Riktiga seedHost/Rise INSERT och `TestColonistPurse_FixedReservesDoNotFollowUpkeep` visar480/60 och30/60/90, även halv kohort. Befintliga purseconservation/returnprov kvar. |
| Roster/API | `TestUnitRosterRationsAndExplicitZeroSalary` visar0 i faktiskJSON, professionella kostnader och forming-kontext. Native verkligAPI bevisar founder-subsistence→settled-active. |
| Migrering177→178→177→178 | `migration.py`:worldtick100/20 bevarar eff.silver474/57, grain1197/1200, inactivefp orörd; carried30/60 och elitunpaid71 kvar, spjutunpaid0; historiska RiteCast/UpkeepUnpaid identiska. |
| Historiska läsare | Befintligt `TestSilverCalibrationOfferWorkAndLegacyPayloadRead` fullsvit grönt med riktigLoadStream; klienternas äldre eventformatter oförändrad. |
| Fysiska mutationer | spjutsold1, elitsold0, härledd kolonipung, härleddRise, härleddnyvärd blev alla **namngivet röda**, återställda bytevis, gemensam **färsk** namngiven svit grön. |
| Fullt | `tools/gotest.sh` påb2fd9f11, **tomPG16/mig178 exit0** (world93,858s); vet exit0; **611JS**,0fail/skip. |

Initiala affected-paketskörningen visade gamla1-soldförväntningar. Kontraktsprov ändrades till0;
test som skall bevisa betalning/cirkulation/desertion använder i stället en verklig skadad
50man-elit(.3grain/1silver), med faktiskdesertionsjämförelse50→lägre. Gamla finansiella skydd
avaktiverades inte. Två hela affected-paket och den slutliga fullsviten är gröna.

## Riktiga spelarflöden och bilder

**Två separata tommaPG16/Redis/native-server/keryx-körningar**, healthzb2fd9f11/mig178 och
tom server/webdiff vidbuild. Vanligregister→join→found, **ingen SQL-fixtur eller SQL-skrivning**,
inga payload-/HTTP-/CSS-overrides. Föregrundning480/rate0 och riktig Keryx "after settling";
eftergrundning oförändrad730/1278-kassa och tvåspjut med.5grain/0silver i verkligAPI/Keryx/Firefox.
Firefox1280×900/390×844 visar båda enhetsrader utan sidotrunkering eller JS-fel. Kodex bildrad
bedömd vid1:1 av Codex enligt delegerad verifiering: text/sold0 läsbar, march/stance-knappar nåbara.

[Första körningen](live/verification.json), [andra](live-repeat/verification.json),
[desktop](live/desktop.png), [mobil](live/mobile.png), [desktop igen](live-repeat/desktop.png),
[mobil igen](live-repeat/mobile.png). Detta bevisar naturlig förstaåtkomst och exaktkostnadsrad,
inte en genomspelad brons-/elitkedja eller kalenderår av försörjning.

## Reproduktion och grindar

```sh
python3 docs/reviews/spjut-utan-sold/migration.py
python3 docs/reviews/spjut-utan-sold/mutate.py
tools/gotest.sh
python3 docs/reviews/spjut-utan-sold/live.py
B_PROOF_OUT=live-repeat python3 docs/reviews/spjut-utan-sold/live.py
```

FullJS: node--test för alla web/*.test.mjs; vet: frånserver med env-iHOME/PATH go vet./....
Verktygen använder egna containrar/portar och städar enbart dessa. Mutationerna återställer
källbyte i finally; kör inte annan kompilering mot sammaworktree under dem. `live.py` kräver
committad production innanbuild och loggar healthz/provenance. Tick3600 för stabil layout,
inte ett finansielt konstgjort scenario; tickmekanikens80steg provas separat i riktigDB.

Kodgrind/semantiskgrind/användargrind gröna. Visuellt: existerande enhetskort med ny kostnadsrad,
egna1:1-bilder ovan, inga nya bildfamiljer. Driftgrind verifieras efter eget merge/push/deploy
enligt ny roll: rapportera branchhash+LIVEcommit+mutation+healthz till Claude. Viktiga kanonfrågor
parkeras i BESLUT och oberoende köarbete fortsätter enligt besked07:17.

## Kända avgränsningar

Marschens egna soldrad och komplettSilverAudit är nästa separata slicer. Navalstatus/prognosens
befintliga skillnader och AI-guvernörens tysta avdrag byggs inte här. Inga nya deadlines för
subsistensvärdar eller ekonomiska sänkor tillkommer. Startreserver betyder nu buffert, inte365ticks
av spjutens faktiska sold. README/fem mutationer/611JS/färskGo och tvånative flöden är bevisen;
ingen oberoende Claude-mutation påstås för B, rollen flyttades till Codex.

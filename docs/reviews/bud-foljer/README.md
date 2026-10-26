# Bud följer flyttad enhet

Kontrakt2026-10-26 före kod. Timothy beställde25okt att ett bud som missar en flyttad enhet tar nytt fysiskt ben och fortsätter tills ifatt; nästa slice vald på hans26oktmandat.
Problem: leveransclaim avslutar budet på ett gammalt rendezvous, eller order exekveras på distans när målgruppen hunnit flytta.
Spelarsanning: Runner bär kvar samma order, följer landförbandet med ny restid och kan inte ändra förbandet innan de möts.
Invariant: ingen teleport/instantorder; varje nytt ben startar vid budets faktiskt nådda hex med sparad väg/klocka och ny generation; replay/gamla deadlines gör inget. Ägarskap/värld/liv valideras. Gamla kuvert behåller frusen betydelse; bara nyexplicit opt-in följer.
Scope: landförbands march/recall/redirect/stance-kuvert, leverans/nya ben, serverägda pendingmetadata, webb/Keryx/Codex, verkliga DB/HTTP/workerprov. Befintlig passage återanvänds när ett nytt ben behöver hav.
Non-scope: budets synminnes-backfill, nya budtyper, nya order till skepp på havet, ändrad befintlig eventtolkning, retroaktiv ändring av redan avslutade missar.
Acceptans: båda enheterna måste mötas; två flyttar ger två fysiska ben utan förtida verkan; replay/generation/earlydeadline säkra; död/ägarskifte terminalt utan effekt; nytt pendingmål/tid läsbart på alla ytor.
Stopvillkor: ny kanon/informationskonflikt utanför beslutad förföljning tas till Timothy.
Bevis: genuint baseline-rött DBprov; samtliga fyra orderfamiljer, gammalt kuvert kvar som förr, mutanter/exaktrestore, hel färskGo/JS/vet, riktig HTTP→worker och nativeFirefox/Keryx, livehash/grunddata.
Gate: proves fysisk ordergivning genom geografisteget.


## Resultat

Kärna `65202f6e`, rebasad på `af981581`/migration217.

Nya landorder följer förbandet från budets faktiskt nådda hex. Samma Runner och order får ny frusen fysisk rutt, ankomsttid och generation när förbandet flyttat sig; väntan på tillfälligt otillgängligt förband behåller budet vid nådd hex. Ingen verkan före mötet. Äldre kuvert går fortsatt genom den äldre leveransgrenen. Alla fyra ytor beskriver nästa möte/ny rutt/väntan, utan leveransgaranti eller främmande ordermetadata.

Tre verkliga felslut i WIP rättades: den sista marschkontrollen tillät bara stillastående möten, och v2-interceptionen använde världens fas i stället för ruttens sparade klocka. Det rörliga undantaget är dessutom strikt runtime-opt-in för nya följkuvert så att gamla kuvert inte omtolkas. [Täckning och gränser](coverage-and-mutations.md) · [exakta sed-rader](exact-sed.md) · [mutationsmanifest server](server-mutations/verified.json) och [klient/API](client-mutations/verified.json).

Användarprov: normal registrering/join/grundning i egen färsk PG217/Redis/server, uttrycklig startfixtur, riktig CLI-marsch följd av senare riktig CLI-hållningsorder. Utan DB-ingrepp efter dispatch rör första ordern förbandet, den senare Runner missar gamla punkten, tar ett nytt ben och applicerar hållningen först vid mötet. [API/DB/worker-bevis](acceptance-legacy-scoped/proof.json), [Keryx](acceptance-legacy-scoped/keryx-replanned.txt), [native Firefox1280/390](index.html). 963 produktionskällhashar kontrolleras före/efter; se exakt antal i bevisfilen. Inga testorder skickades till den delade världen.

Provenance för den egna riggen (motsvarande acceptance.sh-format, inte den delade acceptanscontainern): `rigg: commit 65202f6e+order-pursuit · migration 217/217 · värld 0a48275d · TICK_SECONDS=10 · 2026-10-26 20:11`. Slutriggen byggde committad 65202f6e; SHA-beviset och mutanternas slutkällmatch anger exakt källa. Två tidigare gröna spelarprov finns i acceptance/ och acceptance-final/; slutriggen ersätter dem som deployunderlag. Bara test-/verktygs-/rapportfiler ändrades därefter.

19 huvudmutanter assertion-röda, byte/SHA-exakt restore och färskgrönt efteråt. Två lärdomar redovisas öppet: en initial deadline-mutant överlevde en svag outbound-assertion (skärpt till inga nya jobb/ben och nu röd); en redundant inline-sekretessguard överlever eftersom sista främmandeprojektionen ändå stryker privata fält. Första fullsviten hittade sju äldre tester som levererade framtida nya kuvert vid time.Now; deras klockfixturer går nu till riktig deadline och aktuellt generationsjobb, med ursprungliga slut-/replayassertions kvar.

Avgränsning: inga nya order till skepp på havet, ingen omskrivning av gamla event/avslutade missar, inget synminnes-backfill. Bordning/räddning/landstigning återanvänder befintlig passageägare; nya prov täcker dess spärrar och väntan men inte en ny full sjöresa. Claim före domänexekvering behåller befintlig transaktionsgräns; detta är ingen generell omskrivning av ordermotorn.

Kodgrind slutkod: hel färsk Go (26 paket; [logg](full-go-green.log)), 803 JS gröna, vet och fyra språkspärrar gröna. 19 assertion-röda mutanter/exakt restore, tre genomförda separata HTTP/worker/Keryx/Firefoxprov; sista byggd från65202f6e är deployunderlaget. BILD A1/A2 granskat vid1:1 och visas Timothy i efterhand.

Drift LIVE `0b446a9e`/migration217ren: nyprocess bevisad före schemaläsning, samma värld/15spelare/6städer/18enheter/102varurader, tio ändrade statiska filer byteexakta. [Deploybevis](deploy-proof.json) och [ombyggnads-/startlogg](deploy-journal.log). Inga testorder eller datarättningar i livevärlden.

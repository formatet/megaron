# Dispatchtexter och sanna omen

Arbetsgren `codex/dispatchtext-omen`, återstart från sparad545f6881, rebasad på origin/masterfc2741e2 före fortsättningen. Integrerad och LIVE2026-10-26 14:10: `be877c2d`, process `be877c2`, migration215ren.

## Kontrakt

Problem: färdiga byggnader/enheter saknar stad/hex/spelarnamn, dispatchknappen säger inte vad den öppnar, och gamla omen är påhittade meningar. Spelaren ska kunna bemanna rätt byggplats eller agera på rätt post och förstå ett omen som sant när det observerades.
Invariant: frysta gamla eventfält tolkas som förr; nya namn/metadata är valfria tillägg. Ingen ny kunskap via vanliga dispatchmål. Omen följer uttryckligt godkänd separat informationsregel; ingen egen handel återberättas som rykte.
Scope: orderns fyra ytor, completionpayload/queue, dispatchrouting, kharis omen, gossip, migration215. Ingen ny frågeorakelmekanik eller balansändring utanför godkänd justerbar omentrappa.
Acceptans: namn+plats; vald handlingsyta; varje omenmall sann/tyst om falsk; hög/låg precision och nåbart topptak; egna transporter stannar utanför egna rykten. Stopvillkor: ny kanonkonflikt. Bevis: riktiga DB/API-prov, pertyp routing, mutationer, schema fram/bak, fullfärsk Go/JS/vet och nativeFirefox.
Gate: bevisar att den befintliga spelaren kan handla på produktion/logistik/gudomliga tecken i geografi→brist→brons→elit.

## Godkänd omenregel

Claude16:30 2026-10-25 godkände Timothys form: kharis<5/5–29/30–59/60–89/≥90 har minsta gap12/8/5/3/2ticks och räckvidd3/5/8/12/16hex från eget tempel. Vanliga sanna tecken har ingen extra slumpgrind; ingen sann kandidat ger tystnad. Precisionen ökar från vagt till kategori/riktning, avståndskänsla och varutyp. Högsta nivån kan i10%av fallen ge konkret hex endast för ett verkligt brådskande kuststormsfaktum. Inga okända stadsnamn, exakta styrkor/tider/rutter. Level3tempel/EffectiveKharisCap100 når toppsteget i verkligt DB-prov; level2 når det inte.

## Omenägare och livscykel

`omen.go` äger trappa, villkor, text och atomisk observation. Den sparar realm-cooldown i befintlig kharis_notice_state, OmenObservedV1-event och gossip/fact_data i samma TX. Stormens senaste verkliga spår, verklig metalldepositflagga och civil matproduktion minus invånarnas konsumtion är de tre faktakällorna (två matmallar). Fler mallar införs inte utan egna sanningsprov. FactID är ett slumpmässigt observations-ID: en hash av en liten kartkoordinatdomän hade kunnat brute-forceas och avslöjat lågprecisionsfakta. Bara det sällsynta tillåtna toptecknet lagrar/visar exakt hex på spelaråtkomligt utfall. Ett omen är en tidsstämplad observation, ingen framtidsprognos.

`transport_owner_id` följer handelsryktet genom Broadcast och kontaktpropagering, så det inte kommer tillbaka till transportägaren. API läser bort ägarens sådana rader. Gamla trade-formulerade rykten utan ägarmetadata döljs när ursprungsstaden är spelarens egen; historisk ägare kan inte rekonstrueras säkert. Gamla atmosfäriska omen utan faktisk utfallsmetadata döljs i spelarflödet men behålls i DB-arkivet.

## Baslinje och delbevis

`omen-baseline-red.log`: gamla generateOmen uppfann ett omen trots tom kandidatvärld. `omen-db-green.log`: tystnad, varje malls falska villkor, level3tak, low-vs-high precision, retry/realm-cooldown även via andra templet, räckvidd. `omen-transport-green.log`: ursprungsägaren får inte transportgossip vare sig direkt eller genom annan Wanax kontakt. `gossip-api-green.log`: inget eget transportgossip eller gammalt obelagt omen, andra personers handel och sanna metadata kvar. `omen-ui-red/green.log`: egna Omen-markören.

Rigglärdom: initiala depositfrågan läste boolesk false som IS NOT NULL, vilket sanningsprovet stoppade; frågan kräver nu faktiskt true. Ett andra-tempel-fixturprov är explicit is_capital=false (DB-default är true); annars valde det fel matstad. Inga grönbevis på de felaktiga körningarna påstås.

## Slutbevis

Slutkod `c4a66c74`, rebasad på färsk origin/master `fc2741e2` före slutsviten. Hel färsk Go-svit på migration215,789JS-prov, vet och fyra dagsvaktytor. Resultat i `full-go-final.log`, `full-js-final.log`, `vet-final.log`, `day-guard.log`; inget tidigare mellanresultat ersätter slutkörningen.

20 fysiska assertion-röda mutanter:14Go/färskaDB och6JS, exakt återställning. `coverage-and-mutations.md` listar varje nytt provs täckta/otäckta anrop/grenar och exakta byteverifierade sed-rader. Den initiala train-mutantens byggfel räknas inte; korrigerad mutant faller på den riktiga assertionen. Land/spill-war_chariot-skulden är täckt och båda mutanterna röda.

NativeFirefox1280/390: verkliga workerutfall och API-data i egen värld; byggplatsen väljs i Petras, Emporos markeras, Kult öppnas i Petras, handelsrutten markeras och sanna omen syns medan egen handel filtreras. Bilder/proveniens/gränser i `bild/README.md`. Bildgranskning hittade en täckande Notifications-panel; rött→grönt regressionsprov och slutlig normal klickväg bevisar nu att målytan visas utan att riggen stänger paneler åt produkten.

Integration/deploy verifieras separat med livehash, ren migration, bevarad värld och byteexakta publika assets; se `deploy-proof.json` när den finns.

Deploybevis: samma aktiva värld8d1f34f1, tick2674→2683,15spelare/5städer/18enheter/85varurader bevarade;21publikaassets byteexakta. Go-slutpaketet world153.175s. Ingen omstart på rutin och ingen speldataåterställning. Lokal Keryx ombyggd. Se `deploy-proof.json`.

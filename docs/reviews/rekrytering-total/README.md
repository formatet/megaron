# Rekrytering: totalpris och verkliga grindar

Bas `762c5672`, egen gren `codex/rekrytering-total`. Ingen merge/push/deploy.

## Kontrakt

Problem: War delade katalogens totalpris med besättningen och visade /man, PopCost och generiskt insufficient. Läsytan erbjöd rekrytering som själva ordern nekade vid invånargräns och full sjökö.
Spelarsanning: en hel rekrytering har ett serverhärlett totalpris, faktisk draft och konkret avslagsorsak; timber kan ersättas med cedar. Invariant: priser, besättning och kanon ändras inte; betalda varor återbetalas som samma varor.
Scope: kostnads-/rekryteringsläsning, betalningskvittens/reservation, webb War, Keryx, enhetsartiklar. Ingen ny balans/migration eller landsättningskod.
Acceptans: totalpris från katalog; alla grindar speglar Recruit; cedar visas/räknas korrekt; rekrytering/avbokning bevarar faktisk vara; native Firefox desktop/mobil.
Stopvillkor: nya kanonbeslut. Bevis: riktiga HTTP+DB-prov, mutationer per grind, full färsk Go/JS/vet, isolerat användarprov.

## Baslinje och inventering

5392b18 har redan rättat batchmultiplikation och cedar-affordability; ingen falsk röd reproduktion för det felet påstås. `baseline-red.log` visar däremot att20invånare erbjuder20personers galär fast Recruit kräver en kvarvarande invånare. `cedar-receipt-red.log` visar saknad faktisk betalning i kvittot.

Bygge, rekrytering och reparation använder samtliga handlers/helpers.go deductGoods→coverTimberWithCedar. Genuina cedarkrav reserveras först; timber dras före överskottscedar1:1. Ingen parallell betalningsägare införs. SQL/tools-sökningens övriga träffar är migreringar, prisdata, testfixturer och transporter av timber som last, inte nya kostnadsavdrag.

## Ändring

`recruitReasons` ger serverns konkreta helbatch-, byggnads-, lager- och sjököorsaker; katalog och läsyta använder samma batchstorlek. `deductGoodsPaid` är befintlig transaktionella betalningsägare med faktisk betalningsretur, dess gamla wrapper bevaras. Rekryteringskvittot och reservationsraderna använder faktiskt debiterade varor: avbokning kan inte förvandla cedar till timber. Nominella prislistor behålls.

Klienterna visar hela priset, draften, timber(or cedar), serverns orsaker och faktiskt betalda varor. PopCost tas bort från presentationen, ligger kvar kompatibelt i API. Fyra ytor uppdaterade. Detta bevisar logistiken i geografi→brist→brons→elit på redan nåbara ytor.

## Verifiering

Kod `8e19cdd7`. Hela färska Go-sviten grön:26paket med tester, world152.828s. Inga andra produktionsändringar efter kodcommitten. `gates-green.log` visar riktig HTTP GET/POST-paritet för lager, byggnad, cedar och full sjökö. `first-green.log` täcker invånargränsen samt cedarrekrytering/avbokning med exakt återställning av varor.


771 JavaScriptprov, hela Keryx-paketet, vet och fyra day-guards gröna. Sex fysiska mutanter assertion-röda: invånargräns, byggnad, helbatchkostnad, cedar, sjökö och faktiskt betalda återbetalningsvaror. Exakta originalbytes återställda, därefter nya färska API-prov gröna. Mutationerna kördes i en separat egen checkout av8e19cdd7 för att inte påverka den samtidigt stabila fullsviten/bildriggen. En första mutantskriptkörning stoppade på en Docker-portkollision (inte påstådd testmutation); skriptet väljer därefter en ledig port, alla sex kördes om och gav riktiga assertionfel.

[BILD](index.html): sex native Firefoxbilder1280/390, betalbart/namngiven brist/faktisk order. Riktig registrering, join och founding; explicit isolerad stadsfixtur. 0timber749cedar innebär enligt cedarbeslutet att **både galär och Emporos är betalbara**, vilket ersätter den äldre bildbeskrivningens antagande att galären ska nekas. Vid10timber0cedar visas exakt brist. Två riktiga webborders galär betalade30cedar vardera; DB689cedar/timber0. API och verklig Keryx verifierade. Alla945produktionshashar matchar slutkoden;0sidfel. Första riggens assertion förväntade att nollvärden saknades i paid_costs, rättades endast i riggen och ersattes av ny färsk grön körning. Samtliga sex nativebilder granskade1:1.

Avgränsning: redan känd knapp väntar på ny omladdning efter lyckad beställning i samma vy; betalningslås/refresh förändras inte i denna slice. Ingen mänsklig textplaytest påstås. Inga migändringar.

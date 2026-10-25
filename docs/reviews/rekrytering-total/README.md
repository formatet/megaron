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

Pågående; slutresultat, mutationer och bilder fylls före överlämning. `gates-green.log` visar riktig HTTP GET/POST-paritet för lager, byggnad, cedar och full sjökö. `first-green.log` täcker invånargränsen samt cedarrekrytering/avbokning med exakt återställning av varor.

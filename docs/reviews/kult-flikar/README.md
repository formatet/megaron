# Kult: Kharis, Prayers och Temples

## Sammanfattning

Kult läste böner från en vald stad och dolde därmed andra gudars tempel och böner. Nu läser Kult en ägarprivat riksvy med tre flikar. Varje bön, arbetsplatslänk och rivning behåller sin stad; samma gud i två städer ger två skilda handlingsrader. Byggda tempel och direkt betalbara böner visas först.

Slutkod `b85a8309` (bas `b1080116`); hel färsk Go GRÖN (world154.930s),808JS/vet/fyra tidsgrindar och10mutanter gröna. Native12tabbilder+1rivningskvitto från riktiga flöden. Deploykvittot tillkommer efter utrullning. Ingen migration, balansändring eller ändrad eventtolkning. **Bevisar kedjegrinden** genom att spelaren kan hitta och använda sina verkliga tempel och böner.

## Slice-kontrakt och baslinje

Order: `.agents/order-kult-flikar-20261025.md`. Tre tempel/två gudar i olika städer ska visas på webb och Keryx. Byggda tempel först, obyggda kultur-/stadsgiltiga kandidater sist; böner man kan betala direkt före komposition/andra hinder och saknade tempel sist.

Baslinje: offerplanen var redan rikstäckande, men `available_prayers` och handlingsrutterna utgick från en enda stad. Inventeringen motiverade en riksvy, inte en ny uppsättning böneregler.

## Lösning och experiment

- `GET /worlds/{worldID}/cult` väljer Wanax enbart ur autentiseringen. Städer, tempel, bemanning, nästa dagliga offer och rivningskonsekvenser hämtas genom befintliga ägare. Bönernas läsmodell har flyttats till `loadPrayerRows`, som också används av vanlig province-inspektion.
- Tre flikar använder samma tabbstil som War. Kharis beskriver riket, inklusive rikets sammanlagda bemanning. Temples visar varje byggnad och stad, nivå, bemanning, offer och rivning. Prayers använder kombinationen settlement-ID + prayer-ID som formulärnyckel; klicket skickar den ursprungliga bönen till just den stadens settlement-endpoint.
- Keryx status visar rikets tempel och riktiga staff/demolish-kommandon med stad och gud. `rite --list` visar alla stadsbundna böner; explicit `--settlement` filtrerar listan.
- Ett nytt DB-fall där Amnissos behöver komponera ett alternativ men Kommos kan betala sitt recept visade sorteringsfelet assertion-rött före rättningen (`sort-baseline-red.log`), grönt efter (`target-green.log`).
- Nativebilden fångade att offeralternativ hade sammanfogats med plus; nu står det “or”. Första riggens felaktiga knappnamn, en lokal testportkollision och PG:s tillfälliga initserver dokumenterades som underkända riggkörningar. Riggen väntar nu på PG via TCP; de rena slutkörningarna ersätter dessa försök.

## Grindar

| Grind | Bevis |
|---|---|
| Kod | `full-go.log`, `js.log`, `vet.log`, `dayguards.log`; nya DB-testet jämför varje realm-prayer med normal province-läsning |
| Mutation | Fyra server- och sex klientmutanter i `mutations-server-final/proof.json` och `mutations-client-final/proof.json`, namngivna assertion-röda prov, exakt återställning och färska gröna kontroller |
| Semantik | Tre tempel/två gudar/tre städer, privat ägare, korrekta offer-/quote-/arbetsplats-ID, gemensam väntetid, betalbar först, saknat tempel sist; Kharis visar realm-total som realm-total |
| Användare/BILD | `native/proof.json`: processhash/migration, riktig API/Keryx/Firefox, tolv tabbilder från två körningar i1280/390, native Manage in City till kolonin Kommos, faktiskt offer i Kommos utan lagerdelta i Amnissos, färsk rivningsquote/cancel/bekräftad DELETE för endast Kommos tempel |
| Drift | `ground-before.json` och `deploy-proof.json`: ny process, migration217ren, samma värld och grundantal, byte-exakta ändrade statiska filer |

BILD för Timothy: [K1–T2](index.html). Normal spelstorlek, ingen horisontell panelöverströmning. Mänsklig textbedömning sker i spel.

## Reproduktion

- `python3 tools/kult_tabs_bild_20261027.py OUTPUT`: egen PG/Redis/server och verklig register/join/found; andra/tredje staden, tre tempel och rikliga varor är uttryckliga lokala fixturer. Bemanning sker genom placements-API. HTTP-svar och DOM ersätts inte. Produktionshashar före/efter måste vara identiska.
- `python3 tools/kult_tabs_server_mutations_20261027.py CHECKOUT OUTPUT` och `python3 tools/kult_tabs_client_mutations_20261027.py CHECKOUT OUTPUT`: använd separat checkout. Varje mutation skriver exakt ersättning, förväntat prov och röd logg; finally återställer bytes.
- `python3 tools/kult_tabs_deploy_verify_20261027.py --before`: läs driftens grundläge. Utan flaggan efter deploy verifieras kod, process och ändrade assets.

## Täckning och avgränsningar

- Nya serverprovet täcker tre egna städer, två gudar, dubbel Asterion, bemanning/offer/rivningsquote-ID, exact province-paritet, betalbar/komposition/saknad-kategorier, väntetid över samma guds tempel, query-ID som inte får byta ägare, anonym åtkomst och Wanax före grundning.
- Nya klientproven täcker unika formulär, korrekt POST-stad, bevarat bön-ID, tempel-/stadsnamn och escaping, realm-bemanningsetikett, Keryx status och stadsfiltrering/kommandon.
- Nativeprovet täcker standardoffer och bekräftad rivning i Kommos. Alla fem böner listas men deras samtliga effekter körs inte på nytt; äldre domänprov ligger kvar i fullsviten. Ingen native byggbeställning av de obyggda templen; länken öppnar vanlig City-byggyta. Ingen ny värld eller spelare skapas i drift.

## Metodisk lärdom

En riksvy ska behålla de lokala handlingsidentiteterna och återanvända sina läsägare. “Kan förbereda” och “kan betala nu” behöver skilda prioriteringar. Ett grönt riggprov för processstart kräver den slutliga databasens TCP-lyssnare, inte initserverns lokala socket.

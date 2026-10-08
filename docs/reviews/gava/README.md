# S — gåva och tribut

Transfer kunde bara nå egna städer. Nu kan spelaren skicka varor och silver till
en kontaktad främmande stad utan samtycke eller motprestation, med fysisk last,
exakt leveransutfall och en gåvorad bland breven.

**Överlämnad för Claudes granskning och Timothys BILD, inte mergad/deployad.**
Gren `codex/gava`, bas `eb523aa2`; slutlig produktionskod **e8c5200f**.
Sluthash för bevispaketet står i överlämningen/git log. T startas inte.

| Grind | Bevis |
|---|---|
| Kod | Full fresh Go exit0 på ny PG16/migration160; vet exit0; **441 JS pass**. `full-go-final.log`, `vet-final.log`, `js-final.log`. |
| Mutation | Sex fysiska mutationer ger namngivna röda prov; återställd kod grön. `mutation-*.log`, `mutations-restored.log`. |
| Användare | **Två nya rena världar**, riktig Firefox-gåva silver50, Keryx livestock2, mottagarens lager/notiser, vanlig marsch/sentry och fysisk raid. `live-after1/2/proof.json`. |
| BILD | Firefox146.0.1 **först**, Chromium145.0.7632.6, WebKit26.0; desktop1280×900 +390×844 touch. 12 bilder per slutarm. **Timothy dömer före merge.** |
| Provenance | Båda actual `/healthz`: **e8c5200f /160/ok**, schema **160\|f**; samma fasta serverbinär/assets, separata PG/Redis. `assets-sha256.json`. Noll browserfel/overflow/SQL-ingrepp. |

Bilder att börja med: [Firefox Transfer mobil](live-after2/transfer-firefox-mobile.png),
[Firefox tråd mobil](live-after2/thread-firefox-mobile.png),
[Firefox Transfer desktop](live-after2/transfer-firefox-desktop.png),
[Firefox tråd desktop](live-after2/thread-firefox-desktop.png).
Motsvarande `chromium`/`webkit`-bilder finns i samma katalog.

## Kontrakt och slutlig implementation

Fyra ytor: server/transport och capabilities; `keryx transfer` och notis;
Economy Transfer + Diplomacy-rad + webbnotis/uppdatering; Codex-artikel `gifts`.
Inga nya dialoger, relationsliggare, summeringar, skulder eller tributautomation.
Silver är en vanlig vara; katalogens `weight>0` avgör givbara varor. Trade-offers
behåller sitt eget silverfilter. Kolonins gamla Gift-from-capital är separat.

- Ny mottagarlista använder exakt brevets `VisibleOrigins` +
  `VisibleFrom(...,6)` och visar främmande Wanax; dolda namn/id:n lämnar inte
  endpointen. Samma grind prövas vid faktisk dispatch, med ändpunktsägarlås.
- Debit, fysisk interceptable mover, GiftDispatched-audit och ny GiftDelivery
  schemaläggs i samma TX. Nytt utfall krediterar lazy stock under lås, respekterar
  cap, och sparar audit + bestående notiser i samma TX. Replay av samma eller
  annan timer-id för samma last ger inte ny kredit/utfallsnotis.
- **Timothys B, PLAYTESTFRÅGA:** destinationens faktiska nya ägare får godset;
  sändare, avsedd mottagare och faktisk mottagare underrättas, deduplicerade.
  Fullt lager får det som ryms; resten förloras med exakta mängder. Fallen,
  ägarlös eller hårdraderad mottagarstad får inget gods, GiftLost. Ingen
  godsretur i dessa leveransfall. Ny payload bär båda ursprungliga Wanax-id,
  båda stad-id samt faktisk mottagare och credited/lost/returned_quantity.
- Befintligt verkligt skepp binds och avslutar vanlig leverans med tom fysisk
  hemresa. Prov följer det till hemmagarnison, även efter raderad destination.
- **Sjörån:** gamla captured/limped/sunk lämnas oförändrade. Ny atomisk audit
  `TransportDamagedReturnDispatched` länkar originalet till returen/manifestet.
  GiftLost visar exakt den del som förlorades och den del som **skickades hem**.
  `returned_quantity` betyder last på den fysiska damaged_return-resan, inte
  en omedelbar lagerkredit; returen kan själv rånas. Riktigt scan→limped-prov:
  50 debiteras, 25 förloras/25 skickas hem, inget instant refund, sedan endast
  25 krediterat en gång vid hemkomst och skeppet garrison.
- Ingen egen gåvorisk eller nya platta-tärningsprov. Legacy trade och gåvor
  använder samma befintliga `deliveryLoss`; T tar bort den gamla tärningen på
  ett ställe senare. Gamla event/payloads och egna transferregler omtolkas inte.
- `/gifts` visar sändarens avsändning, parternas senaste leveransutfall per last.
  Mottagaren får inte läsa avsändningen före ankomst; utomstående får inget.
  Gåvan är en rad bland breven, med exakta mängder och ägarbyte, utan erbjudandets
  Accept/Decline/Reply-knappar. Webb/Keryx formatterar samma bestående utfall.
  Nytt JS-prov läser **verklig** PG/scan-payload `payload-limped.json`.

## Baslinje, experiment och förkastade armar

Bas `eb523aa2`: fresh intern-transfer/karavan grön, **437 JS pass**.
Riktig DB/handler-repro främmande silver var röd på **403** own-only (ordertexten
angav400). Dold dispatch avvisades utan debit/mover. Basbilder i `before/`:
arkiverad **client eb523aa2**, separat färsk värld med en egen stad, på server
**e8c5200f**. De bevisar gamla klientens två-egna-städer-spärr, inte gammal
serversemantik; den bevisas av `red.log` på den gamla serverkoden.

| Hypotes/fynd | Utfall och beslut |
|---|---|
| Foreign dispatch nekas | Rött före, 201 + verklig last/kredit grönt efter. |
| Silver saknas i capability | WIP full Go var röd på enbart-silver; byt enbart transferns varufilter till weight>0, contacted/hidden-prov gröna. Gamla röda `full-go.log` behålls som bas för rättelsen. |
| Förlust efter limped | WIP sade felaktigt lost50. Riktig scan/retur visar lost25/returned25 och faktisk hemkredit25; separat audit länkar den fysiska returen. |
| Silver2 måste ge ökande saldo | Förkastat `rejected/silver-drift`: riktig sold under färden kan överstiga gåvan. Slutrigg skickar silver50 och visar krediten samt övrig saldoförändring separat. |
| Gamla Economy-briefen | Den sade silver alone crosses borders. `rejected/old-brief-native-fields` var funktionellt grön men texten motsade nya gåvor. Enbart Economy-briefen rättad, med Claudes godkännande; Wants-meningen och övriga briefs/dismiss kvar. |
| Native WebKit-fält | Första WebKit-bilder hade rundade hörn och annan textstorlek. Transfer-fält får font:inherit och border-radius:0; nya rena armar tagna efter fix. |

Mutationer: ta bort dispatch-FOW → hidden-provet rött; ta bort kredit →
leverans/lagersaldoprov rött; slopa cap → capacity rött; slopa avsedd mottagares
notis → ägarbytesprov rött; returned_quantity=0 → riktigt limped-prov rött;
återinför handelsvarufilter för transfer → enbart-silver capability rött.
Mottagarlistans FOW-mutation är också verifierad tidigare. Ingen ny tärningsregel.

## Vad spelarriggarna faktiskt visar

Båda slutarmar använder vanliga register/join/founding. För att hitta en nåbar
partner användes **7 respektive14 vanliga joins**, två av dem deltar med städer;
övriga är Nomadic Hosts. Ingen fixture eller flytt av koordinater i DB.
A skickar silver50 i Firefox, sedan livestock2 via Keryx med destinationsnamnet.
B:s bestående GiftDelivered-payload visar exakt 50 respektive2 krediterat.
Livestock **10→12** i båda; silver ökade **45.8** respektive **44.75** efter
övrig faktisk sold under färden (**−4.2**, **−5.25**), redovisat i proof.json.
B marscherar en riktig enhet och ger sentry-order; nästa livestock1-karavan
rånas faktiskt, med CaravanSeized och GiftLost: credited0/lost1/returned0.
Serverloggar har inga handlerfel/dead letters/ERROR. Audit-SQL är enbart läsning.

Rigg: `tools/gift_live.py OUT e8c5200f firefox FROZEN_WEB` (OUT innehåller byggd
Temenos/Keryx). `tools/gift_before.py` kör basclientens bilder. Alla egna
processer/PG/Redis tas bort. Motorerna installerades med Python-Playwright under
HOME för att matcha det installerade driverpaketet. WebKits saknade Ubuntu-ABI-
bibliotek hämtades/extraherades till `/home/tk/.cache/megaron-webkit-libs`, med
länkar i egen browsercache; systempaket ändrades inte.

## Synliga browserskillnader och avgränsningar

Alla tre visar hela mottagarnamnet, gåvomärkningen och båda gåvoradernas exakta
mängder, på desktop och mobil, utan horisontell overflow. Skillnader kvar:
WebKit har högre native select-fält och andra pilar; Firefox/Chromium har olika
nummer-spinners. Native hjälprutornas bakgrund/kanter, fontmått/rasterisering
skiljer sig något, vilket flyttar radbrytningar och vertikala positioner.
Rundade Transfer-hörn är borttagna. Funktionella assertions är lika i alla tre.
Playwright WebKit är motorbevis, inte ett fysiskt iOS/Safari-test.

Ägarbyte, cap, kollaps, hard delete, sjörån limped, idempotens och naval hemresa
bevisas med verklig PG/HTTP/scheduled/scan/arrival men kontrollerade fixtures;
**de är inte utförda som erövring/kollaps i spelarriggen**. Spelets befintliga
interceptionsutfall för captured/sunk är oförändrade och täcks av fullsviten.
Den gamla platta leveranstärningen finns kvar enligt uttrycklig S-order.
BILD väntar på Timothy; begripligheten i notiserna hör till TEXT/playtest.

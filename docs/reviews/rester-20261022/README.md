# Två rester kontrollerade 2026-10-22

**Slutsats 1:** De 13 hexarna är verkligt avskilda inom den spelbara kartan: en
kantvik, kort flod och källsjö. Detta är **korrekt enligt nuvarande kartregler**,
inte ett pathfinding-/kust-/deltaproblem. Generatorn räknar varje kantansluten
havsdel som giltigt mynninghav; den garanterar inte att alla sådana delar är
seglingsmässigt förenade. Vill man att varje hamn ska nå det stora havet behövs
ett uttryckligt nytt genereringskrav. Ingen sådan ändring föreslås här.

**Slutsats 2:** Todo-raden **”freighting-skepp äter ingen proviant” är inaktuell**.
Nuvarande kod lastar proviant vid avfärd och drar den ur skeppets eget lager
varje upkeep-tick även under `freighting`. Efter `a7151d3d` m.fl. återstår inte
den angivna luckan. Befintliga skepp har inte fått påhittad mat genom backfill.

## Underlag och avgränsning

Endast SELECT i `BEGIN READ ONLY` via befintliga `tools/alfa_db.py`; inga live-
ordrar, skrivningar, produktionsändringar, merge, push eller deploy. Terräng
läst ur DB, inte härledd ur dagens seed-generator. Frågorna kördes i separata
read-only-transaktioner; detta är inte en global samtidssnapshot.

- Värld `2178e795-1303-4cc3-865a-6386847d68d1`, 40×40, seed
  `1792505943667424487`; första fulla avläsningen 2026-10-22 15:41:21 UTC,
  tick 2812. 1 600 terränghexar. Följdfrågan visar processade upkeep 2811–2814.
- Faktisk `/healthz`: `commit=d6b0032`, `migration=198`, `status=ok`;
  DB migration 198, `dirty=false`. Serverns checkout är `5262aa6e…`.
  Processen rapporterar alltså föregående Go-build trots nya webbcheckouten.
  De tio lästa kärnfilerna är SHA256-identiska mellan processcommit d6b0032c,
  live-checkout och analysbas 5262aa6e: `source-comparison.json`.
- Ingen `freighting`-enhet fanns vid avläsningen; fyra aktiva skepp totalt
  (tre garrison, ett marching). Alla tio sparade sjötransportben var delivered.
  Därför påstås **ingen direkt före/efter-mätning av en levande fraktares mat**.
  Slutsats 2 är en genomgång av den faktiskt driftsatta kodens normala flöde.
- Klassning: mätning som bevisar geografi/logistik i kedjegrinden. Ingen ny
  BILD/TEXT-grind eller ny kanon. Inga produktionstester eller mutationer körda: ingen kod ändrad och live
  ska bara läsas. Befintliga tester lästa som stöd, inte redovisade som nykörda.

## 1. Pylos och komponenterna

Pylos är `(39,27)`, settlement `47d92957-6871-43e8-9a7e-91d934f7cf5c`.
Kontot heter **Talthybios**, Wanax-namnet i DB är **Theseus**. Hamnens enda
hexgranne som skepp får beträda är `(39,28)`.

Flood fill med de sex axiala grannarna `(1,0), (1,-1), (0,-1), (-1,0),
(-1,1), (0,1)`, över exakt de fyra typer som `province.isPassable(...,"naval")`
tillåter, ger tre komponenter: **1 204**, **13**, **1** hexar. Ingen FOW-filter,
äganderätt eller fart påverkar denna topologi.

| Del | Samtliga hexar i 13-komponenten |
|---|---|
| Källsjö, coastal_sea | (34,28), (34,29), (35,28), (35,29) |
| Flod, river | (36,28), (36,29), (37,29), (38,29) |
| Kantvik, coastal_sea | (39,28), (39,29), (39,30), (38,31), (39,31) |

Floden binder alltså verkligen sjön till viken. Ingen river_ford saknas i
räkningen; det finns ingen sådan i denna komponent. Kantviken möter q=39,
kartans sista kolumn. Ingen spelbar q=40-rad finns runt vilken man kan segla.

**Närmaste andra vattenkomponent är 1 204-komponenten**, vid `(38,33)`.
Minsta axiala hexavstånd är 2, från antingen `(38,31)` eller `(39,31)`:

| Kortaste förbindelse som skulle krävas | Mellanhexens verkliga terräng |
|---|---|
| (38,31) → (38,32) → (38,33) | forest_olive_grove, coastal=true, landmass 7 |
| (39,31) → (38,32) → (38,33) | samma |
| (39,31) → (39,32) → (38,33) | forest_olive_grove, coastal=true, landmass 7 |

`coastal=true` betyder strandnära **land**, inte segelbar terräng.
Båda mellanhexarna är olivlund. Det finns inget färdigt sund som pathfinding
förbiser. Deltahexarna `(38,28)`, `(38,30)` och Pylos `(39,27)` är också land,
men ligger inte i denna närmaste spärr. Även om delta felaktigt räknades som
vatten blev komponenten bara 16 hexar, fortfarande avskild.

**Varför räknas detta som tillåten geografi?**
`world.mainSeaComponent` (`mapgen.go:2843`) seedar flood fill från **varje**
havshex vid domänens kant, inte endast från en största sammanhängande komponent.
`firstSeaNeighbour` (`:2833`) tillåter en flodmynning i den unionen. Den femhexiga
viken uppfyller det villkoret. `placeSourceLake` (`:3119`) tillåter den lilla
sjön i andra änden. Den arkiverade flodplanens §7a föreskriver just flood fill
från kartans kant; World-Rim-planen definierar området utanför som ospelbart.
Ingen regel granskad här kräver att varje kantvik når varje annan kantvik.
Det är en geografisk begränsning som kan vara oönskad för en startstad, men inte
bevis för att kust eller flodmynning har brutit sitt nuvarande kontrakt.

Råhexar, alla gränsgrannar och samtliga närmaste par: `components.json`.
Oberoende rekursiv SQL mot live bekräftar exakt samma 13 hexar och minimiavstånd
2: `independent-sql-check.json`.

## 2. Fraktresans mat, steg för steg

| Steg | Faktisk kod och effekt |
|---|---|
| Bindning | `transport/carrier.go:100`, BindShip: garrison → freighting i avfärdens transaktion. |
| Alla nya fraktben | `transport.go:65` Dispatch och `:83` CreateShadow anropar båda prepareJourney. Transfer/standing använder Dispatch; trade/gift kan använda CreateShadow. |
| Proviantering | `transport.go:216` verifierar faktisk ägd avfärdshamn via FreightPort för utgående sjöben. `:242` beräknar tur/returbehov; stående rutt får även väntreserv till nästa makrotick. `:248` LoadProvisions. |
| Lastning, en gång | `economy/ship_load.go:22` lastar bara skillnaden mot befintligt lager: TakeFood vid den verifierade hamnen, sedan AddProvisions. Brist ger ProvisionShortage och avfärdstransaktionen rullas tillbaka. |
| Tickurval | `combat/upkeep.go:229` inkluderar uttryckligen freighting. main.go:234 registrerar UpkeepTick; upkeep.go:521 schemalägger nästa med MacroTickInterval=1. |
| Ranson | `economy/ship_ration.go:42` och UnitUpkeep: Galley 0,04 / Emporos (merchantman) 0,03 matenheter per tick och skepp. Naval ranson är oförändrad av status; freighting blir inte gratis. |
| Ombord under färd | `combat/upkeep.go:354`, provisionSourceFor (`:707`) väljer skeppets eget id för alla naval. eatProvisions (`food_provisions.go:19`) anropar economy.EatProvisions i en transaktion. |
| Beständig dragning | `economy/ship_ledger.go:59` planerar ransonen, drar från provision_goods och skriver både den och provisions; combat-adaptern committar vid full måltid. Då sätts grainNeed=0, så hamnen debiteras inte igen. |
| Slut på mat till sjöss | FreightPort ger nil när ett transportben är in_transit. atHomePort är då false, trots kvarvarande hemstads-id. Normal tom-proviantväg ger attrition, inte mat via fjärrhamnen. |
| Mellan ben | Skeppet kan fortsätta vara freighting. FreightPort accepterar endast faktisk aktiv ägd hamn utan pågående ben. Finns proviant äts den först; med tomt lager får just den hamnen mata skeppet. Främmande hamn/lost-ben ger inte sådan tillgång. |
| Retur | `prepareJourney` hoppar över ny lastning för return-ben: resterna seglar tillbaka. `transport/arrival.go:144` frigör vid färdig ship_return/damaged_return; ReleaseShip (`carrier.go:119`) sätter garrison och ReturnProvisions återför kvarvarande varor en gång. Trade har motsvarande returägare i economy/trade_return.go. |
| Stående rutt | `combat/standing_orders.go:512` förbetalar extra mat endast för landkaravanen; sjöbenet lastar via samma prepareJourney, ingen andra sjödebitering. |

Exempel ur sparade liveben: Galley `c082acf0-e271-4e87-90be-673146624975`
hade en gift-resa 2690→2694 och ship_return 2694→2698; båda delivered. Kodens
ranson är 0,04/tick och normal symmetrisk åttaticksresa behöver 0,32 ombord vid
avfärd. Det är **beräknat ur koden**, inte en återfunnen historisk lastningslogg;
DB sparar inte proviantens värde före/efter varje gammalt tick på unit-raden.

Lästa befintliga regressioner: `transport/freight_provisions_test.go`
TestFreightProvisionsDispatchAndShadowLifecycle (fyra utgående typer, lastning,
ombordmåltid, retur utan fjärrdebitering, dubbelfrigörande), shortage-rollback
samt TestFreightPortFollowsPhysicalLegNotHomeReference. De stöder kodläsningen;
ingen test eller order kördes mot live.

## Överlämning

Claude kan avföra just ”freighting-skepp äter ingen proviant” från restlistan.
Övriga punkter på samma todo-rad har inte granskats. Kartfrågan kan avföras som
routingbugg; om spelkravet i stället är att varje startstad ska ha havsförbindelse
är det ett separat beslut om spawn/mapgen. Ingen havshex eller stad ändrad.

Revisionsbas: `5262aa6e`. Bevisfilernas kontrollsummor: `SHA256SUMS`.

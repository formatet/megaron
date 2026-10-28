# Flod i dimkant och flödesriktning

Kontrakt före kod: en känd flod fortsätter ut genom rätt kanter även när grannen ligger i dimman; känd källa/mynning får ingen påhittad fortsättningsgren. Riktning visas från faktisk grafdata, med text i hex/Keryx och två–tre pixelalternativ för BILD. Bevisar geografigrinden genom att en scoutad flodbit går att läsa.

Bas f36e992f. Renderer bygger grenar enbart från kända vattengrannar och har en syntetisk rak fåra när ingen granne syns. Servern sparar terrain men inte mapgens ordnade source→mouth-path eller höjd; source lakes skrivs som coastal_sea. Seed har ingen generatorversion. Faktisk gammal värld inventeras innan ett eventuellt kanonval om tvetydiga riktningar behövs.

FOW-kontrakt enligt Timothys precisering28okt: kant/riktning och sort på den anslutande vattenhexen framgår på känd flod, likt bränningar visar havets granne. Namn, resurser och enheter i osedd granne döljs fortsatt. API river_edges använder samma sex axialoffsets som renderer. Explicit tom lista betyder faktiskt inga vattenanslutningar. Metadata sitter endast på känd flod; dimhexen själv är fortsatt fog utan dessa fält. Riktning/kanter sparas i221 som små bitmasker, produceras vid generering och återfylls atomiskt en gång för äldre kartor. Map gör ingen riktningstopologi på FOV-läsning. Anslutande hextyp läses från den redan sparade grannterrängen utan duplicerad terrängägare. Workerbakning och fallback tar samma godkända kantdata och invalidation.

Prov: assertion-röd kant i dimman; ändpunkt och mynning; API dimgranne med hemliga data förblir fog och nollad; uppdaterade workerindata/cacheprov; faktisk Firefox1280/390 källa/mynning/dimkant samt Keryx. Full fresh Go/JS/vet, fysisk mutation och deployhash/ground/assets före klart.

## Slutlig implementation

Migration **221** (220 fanns redan live). `world.PopulateRiverMetadata` äger geografin; alla tre tile-writers sparar maskerna i samma INSERT. Befintliga kartor kompletteras i en atomär startup-transaktion före arbetare/HTTP. Redan sparade värden skrivs aldrig över. Nuvarande värld inventerades läsande: samtliga32 flod/vad-hexar har bevisbar riktning med befintlig hav+delta-geografi. Osupporterade grafer får explicit okänd riktning, ingen gissad pil. Befintlig sjöterräng och balans ändras inte.

`Map` avkodar sparade bitmasker och filtrerar per känd hex. Endast anslutningarnas kant, riktning och vattenterräng får passera dimgränsen enligt Timothy; grannens faktiska API-post är fortsatt fog utan metadata, namn, fyndigheter eller styrkor. Anslutningstyp har en enda ägare: sparad grannterräng. En sparad riktning som avsiktligt skiljer sig från härledd topologi bevaras vid FOV-läsning och startup.

Webben använder samma kantlista i raster/fallback/worker; explicit tom lista ger inget påhittat genomlopp. Strömpilar är hela pixlar, ett litet nedströmstecken per utloppskant (alternativ A). Hextext och Keryx `map` ger riktning, fortsatt dimkant och anslutningstyper. Codex beskriver samma kontrakt. Äldre terrängartiklars icke-existerande `keryx hex`-hänvisning rättades till verkliga `keryx map`.

## Bevis

- [Native Firefox](native/proof.json): egen färsk PG/Redis/värld, riktig egen spanare på stranden; källa, vad/dimkant och mynning, två omgångar1280/390, riktig API och Keryx, källhashar oförändrade under fångst. Fixtur-SQL är loggad. Inga ersatta DOM/API-svar.
- [Tre bildalternativ](variants.html), [samlad bildgranskning](index.html). Implementerat A; Timothy kan korrigera efter merge.
- [Verklig create-world och221→220→221](cli-writer/proof.json):15/15 floder med sparade värden; noll på annan terräng; gammal kartdata bytehash-oförändrad. API-writer provas direkt och serverns ensureWorld av native-riggens kontroll före fixtur.
- [Åtta fysiska mutationer](mutants/proof.json): startup skriver över sparat värde; mynning utan delta; writer tappar metadata; FOW-filter borttaget; renderer tappar godkänd dimkant; klippning döljer godkänd fåra; pil pekar uppströms; FOV räknar om sparad riktning. Samtliga assertion-röda och återställt gröna på färsk DB där relevant. Varje arm har separat logg.
- [836 JS](js.log), Go vet och fyra tidsvakter samt Forces-vakt gröna. [Hel färsk Go-svit221](go-full-fresh.log) grön, inklusive world156,246s; [Liveverifiering](deploy-proof.json):1843878f, ren221,32/32 sparad riktning, oförändrad gammal kartdata/grundtillstånd,21 byte-exakta statiska filer.

En helkörning hittade jämförelseprovet för rökelse: förväntad karta behövde samma nya metadata-pass för att fortsatt bevisa att enbart rökelseflaggan ändras. Nästa helkörnings databas dödades externt (Docker kill/die137); den räknas inte som produktfel eller grönt bevis och ersätts av en helt ny körning.

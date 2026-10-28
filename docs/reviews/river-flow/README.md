# Flod i dimkant och flödesriktning

Kontrakt före kod: en känd flod fortsätter ut genom rätt kanter även när grannen ligger i dimman; känd källa/mynning får ingen påhittad fortsättningsgren. Riktning visas från faktisk grafdata, med text i hex/Keryx och två–tre pixelalternativ för BILD. Bevisar geografigrinden genom att en scoutad flodbit går att läsa.

Bas f36e992f. Renderer bygger grenar enbart från kända vattengrannar och har en syntetisk rak fåra när ingen granne syns. Servern sparar terrain men inte mapgens ordnade source→mouth-path eller höjd; source lakes skrivs som coastal_sea. Seed har ingen generatorversion. Faktisk gammal värld inventeras innan ett eventuellt kanonval om tvetydiga riktningar behövs.

FOW-kontrakt enligt Timothys precisering28okt: kant/riktning och sort på den anslutande vattenhexen framgår på känd flod, likt bränningar visar havets granne. Namn, resurser och enheter i osedd granne döljs fortsatt. API river_edges använder samma sex axialoffsets som renderer. Explicit tom lista betyder faktiskt inga vattenanslutningar. Metadata sitter endast på känd flod; dimhexen själv är fortsatt fog utan dessa fält. Riktning/kanter sparas i221 som små bitmasker, produceras vid generering och återfylls atomiskt en gång för äldre kartor. Map gör ingen riktningstopologi på FOV-läsning. Anslutande hextyp läses från den redan sparade grannterrängen utan duplicerad terrängägare. Workerbakning och fallback tar samma godkända kantdata och invalidation.

Prov: assertion-röd kant i dimman; ändpunkt och mynning; API dimgranne med hemliga data förblir fog och nollad; uppdaterade workerindata/cacheprov; faktisk Firefox1280/390 källa/mynning/dimkant samt Keryx. Full fresh Go/JS/vet, fysisk mutation och deployhash/ground/assets före klart.

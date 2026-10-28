# Flod i dimkant och flödesriktning

Kontrakt före kod: en känd flod fortsätter ut genom rätt kanter även när grannen ligger i dimman; känd källa/mynning får ingen påhittad fortsättningsgren. Riktning visas från faktisk grafdata, med text i hex/Keryx och två–tre pixelalternativ för BILD. Bevisar geografigrinden genom att en scoutad flodbit går att läsa.

Bas f36e992f. Renderer bygger grenar enbart från kända vattengrannar och har en syntetisk rak fåra när ingen granne syns. Servern sparar terrain men inte mapgens ordnade source→mouth-path eller höjd; source lakes skrivs som coastal_sea. Seed har ingen generatorversion. Faktisk gammal värld inventeras innan ett eventuellt kanonval om tvetydiga riktningar behövs.

FOW-kontrakt: endast kant/riktning på redan känd flodhex; inga terrängslag, namn, resurser eller enheter på den okända grannen följer med. API river_edges använder samma sex axialoffsets som renderer. Explicit tom lista betyder faktiskt inga vattenanslutningar. Metadata får inte nå dimhexen själv. Workerbakning och fallback tar samma godkända kantdata och invalidation.

Prov: assertion-röd kant i dimman; ändpunkt och mynning; API dimgranne med hemliga data förblir fog och nollad; uppdaterade workerindata/cacheprov; faktisk Firefox1280/390 källa/mynning/dimkant samt Keryx. Full fresh Go/JS/vet, fysisk mutation och deployhash/ground/assets före klart.

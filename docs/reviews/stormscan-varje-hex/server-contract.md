# Serverkontrakt: stormscan varje hex

| Fält | Kontrakt |
|---|---|
| Problem | Tickprov missar stormar på mellanliggande sjöhexar hos snabba skepp och sammanblandar olika storm-ID. |
| Spelarsanning | Alla sjöhexar på sparad färd utsätter skeppet för verkliga stormar, med samma risk för alla ärenden. |
| Invariant | En sammanhängande passage genom samma storm-ID ger ett slag; olika ID ger egna slag. Frysta rutter och befintlig skada/täthet/drift ändras inte. |
| Scope | `sea_storm.go`, stormtester och RÖ1-simuleringsverktyget; ingen migration. |
| Non-scope | Balansändring, FOW, ny notis eller grafik. |
| Acceptans | Mellanhex träffas; samma ID fortsätter utan upprepad skada; rörlig storm träffar långsamt skepp; Stillhet och dubbelscan fungerar; sen scan motsvarar dagliga scaner. |
| Stopvillkor | Konflikt med sparade rutters auktoritativa tidsmodell eller kanon som inte avgjorts i ordern. |
| Bevisplan | Färsk combat-baslinje, riktiga DB-scaner, riktade mutationer röd→byte-identisk återställning→färsk grön, parade simuleringar med oförändrade seeds och tal. |

Grind: bevisar kedjegrinden genom sjöfartens fysiska transport- och förlisregler. FUNKTION; ingen BILD.

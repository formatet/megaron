# Kanonfråga: varifrån kommer nästa sjöavgångs proviant?

Status: BESVARAD av Timothy2026-10-28: ”båtar avgångshamnen, alltid hamnen de ligger i”. Missionsproviant rättad och verifierad LIVE `d09a74af`; support/upkeepankare behålls. Nedanstående inventering är den historiska frågan före beslutet. Inga spelregler eller artiklar ändrade i denna slice. Bas `f9926f57`.

Efter att ett skepp dockat i en annan egen stad har det fysisk hamn där men behåller sin tidigare försörjningsstad.

| Yta | Faktiskt beteende | Belägg |
|---|---|---|
| Ny vanlig seglings-/missionsorder | Maten tas från `SupportSettlementID`, alltså den gamla försörjningsstaden. | `server/internal/combat/march_start.go` avsnittet skeppsproviant; `TestCityLandingAllPassengersStayAndSupportUnchanged` i `land_city_test.go` beställer nästa resa och kontrollerar minskat lager i gamla staden. |
| Ny fraktavgång | Maten lastas från `OriginID` i skeppets verkliga avgångshamn. | `server/internal/transport/transport.go:prepareJourney`, kontrollen `FreightPort == OriginID` och `LoadProvisions(...OriginID...)`; befintliga `freight_provisions_test.go`. |
| Normativt beslut25okt | Fysisk hamn ändras, support behålls och gamla staden levererar nästa avgångs proviant. Det står inte uttryckligen hur detta förhåller sig till fraktägaren. | Vault `temenos_enheter.md` §Timothy2026-10-25 — landsättning i egen stad; `docs/reviews/landsatt-i-stad/README.md` §BeslutA. |
| Nuvarande wiki | sea.md säger generellt att gamla staden försörjer nästa resa, samtidigt som fraktartiklar beskriver lastning vid avgång. | `sea.md` avsnitt Carry troops to your own city och At sea; `fact-inventory.json` markerar källblocket `canon-review-needed`. |

Ingen bugg eller ny regel antas. Frågan gäller om skillnaden är avsiktlig och därmed ska läras ut, eller om en enhetlig princip kräver en separat mekanikslice.

1. **Rekommendation: behåll och beskriv dagens skillnad.** Inga motorändringar; separata sakartiklar anger vilken order som tar mat var.
2. **Alltid faktisk hamn.** Vanliga missionsorder ändras jämfört med beslutA och befintlig regression; försörjningsankare för löpande underhåll behöver inte flyttas.
3. **Alltid försörjningsstaden.** Fraktens lastningsägare och dess fysiska hamnkontrakt ändras; mat kan tas från en annan stad vid avgång.

Oberoende inventering är sparad:65artiklar/634faktablock plus katalogägarens kodinventering. Nästa handling efter svar: lås proviantartikelns fakta och bygg katalogtäckning/rankad aliassök/kompakta sakartiklar utan att tappa gamla fakta.

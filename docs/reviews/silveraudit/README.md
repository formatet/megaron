# Komplett silverrevision

Kontrakt före kod: världens ägda och fysiskt reserverade silver måste räknas
en gång, oavsett om det finns i stad, host, kolonipung, offerescrow eller last.
SilverAuditV2 använder verkliga realiseringskrediter för gruvproduktion;
SilverAudit V1 läses och emitteras med sin gamla innebörd. Scope: intern
revision, realiseringsinstrumentering/migration, analysverktyg och prov.
Ingen ny spelarnotis eller publik förmögenhetslista (FOW oförändrad).
Acceptans: en verklig bevarandesekvens, carried-mutation rött/byte-grönt,
färsk Go/vet, kompatibilitet och migration, merge/deploy/healthz.
Kanonkonflikt parkeras i BESLUT; andra köarbeten fortsätter.

Baslinje c7b9abad/mig178: färsk TestUpkeepSilverBookkeeping grön.

Inventering: combat/silver_audit.go V1 räknar bara aktiva ägda städer +
pending BUY, och uppskattar mined som aktuell rate×elapsed. Tillkommande
fysiska poster: active founder_phase (founded kopia ej pengar), levande
units.carried_silver (disbanded förlorad), transport_goods på in_transit
transport (levererad/intercepted/lost kopia ej pengar), pending SELL med
silver som själva varan, samt accepterad SELL:s reserverade silverretur
i scheduled_events.then_return medan första transporten är in_transit.
Efter leverans ligger samma pengar i returmanifestet och reservationen
ska därför inte räknas igen. Neutral stadsstock redovisas som ägarlös,
medan sunk/collapsed städer är förlorade/inaktiva liksom V1.

Status: under implementation; ej mergad/deployad.

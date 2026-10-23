# Flodtransport — avgränsat integrationsprov

Kontrakt före slutverifiering: en fysisk varutransport mellan två egna flodstäder ska segla en frusen flodrutt genom ett vadställe, krediteras en gång och återföra/fritställa samma skepp först efter returen. Utan skepp och utan landväg ska HTTP-order avvisas utan debitering eller fysisk transport.

Scope: nytt handlerintegrationsprov och bevis. Ingen ändring av spelmodell, terräng, ransoner eller transportmotor. Provet bevisar en del av kedjegrinden, inte naturlig geografi→brons→elit för en människa.

Metod: färsk DB på aktuell migration. Riktig HTTP-handler för order, riktiga schemalagda payloads och delivery/arrival-handlers vid sparade due_tick-gränser. Sparad resa måste innehålla vadstället. Leverans och retur spelas båda om för att kontrollera idempotens. Ingen väntan på bakgrundspollern, ingen browserklickad transport och ingen naturlig stadsgrundning påstås.

Resultat: båda nya namngivna handlerprov gröna på färsk migration205. Fysisk mutation som tar bort naval passage för river_ford gav HTTP422 och TestTrade_RiverFordRoundTrip RÖTT. pathfind.go återställdes byte-exakt i finally; riktad återkörning inklusive befintlig sjöretur och samtidiga överföringar GRÖN. Se targeted.log, mutation.log och restored.log. Hela Go-sviten körd EN gång mot färsk migration205: 26 paket gröna, world154.537s (full-go.log). go vet ./... grönt.

Stoppvillkor: mekanikfel eller kanonkonflikt ska först reproduceras och avgränsas; ingen balansändring i verifieringen.

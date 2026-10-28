# Handelsbud:24ticks från mottagandet

Kontrakt före kod: nya bud får24ticks svarstid efter ankomst; samma deadline för offert och budets väntan. Redan schemalagda ScheduledOfferExpiry behåller lagrad deadline även om budet anländer efter deploy eller dess sjöpassage löses på nytt. Ingen migration eller ändrad OfferExpired-notis. Bevisar kedjegrinden genom fysisk handel med kortare escrowbindning.

Bas54b4b45e/produktionwiki ecd224bd. Före: konstant168; web/Keryx läser redan serverns absoluta deadline utan egna168tal. Passageomplanering kunde skapa ytterligare expiry-timer medan den första fortfarande kunde förfalla offerten. Därför väljer denna slice befintlig pendingdeadline som ägare i stället för att lägga till en ny.

Fyra ytor: server24+bevarade sparade deadlines; webb Letters visar den faktiska klocktiden; Keryx outbox visar expires; Codex anger24game days och gamla bud behållerdeadline. Spelarytornas dynamiska klockvisning ska inte hårdkoda ett ersättningstal.

Acceptans `738fb362`: två verkliga Keryx-köp/säljbud i ny isolerad värld, sparad deadline exakt24ticks/24timmar vid3600s/tick, därefter faktisk Arrival-worker och stay==expiry. In-/outbox och Letters deadline/Accept/Decline på1280/390 två gånger; BILD [index.html](index.html), oförändrad produktionskälla med hashlista. Timerproven täcker även gamla168deadlines vid sen ankomst och ändrad passage. Tre fysiska mutanter fångar168återställning, ignorerad sparad väntan och ignorerad sparad expiry, var och en med färskt grönt efter byte-exakt restore.

Kod: full färsk Go +827JS +vet +fyra dayguards. Första nativeförsöket behövde stöd för nuvarande MessengerArrivalV2 i provfrågan; nästa behövde räkna även knappen i hopfälld brevpost. Produktionskoden ändrades inte för dessa riggfel. Slutbeviset i native/proof.json kommer från komplett ren omkörning. Ingen ny API-yta eller händelsetyp.

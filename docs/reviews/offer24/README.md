# Handelsbud:24ticks från mottagandet

Kontrakt före kod: nya bud får24ticks svarstid efter ankomst; samma deadline för offert och budets väntan. Redan schemalagda ScheduledOfferExpiry behåller lagrad deadline även om budet anländer efter deploy eller dess sjöpassage löses på nytt. Ingen migration eller ändrad OfferExpired-notis. Bevisar kedjegrinden genom fysisk handel med kortare escrowbindning.

Bas54b4b45e/produktionwiki ecd224bd. Före: konstant168; web/Keryx läser redan serverns absoluta deadline utan egna168tal. Passageomplanering kunde skapa ytterligare expiry-timer medan den första fortfarande kunde förfalla offerten. Därför väljer denna slice befintlig pendingdeadline som ägare i stället för att lägga till en ny.

Fyra ytor: server24+bevarade sparade deadlines; webb Letters visar den faktiska klocktiden; Keryx outbox visar expires; Codex anger24game days och gamla bud behållerdeadline. Spelarytornas dynamiska klockvisning ska inte hårdkoda ett ersättningstal.

# Recall all — enkel klientloop

Kontrakt före kod. Problem: återkalla flera egna marscher kräver ett klick/anrop
per enhet; grupperingsformen är förkastad. Spelarsanning: EN Recall all-knapp
och CLI recall --all skickar samma vanliga recall till varje marscherande egen
enhet, en kurir per enhet enligt serverns befintliga regler. Invariant: ingen
ny serverkod/rutt/filter/eventsemantik och inget tyst bortfall vid avslag.
Scope webbknapp/loop i War, CLI-flagga/loop, en Codex-mening, verblisterad och
prov/egen rigg. Non-scope parkerade aggregat-/batchgrenar, backend, ny design.
Acceptans: alla marscherande egna får försök, avslag namnges, en kort summa,
ingen aktiv knapp utan marscher, befintlig singel-recall bevarad; web/CLI
faktiska audits+garnison på varsin ny DB; tester/mutation/fullsvit/vet/JS gröna.
Stoppa vid canonkonflikt eller behov av serverändring (rapportera till Claude).
Bevisar kedjegrindens nåbara order/återkomstyta. BILD för Timothy senare,
TEXT i playtest. Bas masterb353d54f; egen gren codex/recall-all-enkel.

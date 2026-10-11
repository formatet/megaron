# Flod — doparrätt

Bas c71bf82f, codex/flod-doparratt.
Problem: färdiga flodnamn ersätter felaktigt spelarens rätt att döpa floden.
Spelarsanning: första ägda fysiska förflyttaren som når kedjan ger en bestående engångsrätt, synlig och användbar på fyra ytor.
Invariant: synlighet ger ingen rätt; en kedja/ankare har en rättighetshavare; okända hexar läcker varken namn eller rätt.
Scope: statisk flodidentitet, additiv migration, gemensam fysisk inträdesregistrering, API, CLI, stormlik webbnamnruta, Codex/notis/verblista/Umami.
Non-scope: flodgenerering, rörelsebalans, namnfilter utöver stormreglerna, backfill av gamla inträden.
Acceptans: första inträde+omkörning; behörighet/engång/unikhet/admin; FOW; fyra ytor; färsk full Go/JS/vet, tre mutationer, Firefox1280+390 verklig namngivning.
Stopvillkor: rörelsens faktiska ägarkarta motsäger orderns enda skrivsteg; rapporterat till Claude före rörelseimplementation.
Bevisplan: oförändrad färsk baslinje, DB/API-lifecycle, fysisk mutation, isolerad rigg med vanlig förflyttning och användarnamn, bilder.
Grind: bevisar geografi-ytans handling och återkoppling; själva namngivningen ändrar inte brons/elit-regler.

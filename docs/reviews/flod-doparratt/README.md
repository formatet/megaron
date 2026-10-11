# Flod — doparrätt

Bas ursprung c71bf82f, rebasad origin/master 0db368c5 (inkl bb49ef0d), codex/flod-doparratt.
Timothys tillägg 22:40: generiskt ortnamnssubstrat, ingen separat flodmodell.
Problem: färdiga geografiska namn ersätter felaktigt spelarens rätt att döpa floden.
Spelarsanning: första ägda fysiska förflyttaren som når ett geografiskt objekt ger en bestående engångsrätt, synlig och användbar på fyra ytor.
Invariant: synlighet ger ingen rätt; kind+kanoniskt ankare har en rättighetshavare; okända hexar läcker varken namn eller rätt.
Scope: statisk platsidentitet, additiv migration, gemensam fysisk inträdesregistrering, API, CLI, stormlik webbnamnruta, två kartetikettvarianter ?ortnamn=a|b, Codex/notis/verblista/Umami.
Non-scope: flodgenerering, rörelsebalans, namnfilter utöver stormreglerna, backfill av gamla inträden.
Acceptans: första inträde+omkörning; behörighet/engång/unikhet/admin; FOW; fyra ytor; färsk full Go/JS/vet, tre mutationer, Firefox1280+390 verklig namngivning.
Stopvillkor: rörelsens faktiska ägarkarta motsäger orderns enda skrivsteg; rapporterat till Claude före rörelseimplementation.
Bevisplan: oförändrad färsk baslinje, DB/API-lifecycle, fysisk mutation, isolerad rigg med vanlig förflyttning och användarnamn, bilder.
Grind: bevisar geografi-ytans handling och återkoppling; själva namngivningen ändrar inte brons/elit-regler.

Generisk del ecc0a68f: PlaceNames API/identity DB-tester gröna på färsk mig172. Rörelseintegration återstår; denna checkpoint är INTE en färdig leverans.

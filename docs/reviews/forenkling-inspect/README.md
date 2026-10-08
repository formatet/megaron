# L Inspect — BILD

## Slice-kontrakt
Problem: främmande stad visar Culture/Walls-block/DP; Host förråd blandar tick och verklig tid.
Spelarsanning: owner + Defence i ord; Host visar food/pay i game days och tal i ord.
Invariant: samma dataåtkomst/FOW och alla kontroller; inga nya mekaniska försvarströsklar, ingen silver→mat-förväxling.
Scope: map.html/map.js paneltext, Codex och prov/rigg/BILD.
Non-scope: server/keryx/CSS, fow/renderer/ordrar, andra förenklingsslicer.
Acceptans: Culture/Walls/DP bort; Defended om redan visade murar eller positiva redan visade markförsvarare, No defenders seen om båda känt noll, Defence unknown när underlag saknas; owner/allied bevarat; controls/FOW bevarade; finite/infinite förråd från ticks_left utan skalning.
Stopvillkor: kanon/serverändring krävs → Claude. Hash och stopp efter EN slice.
Bevisplan:422JS renbaslinje, actual map-klick regression först och fysiska mutationer; riktig fresh PG16/Redis register/join/Host+kontaktstad via vanliga API:er, desktop390 före/efter BILD; full fresh Go/vet. Bevisar geografi→brist-grindens panelnåbarhet.

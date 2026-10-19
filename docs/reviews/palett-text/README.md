# Läsbara textfärger — slice 23

## Kontrakt före kod

Problem: positiva värden på papyrus är svåra att läsa. Spelarsanning: produktion,
framgång och status ska kunna läsas med normal textstorlek.
Invariant: ljusa signaler som fyllning, kant och ikon behåller sina färger.
Scope: CSS-texttoken, deras befintliga textkonsumenter, JS-kontrasttest och Firefoxbevis.
Server, keryx, Codex-innehåll och spelregler berörs inte; detta är webbtypografi.
Acceptans: alla `*-text`-token ≥4,5:1 mot bg-panel; befintliga funktionstester gröna;
bara textdeklarationer byter token; Firefox 1:1 före/efter; beslut visas separat.
Stopp: danger/diplomacy och text-dim får bara illustrerade förslag, inget produktionsbyte.
Bevisplan: oförändrad CSS-baslinje, JS-test, fysiska mutationer, Firefox på riktiga
spelytor i färsk isolerad värld samt komplett selectorprov för sällsynta texttillstånd.
Kedjegrind: bevisar läsbar produktion och orderstatus på vägen brist → brons.

## Verifierad baslinje (master 07e2eac4)

| Påstående | Faktiskt resultat |
|---|---|
| bg-raised/warm-white/border/border-in dubblerade | Falskt: en definition vardera i :root, ingen borttagning |
| safe som text har cirka 1,8:1 | Sant: 1,8014:1 |
| trade/city/oikos/system behöver textvariant | Sant: 2,1552 / 4,4968 / 4,1340 / 4,4844:1 |
| danger och diplomacy är båda lila | Sant, men kontrast mot panel är 7,1177 / 5,2189:1; semantiskt beslut |
| text-dim är orangebrun | Sant; 5,4166:1, läsbarhetstest kräver ingen ändring |

Kontrast avser WCAG sRGB relativ luminans, orundad kvot mot #F2EAD3.
Det mäter färgkontrast, inte textstorlek, begriplighet eller färger på alla bakgrunder.
Trade-accentens befintliga enda teckenkonsument är vågikonen i diplomatin; ikonen behålls.
Oikos/system har inga direkta textkonsumenter i dagens CSS/JS; token förbereds men inga
chipkanter, prickar eller bakgrunder byts. City används som text för "carrying unit".

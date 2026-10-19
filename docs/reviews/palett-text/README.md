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

## Resultat och bevis

Produktion **8e420c4d**. Fem textvarianter införda. safe-text 4,9314:1,
trade-text 4,9819:1, city-text 5,3660:1, oikos-text 5,0937:1 och system-text
5,0567:1 mot panel. Befintliga textkonsumenter för safe och city byter token.
Vågikonen, templets ensamma bock, arbetsprickar, chipbakgrunder och kanter behålls.
Inga balans-, server-, keryx- eller Codex-förändringar.

| Grind | Bevis |
|---|---|
| Kod | Ursprungliga 650 JS-test gröna (`baseline-suite.log`); kontrakttestet namngivet rött på baslinjen (`baseline-test.log`), slutligen 652 gröna (`final-suite.log`). Ingen Go-/SQL-ändring, därför ingen Go-/DB-testsvep. |
| Mutation | `python3 docs/reviews/palett-text/verify.py`: fem fysiska färgmutationer, tokennamngivet RED → byte-exakt återställning → fresh GREEN; `mutations.json`. |
| Inneslutning | `containment.json`: CSS byte-identisk mot bas efter enbart borttagning av nya token och återbyte av textreferenser; samma för varje JS-produktionsfil. danger/diplomacy/text-dim och ikoner/yta förblir alltså identiska. |
| Visuell | `browser.py`/`browser.json`: Firefox DPR1/390, alla 15 CSS-textselektorer; geometri, synliga kanter och fyllningar identiska före/efter. Den första kontrollen jämförde även osynliga `currentColor`-kanter med bredd 0; proben rättades till synliga kanter, ingen produktionsändring. |
| Användare | `native.py`: två helt färska världar, riktig register → CLI login/join/found/status/city → fyra riktiga webbpaneler. Ingen SQL-skrivning eller API-replay. `native-a/proof.json` och `native-b/proof.json`: processens healthz commit **8e420c4d318280913e1b7a72f37c75b571548c24**, migration **189**, status ok; Firefox DPR1/1280+390, noll pageerrors/overflow. |
| Semantik | Bara färgreferenser, inga värden, ord, order eller flöden förändras. Nativebilder visar faktiska positiva värden i City/Economy. Kult/War är öppningsprov; de färska städerna har ännu inga positiva ritual-/ordermeddelanden. Sällsynta tillstånd täcks av selectorprov och befintliga funktionstester, inte påstått native-spelade. |
| BILD/drift | Byggargranskat vid 1:1. Timothy bedömer färgval före merge genom Claude. Ingen merge/push/deploy utförd. |

Förebilderna använder samma verkliga DOM och data med de gamla textfärgerna
återställda via två CSS-variabler. De är ingen separat äldre serverkörning.
Första nativebilderna hade hjälppaneler/positiva tal utanför utsnittet; båda rena
världarna kördes om med hjälppanelerna stängda och ändrade värden i bild.
Slutbeviset är `native-a`/`native-b` från dessa två fullständiga omkörningar.
Bara offentliga proof-/CLI-utdata och bilder har kopierats, inga kontokonfigar eller token.
Kontrastkravet här gäller bg-panel; det är inte en revision av alla befintliga färger,
storlekar, ikonkontraster eller alla andra bakgrunder.

## BESLUT till Timothy — inte implementerat

Full inventering av faktiska konsumenter: [uses.md](uses.md). danger används i
fel-/förlust-/kollapsstatus, destruktiva knappar, hungersignaler och markeringar.
diplomacy används för domänens chipkant/prick och dess ljusa chipbakgrund.
text-dim används brett för metadata, etiketter, hjälprader, tabellrubriker och sekundär text;
dessutom vissa kanter/fyllningar. Ett globalt byte påverkar därför mer än brödtext.

[Nuvarande bild](decision-after.png) och [beslutsförslag](decision-decision.png)
är Firefox 1:1, 390 px. Galleriet visar dem också bredvid varandra.

- **Danger/diplomacy:** A behåll de två lila färgerna; B låt danger använda befintlig blood-red,
  diplomacy kvar lila. B särskiljer domänerna men för danger närmare krig/blod och ändrar
  alla dess kanter/fyllningar också. Rekommendation: bedöm B i bilden innan något byte.
- **text-dim:** A behåll varm orangebrun; B neutral varmgrå `#67635C` i beslutsbilden.
  B gör metadata lugnare men tar bort en del av den varma karaktären. Rekommendation:
  välj B om orangebrunt upplevs som en signal; kontrastfelet mot papyrus kräver inte bytet.

Dessa alternativ bor bara i review.html:s `mode=decision`, aldrig i produktions-CSS.
Claude tar besluten och BILD till Timothy; inga vault-regler ändras här.

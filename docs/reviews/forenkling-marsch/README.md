# H — gemensam marschmeny

Bas: `codex/recall-all-enkel` @ba96c79b (Claudes styrning 21:59). Bevisar geografikedjans nåbara orderyta. BILD väntar Timothy i H–K-bunten; ingen merge/push/deploy.

| Kontrakt | Innehåll |
|---|---|
| Problem | Kartmenyn börjar med inga enheter; War kräver separat Q/R-inmatning. |
| Spelarsanning | Första valda gruppens lediga enheter är förvalda; War → March väljer en enhet och ett kartmål i samma meny. |
| Invariant | Befintliga per-enhetsrutter, FOW, redirect, explore, kolonisering och lastat skepps landstigning bevaras. |
| Scope | marchctx.js, war.js, main.js, state.js, number_words.js (gemensamt spelarprosaformat), kartans målklick, map.html; funktionstester och egen isolerad webbrigg. |
| Non-scope | Server, CLI, kanon, War-kortens övriga handlingar (I), nya färger/CSS-klasser. |
| Acceptans | Alla lediga i första gruppen förvalda; övriga grupper noll. More gömmer längd/namn/stance. War filtrerar exakt vald enhet. Landstigning/kolonilast har samma body. Mobil har normalt målklick. |
| Stopvillkor | Om förmågan kräver regeländring, fråga Claude/Timothy; bygg övrig UI oberoende. |
| Bevis | Oförändrad bas actual health/mig160, desktop/390px före/efter, JS kontraktsprov, fysisk mutation, färsk spelarresa med riktiga POST och read-only audit. |

Resume: kontrakt före kod; baslinjerigg byggs. Inga produktionsfiler ändrade ännu.

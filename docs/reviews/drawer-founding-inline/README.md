# Economy→City och grundning inline — BILD

## Slice-kontrakt

Problem: Economy ligger kvar ovanpå City; grundning blockerar med en browserdialog.
Spelarsanning: stadslänken visar City ensam; grundning kräver ett andra uttryckligt klick bredvid handlingen.
Invariant: ingen grundningsorder vid första klick/avbryt; högst en samtidig POST; samma endpoint, body, regler och reload.
Scope: economy.js, map.js, gemensam inline_result.js, Codex och reproducerbara prov.
Non-scope: andra drawer-vägar, regler/API/server/keryx/CSS och reservkön.
Acceptans: rätt province-ID, Economy stängs före City; varningen om permanent upplöst Host; avbryt skickar inget; dubbelskick spärras; serverfel läsbart inline.
Stopvillkor: server-/kanonändring behövs → Claude. En slice, hash och stopp.
Bevisplan: ren419-JS-baslinje, regression först, fysiska mutationer, full fresh Go/vet, verklig register/join/webbgrundning→Economy→City i separata färska PG16/Redis-armar desktop/390, BILD.
Två symptom i samma slice enligt Claudes uttryckliga order: återstående avbrott i klientens handling→vy-flöde; inga domänändringar. Bevisar nåbarheten i geografi→brist-kedjan.

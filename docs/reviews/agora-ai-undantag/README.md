# Slice10 — AI-konton utan Agora

Bas9c071100, gren `codex/agora-ai-undantag`. Claude granskar/integrerar; hash+STOPP.

| Kontrakt | Innehåll |
|---|---|
| Problem | AI-stadägare får automatiskt ett mänskligt OOC-chattkonto. |
| Spelarsanning | Undantagna konton har inget tillgängligt chattkonto och ingen lösenordsväg. |
| Invariant | players.agora_exempt stoppar provisionering, status och lösenord; vanliga konton fungerar oförändrat. |
| Scope | En migration/backfill, Agora-orchestrering/API, CLI/webb/Codex, bevis. |
| Non-scope | Livekonton/Matrix-avaktivering, GPT-grundning och restfolkets regel. |
| Acceptans | Undantagen stadägare/skadad äldre ready/reservation får ingen reconcile/remote/password/notis; vanlig spelare fungerar; prefixbackfill korrekt och nya konton defaultfalse; fyra ytor. |
| Stopp | Krävs ändrad spelarregel utanför uttryckligt AI-undantag. |
| Bevis | Oförändrad runtime på nytt schema visar felet; färska PG16-prov, fysiska mutationer, fullfreshGo/vet/JS och isolerat användarprov. |

Blockerar GPT-speltestets ingång till kedjegrinden. Explicit flagga; migrationen markerar befintliga
luna-/gpt-konton en gång, inte en ny automatisk prefixpolicy i varje anrop. Framtida AI-konton ska
markeras av administratören före grundning. Gamla Matrix-konton avaktiveras av Timothy separat.

## Resultat och baslinje

`baseline.log`: oförändrad9c071100-runtime, med den nya kolumnen tillgänglig på separat färsk183-DB,
försökte provisionera en undantagen stadägare (1reconcile-lås/1Create) och visade ready.
Det är ett namngivet funktionsfel, inte ett schema-/byggfel. Efter ändringen är samma prov grönt.

Migration183 inför `players.agora_exempt NOT NULL DEFAULT false` och markerar befintliga
`luna-%`/`gpt-%` en gång. Framtida vanliga registreringar får false, även med sådant prefix;
nya AI-konton ska administratören markera före grundning. Testet kör verklig PostgreSQL-DDL/backfill,
default/NOT NULL samt down→up i isolerad pg_temp; naturliga provet verifierar riktig182→183-uppgradering.
Flaggan hör till spelkontot och påverkas inte av namnbyte, stadägande eller världsreseed.

`pass()` filtrerar före reconcile. Reconcile läser flaggan igen för direktanrop och äldre shortlist.
GET returnerar `enabled:true,state:exempt` utan Matrix-ID/homeserver, även med äldre ready-data;
POST password ger403 med uttryckligt inget-konto-besked och inget fjärranrop. Webb/CLI/Codex och
alfatestinformationen säger samma sak. Inget nytt notisevent eller ändrad eventsemantik.
Befintlig avaktiveringskö fungerar fortfarande för flaggade konton; denna migration avaktiverar
inte Matrix-konton själv. Timothy äger de redan skapade Haiku-kontonas avaktivering.

Livscykelinventering: auth.Register är ordinarie players-INSERT och får DB-default; erase-player
behåller flaggan och sin befintliga agora_deactivate_pending. Provisioneringens state/claim/ready
styrs inom samma service, nu spärrad vid inträdet. Testsfixturer får false. Inga andra runtime-läsare
eller nya skrivande admin-/spelarverb har införts. Undantaget sätts innan ett nytt AI-konto grundar;
det försöker inte avbryta redan pågående extern provisionerings-I/O.

## Bevis

- Tio fysiska mutationer i `mutations.py`: passfilter, direktreconcile, konto/lösenord, GPT-backfill,
  API403, två CLI-grindar och två webbgrindar. Varje ändring kompilerar, ger namngiven assertionRED,
  återställs byte-exakt och går GREEN. Ny tom PG16 för varje DB-arm. `mutations.log` bär SHA256.
- `targeted.log`: alla Agora-proven över server/API/CLI; `web-targeted.log`:15kontroller gröna.
- `native.py`: egen PG16/Redis/Continuwuity26.9.1, nya lokala gpt-/luna-/human-konton registrerade
  genom gammal182-server, riktig migration till183, vanlig join och grundning, omstart som kör
  produktionsreconciler. **Inga SQL-skrivningar, API-overrides, GPT-configar eller livekonton.**
- `native-final/evidence.json`: baselinehealth9c071100/182 → health9d211315/183. AI-kontonexempt,
  lösenord403; människa ready och lyckad riktig Matrix-lösenordsinloggning. Endast booleskt
  inloggningsbevis sparas, inget lösenord/token. Read-only DB: AI har vardera1stad, ingen claim/ready/notis;
  människan har1stad, ready och1notis. Firefox1280×900/390×844,0pageerrors,1:1-inspekterat.
- Första naturliga försöket startade gamla binären i nya worktreen och fick därför migration183
  för tidigt. Riggens schemaassertion stoppade försöket; det räknas inte som uppgraderingsbevis.
  Korrigerad rigg startar gamla binären i dess182-katalog. Slutprovet är från ny tom DB/Matrix.

Bevisfilerna innehåller inga privata riggkonfigar, token eller binärer. TEXT följs upp i speltest;
befintlig kontoruta används utan ny grafisk familj. Claude äger merge/push/deploy och GPT-start därefter.

Reproduktion: native-skriptet skapar och tar bort en egen detached-baslinjeworktree på9c071100,
vilket gör182→183-provet oberoende av vilken commit main senare står på. Alla tjänster har egna
slumpportar och stängs i finally; Matrix är en egen26.9.1-container, aldrig produktions-Agora.
Kör `python3 docs/reviews/agora-ai-undantag/native.py /tmp/<ny-privat-katalog>`.
Go-mutationsreceptet använder tools/gotest.sh och eget ledigt GOTEST_PORT; API/DB återanvänds inte.

## Slutkontroller och överlämning

FullfreshGo183 **GREEN**, alla paket (world149.704s); `go vet ./...` **GREEN**;
**654JS GREEN,0skip**. Tio kompilerande namedRED→exakta bytes→GREEN.
Naturligt slutprov och verklig mänsklig Matrix-lösenordsinloggning **GREEN**.
Go-kod/tester/migration är exakt9d211315, även i fullsvit/native-processen.
37283a82 förtydligar bara Codex/alfatesttext; slutlig JS-svit inkluderar det.
Ingen push, merge, deploy eller aktivitet på GPT-livekonton. Claude granskar/integrerar;
efter live183 kan GPT-spelarnas separata order starta. **Hash + STOPP.**

# F — Host-utkorg före grundning

Kontrakt före kod: en wandering host kan skicka brev via kartans stadsruta,
men Correspondence läser ingen utkorg utan MY_SETTLEMENT_ID. Verifiera detta
med verklig spelare/browser på egen färsk PG16/Redis, aldrig SQL-fixtur.
Använd GET founding/messengers i befintliga trådrader när stad saknas.
Scope diplomacy.js, verkligt drawerprov och reproducerbar webbrigg/rapport.
Ingen server-, API- eller designändring. Stoppa vid kanonkonflikt.
Acceptans: faktisk hostpost syns med text/mottagare/status; grundad spelare
fortsätter läsa sin stad. Test före→efter + fysisk mutation, full freshGo/vet/JS.
Befintlig layout återanvänds; nya bilder märks BILD för Timothy senare.

## Resultat och bevis

Bekräftad baslinje: verkliga register/join, grundad mottagare och fortfarande
vandrande avsändare. Hosten marscherade på vanligt spelar-API för scouting,
klickade stadens hex, skrev brev och klickade Send Messenger. POST gav kvitto
och GET founding/messengers innehöll brevet; Correspondence var ändå tom.
[Baslinje](live-web/baseline-proof.json), [baslinjebild](live-web/baseline-host-outbox-mobile.png).

Ändring: samma faktiska trådrenderer läser founding/messengers utan egen stad,
annars stadens befintliga endpoint. Ingen ny layout eller stil. Ingen regel/API
ändrad. [Slutrigg](live-web/proof.json): /healthz commit40276287/migration160,
real map-click POST, fortsatt founder active, text/mottagare/status i webben,
inget browserfel och noll SQL-mutationer. [Desktop](live-web/host-outbox-desktop.png)
och [mobil390](live-web/host-outbox-mobile.png) är nya 1:1 BILD-underlag som
återanvänder befintlig utkorgsstil; inte Timothy-godkända.

Verkliga drawer + fetchAuth + renderer testad med stubbat nät/DOM, både host
och stadsgren och HTML-escaping. Före korrekt assertionrött; efter grönt.
Fysisk mutation till gamla nullgrenen kräver samma namngivna assertionrött,
återställning grön. Loggar /tmp/megaron-host-outbox-{red,green,mutation,restored}.log.
381 JS gröna (/tmp/megaron-host-outbox-all-js.log); full tools/gotest.sh NY
PG16/migration160 alla paket gröna inkl world89.076s, full env-i vet grön.
Loggar /tmp/megaron-host-outbox-{full-go,vet}.log. git diff-check rent.

## Resume checkpoint
Gren codex/host-outbox-web, /tmp/megaron-codex-host-outbox-20261007,
bas master0d160fc4; kontraktb7c4dbe7, produktion+prov40276287.
Reproduktion: bygg temenos med -X main.buildCommit=40276287, kör
python3 tools/host_outbox_live.py OUT 40276287 fixed. Docker och Python
Playwright/Chromium behövs. Baslinjeläget körs med ofixad diplomacy.js; ingen
rigg ändrar källan. Setup via spelar-API, egen tom DB och rena server-env,
endast egna processer/containers tas bort (docker rm -fv).
Tidiga kartklick innan scouting missade legitimt stad i fog, inte acceptans.
Nästa Claude: granska/integrera; bilder för Timothy senare. Ingen merge/push/deploy.

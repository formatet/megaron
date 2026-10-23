# Stående leveranser — synliga varuinställningar

Kontrakt: listorna i Economy/Automation och Keryx routes ska visa varje konfigurerad vara, mållagret hos mottagaren och lagret som lämnas kvar vid återlastning. Det är inställningar, inte ett påstående om faktiskt skickad last. Noll och decimaler bevaras; ingen retur ska framgå. Scope: webblista/CSS, Keryx text, Codex, riktade prov och verklig isolerad browser/API/CLI. API använder redan outbound/return; inga server-/mekanikändringar. Invariant: samma inställningar visas som servern skickar. Stoppvillkor: behov av ny handels-/lastregel. Bevisplan: gamla renderingar röda på saknat innehåll, nya gröna; fysisk mutation; riktig API-skapad rutt i egna två teststäder, 390/1280 två rena browserkörningar; fullfresh en gång vid slutet.

Baslinjeinventering: fritextformuläret ersattes redan i 8b2b8315. rough-edges.md:s aktuella kvarvarande fynd är att ruttlistan saknar varuinnehåll; det stämmer i både webben och Keryx. Serverns list-API bär redan outbound[{good_key,threshold}] och return[{good_key,floor}].

## Riktade bevis

Webb och faktisk Keryx HTTP-konsument var röda på gammal kod för saknade varuinställningar (baseline-js.log/baseline-cli.log); gröna efter ändringen. Webbprov täcker flera varor, noll, decimaler inklusive 0.04/0.125, ingen retur och HTML-escaping. CLI-provet täcker samma konfiguration och bevarad JSON-form inklusive nollgolv. Inga nya API-nycklar eller verb behövs; serverfält fanns redan. Codex routes uppdaterad och det aktuella rough-edges-fyndet borttaget.

Fysiska mutationer: webben visar returgräns+1 respektive CLI läser threshold i stället för floor; båda namngivna prov röda, byte-exakt återställning och prov gröna. Slutlig JS-körning efter decimalprecisering:739/0. En full Go-körning mot färsk migration205:26 paket gröna, world151.473s. go vet ./... grönt.

## Verktygsfynd och avgränsningar

Äldre economy_goods_live hade två inaktuella antaganden: migration160 och knapptext Transfer→. De är rättade mot arbetskopians migrationer respektive befintlig handlingsselektor. Tilläggets första före-bildsida använde ett andra new_page i Playwrights ägda engångscontext; detta gav verktygsfel och har rättats med en separat autentiserad sida. Run1 nådde korrekt webblista+Keryx innan den äldre överföringsknappen stoppade provet; run2 nådde kolonin men före-bildsinstrumentet stoppade; run3 avbröts med SIGINT/finally när samma instrumentfel var känt. Dessa är inte rena fullflödesbevis. Slutliga körningar redovisas separat.

Ingen mekanikändring, inget ingrepp i den delade acceptansvärlden eller produktionens speldata. Körningarna använder egna färska DB/Redis och städar bara egna processer/containrar. Framsteg fram till kolonin sker genom normala order/bud/förflyttning, utan SQL.

## Slutliga rena användarprov

Run4 och run5: två helt nya oberoende PG16/Redis-världar, normala register/join/found, fysisk kolonn och bud till verkligt grundad koloni. Webbformuläret skapar flervaruorder (grain200/fish50; retur silver0/stone20), listan visar exakt de sparade inställningarna och en nybyggd faktisk Keryx routes visar samma inställningar. Vanlig engångsöverföring via samma ekonomi-UI är fortsatt grön. Ingen sidledsöverfyllnad vid1280/390, inga pageerrors. Chromium i dessa flöden; ingen ny tre-motorers paritetskörning påstås.

Processernas `/healthz`: `cdf3df6+routes`, migration205, ok. Detta är en avsiktligt namngiven kandidatbuild före slutcommit, med arbetskopians aktuella webbassets; serverkoden är oförändrad. `run4/proof.json` och `run5/proof.json` sparar processhärkomst, verkliga resor, POSTs och kvitton utan token. Varje värld revs i finally efter provet. Gemensam acceptansrigg och livevärld orörda.

Run5:s extra före-bilder använder oförändrad economy.js ur cdf3df62 mot samma skapade rutt i en separat autentiserad browsersida. Den nya CSS-klassen saknar konsument i basen. Före-bilden visar saknade varuinställningar; efterbilden visar dem. Bilder vid faktisk skala på index.html A1–A4. Desktopviewport1280, lådan420px; mobil390. Desktop och mobil inspekterade i1:1.

## Levererat

Produktionsändring bf5e9257 mergad/pushad och webbassets ute. Alla fyra levererade JS/CSS/Codex-filer byte-identiska med committen; SHA i deploy.json. Samma aktiva värld2178e795, migration205 dirty=false. Backendkoden är oförändrad och processen kör fortsatt fc259bc; ingen omstart behövdes för statiska filer/CLI. Journalens senaste tio minuter utan ERROR/fatal/panic. Installerad ~/go/bin/poleia anger vcs.revision bf5e92573ed9f9a2ce6d84d0bf36658306a0b38d och vcs.modified=false. BILD för mänsklig efterbedömning: index.html A1–A4.

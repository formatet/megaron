# Keryx bekräftar den valda gåvan

Kontrakt: interaktiv rite --offer ska före y/N visa samma komponerade varor/mängder som skickas. Decimaler får inte rundas till en annan gåva. No ska inte debitera eller kasta; Yes ska debitera den begärda gåvan. Scope: CLI-parsning/förhandsvisning, befintlig formattering, Codex och prov. Inte scope: server/Rite, priser, recept, chans eller nya multiplier-regler; traditionell preview behålls när ingen komposition begärts. Invariant: samma parsade karta går till preview och POST, utan att basreceptet skrivs över. Stoppvillkor: behov av serverquote-/betalningsregel. Bevisplan: gammalkods-PTY genom riktig CLI mot egen ny server, sedan precisionsprov, source-mutant, fullfresh en gång och två oberoende No/Yes-flöden med lager/eventkontroll.

## Bevis

Baslinje: faktisk Keryx i PTY med --offer incense=5.25 visade Offer: grain×10 silver×3. No valdes och lager samt frånvaro av RiteCast kontrollerades innan provet blev rött (baseline-live.log, baseline/declined.txt). Ett separat befintligt formatteringsanrop rundade incense5.25/wine0.04 till5/0 (baseline-precision.log).

Nu parsas gåvan en gång före bekräftelsen; samma karta skickas till bekräftelse och POST. Formatteringen behåller valda decimaler. Utan komposition används fortfarande det traditionella receptet; legacy multiplier-regler och servern ändras inte. En fysisk mutation som åter väljer basreceptet gav rött i det faktiska PTY-provet efter No; källan återställd byte-exakt i finally.

Run1 och run2 är två egna nya PG16/Redis-runtimes. Normal registrering/grundning, sedan explicit tempel/stock/fyndfixtur före handlingarna. Interaktiv Keryx körs i verklig PTY utan --yes eller --json. Första anropets n lämnar incense100/grain0/silver0 och0RiteCast. Andra anropets y visar incense×5.25 och eventets faktiska offering är exakt incense5.25; bara94.75incense återstår. Efteråt syns samma24dagars väntan i Firefox1280/390 och faktisk Keryx-lista. Ingen sidledsöverfyllnad eller pageerrors. Egna processer och containrar städade i finally. Resultatet i proof.json är uttryckligen faktisk RiteCast-effekt plus terminalens stdout, inte en konstruerad HTTP-kvittens.

Riktade CLI-prov, 742 JS-prov och go vet är gröna. En full Go-svit på färsk migration 205 är grön (world 149.133 s). Processbuild eba8355+confirmation och käll-diffhash i respektive proof.json. Inga SQL-skrivningar under handlingarna, inga ingrepp i gemensam acceptansvärld eller produktionens data. Inga nya API-fält, händelser eller betalningsregler.

## Leverans

Kod 8bb6c9e9 är mergad och pushad. Lokal ~/go/bin/poleia är byggd från detta rena commit (vcs.modified=false). Publicerad Codex är byteidentisk med källan. Backendprocessen är fortsatt 92d183c, health ok, migration 205 dirty=false och samma aktiva värld; ingen serverändring eller restart behövdes. Ingen ERROR/fatal/panic i granskade fem minuter. Detaljer i delivery.json. Timothy bad att avsluta denna punkt och inte starta något nytt.

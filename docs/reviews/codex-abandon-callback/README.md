# D — Abandon och call-back i Codex (TEXT)
Kontrakt före ändring: problemet är saknad abandon-instruktion och saknad
sökbar CLI/button-ingång för call-back. Kodens ägarskap/villkor/utfall ska
beskrivas utan nya regler. Scope: colonies.md, sea.md, sökfixtur i codex.test.mjs
samt rapport. Non-scope: server/UI/balans och andra artikelfel. Acceptans:
artikelns instruktion matchar kod, verklig Codex-sökning hittar rätt artikel
på abandon/call-back, länkar/index grön. Stoppa vid kod/kanonkonflikt.
Bevisar kedjegrindens nåbara handlingar; TEXT prövas i playtest, ingen BILD.

Premissrättning: sea.md har REDAN Call it back + egen-hamn-regeln och
undelivered-utfallet. Paritetsaliaset missade 'Call it back'. Vi lägger alltså
inte en ny mekanikförklaring; vi gör den sökbar på CLI-namnet och beskriver
var knappen finns/returkvittot. Claude underrättad. Abandon saknar instruktion.
Kod läst: settlement.go Abandon, messenger/call_back.go, PassageHandler.CallBack,
CLI cmd_abandon/cmd_messenger, warAbandon och dipCallBack. R5 i vault
megaron_plan_ordna_passage bekräftar outbound/egen hamn; ingen kanonkonflikt.

## Resultat och Resume checkpoint

Colonies beskriver ägd aktiv koloni, oåterkalleligt abandon, huvudstadsförbud,
upplöst garnison/embarkerade trupper och att folket inte flyttas till huvudstaden.
Sea beskriver befintlig Call it back i Correspondence/PassageStalled, CLI
call-back --id efter outbox, egen hamn/utresa samt fortsatt hemresa utan leverans.
Inga regler ändras. TEXT väntar playtest av någon som inte vet svaret.

Verklig Codex-sökning: nytt test gav korrekt assertionrött före artiklarna
(/tmp/megaron-codex-verbs-red.log), därefter grönt. Alla 380 JS-tester gröna
(/tmp/megaron-codex-verbs-all-js.log). Full tools/gotest.sh på NY PG16/migration160
alla paket gröna inklusive world 91.470s (/tmp/megaron-codex-verbs-full-go.log).
Full env-i go vet ./... grönt (/tmp/megaron-codex-verbs-vet.log), diff-check rent.
Ingen produktionskod ändras; sökprovets red→green bevisar textens nåbarhet.

Resume: gren codex/codex-abandon-callback i
/tmp/megaron-codex-codex-verbs-20261007 från cccae5a3; kontrakt b899532a.
Nästa ansvar Claude: granska/integrera; Timothy/playtest läser TEXT senare.
Ingen merge/push/deploy av Codex.

# Bönens kända väntetid

Kontrakt: efter besvarad bön ska spelaren se serverns kända väntetid/aktiva gåva, inte det generiska “insufficient offering or a waiting time applies”. Saknat tempel och låg kharis ska fortfarande vara tydliga spärrar. Scope: serverns prioritetsregel för unavailable_reason, befintliga presentationskonsumenter och bevis. Ingen ändring av offer, godwait, kharis, cooldown eller ny API-nyckel. Invariant: strukturerad väntan/aktiv gåva ska inte döljas av en ospecifik afford-förklaring. Stoppvillkor: behov av att ändra när en bön går att säga. Bevisplan: verklig HTTP-cast följd av provinsstatus och gammalkodsRED; rena tillståndsprov som skyddar tempel/kharis/materialbrist; source-mutantRED; två fristående Firefox/Keryx-flöden, en fullfresh vid slutet.

Baslinje observerad i föregående rökelseprov: docs/reviews/incense-parity/run2/mobile.png. Province status bär redan cooldown_remaining_game_days, active_remaining_game_days och departure_pending, men sätter samtidigt ett generiskt unavailable_reason när afford=false. Webbkonsumenten visar reason först.

## Bevis

Gammal kod gav generisk reason vid faktiskt godwait24, aktiv skörd25 och väntande Thread12; logg baseline-api.log. Materialbrist utan väntan gav samma tvetydiga text. CLI dolde konkret tempelspärr bakom cooldown (baseline-cli.log).

Nu utelämnas bara den generiska orsaken när exakta vänt-/aktiv-/pendingfält redan förklarar stoppet. Tempel- och kharisspärr prioriteras fortsatt. Utan väntan anges materialbrist som insufficient offering. Affordable och cast-handlern är oförändrade. Keryx ger konkreta blockerare samma prioritet som webben; ingen ny API-nyckel. Codex rites förklarar listan.

Sex nya verkliga handlerförlopp och befintliga temple-/kravprov gröna på färsk205. CLI-readinessprov och webbens blockerar-/waitprov gröna. Fysiska mutanter återinför generic reason vid väntan respektive tar bort CLI-prioriteten: namngivna röda prov, byte-exakt restore. 740 JS-prov och go vet ./... gröna. Full Go-svit körd EN gång på färsk migration205:26 paket gröna, world156.730s.

Två separata färska PG16/Redis-runtimes: normal grundning och explicit egen tempel/stock/fyndfixtur före native Firefox Offer. Efter svaret läser verklig provins-API wait24/affordablefalse/ingen generell reason; webben visar on cooldown24days och ingen ny Offer-knapp, faktisk Keryx rite --list visar cooldown/24daysremaining. Både1280/390, inga pageerrors eller sidledsöverfyllnad. Process8155c3f+prayerwait/migration205 och käll-diffhash i respektive proof.json. Ingen låtsad naturlig tempel-/rökelseupptäckt. Egna processer/containrar städade i finally.

Verktyget tools/incense_live.py återanvänds med PRAYER_WAIT_PROOF=1 och INCENSE_PROOF_REPORT=prayer-wait-reason; vanliga rökelseprovet är fortfarande förval. Den första rökelsebilden före fixen finns i föregående slices run2/mobile.png; nya bilder fokuserar samma bön och visar väntetiden. Inspekterat1:1.

## Levererat

8c023eb2 mergad/pushad, NY liveprocess8c023eb/migration205/ok verifierad. Grund:205 dirty=false och samma aktiva värld2178e795. Codex rites byte-exakt. Installerad Keryx vcs.revision8c023eb2b9aa18d16e1a9d91fe72d3c209386877, vcs.modified=false. Ingen migration, reseed eller ändring av bönernas regler.

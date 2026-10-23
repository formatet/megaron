# notifications --all

Problem: Haiku försökte läsa arkivet med notifications --all men flaggan saknas.
Spelarsanning: --all visar varje notisslag som API:t returnerar, utan standardfiltrering eller gruppering.
Invariant: samma autentiserade serverarkiv och gräns på senaste100; inget lästmarkeras och inga gamla payloadsemantiker ändras.
Scope: Keryx flagga/query/rendering, beteendetester och Codex keryx-artikel.
Non-scope: ny API-pagination, pushpreferenser, notisproduktion eller webbflöde.
Acceptans: flaggan godtas, GET utan exclude, historiska rader visas var för sig; --unread fungerar med den; explicit kind/exclude krockar tydligt; JSON orört.
Stopp: ändring kräver servermodell eller utökat informationsomfång.
Bevisplan: verklig Cobra→HTTP-regression röd före fix, fysisk mutant, freshGo+JS+vet, byggd CLI --help, deploykontroll.
Huvudgrind: bevisar spelarens återläsning av händelser.

## Resultat

Flaggan inkluderar alla slag och visar arkivrader separat inom API:ts befintliga senaste100-fönster. --unread kan kombineras; --kind/--exclude/--mark-read avvisas i kombination med --all före HTTP. JSON behåller alla fält. Ingen API-, webb-, push- eller databasändring; Codex-artikeln beskriver flaggan och gränsen.

Cobra→riktig HTTP-server regression RED på original, GREEN efter. Två fysiska mutanter (återinförd standardfiltrering respektive gruppering) ger namngivet RED; byteexakt restore före fullsviten. EN fullfreshGo205 GREEN (world150.880s),738JS/0fail, vet GREEN. Se respektive logg. Byggd Keryx --help verifierad; verklig auth→join→founding→notifications --all mot acceptansrigg a05cddc+dirty/mig205/3600s visar två separata verkliga notiser. Egna BrowserCity23/BrowserHost23; fem kharis-textkonton lämnas orörda.

Separat webbläsarparitet pågår; första körningen stannade i verktygets antagande om en Ships-rad i Host-panelen. Detta är inte bevis för ett produktfel och utreds i egen slice.

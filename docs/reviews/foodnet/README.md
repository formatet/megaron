# Matnettots pin-test

Test-only: staden växer bara vid positivt canonical grain+fish-netto när den är
född, unmet=0 och under populationstaket. Inga produktionsändringar.
Bevisar kedjegrinden genom att skydda matbalansens betydelse för tillväxt.

## Kontrakt och premiss
Vault: megaron_arkitekturprogram §Matnettots pin-test (2026-10-04).
Scope: growth_gate_foodnet_test.go samt reproducerbart mutationsrecept/bevis.
Stoppa om testet avslöjar produktionsfel eller kräver ändrad kanon.
Master d9d6d58c saknar testet (tick.go nämner endast namnet i en kommentar).
Sedan gamla WIP-basen cdfa4d7e är tick.go oförändrad; recompute.go har bara
provincens gemensamma lagertak/import tillagt. WIP dc7bdaaa återbaserad till
b061a810 utan konflikt.

SQL i kharis/tick.go läser economy.NetFoodGoods och subtraherar samma
GrainConsumptionPerCitizenPerTick som FoodNet. API province.go loadFoodSummary
läser samma lista och FoodNet; settlement-overview återanvänder sammanfattningen.
Webb city.js läser food_net_per_tick, Keryx läser serverns Sitos-sammanfattning.
Ingen ny regel eller spelaryta.

## Bevis
Master-spegelbaslinje på egen tom PG16/mig160 grön:
/tmp/megaron-foodnet-baseline.log. Återbaserat pin + spegel på annan tom DB gröna:
/tmp/megaron-foodnet-pin.log.

13 fall: grain/fish/mixed var för sig negativt, noll, positivt; livestock,
wine, oil, samt enbart lager. Alla dietvaror finns i stora lager även vid
underskott. Riktig applyDecay; oracle läser persisterade rates och canonical
Go-lista/FoodNet. Endast populationens riktning provas, ingen kopierad
SQL- eller tillväxtformel.

Reproducerbara mutationer: `python3 docs/reviews/foodnet/mutate.py OUT`.
Varje arm får egen ny PG16/mig160 genom tools/gotest.sh; produktion återställs
med finally. Grain-only, >=0 och ovillkorlig född tillväxt ger assertion-rött,
och varje separat återställd arm grönt. Logs: /tmp/megaron-foodnet-mutations/.

Full tools/gotest.sh på ny PG16/mig160: alla paket gröna, inklusive world
89,803s. Log /tmp/megaron-foodnet-full.log. Full env-i go vet ./... exit0:
/tmp/megaron-foodnet-vet.log.
Ingen BILD/TEXT-grind eller spelarresa krävs för denna test-only-slice;
funktionen provas mot riktig DB, befintliga spelar-API-kontrakt bevaras.
Ingen merge/push/deploy.

## Resume checkpoint
Gren codex/foodnet-pin, worktree /tmp/megaron-codex-foodnet-20261004.
Bas d9d6d58c, test b061a810. Kör `tools/gotest.sh` och från server/:
`env -i HOME="$HOME" PATH="$PATH" go vet ./...`.
Nästa ägare Claude: core-review, eget mutationsprov, integration.

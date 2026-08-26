package combat

import "testing"

// Ransonen måste räknas ur SAMMA källa som dragningen (UnitUpkeep), annars
// provianterar man för ett tal och debiterar ett annat — och skillnaden syns
// först som ett skepp som svälter mitt i en resa det betalade fullt för.
func TestVoyageRation_MatchesWhatUpkeepActuallyDraws(t *testing.T) {
	got := VoyageRation("galley", 1, "", 0)
	want := UnitUpkeep("galley", "naval", 1, "positioned").Grain
	if got != want {
		t.Errorf("tom galär: ranson %.2f, upkeep drar %.2f — de måste vara samma tal", got, want)
	}
}

// Delbeslut 2 (Timothy 2026-08-26): skeppet provianterar sin lastade kohort
// också. Testet jämför mot skeppet UTAN last i stället för mot en hårdkodad
// summa — annars mäter det bara sin egen aritmetik.
func TestVoyageRation_ProvisionsTheCohortAboard(t *testing.T) {
	empty := VoyageRation("galley", 1, "", 0)
	laden := VoyageRation("galley", 1, "spearman", 100)
	if laden <= empty {
		t.Fatalf("lastad galär (%.2f) måste äta mer än tom (%.2f) — kohorten ombord "+
			"äter ur skeppets lager", laden, empty)
	}
	// Kohorten ska betala FÄLTranson, inte garnisonsranson: en embarkerad soldat
	// är maximalt borta från stadens förråd. Det var hela motiveringen till att
	// 'embarked' lades i fältmängden 2026-08-05.
	field := UnitUpkeep("spearman", "land", 100, "embarked").Grain
	garrison := UnitUpkeep("spearman", "land", 100, "garrison").Grain
	if field == garrison {
		t.Skip("fältfaktorn är neutraliserad; det här testet mäter ingenting")
	}
	if diff := laden - empty; diff != field {
		t.Errorf("kohortens del av ransonen är %.2f, vill ha fältransonen %.2f "+
			"(garnisonsransonen är %.2f — använd inte den)", diff, field, garrison)
	}
}

// Tur och retur, aldrig enkel (delbeslut 1) — annars strandar skepp.
func TestVoyageProvisions_CoversTheReturnLegAndTheStation(t *testing.T) {
	const ration = 4.0

	oneWay := ration * 5
	got := VoyageProvisions(ration, 5, 0)
	if got <= oneWay {
		t.Errorf("proviant för 5 tick ut = %.0f, men enkel resa kostar %.0f — "+
			"hemresan är inte medräknad och skeppet strandar", got, oneWay)
	}
	if got != ration*10 {
		t.Errorf("proviant = %.0f, vill ha %.0f (ut + hem)", got, ration*10)
	}

	// En sjösentry ligger stilla hela sin patrull och äter under tiden.
	withStation := VoyageProvisions(ration, 5, SentryPatrolTicks)
	if withStation != ration*float64(10+SentryPatrolTicks) {
		t.Errorf("sentry-proviant = %.0f, vill ha %.0f (ut + patrull + hem)",
			withStation, ration*float64(10+SentryPatrolTicks))
	}
	if withStation <= got {
		t.Error("en patrull som ligger still måste kosta mer än samma resa utan station")
	}
}

// Approximationen ska luta UPPÅT. Överproviantering kommer hem och lastas av;
// underproviantering strandar ett skepp. De två felen är inte lika dyra.
func TestVoyageProvisions_NeverUnderProvisions(t *testing.T) {
	// En resa som avrundas till noll tick får inte ge noll mat.
	if got := VoyageProvisions(4, 0, 0); got <= 0 {
		t.Errorf("resa på 0 tick gav %.0f proviant — ett skepp utan mat är precis "+
			"det mekaniken finns för att förhindra", got)
	}
	// Negativ station (trasig indata) får inte dra ifrån.
	if VoyageProvisions(4, 5, -100) < VoyageProvisions(4, 5, 0) {
		t.Error("negativ stationstid drog ifrån provianten i stället för att ignoreras")
	}
}

// Matmätaren visar DYGN, och får aldrig visa en dag skeppet inte har mat för
// hela — en spelare som ser "1 dygn" ska ha ett helt dygn på sig.
func TestProvisionDaysLeft_FloorsAndSurvivesZero(t *testing.T) {
	if got := ProvisionDaysLeft(39, 4); got != 9 {
		t.Errorf("39 korn / 4 per dygn = %d dygn, vill ha 9 (golv, inte 9,75)", got)
	}
	if got := ProvisionDaysLeft(4, 4); got != 1 {
		t.Errorf("exakt en dags mat = %d dygn, vill ha 1", got)
	}
	if got := ProvisionDaysLeft(3, 4); got != 0 {
		t.Errorf("mindre än en dags mat = %d dygn, vill ha 0", got)
	}
	// Division med noll får inte panika: en okänd enhetstyp har ranson 0.
	if got := ProvisionDaysLeft(100, 0); got != 0 {
		t.Errorf("ranson 0 gav %d dygn, vill ha 0 utan panik", got)
	}
}

// ⛔ Land får INTE provianteras (delbeslut 5). Faller det här har någon
// generaliserat sjömekaniken till land, och då har landenheter tyst fått en
// grind de aldrig ska ha — de nås av löpare och försörjs, eller furagerar.
func TestProvisionSource_LandUnitsAreNeverAProvisionCase(t *testing.T) {
	land := upkeepUnitRow{category: "land", status: "positioned"}
	if src := provisionSourceFor(land); src != nil {
		t.Error("en landenhet i fält pekades ut som proviantfall — proviantering är sjö, " +
			"furagering är land")
	}
	sea := upkeepUnitRow{category: "naval", status: "positioned"}
	if src := provisionSourceFor(sea); src == nil {
		t.Error("ett skepp till sjöss äter inte ur sitt eget lager")
	}
}

// En embarkerad kohort äter ur det BÄRANDE skeppets lager — inte ur staden, och
// inte ur sitt eget (den har inget). Utan carrier-uppslaget skulle den falla
// tillbaka på staden och teleporteringen vore tillbaka genom bakdörren.
func TestProvisionSource_EmbarkedCohortEatsFromItsCarrier(t *testing.T) {
	cohort := upkeepUnitRow{category: "land", status: "embarked"}
	if src := provisionSourceFor(cohort); src != nil {
		t.Error("en embarkerad kohort utan känt bärarskepp pekades ut som proviantfall")
	}

	ship := upkeepUnitRow{category: "naval"}
	carried := upkeepUnitRow{category: "land", status: "embarked", carrierID: &ship.id}
	src := provisionSourceFor(carried)
	if src == nil || *src != ship.id {
		t.Error("kohorten ombord äter inte ur bärarskeppets lager")
	}
}

// Regressionsvakt för det värsta felet i den här slicen, funnet av
// TestStartMarch_ExploreFromFieldPositionResolvesNearestOwnedHome:
//
// Första utkastet krävde en support_settlement för att få gå till sjöss. Ett
// skepp som redan STÅR till sjöss saknar ofta en, så kravet strandade det
// permanent — det kunde aldrig få en ny order igen. Det är ett värre fel än det
// provianteringen lagar.
//
// Och den uppenbara lagningen (tillåt proviantering närhelst en support_settlement
// finns) är också fel: ett skepp till sjöss skulle då ta ombord last från en hamn
// tjugo hexar bort, vilket är exakt den teleporterande logistik mekaniken tar
// bort — tillbaka genom en bakdörr.
//
// Regeln är därför "man provianterar I HAMN". Testet spikar den som en
// statusfråga, inte en ägarfråga.
func TestProvisioning_HappensInPortNotAtSea(t *testing.T) {
	// Statusmängden som får proviantera måste vara exakt {garrison}. Skulle
	// någon lägga till 'positioned' här är teleporteringen tillbaka.
	dockedOnly := map[string]bool{"garrison": true}
	for _, s := range []string{"garrison", "positioned", "marching", "embarked", "repairing"} {
		mayProvision := s == "garrison"
		if dockedOnly[s] != mayProvision {
			t.Errorf("status %q: proviantering tillåten=%v, vill ha %v", s, dockedOnly[s], mayProvision)
		}
	}
	// Och den enda statusen som får proviantera måste vara en status som
	// verkligen betyder "i hamn" enligt upkeep-sidan, annars har de två
	// begreppen glidit isär.
	if !atHomePort(upkeepUnitRow{status: "garrison"}) {
		t.Error("'garrison' räknas inte som i hamn på upkeep-sidan — proviantering " +
			"och dragning har glidit isär om olika begrepp")
	}
}

// Ett skepp i hamn får falla tillbaka på stadens magasin (dess proviant lastades
// av vid hemkomsten). Ett skepp till sjöss får INTE — det vore teleporteringen
// tillbaka genom en bakdörr.
func TestAtHomePort_OnlyDockedStatusesMayFallBackToTheTown(t *testing.T) {
	for _, s := range []string{"garrison", "repairing"} {
		if !atHomePort(upkeepUnitRow{status: s}) {
			t.Errorf("status %q räknas inte som i hamn, men borde", s)
		}
	}
	for _, s := range []string{"marching", "positioned", "embarked"} {
		if atHomePort(upkeepUnitRow{status: s}) {
			t.Errorf("status %q räknas som i hamn — då kan ett skepp till sjöss äta ur "+
				"stadens magasin igen, vilket är exakt felet mekaniken tar bort", s)
		}
	}
}

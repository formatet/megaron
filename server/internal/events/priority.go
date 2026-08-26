package events

// Körordningen inom ett tick.
//
// Ett tick är ett dygn (CLAUDE.md). Dygnet har en ordning, och den ordningen är
// kanon — inte en följd av vilken rad Postgres råkade returnera först. Före
// migration 128 fanns ingen: processBatch sorterade sin SUBQUERY, vilket bara
// avgör VILKA rader som plockas, aldrig i vilken ordning RETURNING lämnar
// tillbaka dem. Mätt 48,2/51,8 % över 438 tick — ett myntkast per tick mellan
// varje par av handlers som förfaller samma tick.
//
// Myntkastet var inte kosmetiskt. KharisTicks spannmålsfinansierade tillväxt
// drar lagret ned till (nära) noll med flit — den köper så många nya invånare
// som lagret räcker till. På de tick den vann kastet fann UpkeepTick ingenting
// kvar, och garnisonen tog grain_shortage-attrition i en stad med god
// nettotakt. Reproducerat i acceptansvärlden 2026-08-24, tick 1, i BÅDA
// huvudstäderna samtidigt.
//
// Dygnets ordning, med skälet till varje steg:
//
//	10 ankomst      — världen rör sig först. Det som landar denna dag är
//	                  närvarande när dagen räknas samman.
//	20 blick        — ögonen läser läget EFTER rörelsen, aldrig före.
//	30 strid        — stål avgör innan ekonomin bokförs.
//	40 reserv       — magasinet först: en stad under svältgränsen ska äta ur
//	                  sin reserv INNAN armén svälter, annars kommer räddningen
//	                  efter desertionen. Åt andra hållet skummar magasinet bara
//	                  över täckningströskeln, där undanläggningen är liten mot
//	                  arméns behov.
//	50 plikt        — armén äter och får sold. En stående förpliktelse betalas
//	                  före allt som är frivilligt.
//	55 föda         — befolkningen äter, ur LAGRET, kanonordning grain → fisk
//	                  → boskap (megaron_plan_utfodringsordningen.md). Efter
//	                  plikten och före tillväxten: allt staden försörjer äter
//	                  före befolkningen (Timothy 2026-08-25), och tillväxten
//	                  (60) får bara se vad dagen har kvar när BÅDA är betalda.
//	60 tillväxt     — kharis, det gudomliga och det som ÖVERSKOTTET föder.
//	                  Tillväxt är per definition det dagen har kvar när
//	                  plikterna är betalda. En stad får inte föda sig själv in
//	                  i att svälta sin egen garnison.
//	70 följd        — lojalitet, kolonistraff, lånad armé: de reagerar på det
//	                  läge dagen slutade i.
//	80 hushåll      — utgångna anbud och kollaps sist, när allt annat är sagt.
//
// Ändra inte ett tal här utan att uppdatera megaron_tickordning.md. Talen är
// glesa med flit — det finns plats mellan stegen för en ny typ utan att någon
// befintlig behöver flyttas.
const (
	tickPriorityArrival     = 10
	tickPrioritySight       = 20
	tickPriorityBattle      = 30
	tickPriorityReserve     = 40
	tickPriorityUpkeep      = 50
	tickPriorityFood        = 55
	tickPriorityGrowth      = 60
	tickPriorityConsequence = 70
	tickPriorityHousekeep   = 80
)

// DefaultTickPriority är vad en typ utan uttryckligt beslut får, och samma tal
// som kolumnens DEFAULT i migration 128. En ny typ ska inte tyst hamna här:
// TestEveryScheduledTypeHasAPriority faller när någon lägger till en typ utan
// att placera den i dygnet.
const DefaultTickPriority = 50

var tickPriorities = map[ScheduledEventType]int{
	// 10 — det som landar.
	ScheduledArmyArrival:      tickPriorityArrival,
	ScheduledUnitArrival:      tickPriorityArrival,
	ScheduledTransportArrival: tickPriorityArrival,
	ScheduledLogisticsArrival: tickPriorityArrival,
	ScheduledRecallArrival:    tickPriorityArrival,
	ScheduledSentryReturn:     tickPriorityArrival,
	ScheduledMarchRecall:      tickPriorityArrival,
	ScheduledOrderDelivery:    tickPriorityArrival,
	ScheduledMessengerArrival: tickPriorityArrival,
	ScheduledMessengerReturn:  tickPriorityArrival,
	ScheduledTradeDelivery:    tickPriorityArrival,
	ScheduledTradeReturn:      tickPriorityArrival,
	ScheduledBuildComplete:    tickPriorityArrival,
	ScheduledTrainComplete:    tickPriorityArrival,
	// Reparationen är en färdigtimer av exakt TrainCompletes form (Slice C
	// bygger på train.go:89-100) — den flippar 'repairing'→'garrison' i stället
	// för att skapa en enhet. Skeppet ska vara helt och närvarande innan dagens
	// blick, strid och plikt räknar med det.
	ScheduledShipRepairComplete: tickPriorityArrival,

	// 20 — vad ögonen ser när rörelsen är gjord.
	ScheduledInterceptScan:      tickPrioritySight,
	ScheduledUnitInterceptScan:  tickPrioritySight,
	ScheduledMarchSightingScan:  tickPrioritySight,
	ScheduledMarchEncounterScan: tickPrioritySight,

	// 30 — strid.
	ScheduledBattleTick:      tickPriorityBattle,
	ScheduledOccupationCheck: tickPriorityBattle,

	// 40 — magasinet.
	ScheduledSitosTick: tickPriorityReserve,

	// 50 — plikten.
	ScheduledUpkeepTick: tickPriorityUpkeep,

	// 55 — befolkningen äter.
	ScheduledFoodTick: tickPriorityFood,

	// 60 — tillväxt och det gudomliga.
	ScheduledKharisTick: tickPriorityGrowth,
	ScheduledDivineRoll: tickPriorityGrowth,

	// 70 — följderna.
	ScheduledLoyaltyDecayTick:   tickPriorityConsequence,
	ScheduledLoyaltyWelfareTick: tickPriorityConsequence,
	ScheduledColonyPenaltyTick:  tickPriorityConsequence,
	ScheduledBorrowedArmyTick:   tickPriorityConsequence,

	// 80 — hushåll.
	ScheduledOfferExpiry:        tickPriorityHousekeep,
	ScheduledCollapseCheck:      tickPriorityHousekeep,
	ScheduledCollapseSettlement: tickPriorityHousekeep,
	// Kapitulationen är en kollaps av samma slag som CollapseSettlement och hör
	// därför hit, inte till följdsteget: den byter ÄGARE på staden. Låg man den
	// på 70 skulle lojalitet, kolonistraff och lånad armé räknas på en stad som
	// redan bytt hand mitt i dygnet. Dygnet bokförs på den som höll staden under
	// det; först när allt annat är sagt faller den.
	ScheduledSiegeCapitulation: tickPriorityHousekeep,
}

// TickPriority returns where in the game day an event type runs. Lower runs
// first. Types with no explicit placement get DefaultTickPriority.
func TickPriority(t ScheduledEventType) int {
	if p, ok := tickPriorities[t]; ok {
		return p
	}
	return DefaultTickPriority
}

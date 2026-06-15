package unit

import (
	"github.com/google/uuid"
)

// Event type name constants for events appended to the events.Store (immutable log).
// These are frozen in semantics forever (CLAUDE.md C1 rule). To evolve: create a
// new type (e.g. UnitFormedV2), never reinterpret an existing one.
//
// Stream: events.StreamUnit (defined below — same package so no circular import).
// All payloads record OUTCOMES, not intentions (e.g. actual men drawn, not a pending roll).
const (
	EventUnitFormed         = "UnitFormed"         // a new unit row created (forming)
	EventUnitReinforced     = "UnitReinforced"      // men added to an existing unit
	EventUnitDeployed       = "UnitDeployed"        // unit transitions forming → garrison
	EventUnitMarchOrdered   = "UnitMarchOrdered"    // unit begins moving; departure logged
	EventUnitArrived        = "UnitArrived"         // unit reached its destination
	EventUnitCombatResolved = "UnitCombatResolved"  // combat outcome applied to this unit
	EventUnitDisbanded      = "UnitDisbanded"       // unit dissolved; men returned to pop
	EventShipLoaded         = "ShipLoaded"          // land unit embarked on vessel
	EventShipUnloaded       = "ShipUnloaded"        // land unit disembarked from vessel
	EventUnitIntercepted    = "UnitIntercepted"     // sentry triggered interception combat
	EventCityCollapsed      = "CityCollapsed"       // settlement exhausted; warband spawned
)

// StreamUnit is the events.StreamType value for unit streams.
// Defined here so callers can use unit.StreamUnit without importing events.
// Cast to events.StreamType at call sites: events.StreamType(unit.StreamUnit).
const StreamUnit = "unit"

// ---- Payload structs ---------------------------------------------------------
// Every field records what HAPPENED (outcome), never what was requested.
// Payloads are JSON-serialised; field names are frozen.

// UnitFormedPayload is emitted when a new Unit row is created.
type UnitFormedPayload struct {
	UnitID       uuid.UUID `json:"unit_id"`
	OwnerID      uuid.UUID `json:"owner_id"`
	WorldID      uuid.UUID `json:"world_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	UnitType     string    `json:"unit_type"`
	Category     string    `json:"category"`
	InitialSize  int       `json:"initial_size"` // men added in this batch (≤ 100)
	Crew         int       `json:"crew"`          // for naval: men drawn from population
	PopDrawn     int       `json:"pop_drawn"`     // population actually deducted
}

// UnitReinforcedPayload is emitted when men are added to an existing forming unit.
type UnitReinforcedPayload struct {
	UnitID      uuid.UUID `json:"unit_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	SizeBefore  int       `json:"size_before"`
	SizeAfter   int       `json:"size_after"`
	PopDrawn    int       `json:"pop_drawn"`
}

// UnitDeployedPayload is emitted when a unit reaches size 100 and becomes deployable.
type UnitDeployedPayload struct {
	UnitID       uuid.UUID `json:"unit_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	UnitType     string    `json:"unit_type"`
}

// UnitMarchOrderedPayload is emitted when a march is ordered. Stores the
// actual scheduled departure and arrival times (not the intent).
type UnitMarchOrderedPayload struct {
	UnitID     uuid.UUID `json:"unit_id"`
	OriginQ    int       `json:"origin_q"`
	OriginR    int       `json:"origin_r"`
	TargetQ    int       `json:"target_q"`
	TargetR    int       `json:"target_r"`
	Stance     string    `json:"stance"`      // fortify|storm|sentry at destination
	DepartsAt  string    `json:"departs_at"`  // RFC3339
	ArrivesAt  string    `json:"arrives_at"`  // RFC3339
}

// UnitArrivedPayload is emitted when a unit completes its march.
type UnitArrivedPayload struct {
	UnitID  uuid.UUID `json:"unit_id"`
	Q       int       `json:"q"`
	R       int       `json:"r"`
	NewStatus string  `json:"new_status"` // garrison|positioned
}

// UnitCombatResolvedPayload records the outcome of combat for one unit.
// Sizes are absolute values after resolution (not deltas).
type UnitCombatResolvedPayload struct {
	UnitID     uuid.UUID `json:"unit_id"`
	Role       string    `json:"role"`       // attacker|defender
	SizeBefore int       `json:"size_before"`
	SizeAfter  int       `json:"size_after"` // 0 = destroyed
	Outcome    string    `json:"outcome"`    // attacker_wins|defender_holds
	PopLost    int       `json:"pop_lost"`   // men permanently lost (demographic cost)
}

// UnitDisbandedPayload is emitted when a unit is dissolved.
type UnitDisbandedPayload struct {
	UnitID       uuid.UUID `json:"unit_id"`
	SettlementID uuid.UUID `json:"settlement_id"`
	UnitType     string    `json:"unit_type"`
	SizeAtDisband int      `json:"size_at_disband"`
	PopReturned  int       `json:"pop_returned"` // men returned to population
}

// ShipLoadedPayload is emitted when a land unit embarks on a vessel.
type ShipLoadedPayload struct {
	ShipUnitID  uuid.UUID `json:"ship_unit_id"`
	CargoUnitID uuid.UUID `json:"cargo_unit_id"`
	Q           int       `json:"q"`
	R           int       `json:"r"`
}

// ShipUnloadedPayload is emitted when a land unit disembarks from a vessel.
type ShipUnloadedPayload struct {
	ShipUnitID  uuid.UUID `json:"ship_unit_id"`
	CargoUnitID uuid.UUID `json:"cargo_unit_id"`
	Q           int       `json:"q"`
	R           int       `json:"r"`
}

// UnitInterceptedPayload is emitted when a sentry unit intercepts a marching enemy.
// Contains the actual combat outcome at the interception point.
type UnitInterceptedPayload struct {
	SentryUnitID    uuid.UUID `json:"sentry_unit_id"`
	InterceptedUnitID uuid.UUID `json:"intercepted_unit_id"`
	Q               int       `json:"q"`
	R               int       `json:"r"`
	Outcome         string    `json:"outcome"` // attacker_wins|defender_holds
}

// ScheduledUnitArrivalPayload is the scheduled_events payload for unit arrival.
// The corresponding ScheduledEventType constant is added to events/scheduler.go (C2+).
// Defined here for co-location with the rest of the unit domain.
//
// TODO C2+: register handler for events.ScheduledUnitArrival in main.go.
type ScheduledUnitArrivalPayload struct {
	UnitID  uuid.UUID `json:"unit_id"`
	WorldID uuid.UUID `json:"world_id"`
}

// CityCollapsedPayload is emitted when a settlement's population reaches ≤ 100
// and it ceases to exist as a city. The last 100 inhabitants leave as a warband.
// Cause is "starvation" (daily tick) or "overmobilisation" (recruit drain).
type CityCollapsedPayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	OwnerID      uuid.UUID `json:"owner_id"`
	WorldID      uuid.UUID `json:"world_id"`
	WarbandUnitID uuid.UUID `json:"warband_unit_id"` // the spawned infantry unit
	Q            int       `json:"q"`
	R            int       `json:"r"`
	Cause        string    `json:"cause"`            // "starvation" | "overmobilisation"
	LastSettlement bool    `json:"last_settlement"`  // true if this was the owner's only city
}

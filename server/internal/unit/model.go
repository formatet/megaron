// Package unit implements the discrete unit model for Megaron.
//
// A Unit is a single indivisible military entity: a 100-man cohort (land) or
// one vessel with a fixed crew (naval). Units replace the integer army columns
// on settlements/marching_armies; those columns remain until C8.
//
// G1 placement: this package sits at settlement/province level.
// It may import clock, events, economy — never combat or kingdom.
package unit

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---- Type enumerations -------------------------------------------------------

// Type identifies the kind of unit.
type Type string

const (
	TypeSpearman      Type = "spearman"
	TypeEliteInfantry Type = "elite_infantry"
	TypeWarChariot    Type = "war_chariot"
	TypePriest        Type = "priest"
	TypeGalley        Type = "galley"      // kanonisk standardgalär; crew 20
	TypeWarGalley     Type = "war_galley"  // crew 50
	TypeMerchantman   Type = "merchantman" // crew 10

	// TypeNomadicHost is the founder-phase token: the player's people before they
	// have a capital. It is a single movable marker (size 1) — the 4 000 people it
	// represents live in founder_phase.population, never in units.size. It dissolves
	// permanently when the metropolis is founded.
	TypeNomadicHost Type = "nomadic_host"
)

// Category groups unit types into land and naval.
type Category string

const (
	CategoryLand  Category = "land"
	CategoryNaval Category = "naval"
)

// CategoryOf returns the category for a given unit type.
// Returns CategoryLand for unknown types (safe default).
func CategoryOf(t Type) Category {
	switch t {
	case TypeGalley, TypeWarGalley, TypeMerchantman:
		return CategoryNaval
	default:
		return CategoryLand
	}
}

// CrewFor returns the baseline crew size (men drawn from population) for naval
// unit types. Returns 0 for land units.
func CrewFor(t Type) int {
	switch t {
	case TypeGalley:
		return 20
	case TypeWarGalley:
		return 50
	case TypeMerchantman:
		return 10
	default:
		return 0
	}
}

// ---- Behaviour gates ---------------------------------------------------------
//
// These gate per type, the same way CategoryOf/CrewFor already do, rather than
// adding capability columns to units. One source of truth per question.

// CombatCapable reports whether a unit type may attack, defend, support, raid or
// besiege. The nomadic host may do none of these — it is a people on the move,
// not an army.
func CombatCapable(t Type) bool {
	return t != TypeNomadicHost
}

// CanFoundMetropolis reports whether a unit type may found the player's first
// settlement — a metropolis — dissolving itself in the act.
func CanFoundMetropolis(t Type) bool {
	return t == TypeNomadicHost
}

// MarchHoursFactorFor returns the multiplier to apply to
// province.TerrainMoveHours for this unit type.
//
// It scales HOURS, not speed — a larger number is a SLOWER unit. Naming it for
// speed invites the opposite reading and would make the host twice as fast as an
// army instead of half: the ladder the design fixes is host = ½ spearman's speed
// = double a spearman's hours. Spearmen are the baseline an army marches at
// (messengers already halve it themselves, see TerrainMoveHours' own comment).
func MarchHoursFactorFor(t Type) float64 {
	if t == TypeNomadicHost {
		return 2.0
	}
	return 1.0
}

// ---- Display names -------------------------------------------------------

// displayNames is the ONE canonical display name per unit type (DB key),
// consumed by keryx (CLI), the web/API layer, and notifications so the same
// unit never shows a different name in different channels (A8). DB keys
// themselves are untouched — this is presentation only.
//
// Taxonomy decided 2026-07-10 (Timothy) — clarity first, flavour only where
// it earns its keep: "Hoplites"/"Agema"/"Hiereus" retired. The legacy
// "trireme" key collapses to the canonical "galley" display ("ship" is no
// longer a units.type value after the namn-hygien A rename, mig 084 — see
// Canonical below for the recruit/disband input alias).
// "priest" (mig 060, dead unit) has deliberately no entry — unmapped keys
// fall back to the raw key.
var displayNames = map[string]string{
	string(TypeSpearman):      "Spearmen",
	string(TypeEliteInfantry): "Elite Infantry",
	string(TypeWarChariot):    "War Chariot",
	string(TypeGalley):        "Galley",
	"trireme":                 "Galley",
	string(TypeNomadicHost):   "Nomadic Host",
	string(TypeWarGalley):     "War Galley",
	string(TypeMerchantman):   "Emporos",
}

// DisplayName returns the canonical human-readable name for a unit's DB type
// key, falling back to the raw key for any type not yet in the table (e.g. a
// future unit, or the retired "priest").
func DisplayName(t string) string {
	if label, ok := displayNames[t]; ok {
		return label
	}
	return t
}

// Canonical normalizes a legacy/alias unit-type string to its canonical
// units.type value, so old clients (or the CLI's input aliases) that still
// send "ship"/"trireme"/"chariot" keep working after the namn-hygien A+B
// rename (mig 084): "galley" and "war_chariot" are now the only values ever
// written to units.type. Unrecognized strings pass through unchanged.
func Canonical(t string) string {
	switch t {
	case "ship", "trireme":
		return string(TypeGalley)
	case "chariot":
		return string(TypeWarChariot)
	default:
		return t
	}
}

// Status is the lifecycle state of a unit.
type Status string

const (
	StatusForming    Status = "forming"    // still being recruited (size < 100 for land)
	StatusGarrison   Status = "garrison"   // stationed in a settlement
	StatusMarching   Status = "marching"   // in transit on the map
	StatusPositioned Status = "positioned" // on the map, not moving (sentry/fortify/storm)
	StatusDisbanded  Status = "disbanded"  // dissolved; men returned to population
	StatusEmbarked   Status = "embarked"   // land unit aboard a naval vessel; moves with the ship
)

// Stance is the tactical posture of a stationary unit.
type Stance string

const (
	StanceFortify Stance = "fortify" // defensive bonus, cannot move
	StanceStorm   Stance = "storm"   // besieging adjacent settlement
	StanceSentry  Stance = "sentry"  // patrols sentry_q/r, intercepts enemies within 3 hex
)

// ---- Domain model ------------------------------------------------------------

// Unit is a single discrete military entity.
type Unit struct {
	ID      uuid.UUID
	WorldID uuid.UUID
	OwnerID uuid.UUID

	Type     Type
	Category Category
	Size     int // land: men (0–100); naval: always 1 vessel
	Crew     int // naval: men from population; 0 for land

	// Name is set for naval units (Wanax-chosen or game-suggested at recruit
	// time, ship-build overhaul 2026-07-09); nil for land units.
	Name *string
	// BuildCompleteAt is set while a naval unit is status='forming' (its
	// TrainComplete ETA); cleared (nil) once it flips to garrison. Land units
	// never set it — their forming progress is size-based, not time-based.
	BuildCompleteAt *time.Time

	CargoUnitID *uuid.UUID // naval: land unit being transported

	Status Status
	Stance *Stance // nil when not in a named stance

	SettlementID *uuid.UUID // non-nil when in garrison / forming

	Q *int // map position (non-nil when on the map)
	R *int

	TargetQ   *int
	TargetR   *int
	DepartsAt *time.Time
	ArrivesAt *time.Time
	// DepartTick/ArriveTick are the tick-native mirror of DepartsAt/ArrivesAt
	// (mig 085, K4 tick-contract): the world tick a march left and the tick it
	// arrives. Set in the same UPDATE as the wall-clock pair at every
	// course-setting site, and cleared (nil) on arrival. The API derives
	// arrival_tick/duration_ticks/arrives_at_utc from them.
	DepartTick *int
	ArriveTick *int

	MarchIntent *string // "colonize" or nil (plain march)
	ColonyName  *string // chosen colony name when MarchIntent == "colonize"

	SentryQ *int // patrol centre when stance = sentry
	SentryR *int

	LeaderRole *string // e.g. "dekarchos" — label only, not yet enforced in UI

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ---- Read helpers ------------------------------------------------------------

// Store provides read access to the units table.
// It is deliberately read-only; writes happen via event handlers (C2+).
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const selectCols = `
	id, world_id, owner_id,
	type, category, size, crew, cargo_unit_id,
	status, stance,
	settlement_id,
	q, r,
	target_q, target_r, departs_at, arrives_at,
	depart_tick, arrive_tick,
	sentry_q, sentry_r,
	leader_role,
	march_intent, colony_name,
	name, build_complete_at,
	created_at, updated_at`

func scanUnit(row interface {
	Scan(dest ...any) error
}) (*Unit, error) {
	var u Unit
	var stance *string
	if err := row.Scan(
		&u.ID, &u.WorldID, &u.OwnerID,
		&u.Type, &u.Category, &u.Size, &u.Crew, &u.CargoUnitID,
		&u.Status, &stance,
		&u.SettlementID,
		&u.Q, &u.R,
		&u.TargetQ, &u.TargetR, &u.DepartsAt, &u.ArrivesAt,
		&u.DepartTick, &u.ArriveTick,
		&u.SentryQ, &u.SentryR,
		&u.LeaderRole,
		&u.MarchIntent, &u.ColonyName,
		&u.Name, &u.BuildCompleteAt,
		&u.CreatedAt, &u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if stance != nil {
		s := Stance(*stance)
		u.Stance = &s
	}
	return &u, nil
}

// Get fetches a single unit by ID.
func (s *Store) Get(ctx context.Context, id uuid.UUID) (*Unit, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+selectCols+` FROM units WHERE id = $1`, id)
	u, err := scanUnit(row)
	if err != nil {
		return nil, fmt.Errorf("unit.Store.Get: %w", err)
	}
	return u, nil
}

// ListByOwner returns all non-disbanded units for an owner in a world,
// ordered by created_at.
func (s *Store) ListByOwner(ctx context.Context, ownerID, worldID uuid.UUID) ([]*Unit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+selectCols+`
		 FROM units
		 WHERE owner_id = $1 AND world_id = $2 AND status != 'disbanded'
		 ORDER BY created_at`,
		ownerID, worldID,
	)
	if err != nil {
		return nil, fmt.Errorf("unit.Store.ListByOwner: %w", err)
	}
	defer rows.Close()

	var units []*Unit
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, fmt.Errorf("unit.Store.ListByOwner scan: %w", err)
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

// ListBySettlement returns all non-disbanded units garrisoned in a settlement.
func (s *Store) ListBySettlement(ctx context.Context, settlementID uuid.UUID) ([]*Unit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+selectCols+`
		 FROM units
		 WHERE settlement_id = $1 AND status != 'disbanded'
		 ORDER BY created_at`,
		settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("unit.Store.ListBySettlement: %w", err)
	}
	defer rows.Close()

	var units []*Unit
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, fmt.Errorf("unit.Store.ListBySettlement scan: %w", err)
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

// ListAtHex returns all non-disbanded units whose current map position is (q, r)
// in the given world.
func (s *Store) ListAtHex(ctx context.Context, worldID uuid.UUID, q, r int) ([]*Unit, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+selectCols+`
		 FROM units
		 WHERE world_id = $1 AND q = $2 AND r = $3 AND status != 'disbanded'
		 ORDER BY created_at`,
		worldID, q, r,
	)
	if err != nil {
		return nil, fmt.Errorf("unit.Store.ListAtHex: %w", err)
	}
	defer rows.Close()

	var units []*Unit
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, fmt.Errorf("unit.Store.ListAtHex scan: %w", err)
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

// ---- Army summary (backward-compatible view) ---------------------------------

// ArmySummary is a type→total-size map for backward-compatible display.
// Clients that still expect ArmyComposition-style data can derive it from this.
type ArmySummary map[Type]int

// SummaryForSettlement aggregates the size of all non-disbanded garrison units
// in a settlement, keyed by unit type.
func SummaryForSettlement(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID) (ArmySummary, error) {
	rows, err := pool.Query(ctx,
		`SELECT type, COALESCE(SUM(size), 0)
		 FROM units
		 WHERE settlement_id = $1 AND status != 'disbanded'
		 GROUP BY type`,
		settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("unit.SummaryForSettlement: %w", err)
	}
	defer rows.Close()

	summary := make(ArmySummary)
	for rows.Next() {
		var t Type
		var total int
		if err := rows.Scan(&t, &total); err != nil {
			return nil, fmt.Errorf("unit.SummaryForSettlement scan: %w", err)
		}
		summary[t] = total
	}
	return summary, rows.Err()
}

// SummaryAtHex aggregates unit sizes at a map position (for units in
// positioned/marching status at that coordinate).
func SummaryAtHex(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) (ArmySummary, error) {
	rows, err := pool.Query(ctx,
		`SELECT type, COALESCE(SUM(size), 0)
		 FROM units
		 WHERE world_id = $1 AND q = $2 AND r = $3 AND status != 'disbanded'
		 GROUP BY type`,
		worldID, q, r,
	)
	if err != nil {
		return nil, fmt.Errorf("unit.SummaryAtHex: %w", err)
	}
	defer rows.Close()

	summary := make(ArmySummary)
	for rows.Next() {
		var t Type
		var total int
		if err := rows.Scan(&t, &total); err != nil {
			return nil, fmt.Errorf("unit.SummaryAtHex scan: %w", err)
		}
		summary[t] = total
	}
	return summary, rows.Err()
}

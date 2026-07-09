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
	TypeGalley        Type = "galley"     // kanonisk standardgalär; crew 20
	TypeShip          Type = "ship"       // legacy-alias för galley — DB-kolumn + UnitSpecs-nyckel heter fortf. "ship"; full rename→galley = D-stream/SB7
	TypeWarGalley     Type = "war_galley"  // crew 50
	TypeMerchantman   Type = "merchantman" // crew 10
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
	case TypeGalley, TypeShip, TypeWarGalley, TypeMerchantman:
		return CategoryNaval
	default:
		return CategoryLand
	}
}

// CrewFor returns the baseline crew size (men drawn from population) for naval
// unit types. Returns 0 for land units.
func CrewFor(t Type) int {
	switch t {
	case TypeGalley, TypeShip:
		return 20
	case TypeWarGalley:
		return 50
	case TypeMerchantman:
		return 10
	default:
		return 0
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

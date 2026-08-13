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
	"encoding/json"
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

// CanEmbark reports whether a unit type may be loaded aboard a ship as cargo.
// False for the nomadic host — a people on the move cannot embark (locked
// 2026-07-15; sea travel for the host is a post-MVP design question). Until now
// the host was only blocked by the size<100 forming gate by accident (size=1);
// this gate is the deliberate rule and must survive that gate's removal.
func CanEmbark(t Type) bool {
	return t != TypeNomadicHost
}

// MarchHoursFactorFor returns the multiplier to apply to
// province.TerrainMoveTicks for this unit type.
//
// It scales HOURS, not speed — a larger number is a SLOWER unit. Naming it for
// speed invites the opposite reading and would make the host twice as fast as an
// army instead of half: the ladder the design fixes is host = ½ spearman's speed
// = double a spearman's hours. Spearmen are the baseline an army marches at
// (messengers already halve it themselves, see TerrainMoveTicks' own comment).
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
// it earns its keep: "Hoplites"/"Agema" retired. The legacy
// "trireme" key collapses to the canonical "galley" display ("ship" is no
// longer a units.type value after the namn-hygien A rename, mig 084 — see
// Canonical below for the recruit/disband input alias).
// Unmapped keys fall back to the raw key.
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
// future unit not yet mapped).
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

// ---- Reaction policy (avsiktslagret) ------------------------------------------
//
// megaron_plan_avsiktslagret.md: a positioned/garrisoned unit reacts to a class
// of actor entering its reach (today only "who a sentry seizes") with one of
// four avsikt verbs, per relation class. Encoding decided by Timothy
// 2026-08-06: a single JSONB column (`units.reaction_policy`), not separate
// intent_foreign/intent_own columns — SQL-gates as `reaction_policy->>'foreign'
// = 'intercept'`.
//
// Verb scope (Timothy 2026-08-06, delbeslut 2): all four verbs are storable/
// settable now. `combat/unit_intercept_scan.go`'s unit-vs-unit sentry scan
// (KR3 §7, 2026-08-07) reads all three: `intercept` fights (initiateOrJoinBattle
// — the intercepted march HALTS and joins a persistent battle), `alert`
// notifies without fighting, `escort` never triggers the scan at all.
// `transport/intercept.go`'s caravan-sentry query still only gates on
// `intercept` — `escort`/`alert` remain STUBS there (a Wanax can set and read
// them, but nothing acts on them for caravans yet; a deliberately separate
// mechanic from unit-vs-unit combat, not extended by the KR3 §7 slice). `own` is enforced by
// owner_id<>$N in every sentry query, never by reading this field — it is
// stored anyway so a future policy editor has somewhere to read/write it, and
// per-relation defaults still document intent. `ally` is parked until kingdoms
// return (KINGDOMS_ENABLED, post-MVP): stored, never consulted.
type ReactionVerb string

const (
	ReactionIntercept ReactionVerb = "intercept" // seize/fight
	ReactionEscort    ReactionVerb = "escort"    // never triggers the unit-vs-unit sentry scan (KR3 §7); still a STUB for caravans (transport/intercept.go)
	ReactionIgnore    ReactionVerb = "ignore"    // never react
	ReactionAlert     ReactionVerb = "alert"     // notifies without fighting on the unit-vs-unit sentry scan (KR3 §7); still a STUB for caravans
)

// ValidReactionVerb reports whether v is one of the four avsiktslager verbs.
func ValidReactionVerb(v string) bool {
	switch ReactionVerb(v) {
	case ReactionIntercept, ReactionEscort, ReactionIgnore, ReactionAlert:
		return true
	}
	return false
}

// ReactionPolicy is the two-axis (relation × avsikt) reaction table for one unit.
type ReactionPolicy struct {
	Foreign ReactionVerb `json:"foreign"`
	Own     ReactionVerb `json:"own"`
	Ally    ReactionVerb `json:"ally"`
}

// DefaultReactionPolicy reproduces today's hardcoded sentry behaviour
// (foreign→intercept, own→ignore, ally→ignore) exactly. The migration's column
// default and SetStance's sentry default both equal this, so introducing the
// column changes nothing about how an existing sentry behaves
// (megaron_plan_avsiktslagret.md §3).
func DefaultReactionPolicy() ReactionPolicy {
	return ReactionPolicy{Foreign: ReactionIntercept, Own: ReactionIgnore, Ally: ReactionIgnore}
}

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

	// SupportSettlementID är den stad som RESTE förbandet och betalar dess sold
	// hela dess liv (mig 100). Permanent: den ändras aldrig av marsch, hemkomst
	// eller omstationering, och ingen endpoint får skriva den. Skild från både
	// SettlementID (nuvarande station) och units.home_settlement_id (transient
	// marschorigo, som nollas vid hemkomst och INTE får återanvändas till detta).
	// NULL betyder att staden fallit — då betalas ingen sold och förbandet
	// deserterar via upkeep.go:s vanliga maskineri (megaron_aktorer_plan.md §3.1).
	SupportSettlementID *uuid.UUID
	// Ordinal är regementsnumret inom (försörjande stad, typ). Återanvänds
	// ALDRIG — se AllocateOrdinal.
	Ordinal *int

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

	// ReactionPolicy is the avsiktslagret relation×avsikt table (see above).
	// Every row carries one (NOT NULL DEFAULT, mig 112) — never nil.
	ReactionPolicy ReactionPolicy

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
	settlement_id, support_settlement_id, ordinal,
	q, r,
	target_q, target_r, departs_at, arrives_at,
	depart_tick, arrive_tick,
	sentry_q, sentry_r,
	reaction_policy,
	leader_role,
	march_intent, colony_name,
	name, build_complete_at,
	created_at, updated_at`

func scanUnit(row interface {
	Scan(dest ...any) error
}) (*Unit, error) {
	var u Unit
	var stance *string
	var reactionRaw []byte
	if err := row.Scan(
		&u.ID, &u.WorldID, &u.OwnerID,
		&u.Type, &u.Category, &u.Size, &u.Crew, &u.CargoUnitID,
		&u.Status, &stance,
		&u.SettlementID, &u.SupportSettlementID, &u.Ordinal,
		&u.Q, &u.R,
		&u.TargetQ, &u.TargetR, &u.DepartsAt, &u.ArrivesAt,
		&u.DepartTick, &u.ArriveTick,
		&u.SentryQ, &u.SentryR,
		&reactionRaw,
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
	u.ReactionPolicy = DefaultReactionPolicy()
	if len(reactionRaw) > 0 {
		_ = json.Unmarshal(reactionRaw, &u.ReactionPolicy)
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

package religion

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Prayers of speed (bönesystemet v3 steg 2b). A blessed mover travels
// faster, also when it is already under way: what is LEFT of the journey takes
// DepartureTimeFactor of its time. A journey is sped up at most once.
const (
	EffectBlessedSpeedLand = "blessed_speed_land" // Asterion: Runners and land caravans
	EffectBlessedSpeedSea  = "blessed_speed_sea"  // the Sea Daimons: ships and sea freight

	PrayerAsterionSwift  = "minoan_asterion_swift"
	PrayerStillnessSwift = "minoan_stillness_swift"

	BlessedSpeedTicks = 12

	// SpeedNum/SpeedDen is DepartureTimeFactor (0,75) as an exact fraction for integer routes.
	SpeedNum, SpeedDen = 3, 4
)

// IsSpeedBlessing says whether the effect is one of the two speed prayers.
func IsSpeedBlessing(effect string) bool {
	return effect == EffectBlessedSpeedLand || effect == EffectBlessedSpeedSea
}

// SpedUp is the outcome recorded in RiteCast.sped_up for one journey.
type SpedUp struct {
	Kind          string    `json:"kind"` // unit | transport | messenger
	ID            uuid.UUID `json:"id"`
	OldArriveTick int       `json:"old_arrive_tick"`
	NewArriveTick int       `json:"new_arrive_tick"`
}

// Mover is a journey that a speed prayer can reach.
type Mover struct {
	Kind string // unit | transport | messenger
	ID   uuid.UUID
}

// SpeedBlessingFor returns a live blessing of the effect that covers the mover:
// the whole realm, the mover's own city (origin / support), or the mover named
// (a ship by unit id; a Runner or caravan by mover id). city may be nil.
func SpeedBlessingFor(ctx context.Context, db PrayerDB, owner uuid.UUID, effect string, moverID uuid.UUID, city *uuid.UUID, tick int) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `SELECT b.id FROM prayer_blessings b
 WHERE b.owner_id=$1 AND b.effect=$2 AND b.from_tick<=$5 AND b.until_tick>$5
   AND (b.scope='realm' OR (b.scope='city' AND b.settlement_id=$4) OR (b.scope='unit' AND (b.mover_id=$3 OR b.unit_id=$3)))
 ORDER BY b.until_tick DESC, b.id LIMIT 1`, owner, effect, moverID, city, tick).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}

type moverQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// CoveredMovers lists the journeys under way that a freshly laid speed blessing
// reaches. scope: unit (target = mover id) | city | realm. Only the owner's own
// journeys, and only those that can be recomputed safely: Runners on a free land
// leg (not aboard a carrier), land caravans, ships marching, sea freight.
func CoveredMovers(ctx context.Context, db moverQuerier, world, owner uuid.UUID, effect, scope string, target, city *uuid.UUID) ([]Mover, error) {
	var sql string
	if effect == EffectBlessedSpeedLand {
		sql = `SELECT 'messenger', m.id FROM messengers m
 WHERE m.world_id=$1 AND m.sender_id=$2 AND m.status IN ('outbound','returning') AND m.land_route IS NOT NULL AND m.passage_status IS NULL
   AND m.carrier_transport_id IS NULL AND m.carrier_unit_id IS NULL
   AND ($3='realm' OR ($3='city' AND m.origin_id=$5) OR ($3='unit' AND m.id=$4))
 UNION ALL
 SELECT 'transport', t.id FROM transports t
 WHERE t.world_id=$1 AND t.owner_id=$2 AND t.category='land' AND t.status='in_transit'
   AND ($3='realm' OR ($3='city' AND t.origin_id=$5) OR ($3='unit' AND t.id=$4))`
	} else {
		sql = `SELECT 'unit', u.id FROM units u
 WHERE u.world_id=$1 AND u.owner_id=$2 AND u.category='naval' AND u.status='marching'
   AND ($3='realm' OR ($3='city' AND u.support_settlement_id=$5) OR ($3='unit' AND u.id=$4))
 UNION ALL
 SELECT 'transport', t.id FROM transports t JOIN units u ON u.id=t.ship_unit_id
 WHERE t.world_id=$1 AND t.owner_id=$2 AND t.category='naval' AND t.status='in_transit'
   AND ($3='realm' OR ($3='city' AND u.support_settlement_id=$5) OR ($3='unit' AND u.id=$4))`
	}
	rows, err := db.Query(ctx, sql, world, owner, scope, target, city)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mover
	for rows.Next() {
		var m Mover
		if err := rows.Scan(&m.Kind, &m.ID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

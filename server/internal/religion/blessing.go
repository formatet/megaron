package religion

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Prayers with a target (bönesystemet v3 steg 2a). A prayer is blessed on one
// named unit, on the units of the city where it is said, or on the whole realm;
// the wider the target, the dearer the gift (ScopeCostFactor). The blessing is
// READ when it is needed (a battle round, a storm), wherever the unit is.
const (
	EffectBlessedStrength = "blessed_strength" // Velchanos: land troops roll more dice
	EffectBlessedHull     = "blessed_hull"     // the Sea Daimons: a ship takes one storm unharmed

	PrayerVelchanosStrength = "minoan_velchanos_strength"
	PrayerStillnessHull     = "minoan_stillness_hull"

	ScopeUnit  = "unit"
	ScopeCity  = "city"
	ScopeRealm = "realm"
	// ScopePermanent (steg 4): the blessing lasts for ever, on one NAMED unit only.
	ScopePermanent = "permanent"

	// PermanentUntil is until_tick of a permanent row (int4 max): every reader tests
	// until_tick > tick, so a permanent row needs no special case anywhere.
	PermanentUntil = 2147483647

	// BlessedStrengthFactor is the dice multiplier of a blessed land unit
	// (same size as combat.GarrisonDefenseFactor; its own constant so the two can be tuned apart).
	BlessedStrengthFactor = 1.25
	BlessedStrengthTicks  = 6
	BlessedHullTicks      = 12
)

// ScopeCostFactor is what the target costs: the traditional gift times this
// (Claude's alpha numbers, tune freely).
var ScopeCostFactor = map[string]int{ScopeUnit: 1, ScopeCity: 2, ScopeRealm: 4, ScopePermanent: 20}

// HasPermanentStep says whether a prayer has the fourth step: strength, hull and
// sea speed can be laid on a named unit for ever. Swift Feet (Runners, caravans: one
// journey each) cannot.
func HasPermanentStep(effect string) bool {
	return effect == EffectBlessedStrength || effect == EffectBlessedHull || effect == EffectBlessedSpeedSea
}

// ScopeCostFactorsFor is the step table a prayer offers: the permanent step only where it exists.
func ScopeCostFactorsFor(effect string) map[string]int {
	out := make(map[string]int, len(ScopeCostFactor))
	for k, v := range ScopeCostFactor {
		if k == ScopePermanent && !HasPermanentStep(effect) {
			continue
		}
		out[k] = v
	}
	return out
}

// IsBlessing says whether the effect is a targeted blessing.
func IsBlessing(effect string) bool {
	return effect == EffectBlessedStrength || effect == EffectBlessedHull || IsSpeedBlessing(effect)
}

// BlessingUnitKind is the kind of unit a blessing can be laid on: "land" or "naval".
func BlessingUnitKind(effect string) string {
	switch effect {
	case EffectBlessedStrength:
		return "land"
	case EffectBlessedHull, EffectBlessedSpeedSea:
		return "naval"
	}
	return ""
}

// blessingCovers is the one place that says which blessings cover a unit.
// $1 owner · $2 unit · $3 effect · $4 tick. The owner is checked on both the
// blessing and the unit: someone else's blessing never covers your unit.
const blessingCovers = `FROM prayer_blessings b JOIN units u ON u.id=$2 AND u.owner_id=$1
 WHERE b.owner_id=$1 AND b.effect=$3 AND b.from_tick<=$4 AND b.until_tick>$4
 AND (b.scope='realm' OR (b.scope='unit' AND b.unit_id=u.id) OR (b.scope='city' AND b.settlement_id=u.support_settlement_id))`

// LoadBlessed answers "is this unit blessed with this effect now" for all three scopes in one query.
func LoadBlessed(ctx context.Context, db PrayerDB, owner, unitID uuid.UUID, effect string, tick int) (bool, error) {
	var blessed bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 `+blessingCovers+`)`, owner, unitID, effect, tick).Scan(&blessed)
	return blessed, err
}

// ConsumeWard spends one unspent ward covering the ship, once per (blessing, ship).
// It runs in the same TX as the hull outcome it replaces.
func ConsumeWard(ctx context.Context, db PrayerDB, owner, unitID uuid.UUID, effect string, tick int) (bool, error) {
	tag, err := db.Exec(ctx, `INSERT INTO prayer_blessing_wards_used(blessing_id,unit_id)
 SELECT b.id,$2 `+blessingCovers+`
 AND NOT EXISTS(SELECT 1 FROM prayer_blessing_wards_used w WHERE w.blessing_id=b.id AND w.unit_id=$2)
 ORDER BY b.until_tick,b.id LIMIT 1 ON CONFLICT DO NOTHING`, owner, unitID, effect, tick)
	if err != nil {
		return false, fmt.Errorf("spend ward: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// BlessingActive says whether a live blessing of this effect already covers the
// target, so the same target cannot be blessed twice at once. For scope city a
// realm blessing also covers it; for scope unit all three do.
func BlessingActive(ctx context.Context, db PrayerDB, owner uuid.UUID, effect, scope string, unitID, city *uuid.UUID, tick int) (bool, error) {
	var active bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM prayer_blessings b LEFT JOIN units u ON u.id=$5
 WHERE b.owner_id=$1 AND b.effect=$2 AND b.from_tick<=$6 AND b.until_tick>$6 AND (
   b.scope='realm'
   OR ($3='city' AND b.scope='city' AND b.settlement_id=$4)
   OR ($3='unit' AND b.scope='unit' AND (b.unit_id=$5 OR b.mover_id=$5))
   OR ($3='unit' AND b.scope='city' AND b.settlement_id=u.support_settlement_id)))`,
		owner, effect, scope, city, unitID, tick).Scan(&active)
	return active, err
}

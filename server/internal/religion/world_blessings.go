package religion

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// World blessings (bönesystemet v3 steg 5a): three prayers that are laid on a city or the
// whole realm, never on a unit. Their effects live in the economy and the build queue
// (fertility, a quicker build) and in the wrath roll (the scapegoat), each reading the
// prayer_blessings row at the moment it matters.
const (
	EffectBlessedFertility = "blessed_fertility" // Potnia: people and herd grow half as fast again
	EffectBlessedBuilding  = "blessed_building"  // Velchanos: what is left of a build takes three quarters of the time
	EffectScapegoat        = "scapegoat"         // Potnia: the next divine punishment is borne by the goat

	PrayerPotniaFertility   = "minoan_potnia_fertility"
	PrayerVelchanosBuilding = "minoan_velchanos_building"
	PrayerPotniaScapegoat   = "minoan_potnia_scapegoat"

	FertilityTicks        = 20 // >= economy.HerdGrazeBirthEvery, so a herd always calves within the window
	BuildingBlessingTicks = 12
	ScapegoatTicks        = 30
	BuildTimeFactor       = DepartureTimeFactor // what is left of a build takes this share of the time
	BuildNum, BuildDen    = SpeedNum, SpeedDen  // the same share as an exact fraction
)

// IsWorldBlessing says whether the effect is one of the three world blessings.
func IsWorldBlessing(effect string) bool {
	return effect == EffectBlessedFertility || effect == EffectBlessedBuilding || effect == EffectScapegoat
}

// WorldBlessingScopes is what a world blessing can be laid on: a city or the realm; the
// scapegoat only the realm (the punishment and its shield are the realm's).
func WorldBlessingScopes(effect string) []string {
	if effect == EffectScapegoat {
		return []string{ScopeRealm}
	}
	return []string{ScopeCity, ScopeRealm}
}

// SpendScapegoat lets the goat bear the punishment: it ends the owner's live scapegoat row
// at this tick (the row stays as the record) and returns its id. Locks the row, so a
// punishment and a cast cannot both pass.
func SpendScapegoat(ctx context.Context, db PrayerDB, owner uuid.UUID, tick int) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := db.QueryRow(ctx, `UPDATE prayer_blessings SET until_tick=$3 WHERE id=(
 SELECT id FROM prayer_blessings WHERE owner_id=$1 AND effect=$2 AND from_tick<=$3 AND until_tick>$3
 ORDER BY until_tick,id LIMIT 1 FOR UPDATE) RETURNING id`, owner, EffectScapegoat, tick).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return id, true, nil
}

// ScapegoatUntil is the tick the owner's standing goat is spent at (0 if none).
func ScapegoatUntil(ctx context.Context, db PrayerDB, owner uuid.UUID, tick int) (int, error) {
	var until int
	err := db.QueryRow(ctx, `SELECT COALESCE(max(until_tick),0) FROM prayer_blessings WHERE owner_id=$1 AND effect=$2 AND from_tick<=$3 AND until_tick>$3`,
		owner, EffectScapegoat, tick).Scan(&until)
	return until, err
}

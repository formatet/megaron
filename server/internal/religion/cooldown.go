package religion

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PrayerDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// GodCooldownRemaining reads the same projection as the cast transaction.
// Cast callers already hold their Wanax's PWR lock, including across cities.
func GodCooldownRemaining(ctx context.Context, db PrayerDB, world, player uuid.UUID, god string, nowTick int) (int, error) {
	var until int
	err := db.QueryRow(ctx, `SELECT COALESCE((SELECT until_tick FROM prayer_cooldowns WHERE world_id=$1 AND player_id=$2 AND god_id=$3),0)`, world, player, god).Scan(&until)
	if err != nil {
		return 0, fmt.Errorf("read god cooldown: %w", err)
	}
	return max(0, until-nowTick), nil
}

// SetGodCooldown is called only after a successful answer, in the same TX as
// the effect, sacrifice, standing and outcome event. Silent answers never wait.
func SetGodCooldown(ctx context.Context, db PrayerDB, world, player uuid.UUID, god string, untilTick int) error {
	_, err := db.Exec(ctx, `INSERT INTO prayer_cooldowns(world_id,player_id,god_id,until_tick) VALUES($1,$2,$3,$4) ON CONFLICT(world_id,player_id,god_id) DO UPDATE SET until_tick=EXCLUDED.until_tick`, world, player, god, untilTick)
	return err
}

// PrayerActiveRemaining is shared by the cast guard and the prayer catalogue.
// The owner stamp prevents the new city's owner inheriting a defensive gift.
func PrayerActiveRemaining(ctx context.Context, db PrayerDB, settlement, player uuid.UUID, effect string, nowTick int) (int, error) {
	var until int
	err := db.QueryRow(ctx, `SELECT CASE
 WHEN $3='garrison_defense' AND owner_id=$2 AND garrison_defense_owner_id=$2 THEN COALESCE(garrison_defense_until_tick,0)
 WHEN $3='harvest_blessing' THEN COALESCE(harvest_blessing_until_tick,0)
 ELSE 0 END FROM settlements WHERE id=$1`, settlement, player, effect).Scan(&until)
	if err != nil {
		return 0, err
	}
	return max(0, until-nowTick), nil
}

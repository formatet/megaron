package combat

// §8 mur-modellen (megaron_kr3_stridsutvardering.md beslut 7, Timothy
// 2026-08-07): the wall is a SHIELD, not a strength multiplier — each
// battle-tick it absorbs the first N = wallAbsorbPerLevel × wall_level
// incoming hits on the DEFENDING side before losses are applied.
//
// RED BEFORE this term existed: a walled garrison took the exact same
// per-tick losses as an unwalled one — wall_level was read at siege
// initiation but never reached the dice model at all (battle.go had no
// wall_level/storm columns on `battles`, migration 116). GREEN AFTER: the
// tests below, which pin the battle's dice via a fixed seed (sequenceDice,
// defined in battle_test.go) so two otherwise-identical fixtures — one
// walled, one not — produce bit-for-bit identical raw dice outcomes and
// differ ONLY by the wall term.

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// siegeFixture is newBattleFixture's settlement-siege counterpart: the
// target hex (1,0) carries an actual settlement (with a configurable
// wall_level) instead of empty ground, so startBattle's wall_level snapshot
// lookup (a settlements/provinces join on q,r) has a real row to find.
type siegeFixture struct {
	worldID       uuid.UUID
	attacker      uuid.UUID
	defender      uuid.UUID
	defSettlement uuid.UUID
}

func newSiegeFixture(t *testing.T, pool *pgxpool.Pool, wallLevel int) siegeFixture {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var f siegeFixture
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&f.worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, f.worldID)
	})

	mkPlayer := func(tag string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
			tag+"-"+uuid.New().String(),
		).Scan(&id); err != nil {
			t.Fatalf("create player %s: %v", tag, err)
		}
		return id
	}
	f.attacker = mkPlayer("attacker")
	f.defender = mkPlayer("defender")

	var attCapProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&attCapProv); err != nil {
		t.Fatalf("create attacker capital province: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Attacker Home', 'achaean', $3, 'capital', true, 'active', 8000)`,
		f.worldID, attCapProv, f.attacker,
	); err != nil {
		t.Fatalf("create attacker capital: %v", err)
	}

	var defProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&defProv); err != nil {
		t.Fatalf("create defender province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population, wall_level)
		 VALUES ($1, $2, 'Defender City', 'khemetiu', $3, 'capital', true, 'active', 8000, $4) RETURNING id`,
		f.worldID, defProv, f.defender, wallLevel,
	).Scan(&f.defSettlement); err != nil {
		t.Fatalf("create defender settlement: %v", err)
	}
	return f
}

func mkSiegeAttacker(t *testing.T, pool *pgxpool.Pool, f siegeFixture, size int, storm bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var stance *string
	if storm {
		s := "storm"
		stance = &s
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, target_q, target_r, capture_mode, stance)
		 VALUES ($1, $2, 'spearman', 'land', $3, 0, 'marching', 0, 0, 1, 0, 'sack', $4) RETURNING id`,
		f.worldID, f.attacker, size, stance,
	).Scan(&id); err != nil {
		t.Fatalf("create siege attacker: %v", err)
	}
	return id
}

func mkSiegeGarrison(t *testing.T, pool *pgxpool.Pool, f siegeFixture, size int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', $3, 0, 'garrison', $4) RETURNING id`,
		f.worldID, f.defender, size, f.defSettlement,
	).Scan(&id); err != nil {
		t.Fatalf("create siege garrison: %v", err)
	}
	return id
}

func runOneBattleTick(t *testing.T, pool *pgxpool.Pool, h *BattleTickHandler, worldID, battleID uuid.UUID, tickIndex int) {
	t.Helper()
	ctx := context.Background()
	raw, err := json.Marshal(battleTickPayload{BattleID: battleID})
	if err != nil {
		t.Fatalf("marshal battle tick payload: %v", err)
	}
	if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tickIndex, Payload: raw}); err != nil {
		t.Fatalf("battle tick handle (tick %d): %v", tickIndex, err)
	}
}

func loadRoundSideResult(t *testing.T, pool *pgxpool.Pool, battleID uuid.UUID, tickIndex, roundIndex int, side string) BattleSideRoundResult {
	t.Helper()
	col := "defender"
	if side == "attacker" {
		col = "attacker"
	}
	var raw json.RawMessage
	if err := pool.QueryRow(context.Background(),
		`SELECT `+col+` FROM battle_rounds WHERE battle_id = $1 AND tick_index = $2 AND round_index = $3`,
		battleID, tickIndex, roundIndex,
	).Scan(&raw); err != nil {
		t.Fatalf("load battle_round %s (tick %d round %d): %v", side, tickIndex, roundIndex, err)
	}
	var result BattleSideRoundResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal %s round result: %v", side, err)
	}
	return result
}

// TestSiegeBattle_WallAbsorbsFirstNHitsReducingDefenderLosses is the slice's
// RED-BEFORE/GREEN-AFTER pair (see file comment): a walled garrison must
// take strictly fewer losses per tick than an identical unwalled one, by
// exactly min(rawAttackerHits, wallAbsorbPerLevel×wall_level).
func TestSiegeBattle_WallAbsorbsFirstNHitsReducingDefenderLosses(t *testing.T) {
	pool := testPool(t)
	const wallLevel = 3
	const attackerSize, defenderSize = 3000, 3000 // large enough that raw hits/tick comfortably exceed the wall's budget
	seedInts := []int{55, 2029, 7717, 40961}

	runSiege := func(wl int) (battleID uuid.UUID, worldID uuid.UUID) {
		f := newSiegeFixture(t, pool, wl)
		mkSiegeGarrison(t, pool, f, defenderSize)
		attackerUnitID := mkSiegeAttacker(t, pool, f, attackerSize, false)

		h := newArrivalHandler(pool, &sequenceDice{ints: append([]int(nil), seedInts...)})
		runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
		bID := loadBattleID(t, pool, f.worldID, 1, 0)

		battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
		runOneBattleTick(t, pool, battleH, f.worldID, bID, 1)
		return bID, f.worldID
	}

	walledBattle, _ := runSiege(wallLevel)
	unwalledBattle, _ := runSiege(0)

	walledLosses := loadRoundSideResult(t, pool, walledBattle, 1, 1, "defender").LossesReceived
	unwalledResult := loadRoundSideResult(t, pool, unwalledBattle, 1, 1, "defender")
	unwalledLosses := unwalledResult.LossesReceived
	rawHits := loadRoundSideResult(t, pool, unwalledBattle, 1, 1, "attacker").HitsCaused

	if rawHits < wallAbsorbPerLevel*wallLevel {
		t.Fatalf("fixture too small: raw attacker hits = %d, want >= %d (wallAbsorbPerLevel*wallLevel) so absorption isn't clipped by attHits itself — pick a bigger attackerSize/seed", rawHits, wallAbsorbPerLevel*wallLevel)
	}
	if walledLosses >= unwalledLosses {
		t.Errorf("walled defender losses = %d, want strictly fewer than unwalled defender losses = %d (same dice, same sizes — only wall_level differs)", walledLosses, unwalledLosses)
	}
	wantAbsorbed := wallAbsorbPerLevel * wallLevel
	if gotAbsorbed := unwalledLosses - walledLosses; gotAbsorbed != wantAbsorbed {
		t.Errorf("absorbed = unwalledLosses(%d) - walledLosses(%d) = %d, want exactly %d (wallAbsorbPerLevel(%d) * wallLevel(%d))",
			unwalledLosses, walledLosses, gotAbsorbed, wantAbsorbed, wallAbsorbPerLevel, wallLevel)
	}
}

// TestSiegeBattle_LargeAttackerBreaksWallAnyway is the slice's second
// red-before/green-after half: a besieging force landing far more hits per
// tick than the wall can absorb still grinds the garrison down to
// annihilation — the wall softens a siege, it does not stop one
// (megaron_kr3_stridsutvardering.md beslut 7: "avsiktligt").
func TestSiegeBattle_LargeAttackerBreaksWallAnyway(t *testing.T) {
	pool := testPool(t)
	f := newSiegeFixture(t, pool, 3) // wall_level=3, N=15/tick — trivial next to a 4000-strong siege
	garrisonID := mkSiegeGarrison(t, pool, f, 150)
	attackerUnitID := mkSiegeAttacker(t, pool, f, 4000, false)

	h := newArrivalHandler(pool, economy.NewWallDice())
	runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
	battleID := loadBattleID(t, pool, f.worldID, 1, 0)

	battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
	runBattleToEnd(t, pool, battleH, f.worldID, battleID, 30)

	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM units WHERE id = $1`, garrisonID).Scan(&status); err != nil {
		t.Fatalf("read garrison: %v", err)
	}
	if status != "disbanded" {
		t.Errorf("garrison status = %q, want disbanded (a 4000-strong siege must break a level-3 wall within 30 battle-ticks)", status)
	}
}

// TestSiegeBattle_StormHalvesWallAbsorbAndRaisesAttackerLosses verifies both
// halves of storm's effect (beslut 7): halved wall absorption AND increased
// attacker losses, both gated on wall_level>0 so a field battle's storm
// stance (unrelated to any wall) is never touched by this term.
func TestSiegeBattle_StormHalvesWallAbsorbAndRaisesAttackerLosses(t *testing.T) {
	pool := testPool(t)
	const wallLevel = 3
	const attackerSize, defenderSize = 3000, 800
	seedInts := []int{311, 104729, 15485867, 22801}

	runSiege := func(storm bool) (battleID uuid.UUID) {
		f := newSiegeFixture(t, pool, wallLevel)
		mkSiegeGarrison(t, pool, f, defenderSize)
		attackerUnitID := mkSiegeAttacker(t, pool, f, attackerSize, storm)

		h := newArrivalHandler(pool, &sequenceDice{ints: append([]int(nil), seedInts...)})
		runFieldArrival(t, pool, h, f.worldID, attackerUnitID)
		bID := loadBattleID(t, pool, f.worldID, 1, 0)

		battleH := NewBattleTickHandler(pool, h.eventStore, h.scheduler, nil, h.clk)
		runOneBattleTick(t, pool, battleH, f.worldID, bID, 1)
		return bID
	}

	noStormBattle := runSiege(false)
	stormBattle := runSiege(true)

	noStormDef := loadRoundSideResult(t, pool, noStormBattle, 1, 1, "defender")
	stormDef := loadRoundSideResult(t, pool, stormBattle, 1, 1, "defender")

	wantNoStormAbsorbed := wallAbsorbPerLevel * wallLevel
	wantStormAbsorbed := wantNoStormAbsorbed / stormWallAbsorbDivisor
	if wantStormAbsorbed >= wantNoStormAbsorbed {
		t.Fatalf("test setup invariant broken: storm must absorb less than non-storm (%d vs %d)", wantStormAbsorbed, wantNoStormAbsorbed)
	}
	if stormDef.LossesReceived <= noStormDef.LossesReceived {
		t.Errorf("storm defender losses = %d, want strictly more than non-storm defender losses = %d (halved wall lets more hits through)",
			stormDef.LossesReceived, noStormDef.LossesReceived)
	}

	noStormAtt := loadRoundSideResult(t, pool, noStormBattle, 1, 1, "attacker")
	stormAtt := loadRoundSideResult(t, pool, stormBattle, 1, 1, "attacker")
	// The defender's own HitsCaused (hits it landed ON the attacker) is what
	// becomes the attacker's LossesReceived — NOT the attacker's HitsCaused
	// (that's the attacker's hits on the defender, a different quantity).
	rawDefHits := noStormDef.HitsCaused
	if rawDefHits == 0 {
		t.Fatalf("test setup produced zero raw defender hits — cannot observe stormAttackerLossMultiplier's effect; pick a bigger defenderSize/different seed")
	}
	if noStormAtt.LossesReceived != rawDefHits {
		t.Errorf("non-storm attacker losses = %d, want exactly rawDefHits = %d (no multiplier without storm)", noStormAtt.LossesReceived, rawDefHits)
	}
	wantStormAttLosses := int(math.Round(float64(rawDefHits) * stormAttackerLossMultiplier))
	if stormAtt.LossesReceived != wantStormAttLosses {
		t.Errorf("storm attacker losses = %d, want exactly %d (round(rawDefHits(%d) * stormAttackerLossMultiplier(%v)))",
			stormAtt.LossesReceived, wantStormAttLosses, rawDefHits, stormAttackerLossMultiplier)
	}
	if stormAtt.LossesReceived <= noStormAtt.LossesReceived {
		t.Errorf("storm attacker losses = %d, want strictly more than non-storm = %d (storm bleeds the attacker harder)",
			stormAtt.LossesReceived, noStormAtt.LossesReceived)
	}
}

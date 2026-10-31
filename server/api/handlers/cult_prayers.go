package handlers

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/settlement"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"slices"
	"time"
)

// loadPrayerRows is the single projection of city-local prayer gates consumed by
// both province inspection and the realm Kult catalogue. Casts still validate.
func (h *ProvinceHandler) loadPrayerRows(ctx context.Context, sett *settlement.Settlement, worldID uuid.UUID, kharisNow float64, goodsStock map[string]float64) ([]prayerRow, map[string]float64, error) {
	// Temple presence — a prayer needs its own god's temple (ORDER 15); a
	// legacy prayer without a god takes any temple, as the rite handler does.
	var templeGods []string
	_ = h.pool.QueryRow(ctx,
		`SELECT coalesce(array_agg(god_id), '{}') FROM buildings WHERE settlement_id = $1 AND building_type = 'temple'`,
		sett.ID,
	).Scan(&templeGods)
	hasTemple := len(templeGods) > 0
	hasTempleFor := func(god string) bool { return (god == "" && hasTemple) || slices.Contains(templeGods, god) }

	// available_prayers: the settlement culture's prayers with full affordability:
	// material offering costs + kharis tier gate + temple presence.
	// All three gates mirror the real Rite handler so affordable:true is trustworthy.
	// cooldown_remaining_game_days is >0 when the prayer is on cooldown (game-days,
	// not wall clock — one tick IS one day).
	// Effect (Plan A / A7, megaron_kult_legibilitet_plan.md) surfaces spec.Description
	// so a Wanax knows what a prayer does before casting it. DESIGN INVARIANT
	// (Timothy 2026-07-11, HARD): never add a success_chance/odds field here — the
	// gods are not machines. Gynnsamhet is read via the settlement's kharis_mood
	// sibling field (kharisToMood, web.go), not a computed percentage.

	// The gods' current reckoning — one read for the whole prayer list.
	divineValues, _ := religion.LoadDivineValues(ctx, h.pool, worldID)

	var prayerTick int
	if err := h.pool.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, worldID).Scan(&prayerTick); err != nil {
		return nil, nil, fmt.Errorf("could not read prayer time: %w", err)
	}
	// Each temple rests on its own (bönegrunden): read its level, servants and rest once.
	staff, err := economy.LoadTempleStaffing(ctx, h.pool, sett.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read temple staffing: %w", err)
	}
	templeByGod := map[string]economy.TempleWorkplace{}
	for _, t := range staff.Temples {
		templeByGod[t.GodID] = t
	}
	prayers := []prayerRow{}
	for _, pid := range religion.CulturePrayers[string(sett.CultureID)] {
		spec := religion.PrayerSpecs[pid]
		afford := hasTempleFor(spec.GodID) && kharisNow >= spec.MinKharis
		// Opening the offering form does not require the traditional recipe.
		canPrepareOffering := afford
		if afford {
			for g, need := range spec.Offering {
				if !province.GoodsAffordable(goodsStock[g], need) {
					afford = false
					break
				}
			}
		}
		// Check cooldown: same query as the Rite handler.
		// Use sett.OwnerID (not request auth) so spectators see the owner's cooldown.
		var cooldownRemainingDays int
		if spec.GodID == "" && spec.CooldownTicks > 0 && sett.OwnerID != nil {
			var lastCast time.Time
			if cdErr := h.pool.QueryRow(ctx,
				`SELECT created_at FROM events
			 WHERE world_id = $1
			   AND event_type = 'RiteCast'
			   AND payload->>'player_id' = $2
			   AND payload->>'prayer' = $3
			   AND (payload->>'success')::boolean = true
			   AND stream_id = $4
			 ORDER BY created_at DESC LIMIT 1`,
				worldID, sett.OwnerID.String(), pid, sett.ID,
			).Scan(&lastCast); cdErr == nil {
				elapsed := h.clk.Now().Sub(lastCast)
				remaining := tick.RealUntil(spec.CooldownTicks, 0) - elapsed
				if remaining > 0 {
					cooldownRemainingDays = tick.GameDaysLeft(remaining)
					afford = false
					canPrepareOffering = false
				}
			}
		}
		activeRemainingDays := 0
		restPaused := false
		var templeInfo economy.TempleWorkplace
		if spec.GodID != "" {
			templeInfo = templeByGod[spec.GodID]
		}
		if spec.GodID != "" && sett.OwnerID != nil {
			var cdErr error
			if templeInfo.BuildingID != uuid.Nil {
				cooldownRemainingDays, cdErr = religion.TempleRestRemaining(ctx, h.pool, templeInfo.BuildingID)
				if cdErr != nil {
					return nil, nil, fmt.Errorf("could not read the temple's rest: %w", cdErr)
				}
				restPaused = cooldownRemainingDays > 0 && templeInfo.Staffed < templeInfo.Capacity
			}
			activeRemainingDays, cdErr = religion.PrayerActiveRemaining(ctx, h.pool, sett.ID, *sett.OwnerID, spec.EffectType, prayerTick)
			if cdErr != nil {
				return nil, nil, fmt.Errorf("could not read active prayer: %w", cdErr)
			}
			if cooldownRemainingDays > 0 || activeRemainingDays > 0 {
				afford = false
				canPrepareOffering = false
			}
		}
		departurePending := activeRemainingDays > 0 && (spec.EffectType == religion.EffectNextLandDeparture || spec.EffectType == religion.EffectNextShipDeparture)
		if departurePending {
			activeRemainingDays = 0
		}
		reason := ""
		temple := ""
		if spec.GodID != "" {
			temple = religion.TempleName(spec.GodID)
		}
		if !hasTempleFor(spec.GodID) {
			reason = "no temple"
			if temple != "" {
				reason = "no " + temple
			}
		} else if kharisNow < spec.MinKharis {
			reason = "insufficient kharis"
		} else if !afford && cooldownRemainingDays == 0 && activeRemainingDays == 0 && !departurePending {
			// Timed/pending states already have precise structured fields.
			// A generic reason would hide them in the prayer list.
			reason = "insufficient offering"
		}
		prayers = append(prayers, prayerRow{DeparturePending: departurePending, CanPrepareOffering: canPrepareOffering,
			GodID: spec.GodID, ID: spec.ID, Name: spec.Name, God: spec.God, Temple: temple, EffectType: spec.EffectType,
			Default: spec.ID == religion.DefaultBattleFrenzyFor(string(sett.CultureID)), ActiveRemainingDays: activeRemainingDays,
			Effect:    spec.Description,
			MinKharis: spec.MinKharis, Offering: spec.Offering, Affordable: afford, UnavailableReason: reason,
			CooldownRemainingDays: cooldownRemainingDays,
			RestRemainingDays:     cooldownRemainingDays,
			RestPaused:            restPaused,
			TempleLevel:           templeInfo.Level,
			TempleStaffed:         templeInfo.Staffed,
			TempleCapacity:        templeInfo.Capacity,
			BaseDurationDays:      prayerBaseDuration(spec.EffectType),
			Favours:               religion.FavoursFor(spec),
			OfferingBaseline:      religion.TraditionalBaseline(spec, divineValues),
		})
	}

	return prayers, divineValues, nil
}

// prayerBaseDuration is the length (game days) a lasting prayer holds for the
// traditional gift; 0 for prayers without a length. A larger gift lasts longer.
func prayerBaseDuration(effect string) int {
	switch effect {
	case religion.EffectGarrisonDefense:
		return combat.GarrisonDefenseTicks
	case religion.EffectHarvestBlessing:
		return economy.HarvestBlessingTicks
	}
	return 0
}

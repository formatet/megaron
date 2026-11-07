package handlers

// The three world blessings of bönesystemet v3 steg 5a: Potnia's fertility, Velchanos's
// Builder's Blessing and Potnia's scapegoat. They are laid on a city or the realm.

import (
	"context"
	"fmt"
	"net/http"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// checkWorldBlessingTarget validates the target step inside the cast TX: a city or the realm
// (the scapegoat only the realm), and not the same thing twice at once. code != 0 is the refusal.
func (h *SettlementHandler) checkWorldBlessingTarget(ctx context.Context, tx pgx.Tx, playerID, settlementID uuid.UUID,
	spec religion.PrayerSpec, scope string, nowTick int) (string, int, string) {
	ok := false
	for _, sc := range religion.WorldBlessingScopes(spec.EffectType) {
		ok = ok || sc == scope
	}
	if !ok {
		if spec.EffectType == religion.EffectScapegoat {
			return "", http.StatusBadRequest, fmt.Sprintf("%s is for the whole realm: the punishment and the gods' mercy are the realm's (target_scope realm)", spec.Name)
		}
		return "", http.StatusBadRequest, fmt.Sprintf("choose a target_scope for %s: city or realm", spec.Name)
	}
	var city *uuid.UUID
	if scope == religion.ScopeCity {
		city = &settlementID
	}
	active, err := religion.BlessingActive(ctx, tx, playerID, spec.EffectType, scope, nil, city, nowTick)
	if err != nil {
		return "", http.StatusInternalServerError, "could not read active blessings"
	}
	if active {
		if spec.EffectType == religion.EffectScapegoat {
			return "", http.StatusConflict, "a scapegoat already stands for you"
		}
		return "", http.StatusConflict, "already blessed — wait for the blessing to end"
	}
	return scope, 0, ""
}

// applyWorldBlessing lays the row. Fertility and the Builder's Blessing last a gift-scaled span;
// the scapegoat is fixed (it bears one punishment). The Builder's Blessing also cuts what is
// being built right now, in this same TX.
func (h *SettlementHandler) applyWorldBlessing(ctx context.Context, tx pgx.Tx, worldID, playerID, settlementID uuid.UUID,
	spec religion.PrayerSpec, scope string, nowTick int, giftFactor float64) (map[string]any, string, int, error) {
	duration := 0
	switch spec.EffectType {
	case religion.EffectBlessedFertility:
		duration = religion.DurationTicks(religion.FertilityTicks, giftFactor)
	case religion.EffectBlessedBuilding:
		duration = religion.DurationTicks(religion.BuildingBlessingTicks, giftFactor)
	}
	span := duration
	if spec.EffectType == religion.EffectScapegoat {
		span = religion.ScapegoatTicks
	}
	until := nowTick + span
	var city *uuid.UUID
	if scope == religion.ScopeCity {
		city = &settlementID
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO prayer_blessings(world_id,owner_id,effect,scope,settlement_id,from_tick,until_tick) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		worldID, playerID, spec.EffectType, scope, city, nowTick, until).Scan(&id); err != nil {
		return nil, "", 0, err
	}
	payload := map[string]any{"target_scope": scope, "blessing_id": id, "until_tick": until}
	where := "this city"
	if scope == religion.ScopeRealm {
		where = "the whole realm"
	}
	var message string
	switch spec.EffectType {
	case religion.EffectBlessedFertility:
		payload["duration_ticks"] = duration
		payload["growth_factor"] = economy.FertilityGrowthFactor
		message = fmt.Sprintf("Potnia blesses %s — people and herds grow half as fast again for %s.", where, tick.FormatDays(duration))
	case religion.EffectBlessedBuilding:
		payload["duration_ticks"] = duration
		payload["time_factor"] = religion.BuildTimeFactor
		cities := []uuid.UUID{settlementID}
		if scope == religion.ScopeRealm {
			rows, err := tx.Query(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND owner_id=$2 AND state NOT IN ('sunk','collapsed','razed') ORDER BY id`, worldID, playerID)
			if err != nil {
				return nil, "", 0, err
			}
			cities = cities[:0]
			for rows.Next() {
				var c uuid.UUID
				if err := rows.Scan(&c); err != nil {
					rows.Close()
					return nil, "", 0, err
				}
				cities = append(cities, c)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, "", 0, err
			}
		}
		sped, err := economy.BlessBuilds(ctx, tx, cities)
		if err != nil {
			return nil, "", 0, err
		}
		payload["sped_up"] = sped
		message = fmt.Sprintf("Velchanos steadies the builders of %s — what is left of each building and ship under construction takes three quarters of the time, and so does each one begun, for %s.", where, tick.FormatDays(duration))
		if len(sped) > 0 {
			message += fmt.Sprintf(" %d already under way will be ready sooner.", len(sped))
		}
	case religion.EffectScapegoat:
		message = fmt.Sprintf("Potnia chooses a goat from your herd — it bears the next divine punishment that would strike any of your cities within %s.", tick.FormatDays(span))
	}
	return payload, message, duration, nil
}

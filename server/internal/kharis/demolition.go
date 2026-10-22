package kharis

import (
	"context"
	"math"

	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

const TempleDemolitionPenaltyPerLevel = 10.0

// TempleDemolition is a read-only quote before deletion and the actual outcome
// when read under the demolition transaction's realm and building locks.
type TempleDemolition struct {
	GodID           string  `json:"god_id"`
	GodName         string  `json:"god_name"`
	Level           int     `json:"level"`
	PenaltyPerLevel float64 `json:"penalty_per_level"`
	KharisLoss      float64 `json:"kharis_loss"`
	KharisBefore    float64 `json:"kharis_before"`
	KharisAfter     float64 `json:"kharis_after"`
	CapBefore       float64 `json:"cap_before"`
	CapAfter        float64 `json:"cap_after"`
}

func ProjectTempleDemolition(ctx context.Context, db tick.Queryer, player, world, building uuid.UUID) (*TempleDemolition, error) {
	d := &TempleDemolition{PenaltyPerLevel: TempleDemolitionPenaltyPerLevel}
	var rawCap float64
	var beforeLevel, afterLevel int
	err := db.QueryRow(ctx, `SELECT COALESCE(b.god_id,''),b.level,
 GREATEST($4,settled(pwr.kharis_amount,pwr.kharis_rate,pwr.kharis_calc_tick)),pwr.kharis_cap,
 COALESCE((SELECT MAX(tb.level) FROM buildings tb JOIN settlements ts ON ts.id=tb.settlement_id
 WHERE tb.building_type='temple' AND ts.owner_id=$1 AND ts.world_id=$2 AND ts.state NOT IN ('sunk','collapsed','razed')),0),
 COALESCE((SELECT MAX(tb.level) FROM buildings tb JOIN settlements ts ON ts.id=tb.settlement_id
 WHERE tb.building_type='temple' AND tb.id<>$3 AND ts.owner_id=$1 AND ts.world_id=$2 AND ts.state NOT IN ('sunk','collapsed','razed')),0)
 FROM buildings b JOIN settlements s ON s.id=b.settlement_id
 JOIN player_world_records pwr ON pwr.player_id=$1 AND pwr.world_id=$2
 WHERE b.id=$3 AND b.building_type='temple' AND s.owner_id=$1 AND s.world_id=$2`, player, world, building, Floor).Scan(&d.GodID, &d.Level, &d.KharisBefore, &rawCap, &beforeLevel, &afterLevel)
	if err != nil {
		return nil, err
	}
	if god, ok := religion.TempleGodByID(d.GodID); ok {
		d.GodName = god.Name
	}
	d.KharisAfter = math.Max(Floor, d.KharisBefore-d.PenaltyPerLevel*float64(d.Level))
	d.KharisLoss = d.KharisBefore - d.KharisAfter
	d.CapBefore = EffectiveKharisCap(rawCap, beforeLevel)
	d.CapAfter = EffectiveKharisCap(rawCap, afterLevel)
	return d, nil
}

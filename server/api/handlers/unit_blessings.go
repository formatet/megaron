package handlers

import (
	"context"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

// attachPermanentBlessings fills hull_max and the "for ever" lines of the OWNER's unit list
// (böner steg 4): hull 6/6 for a ship with Stout Hulls, and one line per permanent blessing.
// Other Wanaxer never get this: foreignUnit has no such fields.
func attachPermanentBlessings(ctx context.Context, db province.Queryer, worldID, playerID uuid.UUID, units []*unit.Unit, summaries []unitSummary) {
	index := make(map[uuid.UUID]int, len(summaries))
	for i, s := range summaries {
		index[s.ID] = i
	}
	for _, u := range units {
		i, ok := index[u.ID]
		if !ok || u.Category != unit.CategoryNaval {
			continue
		}
		summaries[i].HullMax = combat.HullCap(u.HullBonus)
		if u.HullBonus > 0 {
			summaries[i].Blessings = append(summaries[i].Blessings, "Stout Hulls for ever")
		}
	}
	rows, err := db.Query(ctx, `SELECT unit_id, effect FROM prayer_blessings WHERE world_id=$1 AND owner_id=$2 AND permanent AND unit_id IS NOT NULL ORDER BY effect`, worldID, playerID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var effect string
		if rows.Scan(&id, &effect) != nil {
			continue
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		switch effect {
		case religion.EffectBlessedStrength:
			summaries[i].Blessings = append(summaries[i].Blessings, "blessed by Velchanos, for ever")
		case religion.EffectBlessedSpeedSea:
			summaries[i].Blessings = append(summaries[i].Blessings, "followed by the Sea Daimons, for ever")
		}
	}
}

package handlers

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"sort"
	"strings"
)

// BuildQuote describes one construction target. NewSite quotes are for a new
// hex building; existing hexes carry their own levels rather than a city MAX.
// It is a price quote, not permission to skip terrain/ownership/queue gates.
type BuildQuote struct {
	Type         string             `json:"type"`
	CurrentLevel int                `json:"current_level"`
	NextLevel    *int               `json:"next_level"`
	AtMax        bool               `json:"at_max_level"`
	Costs        map[string]float64 `json:"costs"`
	CostSilver   float64            `json:"cost_silver"`
	Duration     int                `json:"duration_game_days"`
	HexQ         *int               `json:"hex_q,omitempty"`
	HexR         *int               `json:"hex_r,omitempty"`
	NewSite      bool               `json:"new_site,omitempty"`
	GodID        string             `json:"god_id,omitempty"`
	Name         string             `json:"name,omitempty"`
}

// forGod names a temple quote after its god ("Temple of Potnia"); nil leaves
// every other building as it is.
func (q BuildQuote) forGod(godID *string) BuildQuote {
	if godID != nil {
		q.GodID = *godID
		q.Name = religion.TempleName(*godID)
	}
	return q
}

func quoteBuild(bt province.BuildingType, current int, q, r *int, newSite bool) BuildQuote {
	out := BuildQuote{Type: string(bt), CurrentLevel: current, HexQ: q, HexR: r, NewSite: newSite}
	maxLevel := 1
	if province.LevelledBuildings[bt] || bt == province.BuildingWall {
		maxLevel = province.MaxBuildingLevel
	}
	if current >= maxLevel {
		out.AtMax = true
		return out
	}
	next := current + 1
	out.NextLevel = &next
	spec, ok := province.LevelledSpec(bt, next)
	if bt == province.BuildingWall {
		spec, ok = province.WallLevelSpecs[next]
	}
	if !ok {
		out.AtMax = true
		out.NextLevel = nil
		return out
	}
	out.Costs = spec.Costs
	out.CostSilver = spec.CostSilver
	out.Duration = spec.DurationTicks
	return out
}
func loadBuildQuotes(ctx context.Context, db economy.Tx, sid uuid.UUID) ([]BuildQuote, error) {
	var wall int
	if err := db.QueryRow(ctx, `SELECT wall_level FROM settlements WHERE id=$1`, sid).Scan(&wall); err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT building_type,level,hex_q,hex_r,god_id FROM buildings WHERE settlement_id=$1 ORDER BY building_type,hex_q,hex_r`, sid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	city := map[string]int{}
	temples := map[string]int{}
	var hexes []BuildQuote
	for rows.Next() {
		var bt string
		var level int
		var q, r *int
		var god *string
		if err := rows.Scan(&bt, &level, &q, &r, &god); err != nil {
			return nil, err
		}
		if god != nil {
			temples[*god] = level
		} else if province.HexBoundBuildings[province.BuildingType(bt)] {
			hexes = append(hexes, quoteBuild(province.BuildingType(bt), level, q, r, false))
		} else {
			city[bt] = max(city[bt], level)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var types []string
	for bt := range province.BuildingSpecs {
		types = append(types, string(bt))
	}
	sort.Strings(types)
	out := make([]BuildQuote, 0, len(types)+len(hexes))
	for _, bt := range types {
		if bt == string(province.BuildingTemple) {
			// One quote per god: each god's temple has its own level (ORDER 15).
			for _, g := range religion.TempleGods {
				out = append(out, quoteBuild(province.BuildingTemple, temples[g.ID], nil, nil, false).forGod(&g.ID))
			}
			continue
		}
		current := city[bt]
		if bt == string(province.BuildingWall) {
			current = wall
		}
		out = append(out, quoteBuild(province.BuildingType(bt), current, nil, nil, province.HexBoundBuildings[province.BuildingType(bt)]))
	}
	return append(out, hexes...), nil
}

type buildErrorContext struct {
	BuildQuote
	City string `json:"city"`
}

func (b buildErrorContext) label() string {
	if b.Name != "" {
		return b.Name
	}
	label := strings.ReplaceAll(b.Type, "_", " ")
	if len(label) > 0 {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	if b.HexQ != nil && b.HexR != nil {
		label += fmt.Sprintf(" at (%d,%d)", *b.HexQ, *b.HexR)
	}
	return label
}
func (b buildErrorContext) message(shortage string) string {
	label := b.label()
	if b.AtMax {
		return fmt.Sprintf("%s already has %s level %d, the maximum level.", b.City, label, b.CurrentLevel)
	}
	action := fmt.Sprintf("Building %s at level %d", label, *b.NextLevel)
	if b.CurrentLevel > 0 {
		action = fmt.Sprintf("Raising %s to level %d", label, *b.NextLevel)
	}
	result := action + " needs " + strings.TrimPrefix(shortage, "insufficient resources: ") + "."
	if b.CurrentLevel > 0 {
		result += fmt.Sprintf(" %s already has %s level %d.", b.City, label, b.CurrentLevel)
	}
	return result
}

package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const openingDispatchKind = "OpeningDispatch"

type openingDispatchBody struct {
	Title      string    `json:"title"`
	Paragraphs []string  `json:"paragraphs"`
	Returning  bool      `json:"returning"`
	HostID     uuid.UUID `json:"host_id"`
}

func openingDayLength(seconds int) string {
	value, unit := float64(seconds), "second"
	if seconds%3600 == 0 {
		value, unit = value/3600, "hour"
	} else if seconds%60 == 0 {
		value, unit = value/60, "minute"
	}
	if value != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%g %s", value, unit)
}

func (h *JoinHandler) openingDispatch(hostID uuid.UUID, spec hostSpec) openingDispatchBody {
	body := openingDispatchBody{Title: "Your people await you", Returning: spec.refugee, HostID: hostID}
	host := fmt.Sprintf("Your Host carries %g grain and %g silver. It lives on subsistence until founding; these reserves do not drain. Find your place — founding is final.", float64(nomadicHostDowryGrain), spec.silver)
	if spec.refugee {
		body.Title = "Begin again"
		host = fmt.Sprintf("Your survivors gather into a new Host, with %g grain and %g silver. Your standing with the gods is kept. You live on subsistence until founding; find a new home.", float64(nomadicHostDowryGrain), spec.silver)
	}
	body.Paragraphs = []string{
		fmt.Sprintf("Time. A day passes every %s. Days begin after %d Wanaxes join.", openingDayLength(tick.TickSeconds), h.startWanaxes),
		fmt.Sprintf("Foot soldiers cross a plains hex in %g day, olive woodland in %g, hills or cedar forest in %g, desert in %g. War chariots cross plains in %g day but need %g in olive woodland and %g in hills, cedar forest or desert. A full-crewed galley crosses %g coastal or %g open-sea hexes a day.", province.TerrainMoveTicks("plains"), province.TerrainMoveTicks("forest_olive_grove"), province.TerrainMoveTicks("hills"), province.TerrainMoveTicks("semi_desert"), province.TerrainMoveTicksFor("plains", province.CategoryWarChariot), province.TerrainMoveTicksFor("forest_olive_grove", province.CategoryWarChariot), province.TerrainMoveTicksFor("hills", province.CategoryWarChariot), 1/province.TerrainMoveTicksFor("coastal_sea", "naval"), 1/province.TerrainMoveTicksFor("deep_sea", "naval")),
		host,
		fmt.Sprintf("Food first. A city must feed itself. Starvation makes it shrink; below %d people it falls.", economy.CollapseBelowPopulation),
		fmt.Sprintf("The gods. You are safe from punishment for %d days after founding. A temple takes %d days to build. Build, staff and feed one in time; neglect afterward risks punishment.", kharis.FoundingGraceTicks(), province.BuildingSpecs[province.BuildingTemple].DurationTicks),
		"Orders travel. A Runner must reach a unit in the field before it follows your new order.",
		"Bronze needs others. Copper and tin lie on separate landmasses; trade, colonies or seizures can bring them together.",
		"Rules: ? (Codex). Problems: Report. Community chat: Account.",
	}
	return body
}

// persistOpeningDispatch is in the same transaction as the new Host life.
// The normal persisted-message path pushes only after that transaction commits.
func (h *JoinHandler) persistOpeningDispatch(ctx context.Context, tx pgx.Tx, worldID, playerID, hostID uuid.UUID, spec hostSpec) (notify.Msg, error) {
	body := h.openingDispatch(hostID, spec)
	raw, err := json.Marshal(body)
	if err != nil {
		return notify.Msg{}, err
	}
	msg := notify.Msg{Kind: openingDispatchKind, WorldID: worldID.String(), Payload: body, Level: 4}
	if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json)
	 VALUES($1,$2,$3,$4,$5) RETURNING id`, worldID, playerID, openingDispatchKind, msg.Level, raw).Scan(&msg.ID); err != nil {
		return notify.Msg{}, err
	}
	return msg, nil
}

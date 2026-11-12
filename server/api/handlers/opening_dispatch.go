package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const openingDispatchKind = "OpeningDispatch"

type openingDispatchBody struct {
	Title      string    `json:"title"`
	Paragraphs []string  `json:"paragraphs"` // who you are, then the game-day line
	Steps      []string  `json:"steps"`      // what to do first, shown as a list; older dispatches have none
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

// openingDispatch is the first thing a new Wanax reads: who you are, five things to do first, how long a
// game day is. Every rule it used to carry (movement times, reserves, food, gods, runners, bronze) lives in
// the Codex (welcome, horde, marching, food, kharis, runners, bronze) and nowhere else.
func (h *JoinHandler) openingDispatch(hostID uuid.UUID, spec hostSpec, forming bool) openingDispatchBody {
	body := openingDispatchBody{Title: "Your people await you", Returning: spec.refugee, HostID: hostID}
	who := "You are a Wanax, a ruler. Your people are a wandering Host until you found a city."
	found := "Found your city where the ground suits you. It cannot be undone."
	if spec.refugee {
		body.Title = "Begin again"
		who = "Your survivors gather into a new Host and your standing with the gods is kept. You are still a Wanax, a ruler."
		found = "Found a new city where the ground suits you. It cannot be undone."
	}
	timeLine := fmt.Sprintf("A game day passes every %s in this world.", openingDayLength(tick.TickSeconds))
	if forming {
		timeLine += fmt.Sprintf(" Game days begin after %d Wanaxes join.", h.startWanaxes)
	}
	body.Paragraphs = []string{who, timeLine}
	body.Steps = []string{
		"Look at the land: click a hex to see what lies on it.",
		"Click your Host on the map.",
		found,
		"Put your citizens to work, food first.",
		"Read the Codex (?) for the rest.",
	}
	return body
}

// persistOpeningDispatch is in the same transaction as the new Host life.
// The normal persisted-message path pushes only after that transaction commits.
func (h *JoinHandler) persistOpeningDispatch(ctx context.Context, tx pgx.Tx, worldID, playerID, hostID uuid.UUID, spec hostSpec) (notify.Msg, error) {
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM worlds WHERE id=$1`, worldID).Scan(&state); err != nil {
		return notify.Msg{}, err
	}
	body := h.openingDispatch(hostID, spec, state == "forming")
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

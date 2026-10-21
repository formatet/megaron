package handlers

import (
	"context"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// runnerOwn is one of the caller's OWN runners. Group is where it is now:
// on_the_way, waiting (for a ship), aboard, homebound or no_word. The shape is
// what web and keryx both print — grouping and wording are the server's.
type runnerOwn struct {
	ID    uuid.UUID `json:"id"`
	Group string    `json:"group"`
	// Kind is letter, offer or order. The words themselves are never here;
	// an order's one-line summary is (messenger.OwnOrderSummary, the sender's own).
	Kind         string  `json:"kind"`
	OrderSummary *string `json:"order_summary,omitempty"`
	ToCity       *string `json:"to_city,omitempty"`
	ToWanax      *string `json:"to_wanax,omitempty"`
	// Returning: on its way back home (also while it waits for a ship home).
	Returning bool `json:"returning,omitempty"`
	// ArrivesAt: landfall at the target (on_the_way, aboard) or home (homebound).
	ArrivesAt *time.Time `json:"arrives_at,omitempty"`
	// Waiting for a ship.
	PortName     *string    `json:"port_name,omitempty"`
	WaitingSince *time.Time `json:"waiting_since,omitempty"`
	Explanation  *string    `json:"explanation,omitempty"`
	// Aboard: only what the runner itself lives through — the ship, who owns
	// it, when it went aboard and when the ship makes landfall.
	ShipName  *string    `json:"ship_name,omitempty"`
	ShipWanax *string    `json:"ship_wanax,omitempty"`
	BoardedAt *time.Time `json:"boarded_at,omitempty"`
	LandsAt   *time.Time `json:"lands_at,omitempty"`
	// Affordances, from the same rules ListSent uses.
	CanCallBack       bool                  `json:"can_call_back"`
	CanArrangePassage bool                  `json:"can_arrange_passage"`
	EligibleShips     []eligiblePassageShip `json:"eligible_ships,omitempty"`
}

// runnerForeign is somebody else's runner whose presence the caller KNOWS:
// standing in the caller's city or port, or aboard the caller's ship. It names
// the sender and where it is — never a kind, text, offer, order or route.
type runnerForeign struct {
	Group       string     `json:"group"` // in_your_city | aboard_your_ship
	SenderWanax string     `json:"sender_wanax"`
	Where       *string    `json:"where,omitempty"`       // your city or port it stands in
	WaitingFor  string     `json:"waiting_for,omitempty"` // your_answer | a_ship
	Since       *time.Time `json:"since,omitempty"`
	ShipName    *string    `json:"ship_name,omitempty"`
	HeadingTo   *string    `json:"heading_to,omitempty"` // your ship's own destination
	LandsAt     *time.Time `json:"lands_at,omitempty"`
}

const noWordSQL = `EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.event_type='CarrierPassengerRescuedV1')`

// Runners handles GET /worlds/{worldID}/runners — the Diplomacy → Runners tab.
func (h *MessengerHandler) Runners(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ctx := r.Context()
	anchor, err := tick.LoadAnchor(ctx, h.pool, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load runners")
		return
	}
	own, err := h.runnersOwn(ctx, worldID, playerID, anchor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load runners")
		return
	}
	foreign, err := h.runnersForeign(ctx, worldID, playerID, anchor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load runners")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"own": own, "foreign": foreign})
}

func (h *MessengerHandler) runnersOwn(ctx context.Context, worldID, playerID uuid.UUID, anchor tick.Anchor) ([]runnerOwn, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT m.id, m.kind, (m.trade_offer IS NOT NULL), m.status, m.passage_status, m.passage_port_id, pps.name, pps.owner_id,
		        m.passage_since_tick, ds.name, COALESCE(dpl.wanax_name, dpl.username), os.name,
		        m.arrives_at, m.carrier_name, COALESCE(cpl.wanax_name, cpl.username), m.boarded_at, m.disembark_at,
		        `+noWordSQL+` AND m.status NOT IN ('arrived','lost')
		   FROM messengers m
		   LEFT JOIN settlements ds ON ds.id = m.destination_id
		   LEFT JOIN players dpl ON dpl.id = ds.owner_id
		   LEFT JOIN settlements os ON os.id = m.origin_id
		   LEFT JOIN settlements pps ON pps.id = m.passage_port_id
		   LEFT JOIN units cu ON cu.id = m.carrier_unit_id
		   LEFT JOIN players cpl ON cpl.id = COALESCE(cu.owner_id, m.carrier_owner_id)
		  WHERE m.world_id = $1 AND m.sender_id = $2 AND m.status IN ('outbound','returning')
		  ORDER BY m.sent_at`, worldID, playerID)
	if err != nil {
		return nil, err
	}
	type rawRow struct {
		runnerOwn
		status, passage   string
		passageStatus     *string
		portID, portOwner *uuid.UUID
		sinceTick         *int
		originName        *string
		trade, noWord     bool
	}
	var raws []rawRow
	for rows.Next() {
		var x rawRow
		var arrives time.Time
		if err := rows.Scan(&x.ID, &x.Kind, &x.trade, &x.status, &x.passageStatus, &x.portID, &x.PortName, &x.portOwner,
			&x.sinceTick, &x.ToCity, &x.ToWanax, &x.originName,
			&arrives, &x.ShipName, &x.ShipWanax, &x.BoardedAt, &x.LandsAt, &x.noWord); err != nil {
			rows.Close()
			return nil, err
		}
		x.ArrivesAt = &arrives
		raws = append(raws, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []runnerOwn{}
	order := map[string]int{"waiting": 0, "on_the_way": 1, "aboard": 2, "homebound": 3, "no_word": 4}
	for _, x := range raws {
		it := x.runnerOwn
		switch {
		case it.Kind == "order":
			it.Kind = "order"
		case x.trade:
			it.Kind = "offer"
		default:
			it.Kind = "letter"
		}
		if it.Kind == "order" {
			if s := messenger.OwnOrderSummary(ctx, h.pool, worldID, playerID, it.ID); s != "" {
				it.OrderSummary = &s
			}
		}
		if x.status == "returning" {
			it.ToCity, it.ToWanax, it.Returning = x.originName, nil, true
		}
		ps := ""
		if x.passageStatus != nil {
			ps = *x.passageStatus
		}
		switch {
		case x.noWord:
			it = runnerOwn{ID: it.ID, Group: "no_word", Kind: it.Kind}
		case ps == "awaiting_passage" && x.portID != nil && x.portOwner != nil:
			it.Group = "waiting"
			it.ArrivesAt, it.BoardedAt, it.LandsAt, it.ShipName, it.ShipWanax = nil, nil, nil, nil, nil
			if x.sinceTick != nil {
				at := anchor.WallAt(int64(*x.sinceTick) * 1000)
				it.WaitingSince = &at
			}
			ownPort := *x.portOwner == playerID
			it.CanArrangePassage = true
			if ships, shErr := eligiblePassageShips(ctx, h.pool, worldID, playerID, *x.portID, *x.portOwner); shErr == nil {
				it.EligibleShips = ships
			}
			it.CanCallBack = x.status == "outbound" && ownPort
			var targetName *string
			if q, rr, tErr := messenger.FinalTarget(ctx, h.pool, it.ID); tErr == nil {
				var n string
				if h.pool.QueryRow(ctx,
					`SELECT s.name FROM provinces p JOIN settlements s ON s.province_id = p.id
					  WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active' LIMIT 1`,
					worldID, q, rr).Scan(&n) == nil {
					targetName = &n
				}
			}
			port := ""
			if it.PortName != nil {
				port = *it.PortName
			}
			text := messenger.StalledExplanation(port, targetName, ownPort, len(it.EligibleShips))
			it.Explanation = &text
		case ps == "returning_sealed":
			it.Group = "waiting"
			it.ArrivesAt, it.BoardedAt, it.LandsAt, it.ShipName, it.ShipWanax = nil, nil, nil, nil, nil
			text := "Its ship was lost. The runner is making its way back to port and will wait there for another ship."
			it.Explanation = &text
		case ps == "aboard":
			it.Group = "aboard"
		case x.status == "returning":
			it.Group = "homebound"
			it.BoardedAt, it.LandsAt, it.ShipName, it.ShipWanax = nil, nil, nil, nil
		default:
			it.Group = "on_the_way"
			it.BoardedAt, it.LandsAt, it.ShipName, it.ShipWanax = nil, nil, nil, nil
		}
		if it.Group != "waiting" {
			it.PortName = nil
		}
		out = append(out, it)
	}
	// Stable group order; within a group, send order is kept.
	sorted := make([]runnerOwn, 0, len(out))
	for g := 0; g <= 4; g++ {
		for _, it := range out {
			if order[it.Group] == g {
				sorted = append(sorted, it)
			}
		}
	}
	return sorted, nil
}

func (h *MessengerHandler) runnersForeign(ctx context.Context, worldID, playerID uuid.UUID, anchor tick.Anchor) ([]runnerForeign, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT m.passage_status, COALESCE(spl.wanax_name, spl.username, ''), pps.name, ds.name,
		        m.passage_since_tick, m.carrier_name, m.disembark_at, m.disembark_q, m.disembark_r
		   FROM messengers m
		   JOIN players spl ON spl.id = m.sender_id
		   LEFT JOIN settlements ds ON ds.id = m.destination_id
		   LEFT JOIN settlements pps ON pps.id = m.passage_port_id
		   LEFT JOIN units cu ON cu.id = m.carrier_unit_id
		  WHERE m.world_id = $1 AND m.sender_id <> $2
		    AND NOT `+noWordSQL+`
		    AND ( (m.passage_status = 'awaiting_passage' AND pps.owner_id = $2)
		       OR (m.passage_status = 'aboard' AND COALESCE(cu.owner_id, m.carrier_owner_id) = $2)
		       OR (m.status = 'delivered' AND m.kind <> 'order' AND ds.owner_id = $2) )
		  ORDER BY m.sent_at`, worldID, playerID)
	if err != nil {
		return nil, err
	}
	type landing struct{ q, r *int }
	var items []runnerForeign
	var lands []landing
	for rows.Next() {
		var ps *string
		var port, dest *string
		var since *int
		var it runnerForeign
		var l landing
		if err := rows.Scan(&ps, &it.SenderWanax, &port, &dest, &since, &it.ShipName, &it.LandsAt, &l.q, &l.r); err != nil {
			rows.Close()
			return nil, err
		}
		switch {
		case ps != nil && *ps == "aboard":
			it.Group = "aboard_your_ship"
		case ps != nil && *ps == "awaiting_passage":
			it.Group, it.Where, it.WaitingFor = "in_your_city", port, "a_ship"
			if since != nil {
				at := anchor.WallAt(int64(*since) * 1000)
				it.Since = &at
			}
		default:
			it.Group, it.Where, it.WaitingFor = "in_your_city", dest, "your_answer"
		}
		if it.Group != "aboard_your_ship" {
			it.ShipName, it.LandsAt = nil, nil
		}
		items = append(items, it)
		lands = append(lands, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, it := range items {
		if it.Group != "aboard_your_ship" || lands[i].q == nil || lands[i].r == nil {
			continue
		}
		var n string
		if h.pool.QueryRow(ctx,
			`SELECT s.name FROM provinces p JOIN settlements s ON s.province_id = p.id
			  WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active' LIMIT 1`,
			worldID, *lands[i].q, *lands[i].r).Scan(&n) == nil {
			items[i].HeadingTo = &n
		}
	}
	if items == nil {
		items = []runnerForeign{}
	}
	return items, nil
}

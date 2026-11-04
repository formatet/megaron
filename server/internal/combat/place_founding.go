package combat

// Places inside a new city's own catchment are announced ONCE, at the founding
// (Timothy 2026-11-04): a single PlacesInNewCity notice, not one PlaceFirstReached
// per place hours later when a unit happens to walk there, and not silence.
// The claim is the same one recordPlaceEntriesTx makes (place_names +
// claimed_by IS NULL), so a later step through the catchment finds the right
// already claimed and sends nothing.
import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FoundingPlaces is a committed-on-commit PlacesInNewCity notice. Deliver it
// only after the founding transaction has committed.
type FoundingPlaces struct {
	event *events.Event
	owner uuid.UUID
	id    string
	body  map[string]any
}

// Deliver records the event for sinks and pushes the archived notification.
func (f *FoundingPlaces) Deliver(ctx context.Context, store *events.Store, hub Broadcaster, worldID uuid.UUID) {
	if f == nil {
		return
	}
	store.RecordCommitted(ctx, f.event)
	if d, ok := hub.(CommittedNotificationDelivery); ok {
		d.DeliverCommittedNotification(ctx, worldID, f.owner, f.id, "PlacesInNewCity", 1, f.body)
	}
}

// Event is the appended event, for a caller with no hub.
func (f *FoundingPlaces) Event() *events.Event { return f.event }

func (f *FoundingPlaces) outcome() expeditionOutcome {
	return expeditionOutcome{f.event, f.owner, f.id, "PlacesInNewCity", 1, f.body}
}

// ClaimFoundingPlaces claims, for ownerID, every unclaimed place that touches the
// catchment of the settlement just founded at (q, r), and writes one
// PlacesInNewCity notification. It returns nil when there is nothing to announce.
// Runs inside the founding transaction.
func ClaimFoundingPlaces(ctx context.Context, tx pgx.Tx, store *events.Store, worldID, ownerID, settlementID uuid.UUID, settlementName string, q, r int) (*FoundingPlaces, error) {
	rows, err := tx.Query(ctx, `SELECT q,r,terrain FROM map_tiles WHERE world_id=$1`, worldID)
	if err != nil {
		return nil, err
	}
	var tiles []world.MapTile
	for rows.Next() {
		var t world.MapTile
		if err = rows.Scan(&t.Q, &t.R, &t.Terrain); err != nil {
			rows.Close()
			return nil, err
		}
		tiles = append(tiles, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var tickNow int
	if err = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&tickNow); err != nil {
		return nil, err
	}

	centre := hexgrid.Coord{Q: q, R: r}
	type found struct {
		place world.Place
		hex   [2]int // the catchment hex through which the place is reached
	}
	seen := map[string]bool{}
	var hits []found
	for hex, targets := range placeReachTargets(tiles) {
		if hexgrid.Distance(centre, hexgrid.Coord{Q: hex[0], R: hex[1]}) > hexgrid.CatchmentRadius {
			continue
		}
		for _, t := range targets {
			key := fmt.Sprintf("%s:%d,%d", t.place.Kind, t.place.Anchor[0], t.place.Anchor[1])
			if prev := seen[key]; prev {
				continue
			}
			seen[key] = true
			hits = append(hits, found{t.place, t.hex})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i].place, hits[j].place
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Anchor[0] != b.Anchor[0] {
			return a.Anchor[0] < b.Anchor[0]
		}
		return a.Anchor[1] < b.Anchor[1]
	})

	var listed []map[string]any
	for _, h := range hits {
		p := h.place
		if _, err = tx.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, p.Kind, p.Anchor[0], p.Anchor[1]); err != nil {
			return nil, err
		}
		var named *int
		err = tx.QueryRow(ctx, `UPDATE place_names SET claimed_by=$5,claimed_tick=$6 WHERE world_id=$1 AND kind=$2 AND anchor_q=$3 AND anchor_r=$4 AND claimed_by IS NULL RETURNING named_tick`, worldID, p.Kind, p.Anchor[0], p.Anchor[1], ownerID, tickNow).Scan(&named)
		if err == pgx.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		// The place is now shown to its owner: keep its witness hex seen.
		if _, err = tx.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, ownerID, h.hex[0], h.hex[1]); err != nil {
			return nil, err
		}
		if err = province.RecordSeenSettlements(ctx, tx, worldID, ownerID, []province.MapPosition{{Q: h.hex[0], R: h.hex[1]}}); err != nil {
			return nil, err
		}
		if named != nil {
			continue // admin-spent right never promises a naming action
		}
		listed = append(listed, map[string]any{"kind": p.Kind, "place_id": fmt.Sprintf("%d,%d", p.Anchor[0], p.Anchor[1]), "q": h.hex[0], "r": h.hex[1]})
	}
	if len(listed) == 0 {
		return nil, nil
	}
	body := map[string]any{"settlement_id": settlementID, "settlement_name": settlementName, "places": listed}
	raw, _ := json.Marshal(body)
	var id string
	if err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'PlacesInNewCity',1,$3) RETURNING id`, worldID, ownerID, raw).Scan(&id); err != nil {
		return nil, err
	}
	event, err := store.AppendTx(ctx, tx, worldID, events.StreamWorld, "PlacesInNewCity", body, worldID, nil)
	if err != nil {
		return nil, err
	}
	return &FoundingPlaces{event, ownerID, id, body}, nil
}

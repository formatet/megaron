package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"time"
)

type preparedRecall struct {
	payload  messenger.OrderDeliveryPayload
	text     string
	origin   orderOrigin
	position province.MapPosition
	extra    map[string]any
	inPerson bool
	route    combat.StoredRoute
	routeOK  bool
	unit     *unit.Unit
}

// pendingRecall also sees a Runner awaiting passage before events are queued.
func pendingRecall(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, unitID uuid.UUID) (bool, error) {
	var pending bool
	err := q.QueryRow(ctx, `SELECT EXISTS(
  SELECT 1 FROM scheduled_events WHERE event_type=$1 AND payload->>'unit_id'=$2::text AND payload->>'verb' IN ('recall','redirect','recall_group_member') AND processed_at IS NULL AND failed_at IS NULL
 ) OR EXISTS(
  SELECT 1 FROM messengers WHERE kind='order' AND status='outbound' AND (
   (order_payload->>'verb' IN ('recall','redirect') AND order_payload->>'unit_id'=$2::text)
   OR (order_payload->>'verb'='recall_group_member' AND EXISTS(SELECT 1 FROM jsonb_array_elements(order_payload->'recall_group'->'members') m WHERE m->>'unit_id'=$2::text))
  ))`, string(events.ScheduledOrderDelivery), unitID).Scan(&pending)
	return pending, err
}

type recallFailureWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *recallFailureWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}
func (w *recallFailureWriter) WriteHeader(status int)      { w.status = status }
func (w *recallFailureWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *recallFailureWriter) reason() string {
	var d struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w.body.Bytes(), &d)
	if d.Error == "" {
		return "could not prepare recall"
	}
	return d.Error
}

type recallBatchResult struct {
	UnitID      uuid.UUID  `json:"unit_id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
	MessengerID *uuid.UUID `json:"messenger_id,omitempty"`
}
type recallBatchGroup struct {
	MessengerID   uuid.UUID   `json:"messenger_id"`
	UnitIDs       []uuid.UUID `json:"unit_ids"`
	Q             int         `json:"q"`
	R             int         `json:"r"`
	DueTick       int         `json:"courier_due_tick"`
	ArrivesAt     time.Time   `json:"courier_arrives_at"`
	PassageStatus string      `json:"passage_status,omitempty"`
}
type plannedRecallGroup struct {
	plans            []*preparedRecall
	resultIndices    []int
	arrivesAt        time.Time
	dueTick          int
	passage          *messenger.PassagePort
	passageSinceTick int
	legacy           bool
}

// RecallBatch keeps partial failures explicit and groups only physical meetings.
func (h *UnitHandler) RecallBatch(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	var req struct {
		All     bool        `json:"all"`
		UnitIDs []uuid.UUID `json:"unit_ids"`
		TargetQ *int        `json:"target_q"`
		TargetR *int        `json:"target_r"`
		Intent  string      `json:"intent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.All == (len(req.UnitIDs) > 0) || (req.TargetQ == nil) != (req.TargetR == nil) {
		writeError(w, http.StatusBadRequest, "choose all or unit_ids, and provide both target coordinates or neither")
		return
	}
	ctx := r.Context()
	ids := req.UnitIDs
	if req.All {
		rows, err := h.pool.Query(ctx, `SELECT id FROM units WHERE world_id=$1 AND owner_id=$2 AND ($3::int IS NULL OR (target_q=$3 AND target_r=$4)) AND ($5='' OR march_intent=$5) ORDER BY id`, worldID, playerID, req.TargetQ, req.TargetR, req.Intent)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list units")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				writeError(w, 500, "could not read unit")
				return
			}
			ids = append(ids, id)
		}
		if rows.Err() != nil {
			writeError(w, 500, "could not read units")
			return
		}
		rows.Close()
	} else if req.TargetQ != nil || req.Intent != "" {
		writeError(w, 400, "filters require all")
		return
	}
	// Stable lock/order keys and one result for each distinct requested recipient.
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	results := []recallBatchResult{}
	groups := []plannedRecallGroup{}
	byKey := map[string]int{}
	seen := map[uuid.UUID]bool{}
	var currentTick int
	if err := h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick); err != nil {
		writeError(w, 500, "could not read world tick")
		return
	}
	now := h.clk.Now()
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		name := id.String()
		if u, err := h.store.Get(ctx, id); err == nil && u.OwnerID == playerID && u.WorldID == worldID {
			name = unit.LoadDisplayName(ctx, h.pool, id)
		}
		result := recallBatchResult{UnitID: id, Name: name, Status: "rejected"}
		capture := &recallFailureWriter{}
		plan, ok := h.prepareRecall(capture, ctx, worldID, playerID, id, "recall", nil, nil)
		if !ok {
			result.Reason = capture.reason()
			results = append(results, result)
			continue
		}
		if plan.inPerson {
			applied, err := combat.ExecuteRecall(ctx, h.pool, h.scheduler, h.eventStore, h.clk, *plan.payload.Recall)
			if err != nil {
				result.Reason = "could not apply recall"
			} else if applied == nil {
				result.Reason = "unit was no longer marching"
			} else {
				result.Status = "order_applied"
			}
			results = append(results, result)
			continue
		}
		at, due, passage, since, err := messenger.ResolveDeparture(ctx, h.pool, worldID, playerID, province.MapPosition{Q: plan.origin.q, R: plan.origin.r}, plan.position, now, currentTick)
		if err != nil {
			result.Reason = err.Error()
			results = append(results, result)
			continue
		}
		groupable := false
		if plan.routeOK && passage == nil && plan.unit.ArriveTick != nil {
			anchor, aErr := tick.LoadAnchor(ctx, h.pool, worldID)
			if aErr != nil {
				result.Reason = "could not resolve meeting tick"
				results = append(results, result)
				continue
			}
			// A Runner may arrive before the column. Schedule delivery at the
			// first actual shared tick/place, never the earlier arrival guess.
			for meet := currentTick + 1; meet < *plan.unit.ArriveTick; meet++ {
				pos, pErr := combat.RoutePositionAt(plan.route, int64(meet)*1000)
				if pErr != nil {
					break
				}
				arrival, d, p, ps, rErr := messenger.ResolveDeparture(ctx, h.pool, worldID, playerID, province.MapPosition{Q: plan.origin.q, R: plan.origin.r}, pos, now, currentTick)
				if rErr != nil || p != nil || d > meet || arrival.After(anchor.WallAt(int64(meet)*1000)) {
					continue
				}
				plan.position = pos
				at = anchor.WallAt(int64(meet) * 1000)
				due = meet
				passage = p
				since = ps
				groupable = true
				break
			}
		}

		// No shared ETA guesses: passage and unmatched/legacy positions get singles.
		key := id.String()
		if groupable {
			key = fmt.Sprintf("%d/%d/%d", plan.position.Q, plan.position.R, due)
		}
		index, exists := byKey[key]
		if !exists {
			index = len(groups)
			byKey[key] = index
			groups = append(groups, plannedRecallGroup{arrivesAt: at, dueTick: due, passage: passage, passageSinceTick: since, legacy: !groupable})
		}
		groups[index].plans = append(groups[index].plans, plan)
		groups[index].resultIndices = append(groups[index].resultIndices, len(results))
		results = append(results, result)
	}
	dispatched := []recallBatchGroup{}
	for _, group := range groups {
		out, err := h.dispatchRecallGroup(ctx, group, now)
		for _, index := range group.resultIndices {
			if err != nil {
				results[index].Reason = err.Error()
			} else {
				results[index].Status = "order_dispatched"
				results[index].MessengerID = &out.MessengerID
			}
		}
		if err == nil {
			dispatched = append(dispatched, out)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "groups": dispatched})
}

func (h *UnitHandler) dispatchRecallGroup(ctx context.Context, g plannedRecallGroup, now time.Time) (recallBatchGroup, error) {
	first := g.plans[0]
	out := recallBatchGroup{Q: first.position.Q, R: first.position.R, DueTick: g.dueTick, ArrivesAt: g.arrivesAt, UnitIDs: []uuid.UUID{}}
	if g.legacy {
		captured := &recallFailureWriter{}
		h.sendOrderCourier(captured, ctx, first.payload, first.text, first.origin, first.position, first.extra)
		if captured.status != http.StatusAccepted {
			return out, fmt.Errorf("%s", captured.reason())
		}
		var response struct {
			MessengerID   uuid.UUID `json:"messenger_id"`
			ArrivesAt     time.Time `json:"courier_arrives_at"`
			DueTick       int       `json:"courier_due_tick"`
			PassageStatus string    `json:"passage_status"`
		}
		if err := json.Unmarshal(captured.body.Bytes(), &response); err != nil {
			return out, err
		}
		out.MessengerID = response.MessengerID
		out.ArrivesAt = response.ArrivesAt
		out.DueTick = response.DueTick
		out.PassageStatus = response.PassageStatus
		out.UnitIDs = []uuid.UUID{first.payload.UnitID}
		return out, nil
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	group := &messenger.RecallGroup{Q: out.Q, R: out.R}
	for _, plan := range g.plans {
		// Share the unit row lock with all recall dispatchers: no check/enqueue race.
		var status string
		var owner uuid.UUID
		var raw []byte
		var depart, arrive *int
		if err := tx.QueryRow(ctx, `SELECT status,owner_id,march_route,depart_tick,arrive_tick FROM units WHERE id=$1 FOR UPDATE`, plan.payload.UnitID).Scan(&status, &owner, &raw, &depart, &arrive); err != nil {
			return out, err
		}
		if status != "marching" || owner != plan.payload.PlayerID || !bytes.Equal(raw, plan.unit.MarchRoute) || !sameRecallTick(depart, plan.unit.DepartTick) || !sameRecallTick(arrive, plan.unit.ArriveTick) {
			return out, fmt.Errorf("unit %s changed course before dispatch", plan.payload.UnitID)
		}
		pending, err := pendingRecall(ctx, tx, plan.payload.UnitID)
		if err != nil {
			return out, err
		}
		if pending {
			return out, fmt.Errorf("unit %s already has a recall/redirect Runner on its way", plan.payload.UnitID)
		}
		group.Members = append(group.Members, messenger.RecallGroupMember{UnitID: plan.payload.UnitID, DepartTick: depart, ArriveTick: arrive, Route: raw})
		out.UnitIDs = append(out.UnitIDs, plan.payload.UnitID)
	}
	payload := first.payload
	payload.Verb = messenger.RecallGroupMemberVerb
	payload.RecallGroup = group
	var since *int
	if g.passage != nil {
		since = &g.passageSinceTick
		out.PassageStatus = "awaiting_passage"
	}
	id, err := insertOrderMessenger(ctx, tx, payload, "Runner — recall orders for this column, return home.", first.origin, out.Q, out.R, g.arrivesAt, g.passage, since)
	if err != nil {
		return out, err
	}
	payload.MessengerID = id
	// Persist the same complete envelope for passage to reconstruct later.
	if _, err := tx.Exec(ctx, `UPDATE messengers SET order_payload=$2 WHERE id=$1`, id, mustJSON(payload)); err != nil {
		return out, err
	}
	if g.passage == nil {
		if err := messenger.EnqueueRecallGroup(ctx, tx, h.scheduler, payload, g.dueTick); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	out.MessengerID = id
	return out, nil
}
func sameRecallTick(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

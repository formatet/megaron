package handlers

// The Lawagetas' first steps (megaron_radgivaren.md; order lawagetas-forsta-stegen).
// GET /worlds/{worldID}/first-steps computes four done-rules from state on every
// read — no events, no notifications. This file is the ONLY place the rules and
// their texts live; web and keryx only render what it returns, so they cannot
// disagree. POST .../hide and .../show flip player_world_records.first_steps_hidden.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FirstStepsHandler serves the Lawagetas' first-steps checklist.
type FirstStepsHandler struct {
	pool *pgxpool.Pool
}

// NewFirstStepsHandler creates a FirstStepsHandler.
func NewFirstStepsHandler(pool *pgxpool.Pool) *FirstStepsHandler {
	return &FirstStepsHandler{pool: pool}
}

// FirstStepsWhenToken stands where the surface writes the wall-clock time of ReadyTick
// (web format.js / keryx ETA formatting) — the server never converts ticks.
const FirstStepsWhenToken = "{when}"

const firstStepsClosing = "That is the beginning. The rest is in the Codex."

type firstStep struct {
	Key       string `json:"key"`
	Done      bool   `json:"done"`
	Text      string `json:"text"`
	ReadyTick *int   `json:"ready_tick"`
}

type firstStepsOut struct {
	Hidden    bool        `json:"hidden"`
	DoneCount int         `json:"done_count"`
	Total     int         `json:"total"`
	Steps     []firstStep `json:"steps"`
	// Next is the key of the step the card shows: the first not-done one; while
	// the first building is still going up with every step done, "build" (the
	// wait line, with ready_tick); "" once the beginning is over (closing line).
	Next        string `json:"next"`
	ClosingText string `json:"closing_text"`
}

// computeFirstSteps evaluates the four rules, in order, for one Wanax.
func computeFirstSteps(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID) (firstStepsOut, error) {
	out := firstStepsOut{Total: 4, ClosingText: firstStepsClosing}
	var completed bool
	_ = pool.QueryRow(ctx,
		`SELECT first_steps_hidden, first_steps_completed FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		playerID, worldID).Scan(&out.Hidden, &completed)
	// The advice ends for good once every step was done: no step queries at all,
	// and growth (a free citizen) or a lost city cannot bring the card back.
	if completed {
		out.DoneCount = out.Total
		for _, key := range []string{"found", "work", "build", "explore"} {
			out.Steps = append(out.Steps, firstStep{Key: key, Done: true})
		}
		return out, nil
	}

	// found: the Wanax owns an active settlement; the capital is "the city".
	var cityID uuid.UUID
	cityName := "your city"
	err := pool.QueryRow(ctx,
		`SELECT id, name FROM settlements
		  WHERE world_id = $1 AND owner_id = $2 AND state = 'active'
		  ORDER BY is_capital DESC, id LIMIT 1`,
		worldID, playerID).Scan(&cityID, &cityName)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}

	var workDone, buildDone, exploreDone bool
	var readyTick *int
	var buildingName string
	buildCompleted := false
	if found {
		idle, err := combat.IdleGubbar(ctx, pool, cityID)
		if err != nil {
			return out, err
		}
		workDone = idle <= 0

		// build: an order exists (queued, running or finished) in any own settlement.
		// The free start farm has neither a queue row nor a BuildComplete event.
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM build_queue bq JOIN settlements s ON s.id = bq.settlement_id
			                WHERE s.world_id = $1 AND s.owner_id = $2)
			    OR EXISTS (SELECT 1 FROM scheduled_events se
			                WHERE se.world_id = $1 AND se.event_type = 'BuildComplete'
			                  AND (se.payload->>'settlement_id')::uuid IN
			                      (SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2))`,
			worldID, playerID).Scan(&buildDone); err != nil {
			return out, err
		}
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM events e JOIN settlements s ON s.id = e.stream_id
			                WHERE e.world_id = $1 AND e.event_type = 'BuildComplete' AND s.owner_id = $2)`,
			worldID, playerID).Scan(&buildCompleted); err != nil {
			return out, err
		}
		if buildDone && !buildCompleted {
			var due int
			var btype string
			err := pool.QueryRow(ctx, `
				SELECT se.due_tick, COALESCE(se.payload->>'building_type', '') FROM scheduled_events se
				 WHERE se.world_id = $1 AND se.event_type = 'BuildComplete' AND se.processed_at IS NULL
				   AND (se.payload->>'settlement_id')::uuid IN
				       (SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2)
				 ORDER BY se.due_tick LIMIT 1`,
				worldID, playerID).Scan(&due, &btype)
			if err == nil {
				readyTick = &due
				buildingName = province.BuildingDisplayName(btype)
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return out, err
			}
		}

		// explore: a march ordered by one of the Wanax's units on or after the founding
		// tick. Branch (a) walks the player's own units into the (stream_id, world_tick)
		// index. Branch (b) only matters for units since deleted (a colonist is disbanded
		// on arrival): UnitFormed events of this world since the player joined, owner in
		// the payload, then the same indexed lookup per stream.
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM units u
			    JOIN player_world_records pwr ON pwr.player_id = u.owner_id AND pwr.world_id = u.world_id
			    JOIN events e ON e.stream_id = u.id AND e.event_type = 'UnitMarchOrdered'
			   WHERE u.owner_id = $2 AND u.world_id = $1
			     AND pwr.founded_tick IS NOT NULL AND e.world_tick >= pwr.founded_tick)
			OR EXISTS (
			  SELECT 1 FROM player_world_records pwr
			    JOIN events f ON f.world_id = $1 AND f.created_at >= pwr.joined_at
			                 AND f.event_type = 'UnitFormed' AND f.payload->>'owner_id' = $2::text
			    JOIN events e ON e.stream_id = f.stream_id AND e.event_type = 'UnitMarchOrdered'
			   WHERE pwr.player_id = $2 AND pwr.world_id = $1
			     AND pwr.founded_tick IS NOT NULL AND e.world_tick >= pwr.founded_tick)`,
			worldID, playerID).Scan(&exploreDone); err != nil {
			return out, err
		}
	}

	buildText := "Order a building. Timber is usually what runs short."
	if buildDone && readyTick != nil {
		buildText = "The " + buildingName + " is ready " + FirstStepsWhenToken + ". Come back then."
	}
	workText := "There are idle hands in " + cityName + ". Put them to work, food first."
	out.Steps = []firstStep{
		{Key: "found", Done: found, Text: "Find us a home, Wanax. Click the Host, then the land around it. Found where the land will feed us."},
		{Key: "work", Done: workDone, Text: workText},
		{Key: "build", Done: buildDone, Text: buildText, ReadyTick: readyTick},
		{Key: "explore", Done: exploreDone, Text: "We know little of the land. Send people out to look."},
	}
	for _, s := range out.Steps {
		if s.Done {
			out.DoneCount++
		}
	}
	for _, s := range out.Steps {
		if !s.Done {
			out.Next = s.Key
			break
		}
	}
	if out.Next == "" && readyTick != nil {
		out.Next = "build"
	}
	if out.Next == "" {
		// All done and nothing left to wait for: the advice is over, for good.
		if _, err := pool.Exec(ctx,
			`UPDATE player_world_records SET first_steps_completed = true WHERE player_id = $1 AND world_id = $2`,
			playerID, worldID); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Get handles GET /worlds/{worldID}/first-steps (own player only).
func (h *FirstStepsHandler) Get(w http.ResponseWriter, r *http.Request) {
	worldID, playerID, ok := firstStepsParams(w, r)
	if !ok {
		return
	}
	out, err := computeFirstSteps(r.Context(), h.pool, worldID, playerID)
	if err != nil {
		slog.Error("first steps", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load first steps")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Hide handles POST /worlds/{worldID}/first-steps/hide.
func (h *FirstStepsHandler) Hide(w http.ResponseWriter, r *http.Request) { h.setHidden(w, r, true) }

// Show handles POST /worlds/{worldID}/first-steps/show.
func (h *FirstStepsHandler) Show(w http.ResponseWriter, r *http.Request) { h.setHidden(w, r, false) }

func (h *FirstStepsHandler) setHidden(w http.ResponseWriter, r *http.Request, hidden bool) {
	worldID, playerID, ok := firstStepsParams(w, r)
	if !ok {
		return
	}
	ct, err := h.pool.Exec(r.Context(),
		`UPDATE player_world_records SET first_steps_hidden = $3 WHERE player_id = $1 AND world_id = $2`,
		playerID, worldID, hidden)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	if ct.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "you are not in this world")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"hidden": hidden})
}

func firstStepsParams(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return uuid.Nil, uuid.Nil, false
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return uuid.Nil, uuid.Nil, false
	}
	return worldID, playerID, true
}

package combat

// SetStance: the stance order's validate+execute core, extracted from
// api/handlers.UnitHandler.SetStance (temenos_orderlopare_plan.md Fas 3) so it
// can run both at the HTTP layer (garrisoned units — distance 0) and when a
// runner delivers the order to a field unit. Verbatim move, no
// behaviour change.

import (
	"context"
	"encoding/json"
	"net/http"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StanceOrder is one stance command against one unit.
type StanceOrder struct {
	WorldID  uuid.UUID
	PlayerID uuid.UUID
	UnitID   uuid.UUID
	Stance   string // fortify|storm|sentry|none

	// ReactionForeign optionally overrides the foreign-relation avsikt verb
	// (avsiktslagret) applied when Stance == "sentry". Empty means the default
	// (unit.ReactionIntercept — today's hardcoded sentry behaviour, unchanged).
	// Ignored for every other stance value — see unit.ReactionPolicy's doc
	// comment for exactly what each verb does (KR3 §7: intercept/alert/escort
	// are all behaviourally wired for unit-vs-unit; caravans still only read
	// intercept).
	ReactionForeign string
}

// StanceApplied describes the applied stance (the handler's 200 body fields).
type StanceApplied struct {
	UnitID  uuid.UUID
	Stance  string // "" = none
	SentryQ *int
	SentryR *int
	// ReactionForeign is set only when Stance == "sentry" — the avsikt verb now
	// in effect against foreign units (unit.ReactionPolicy.Foreign).
	ReactionForeign string
}

// SetStance validates and executes one stance order atomically. Any
// *OrderReject return carries the HTTP status + reason exactly as the
// SetStance handler answered.
func SetStance(ctx context.Context, pool *pgxpool.Pool, eventStore *events.Store, o StanceOrder) (*StanceApplied, error) {
	// Validate stance value.
	switch o.Stance {
	case "fortify", "storm", "sentry", "none":
		// valid
	default:
		return nil, reject(http.StatusBadRequest, `invalid stance: must be "fortify", "storm", "sentry", or "none"`)
	}

	// Validate the optional avsikt override (avsiktslagret delbeslut 2, 2026-08-06:
	// all four verbs are settable now even though escort/alert have no combat or
	// notification behaviour yet — see unit.ReactionPolicy's doc comment).
	if o.ReactionForeign != "" && !unit.ValidReactionVerb(o.ReactionForeign) {
		return nil, reject(http.StatusBadRequest,
			`invalid reaction_foreign: must be "intercept", "escort", "ignore", or "alert"`)
	}

	store := unit.NewStore(pool)
	u, err := store.Get(ctx, o.UnitID)
	if err != nil {
		return nil, reject(http.StatusNotFound, "unit not found")
	}
	if u.OwnerID != o.PlayerID {
		return nil, reject(http.StatusForbidden, "not your unit")
	}
	if u.WorldID != o.WorldID {
		return nil, reject(http.StatusForbidden, "unit not in this world")
	}
	if unit.CategoryOf(u.Type) == unit.CategoryNaval {
		return nil, reject(http.StatusUnprocessableEntity, "naval units cannot take a stance")
	}
	if u.Status != unit.StatusGarrison && u.Status != unit.StatusPositioned {
		return nil, reject(http.StatusUnprocessableEntity,
			"unit cannot change stance while %s (must be garrison or positioned)", string(u.Status))
	}

	// Determine new stance value and sentry coords.
	var newStance *string
	var newSentryQ, newSentryR *int
	// newReactionPolicyRaw is nil unless stance == sentry — COALESCE below then
	// leaves the unit's existing reaction_policy untouched for every other
	// stance transition (fortify/storm/none don't touch the avsiktslagret axis).
	var newReactionPolicyRaw []byte
	var appliedReactionForeign string
	if o.Stance != "none" {
		s := o.Stance
		newStance = &s
	}
	if o.Stance == "sentry" {
		// sentry_q/r = unit's current hex position.
		// For garrisoned units, resolve via settlement province.
		var hexQ, hexR int
		if u.Q != nil && u.R != nil {
			hexQ, hexR = *u.Q, *u.R
		} else if u.SettlementID != nil {
			if err := pool.QueryRow(ctx,
				`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
				*u.SettlementID,
			).Scan(&hexQ, &hexR); err != nil {
				return nil, reject(http.StatusInternalServerError, "could not resolve unit hex for sentry")
			}
		}
		newSentryQ = &hexQ
		newSentryR = &hexR

		// Default reaction policy reproduces today's hardcoded sentry behaviour
		// (foreign→intercept) exactly; ReactionForeign leaves an explicit path to
		// choose a different avsikt verb (avsiktslagret §3/S1).
		policy := unit.DefaultReactionPolicy()
		if o.ReactionForeign != "" {
			policy.Foreign = unit.ReactionVerb(o.ReactionForeign)
		}
		appliedReactionForeign = string(policy.Foreign)
		raw, mErr := json.Marshal(policy)
		if mErr != nil {
			return nil, reject(http.StatusInternalServerError, "could not encode reaction policy")
		}
		newReactionPolicyRaw = raw
	}

	// Atomic update inside transaction with FOR UPDATE idempotency guard.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, reject(http.StatusInternalServerError, "could not begin transaction")
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var currentStance *string
	if err := tx.QueryRow(ctx,
		`SELECT status, stance FROM units WHERE id = $1 FOR UPDATE`, o.UnitID,
	).Scan(&currentStatus, &currentStance); err != nil {
		return nil, reject(http.StatusNotFound, "unit not found in transaction")
	}
	if unit.Status(currentStatus) != unit.StatusGarrison && unit.Status(currentStatus) != unit.StatusPositioned {
		return nil, reject(http.StatusConflict, "unit status changed; stance not applied")
	}

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   stance          = $2,
		   sentry_q        = $3,
		   sentry_r        = $4,
		   reaction_policy = COALESCE($5::jsonb, reaction_policy),
		   updated_at      = now()
		 WHERE id = $1`,
		o.UnitID, newStance, newSentryQ, newSentryR, newReactionPolicyRaw,
	); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not update stance")
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, reject(http.StatusInternalServerError, "could not commit stance change")
	}

	// Record stance-before for event payload.
	stanceBefore := ""
	if currentStance != nil {
		stanceBefore = *currentStance
	}
	stanceAfter := o.Stance
	if o.Stance == "none" {
		stanceAfter = ""
	}
	_, _ = eventStore.Append(ctx, o.UnitID, events.StreamType(unit.StreamUnit), unit.EventUnitStanceChanged,
		unit.UnitStanceChangedPayload{
			UnitID:          o.UnitID,
			WorldID:         o.WorldID,
			StanceBefore:    stanceBefore,
			StanceAfter:     stanceAfter,
			SentryQ:         newSentryQ,
			SentryR:         newSentryR,
			ReactionForeign: appliedReactionForeign,
		}, o.WorldID, nil,
	)

	return &StanceApplied{
		UnitID:          o.UnitID,
		Stance:          stanceAfter,
		SentryQ:         newSentryQ,
		SentryR:         newSentryR,
		ReactionForeign: appliedReactionForeign,
	}, nil
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errCounterUnavailable = errors.New("counteroffer requires a pending delivered offer addressed to you, its original city pair and counterpart")

// TradeCounter binds the person-thread action to the original cities without
// exposing geography in that thread. Send revalidates this binding under locks.
func (h *MessengerHandler) TradeCounter(w http.ResponseWriter, r *http.Request) {
	world, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	old, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, 400, "invalid messenger ID")
		return
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	var origin, dest uuid.UUID
	err = h.pool.QueryRow(r.Context(), `SELECT m.destination_id,m.origin_id FROM messengers m JOIN settlements ds ON ds.id=m.destination_id
 WHERE m.id=$1 AND m.world_id=$2 AND ds.owner_id=$3 AND correspondence_recipient(m.id,$3) AND m.trade_offer IS NOT NULL`, old, world, player).Scan(&origin, &dest)
	if err != nil {
		writeError(w, 404, "trade offer not found or not available to you")
		return
	}
	var body map[string]json.RawMessage
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
		writeError(w, 400, "invalid JSON")
		return
	}
	body["counter_offer_id"], _ = json.Marshal(old)
	body["destination_id"], _ = json.Marshal(dest)
	raw, _ := json.Marshal(body)
	r = r.Clone(r.Context())
	r.Body = io.NopCloser(bytes.NewReader(raw))
	chi.RouteContext(r.Context()).URLParams.Add("settlementID", origin.String())
	h.Send(w, r)
}

// lockCounterOfferTx follows TradeAccept's lock order: old messenger and its
// receiving city, then the original sender's city. Escrow is never refunded here.
func lockCounterOfferTx(ctx context.Context, tx pgx.Tx, world, old, player, origin, dest uuid.UUID, requestedCase string, now time.Time) (string, error) {
	var oldOrigin, oldDest, sender uuid.UUID
	var status string
	var expires *time.Time
	var caseID *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT m.origin_id,m.destination_id,m.sender_id,m.trade_offer->>'status',m.expires_at,a.case_id
 FROM messengers m JOIN settlements ds ON ds.id=m.destination_id LEFT JOIN correspondence_letters a ON a.id=m.id
 WHERE m.id=$1 AND m.world_id=$2 AND ds.owner_id=$3 AND correspondence_recipient(m.id,$3)
 AND m.status='delivered' AND m.trade_offer IS NOT NULL FOR UPDATE OF m,ds`, old, world, player).Scan(&oldOrigin, &oldDest, &sender, &status, &expires, &caseID)
	if err != nil || status != "pending" || (expires != nil && !expires.After(now)) || origin != oldDest || dest != oldOrigin {
		return "", errCounterUnavailable
	}
	var owner *uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1 AND world_id=$2 AND state='active' FOR UPDATE`, oldOrigin, world).Scan(&owner); err != nil || owner == nil || *owner != sender || sender == player {
		return "", errCounterUnavailable
	}
	if caseID != nil {
		if requestedCase != "" && requestedCase != caseID.String() {
			return "", errCounterUnavailable
		}
		return caseID.String(), nil
	}
	// Legacy letters have no provable old case; the new dispatch creates its own.
	if requestedCase != "" {
		return "", errCounterUnavailable
	}
	return "", nil
}

func (h *MessengerHandler) finishCounterOfferTx(ctx context.Context, tx pgx.Tx, world, old, next, player, origin, dest, caseID uuid.UUID, terms json.RawMessage) error {
	tag, err := tx.Exec(ctx, `UPDATE messengers SET trade_offer=trade_offer||'{"status":"declined","refund_on_return":true}'::jsonb
 WHERE id=$1 AND world_id=$2 AND trade_offer->>'status'='pending' AND (expires_at IS NULL OR expires_at>$3)`, old, world, h.clk.Now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errCounterUnavailable
	}
	if err = h.releaseOfferBearer(ctx, tx, world, old); err != nil {
		return err
	}
	_, err = events.NewStore(h.pool).AppendTx(ctx, tx, next, events.StreamProvince, "TradeOfferCounteredV1", map[string]any{
		"old_messenger_id": old, "messenger_id": next, "counter_offer_id": old, "countering_player_id": player, "origin_id": origin, "destination_id": dest, "case_id": caseID, "trade_offer": terms,
	}, world, nil)
	return err
}

// Both person-thread and legacy outbox views use the same frozen original terms.
// The sender knows its own counter link before the offer travels; remote answers
// and reservation/refund details must still wait for the returning bearer.
const pendingOfferViewSQL = `CASE WHEN m.sender_id=$2 AND m.status<>'arrived' AND m.trade_offer IS NOT NULL THEN
 (SELECT COALESCE(jsonb_object_agg(f.key,f.value),'{}'::jsonb) FROM jsonb_each(m.trade_offer) f WHERE f.key IN('kind','want_good','want_qty','offer_silver','offer_good','offer_qty','want_silver','counter_offer_id'))
 ||jsonb_build_object('status',CASE WHEN m.trade_offer->>'status'='cancelled' THEN 'cancelled' ELSE 'pending' END)
 ELSE m.trade_offer END`

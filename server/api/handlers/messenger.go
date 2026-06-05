package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
)

// MessengerHandler handles HTTP requests for messenger endpoints.
type MessengerHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
}

// NewMessengerHandler creates a MessengerHandler.
func NewMessengerHandler(pool *pgxpool.Pool, sched *events.Scheduler, clk clock.Clock) *MessengerHandler {
	return &MessengerHandler{pool: pool, scheduler: sched, clk: clk}
}

// Send handles POST /worlds/:worldID/settlements/:settlementID/messengers.
// Creates a messenger travelling from the caller's settlement to the destination.
func (h *MessengerHandler) Send(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	originID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		DestinationID string `json:"destination_id"`
		Message       string `json:"message"`
		TradeOffer    *struct {
			WantGood  string  `json:"want_good"`
			WantQty   float64 `json:"want_qty"`
			OfferSilver float64 `json:"offer_silver"`
		} `json:"trade_offer,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	destID, err := uuid.Parse(req.DestinationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid destination ID")
		return
	}
	if originID == destID {
		writeError(w, http.StatusBadRequest, "cannot send messenger to own settlement")
		return
	}
	if len(req.Message) == 0 || len(req.Message) > 1000 {
		writeError(w, http.StatusBadRequest, "message must be 1–1000 characters")
		return
	}
	if req.TradeOffer != nil {
		if req.TradeOffer.WantQty <= 0 || req.TradeOffer.OfferSilver <= 0 || req.TradeOffer.WantGood == "" {
			writeError(w, http.StatusBadRequest, "trade_offer requires want_good, want_qty > 0, offer_silver > 0")
			return
		}
		if req.TradeOffer.WantGood == "silver" || req.TradeOffer.WantGood == "gold" {
			writeError(w, http.StatusBadRequest, "cannot trade for silver — silver is the payment currency, not a tradeable good")
			return
		}
	}

	// Verify caller owns the origin settlement.
	var ownerID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		originID, worldID,
	).Scan(&ownerID)
	if err != nil || ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	// Reject duplicate pending trade offers to the same destination for the same good.
	// Agents otherwise re-send the same offer every turn, flooding the recipient's inbox.
	if req.TradeOffer != nil {
		var existing int
		_ = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM messengers
			 WHERE origin_id = $1 AND destination_id = $2
			   AND trade_offer IS NOT NULL
			   AND trade_offer->>'want_good' = $3
			   AND trade_offer->>'status' = 'pending'
			   AND status IN ('delivering', 'delivered')`,
			originID, destID, req.TradeOffer.WantGood,
		).Scan(&existing)
		if existing > 0 {
			writeError(w, http.StatusConflict,
				"pending trade offer for "+req.TradeOffer.WantGood+" to that settlement already exists — check your outbox or wait for a reply")
			return
		}
	}

	// Look up province hex coords for distance calculation.
	var oQ, oR int
	err = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p
		 JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		originID,
	).Scan(&oQ, &oR)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not find origin province")
		return
	}

	var dQ, dR int
	err = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p
		 JOIN settlements s ON s.province_id = p.id WHERE s.id = $1 AND s.world_id = $2`,
		destID, worldID,
	).Scan(&dQ, &dR)
	if err != nil {
		writeError(w, http.StatusNotFound, "destination settlement not found in this world")
		return
	}

	// FOW gate: the destination must be within the sender's current visibility.
	// This mirrors the client-side compose-dropdown that uses /provinces (FOW-filtered).
	// A player can only contact cities they have scouted or previously messaged.
	origins := loadVisibleOrigins(r.Context(), h.pool, worldID, playerID)
	if !province.VisibleFrom(province.MapPosition{Q: dQ, R: dR}, origins, 5) {
		writeError(w, http.StatusForbidden,
			"destination is not within your scouted range — send a scout or march closer before contacting this city")
		return
	}

	dist := province.HexDistance(province.MapPosition{Q: oQ, R: oR}, province.MapPosition{Q: dQ, R: dR})
	if dist == 0 {
		writeError(w, http.StatusBadRequest, "settlements are on the same province")
		return
	}
	arrivesAt := h.clk.Now().Add(messenger.MessengerTravelDuration(dist))

	var tradeOfferJSON []byte
	if req.TradeOffer != nil {
		tradeOfferJSON, _ = json.Marshal(map[string]any{
			"want_good":    req.TradeOffer.WantGood,
			"want_qty":     req.TradeOffer.WantQty,
			"offer_silver": req.TradeOffer.OfferSilver,
			"status":       "pending",
		})
	}
	// Trade offers expire 7 in-game days after arrival (so inactive offers clean up automatically).
	// Non-trade messages have no expiry.
	var expiresAt *time.Time
	if req.TradeOffer != nil {
		exp := arrivesAt.Add(7 * 24 * time.Hour)
		expiresAt = &exp
	}
	var messengerID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer, hex_q, hex_r, arrives_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10) RETURNING id`,
		worldID, playerID, originID, destID, req.Message, tradeOfferJSON, dQ, dR, arrivesAt, expiresAt,
	).Scan(&messengerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create messenger")
		return
	}

	_ = h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledMessengerArrival,
		messenger.ArrivalPayload{MessengerID: messengerID}, arrivesAt)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         messengerID,
		"arrives_at": arrivesAt,
		"distance":   dist,
	})
}

// ListSent handles GET /worlds/:worldID/settlements/:settlementID/messengers.
// Returns the last 20 messengers sent from this settlement (owner only).
func (h *MessengerHandler) ListSent(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	originID, err := uuid.Parse(chi.URLParam(r, "settlementID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid settlement ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var ownerID *uuid.UUID
	_ = h.pool.QueryRow(r.Context(),
		`SELECT owner_id FROM settlements WHERE id = $1 AND world_id = $2`,
		originID, worldID,
	).Scan(&ownerID)
	if ownerID == nil || *ownerID != playerID {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, m.destination_id, s.name, m.status, m.reply_text, m.sent_at, m.arrives_at, m.trade_offer
		 FROM messengers m
		 JOIN settlements s ON s.id = m.destination_id
		 WHERE m.origin_id = $1
		 ORDER BY m.sent_at DESC LIMIT 20`,
		originID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load messengers")
		return
	}
	defer rows.Close()

	type item struct {
		ID         uuid.UUID       `json:"id"`
		DestID     uuid.UUID       `json:"destination_id"`
		DestName   string          `json:"destination_name"`
		Status     string          `json:"status"`
		ReplyText  *string         `json:"reply_text"`
		SentAt     time.Time       `json:"sent_at"`
		ArrivesAt  time.Time       `json:"arrives_at"`
		TradeOffer json.RawMessage `json:"trade_offer,omitempty"`
	}
	var result []item
	for rows.Next() {
		var m item
		var tradeOffer []byte
		if err := rows.Scan(&m.ID, &m.DestID, &m.DestName, &m.Status, &m.ReplyText,
			&m.SentAt, &m.ArrivesAt, &tradeOffer); err == nil {
			if len(tradeOffer) > 0 {
				m.TradeOffer = json.RawMessage(tradeOffer)
			}
			result = append(result, m)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Inbox handles GET /worlds/:worldID/messengers/inbox.
// Returns all messengers delivered to any of the caller's settlements.
func (h *MessengerHandler) Inbox(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, m.origin_id, os.name, m.destination_id, ds.name,
		        m.message_text, m.trade_offer, m.status, m.sent_at, m.arrives_at
		 FROM messengers m
		 JOIN settlements os ON os.id = m.origin_id
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.world_id = $1
		   AND ds.owner_id = $2
		   AND m.status = 'delivered'
		   AND (m.trade_offer IS NULL OR (
		       -- keep only pending offers that have not expired
		       m.trade_offer->>'status' = 'pending'
		       AND (m.expires_at IS NULL OR m.expires_at > now())
		       -- solvency check: seller must still have the requested good
		       AND EXISTS (
		           SELECT 1 FROM settlement_goods sg
		           WHERE sg.settlement_id = m.origin_id
		             AND sg.good_key = m.trade_offer->>'want_good'
		             AND sg.amount + EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate
		                 >= (m.trade_offer->>'want_qty')::float
		       )
		   ))
		 ORDER BY m.arrives_at DESC LIMIT 30`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load inbox")
		return
	}
	defer rows.Close()

	type item struct {
		ID         uuid.UUID       `json:"id"`
		FromID     uuid.UUID       `json:"from_id"`
		FromName   string          `json:"from_name"`
		ToID       uuid.UUID       `json:"to_id"`
		ToName     string          `json:"to_name"`
		Message    string          `json:"message"`
		TradeOffer json.RawMessage `json:"trade_offer,omitempty"`
		Status     string          `json:"status"`
		SentAt     time.Time       `json:"sent_at"`
		ArrivedAt  time.Time       `json:"arrived_at"`
	}
	var result []item
	for rows.Next() {
		var m item
		var tradeOffer []byte
		if err := rows.Scan(&m.ID, &m.FromID, &m.FromName, &m.ToID, &m.ToName,
			&m.Message, &tradeOffer, &m.Status, &m.SentAt, &m.ArrivedAt); err == nil {
			if len(tradeOffer) > 0 {
				m.TradeOffer = json.RawMessage(tradeOffer)
			}
			result = append(result, m)
		}
	}
	if result == nil {
		result = []item{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Reply handles POST /worlds/:worldID/messengers/:messengerID/reply.
// Sets the reply text, flips status to 'returning', and schedules the return trip.
func (h *MessengerHandler) Reply(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	messengerID, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Reply) > 1000 {
		writeError(w, http.StatusBadRequest, "reply must be at most 1000 characters")
		return
	}

	// Messenger must be delivered to one of the caller's settlements.
	var destID, originID uuid.UUID
	var offerStatus *string
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.destination_id, m.origin_id, m.trade_offer->>'status' FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.id = $1 AND m.world_id = $2 AND ds.owner_id = $3 AND m.status = 'delivered'`,
		messengerID, worldID, playerID,
	).Scan(&destID, &originID, &offerStatus)
	if err != nil {
		writeError(w, http.StatusForbidden, "messenger not found, not yours, or not yet arrived")
		return
	}

	// A plain reply does NOT execute a trade. If this messenger carries a pending
	// trade offer, point the caller at the verb that actually moves the goods —
	// agents otherwise "accept" trades with prose ("Trade accepted, sending cedar")
	// and nothing transfers.
	if offerStatus != nil && *offerStatus == "pending" {
		writeError(w, http.StatusConflict,
			"this messenger carries a pending trade offer — replying does not execute it; "+
				"use trade-accept --id "+messengerID.String()+" to accept (sends the goods) "+
				"or trade-decline --id "+messengerID.String()+" to refuse")
		return
	}

	// Calculate return trip distance.
	var dQ, dR, oQ, oR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		destID,
	).Scan(&dQ, &dR)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		originID,
	).Scan(&oQ, &oR)
	dist := province.HexDistance(province.MapPosition{Q: dQ, R: dR}, province.MapPosition{Q: oQ, R: oR})
	returnsAt := h.clk.Now().Add(messenger.MessengerTravelDuration(dist))

	_, err = h.pool.Exec(r.Context(),
		`UPDATE messengers SET reply_text = $1, status = 'returning' WHERE id = $2`,
		req.Reply, messengerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save reply")
		return
	}

	// Schedule return. The auto-return (48h from delivery) is harmless — ReturnHandler is idempotent.
	_ = h.scheduler.Enqueue(r.Context(), worldID, events.ScheduledMessengerReturn,
		messenger.ReturnPayload{MessengerID: messengerID}, returnsAt)

	writeJSON(w, http.StatusOK, map[string]any{
		"returns_at": returnsAt,
		"distance":   dist,
	})
}

// TradeAccept handles POST /worlds/:worldID/messengers/:messengerID/trade-accept.
// The recipient accepts a trade offer. Goods are deducted from the seller immediately;
// a ScheduledTradeReturn event carries them back to the buyer (= messenger origin).
func (h *MessengerHandler) TradeAccept(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	messengerID, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Load messenger + trade_offer; verify it's delivered to this player's settlement.
	var destID, buyerSettlementID uuid.UUID
	var wantGood string
	var wantQty, offerGold float64
	var offerStatus string
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.destination_id, m.origin_id,
		        m.trade_offer->>'want_good', (m.trade_offer->>'want_qty')::float,
		        (m.trade_offer->>'offer_silver')::float, m.trade_offer->>'status'
		 FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.id = $1 AND m.world_id = $2 AND ds.owner_id = $3
		   AND m.status = 'delivered' AND m.trade_offer IS NOT NULL`,
		messengerID, worldID, playerID,
	).Scan(&destID, &buyerSettlementID, &wantGood, &wantQty, &offerGold, &offerStatus)
	if err != nil {
		writeError(w, http.StatusNotFound, "trade offer not found or not available to you")
		return
	}
	if offerStatus != "pending" {
		writeError(w, http.StatusConflict, "trade offer already "+offerStatus)
		return
	}

	// Verify buyer (origin) has enough gold.
	var buyerGold float64
	_ = h.pool.QueryRow(r.Context(),
		`SELECT silver_amount + EXTRACT(EPOCH FROM (now()-silver_calc_at))/60*silver_rate FROM settlements WHERE id=$1`,
		buyerSettlementID,
	).Scan(&buyerGold)
	if buyerGold < offerGold {
		writeError(w, http.StatusUnprocessableEntity,
			insufficientTradeMsg("buyer", "silver", offerGold, buyerGold))
		return
	}

	// Calculate distance for return travel time.
	var bQ, bR, dQ, dR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id=p.id WHERE s.id=$1`,
		buyerSettlementID).Scan(&bQ, &bR)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id=p.id WHERE s.id=$1`,
		destID).Scan(&dQ, &dR)
	dist := province.HexDistance(province.MapPosition{Q: bQ, R: bR}, province.MapPosition{Q: dQ, R: dR})

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Deduct want_qty goods from seller (destination).
	tag, err := tx.Exec(r.Context(),
		`UPDATE settlement_goods SET
		     amount = amount + EXTRACT(EPOCH FROM (now()-calc_at))/60*rate - $1,
		     calc_at = now()
		 WHERE settlement_id=$2 AND good_key=$3
		   AND amount + EXTRACT(EPOCH FROM (now()-calc_at))/60*rate >= $1`,
		wantQty, destID, wantGood,
	)
	if err != nil || tag.RowsAffected() == 0 {
		// Tell the accepting seller exactly how much of the requested good it
		// holds, so a blind 422 becomes actionable (it can decline or restock
		// instead of retrying forever). Mirrors the deductGoods shortfall style.
		var have float64
		_ = tx.QueryRow(r.Context(),
			`SELECT COALESCE(amount + EXTRACT(EPOCH FROM (now()-calc_at))/60*rate, 0)
			   FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`,
			destID, wantGood,
		).Scan(&have)
		writeError(w, http.StatusUnprocessableEntity,
			insufficientTradeMsg("seller", wantGood, wantQty, have))
		return
	}

	// Deduct offer_silver from buyer (leg 3 depart).
	if _, err = tx.Exec(r.Context(),
		`UPDATE settlements SET
		     silver_amount = GREATEST(0, silver_amount + EXTRACT(EPOCH FROM (now()-silver_calc_at))/60*silver_rate - $1),
		     silver_calc_at = now()
		 WHERE id=$2`,
		offerGold, buyerSettlementID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct silver from buyer")
		return
	}

	// Mark trade_offer as accepted.
	if _, err = tx.Exec(r.Context(),
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"accepted"}' WHERE id=$1`,
		messengerID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update offer status")
		return
	}

	// Leg 3: schedule silver delivery to seller (physical travel).
	// When silver arrives the delivery handler will chain goods dispatch (leg 4).
	silverArrivesAt := h.clk.Now().Add(messenger.TradeTravelDuration(dist))
	if err = h.scheduler.EnqueueTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
		map[string]any{
			"destination_id":     destID,           // seller receives silver
			"good_key":           "silver",
			"quantity":           offerGold,
			"delivered_quantity": offerGold,
			// Chained leg 4: when silver arrives, dispatch goods to buyer
			"then_return": map[string]any{
				"destination_id": buyerSettlementID,
				"good_key":       wantGood,
				"quantity":       wantQty,
				"messenger_id":   messengerID.String(),
				"travel_mins":    float64(dist) * 30.0,
			},
		}, silverArrivesAt); err != nil {
		writeError(w, http.StatusInternalServerError, "could not schedule silver delivery")
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	goodsArrivesAt := silverArrivesAt.Add(messenger.TradeTravelDuration(dist))
	writeJSON(w, http.StatusOK, map[string]any{
		"good_key":          wantGood,
		"quantity":          wantQty,
		"silver_paid":       offerGold,
		"silver_arrives_at": silverArrivesAt,
		"goods_arrives_at":  goodsArrivesAt,
		"distance":          dist,
	})
}

// TradeDecline handles POST /worlds/:worldID/messengers/:messengerID/trade-decline.
func (h *MessengerHandler) TradeDecline(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	messengerID, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var offerStatus string
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.trade_offer->>'status'
		 FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.id=$1 AND m.world_id=$2 AND ds.owner_id=$3
		   AND m.status='delivered' AND m.trade_offer IS NOT NULL`,
		messengerID, worldID, playerID,
	).Scan(&offerStatus)
	if err != nil {
		writeError(w, http.StatusNotFound, "trade offer not found")
		return
	}
	if offerStatus != "pending" {
		writeError(w, http.StatusConflict, "trade offer already "+offerStatus)
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"declined"}' WHERE id=$1`,
		messengerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update offer")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "declined"})
}

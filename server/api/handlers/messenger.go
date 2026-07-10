package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/capabilities"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/messenger"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/transport"
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
			Kind        string  `json:"kind"`
			WantGood    string  `json:"want_good"`
			WantQty     float64 `json:"want_qty"`
			OfferSilver float64 `json:"offer_silver"`
			OfferGood   string  `json:"offer_good"`
			OfferQty    float64 `json:"offer_qty"`
			WantSilver  float64 `json:"want_silver"`
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
		// Normalize kind: missing or empty → "buy" (backward-compatible).
		if req.TradeOffer.Kind == "" {
			req.TradeOffer.Kind = "buy"
		}
		switch req.TradeOffer.Kind {
		case "buy":
			if req.TradeOffer.WantGood == "" || req.TradeOffer.WantQty <= 0 || req.TradeOffer.OfferSilver <= 0 {
				writeError(w, http.StatusBadRequest, "buy trade_offer requires want_good, want_qty > 0, offer_silver > 0")
				return
			}
			if req.TradeOffer.WantGood == "silver" || req.TradeOffer.WantGood == "gold" {
				writeError(w, http.StatusBadRequest, "cannot trade for silver — silver is the payment currency, not a tradeable good")
				return
			}
		case "sell":
			if req.TradeOffer.OfferGood == "" || req.TradeOffer.OfferQty <= 0 || req.TradeOffer.WantSilver <= 0 {
				writeError(w, http.StatusBadRequest, "sell trade_offer requires offer_good, offer_qty > 0, want_silver > 0")
				return
			}
			if req.TradeOffer.OfferGood == "silver" || req.TradeOffer.OfferGood == "gold" {
				writeError(w, http.StatusBadRequest, "cannot sell silver — silver is the payment currency, not a tradeable good")
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "trade_offer kind must be \"buy\" or \"sell\"")
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

	// Coarse precondition for trade offers — the same checker `poleia actions`
	// uses (temenos_capabilities.md Fas 3). Sound as an early gate: if NO
	// foreign settlement is visible at all, the specific destination this
	// Send targets (itself necessarily a foreign settlement, once the FOW
	// check below runs) cannot be visible either. The per-destination FOW
	// check further down remains the authoritative, more specific gate for
	// "some other city is visible, but not this one".
	if req.TradeOffer != nil {
		cc := capabilities.NewContext(r.Context(), h.pool, h.clk, worldID, uuid.Nil, playerID, originID)
		var v capabilities.Verb
		if req.TradeOffer.Kind == "sell" {
			v = capabilities.CanSell(cc)
		} else {
			v = capabilities.CanTradeOffer(cc)
		}
		if !v.Available {
			writeError(w, http.StatusUnprocessableEntity, capabilities.FirstUnsatisfied(v))
			return
		}
	}

	// Reject duplicate pending trade offers to the same destination for the same good.
	// Agents otherwise re-send the same offer every turn, flooding the recipient's inbox.
	if req.TradeOffer != nil {
		var existing int
		var dupGoodKey string
		if req.TradeOffer.Kind == "sell" {
			dupGoodKey = req.TradeOffer.OfferGood
			_ = h.pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM messengers
				 WHERE origin_id = $1 AND destination_id = $2
				   AND trade_offer IS NOT NULL
				   AND trade_offer->>'offer_good' = $3
				   AND COALESCE(trade_offer->>'kind', 'buy') = 'sell'
				   AND trade_offer->>'status' = 'pending'
				   AND status IN ('outbound', 'delivered')`,
				originID, destID, dupGoodKey,
			).Scan(&existing)
		} else {
			dupGoodKey = req.TradeOffer.WantGood
			_ = h.pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM messengers
				 WHERE origin_id = $1 AND destination_id = $2
				   AND trade_offer IS NOT NULL
				   AND trade_offer->>'want_good' = $3
				   AND COALESCE(trade_offer->>'kind', 'buy') = 'buy'
				   AND trade_offer->>'status' = 'pending'
				   AND status IN ('outbound', 'delivered')`,
				originID, destID, dupGoodKey,
			).Scan(&existing)
		}
		if existing > 0 {
			writeError(w, http.StatusConflict,
				"pending trade offer for "+dupGoodKey+" to that settlement already exists — check your outbox or wait for a reply")
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
	if !province.VisibleFrom(province.MapPosition{Q: dQ, R: dR}, origins, 6) {
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
	var msgSendCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&msgSendCurrentTick)
	msgArrivalDueTick := msgSendCurrentTick + messenger.MessengerTravelTicks(dist)

	var tradeOfferJSON []byte
	if req.TradeOffer != nil {
		if req.TradeOffer.Kind == "sell" {
			tradeOfferJSON, _ = json.Marshal(map[string]any{
				"kind":        "sell",
				"offer_good":  req.TradeOffer.OfferGood,
				"offer_qty":   req.TradeOffer.OfferQty,
				"want_silver": req.TradeOffer.WantSilver,
				"status":      "pending",
			})
		} else {
			tradeOfferJSON, _ = json.Marshal(map[string]any{
				"kind":         "buy",
				"want_good":    req.TradeOffer.WantGood,
				"want_qty":     req.TradeOffer.WantQty,
				"offer_silver": req.TradeOffer.OfferSilver,
				"status":       "pending",
			})
		}
	}
	// Trade offers expire 7 in-game days after arrival (so inactive offers clean up automatically).
	// Non-trade messages have no expiry.
	var expiresAt *time.Time
	if req.TradeOffer != nil {
		exp := arrivesAt.Add(7 * 24 * time.Hour)
		expiresAt = &exp
	}
	var messengerID uuid.UUID

	if req.TradeOffer != nil {
		// Trade offer: escrow atomically with the INSERT and both scheduled events
		// so a crash can never leave goods/silver deducted without a messenger,
		// or a messenger without an expiry that would refund on timeout.
		tx, err := h.pool.Begin(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "transaction error")
			return
		}
		defer tx.Rollback(r.Context())

		if req.TradeOffer.Kind == "sell" {
			// Escrow: deduct offer_qty of offer_good from the seller (origin) now.
			var sellerAmt float64
			_ = tx.QueryRow(r.Context(),
				`SELECT COALESCE(settled(amount, rate, calc_tick), 0)
				   FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`,
				originID, req.TradeOffer.OfferGood,
			).Scan(&sellerAmt)
			tag, err := tx.Exec(r.Context(),
				`UPDATE settlement_goods
				    SET amount    = settled(amount, rate, calc_tick) - $1,
				        calc_tick = current_world_tick()
				  WHERE settlement_id=$2 AND good_key=$3
				    AND settled(amount, rate, calc_tick) >= $1`,
				req.TradeOffer.OfferQty, originID, req.TradeOffer.OfferGood,
			)
			if err != nil || tag.RowsAffected() == 0 {
				writeError(w, http.StatusUnprocessableEntity,
					insufficientTradeMsg("seller", req.TradeOffer.OfferGood, req.TradeOffer.OfferQty, sellerAmt))
				return
			}
		} else {
			// Escrow: deduct offer_silver from the buyer (origin) now.
			var buyerSilver float64
			_ = tx.QueryRow(r.Context(),
				`SELECT COALESCE(settled(amount, rate, calc_tick), 0)
				   FROM settlement_goods WHERE settlement_id=$1 AND good_key='silver'`,
				originID,
			).Scan(&buyerSilver)
			tag, err := tx.Exec(r.Context(),
				`UPDATE settlement_goods
				    SET amount    = settled(amount, rate, calc_tick) - $1,
				        calc_tick = current_world_tick()
				  WHERE settlement_id=$2 AND good_key='silver'
				    AND settled(amount, rate, calc_tick) >= $1`,
				req.TradeOffer.OfferSilver, originID,
			)
			if err != nil || tag.RowsAffected() == 0 {
				writeError(w, http.StatusUnprocessableEntity,
					insufficientTradeMsg("buyer", "silver", req.TradeOffer.OfferSilver, buyerSilver))
				return
			}
		}

		if err = tx.QueryRow(r.Context(),
			`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer, hex_q, hex_r, arrives_at, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10) RETURNING id`,
			worldID, playerID, originID, destID, req.Message, tradeOfferJSON, dQ, dR, arrivesAt, expiresAt,
		).Scan(&messengerID); err != nil {
			writeError(w, http.StatusInternalServerError, "could not create messenger")
			return
		}

		if err = h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledMessengerArrival,
			messenger.ArrivalPayload{MessengerID: messengerID}, msgArrivalDueTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule arrival")
			return
		}
		// Offer expiry: 7 in-game days (7*24 ticks) after arrival.
		if err = h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledOfferExpiry,
			map[string]any{"messenger_id": messengerID.String()}, msgArrivalDueTick+7*24); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule offer expiry")
			return
		}

		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "commit failed")
			return
		}
	} else {
		// Plain message: non-transactional path unchanged.
		err = h.pool.QueryRow(r.Context(),
			`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text, trade_offer, hex_q, hex_r, arrives_at, expires_at)
			 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10) RETURNING id`,
			worldID, playerID, originID, destID, req.Message, tradeOfferJSON, dQ, dR, arrivesAt, expiresAt,
		).Scan(&messengerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not create messenger")
			return
		}

		_ = h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledMessengerArrival,
			messenger.ArrivalPayload{MessengerID: messengerID}, msgArrivalDueTick)
	}

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
		`SELECT m.id, m.destination_id, s.name, m.message_text, m.status, m.reply_text, m.sent_at, m.arrives_at, m.trade_offer, m.expires_at
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
		Message    string          `json:"message_text"`
		Status     string          `json:"status"`
		ReplyText  *string         `json:"reply_text"`
		SentAt     time.Time       `json:"sent_at"`
		ArrivesAt  time.Time       `json:"arrives_at"`
		TradeOffer json.RawMessage `json:"trade_offer,omitempty"`
		ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
	}
	var result []item
	for rows.Next() {
		var m item
		var tradeOffer []byte
		if err := rows.Scan(&m.ID, &m.DestID, &m.DestName, &m.Message, &m.Status, &m.ReplyText,
			&m.SentAt, &m.ArrivesAt, &tradeOffer, &m.ExpiresAt); err == nil {
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
		        m.message_text, m.trade_offer, m.status, m.sent_at, m.arrives_at, m.expires_at,
		        -- Fas 2a: affordability is DATA, not a visibility filter. It used to be
		        -- part of the WHERE clause, which made an insolvent pending offer's
		        -- entire row (and therefore its id) disappear from the inbox — leaving
		        -- capabilities' own "decline the offer" hint (canTradeAccept,
		        -- HintTradeAcceptInsolvent) pointing at something the player could never
		        -- find to act on. Surfaced instead so the CLI can show "can't afford yet".
		        CASE
		            WHEN m.trade_offer IS NULL THEN NULL
		            WHEN COALESCE(m.trade_offer->>'kind', 'buy') = 'buy' THEN EXISTS (
		                SELECT 1 FROM settlement_goods sg
		                WHERE sg.settlement_id = m.destination_id
		                  AND sg.good_key = m.trade_offer->>'want_good'
		                  AND settled(sg.amount, sg.rate, sg.calc_tick)
		                      >= (m.trade_offer->>'want_qty')::float
		            )
		            ELSE EXISTS (
		                SELECT 1 FROM settlement_goods sg
		                WHERE sg.settlement_id = m.destination_id
		                  AND sg.good_key = 'silver'
		                  AND settled(sg.amount, sg.rate, sg.calc_tick)
		                      >= (m.trade_offer->>'want_silver')::float
		            )
		        END AS affordable
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
		ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
		Affordable *bool           `json:"affordable,omitempty"`
	}
	var result []item
	for rows.Next() {
		var m item
		var tradeOffer []byte
		if err := rows.Scan(&m.ID, &m.FromID, &m.FromName, &m.ToID, &m.ToName,
			&m.Message, &tradeOffer, &m.Status, &m.SentAt, &m.ArrivedAt, &m.ExpiresAt, &m.Affordable); err == nil {
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
	var replyCurrentTick int
	_ = h.pool.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&replyCurrentTick)
	replyReturnDueTick := replyCurrentTick + messenger.MessengerTravelTicks(dist)

	_, err = h.pool.Exec(r.Context(),
		`UPDATE messengers SET reply_text = $1, status = 'returning' WHERE id = $2`,
		req.Reply, messengerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save reply")
		return
	}

	// Schedule return. The auto-return (48h from delivery) is harmless — ReturnHandler is idempotent.
	_ = h.scheduler.EnqueueTick(r.Context(), worldID, events.ScheduledMessengerReturn,
		messenger.ReturnPayload{MessengerID: messengerID}, replyReturnDueTick)

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
	// We fetch all fields for both buy and sell kinds; unused fields will be zero/empty.
	var destID, originID uuid.UUID
	var kind, wantGood, offerGood, offerStatus string
	var wantQty, offerSilver, offerQty, wantSilver float64
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.destination_id, m.origin_id,
		        COALESCE(m.trade_offer->>'kind', 'buy'),
		        COALESCE(m.trade_offer->>'want_good', ''),
		        COALESCE((m.trade_offer->>'want_qty')::float, 0),
		        COALESCE((m.trade_offer->>'offer_silver')::float, 0),
		        COALESCE(m.trade_offer->>'offer_good', ''),
		        COALESCE((m.trade_offer->>'offer_qty')::float, 0),
		        COALESCE((m.trade_offer->>'want_silver')::float, 0),
		        m.trade_offer->>'status'
		 FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.id = $1 AND m.world_id = $2 AND ds.owner_id = $3
		   AND m.status = 'delivered' AND m.trade_offer IS NOT NULL`,
		messengerID, worldID, playerID,
	).Scan(&destID, &originID, &kind, &wantGood, &wantQty, &offerSilver, &offerGood, &offerQty, &wantSilver, &offerStatus)
	if err != nil {
		writeError(w, http.StatusNotFound, "trade offer not found or not available to you")
		return
	}
	if offerStatus != "pending" {
		writeError(w, http.StatusConflict, "trade offer already "+offerStatus)
		return
	}

	// Calculate distance for travel time (same for both kinds: origin ↔ destination).
	var oQ, oR, dQ, dR int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id=p.id WHERE s.id=$1`,
		originID).Scan(&oQ, &oR)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id=p.id WHERE s.id=$1`,
		destID).Scan(&dQ, &dR)
	dist := province.HexDistance(province.MapPosition{Q: oQ, R: oR}, province.MapPosition{Q: dQ, R: dR})

	// Owner of the goods dispatched on leg 1 (the origin's Wanax) — the physical
	// caravan belongs to whoever sends that leg, so a third party can raid it.
	var originOwner uuid.UUID
	_ = h.pool.QueryRow(r.Context(), `SELECT owner_id FROM settlements WHERE id=$1`, originID).Scan(&originOwner)

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	var tradeAcceptCurrentTick int
	_ = tx.QueryRow(r.Context(), `SELECT current_world_tick()`).Scan(&tradeAcceptCurrentTick)
	tradeAcceptDueTick := tradeAcceptCurrentTick + messenger.TradeTravelTicks(dist)
	leg1ArrivesAt := h.clk.Now().Add(messenger.TradeTravelDuration(dist))

	if kind == "sell" {
		// Sell offer: origin=seller (escrowed goods at send), destination=buyer (acceptor).
		// Step 1: deduct want_silver from buyer (destination). Goods already escrowed at send — do NOT deduct again.
		tag, err := tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			     amount    = settled(amount, rate, calc_tick) - $1,
			     calc_tick = current_world_tick()
			 WHERE settlement_id=$2 AND good_key='silver'
			   AND settled(amount, rate, calc_tick) >= $1`,
			wantSilver, destID,
		)
		if err != nil || tag.RowsAffected() == 0 {
			// Same hint text as capabilities' trade-accept solvency
			// requirement (temenos_capabilities.md Fas 3 anti-drift) —
			// poleia actions and this 422 can never disagree about what
			// "insolvent" means here.
			writeError(w, http.StatusUnprocessableEntity, capabilities.HintTradeAcceptInsolvent)
			return
		}

		// Step 2: guarded flip to accepted.
		tag, err = tx.Exec(r.Context(),
			`UPDATE messengers SET trade_offer = trade_offer || '{"status":"accepted"}'
			  WHERE id=$1 AND trade_offer->>'status'='pending'`,
			messengerID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not update offer status")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusConflict, "trade offer is no longer available (expired or already resolved)")
			return
		}

		// Step 3: dispatch leg 1 as a PHYSICAL caravan (goods seller→buyer), then chain
		// the silver return (leg 2) on arrival. The shadow transport row carries map
		// position + interceptability; the trade delivery event drives crediting.
		leg1ID, tErr := transport.CreateShadow(r.Context(), tx, transport.DispatchParams{
			WorldID: worldID, OwnerID: originOwner, Kind: "trade",
			OriginID: originID, DestID: destID, Category: "land",
			OriginQ: oQ, OriginR: oR, DestQ: dQ, DestR: dR,
			DepartsAt: h.clk.Now(), ArrivesAt: leg1ArrivesAt, DueTick: tradeAcceptDueTick,
			Manifest: transport.Manifest{offerGood: offerQty}, Interceptable: true,
		})
		if tErr != nil {
			writeError(w, http.StatusInternalServerError, "could not dispatch goods caravan")
			return
		}
		if err = h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
			map[string]any{
				"destination_id":     destID,    // buyer receives goods
				"good_key":           offerGood,
				"quantity":           offerQty,
				"delivered_quantity": offerQty,
				"transport_id":       leg1ID.String(),
				// Chained: when goods arrive at buyer, dispatch silver to seller (leg 2:
				// destID→originID). Coords let the delivery handler build the return caravan.
				"then_return": map[string]any{
					"destination_id": originID.String(), // seller receives silver
					"good_key":       "silver",
					"quantity":       wantSilver,
					"messenger_id":   messengerID.String(),
					"travel_mins":    float64(dist) * 30.0,
					"owner_id":       playerID.String(), // leg-2 caravan belongs to the acceptor (dest owner)
					"origin_q":       dQ, "origin_r": dR,
					"dest_q":         oQ, "dest_r": oR,
				},
			}, tradeAcceptDueTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule goods delivery")
			return
		}

		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "commit failed")
			return
		}

		goodsArrivesAt := h.clk.Now().Add(messenger.TradeTravelDuration(dist))
		silverArrivesAt := goodsArrivesAt.Add(messenger.TradeTravelDuration(dist))
		writeJSON(w, http.StatusOK, map[string]any{
			"good_key":           offerGood,
			"quantity":           offerQty,
			"silver_paid":        wantSilver,
			"goods_arrives_at":   goodsArrivesAt,
			"silver_arrives_at":  silverArrivesAt,
			"distance":           dist,
		})
	} else {
		// Buy offer: origin=buyer (escrowed silver at send), destination=seller (acceptor).
		// Step 1: deduct want_qty goods from seller (destination). Silver already escrowed at send.
		tag, err := tx.Exec(r.Context(),
			`UPDATE settlement_goods SET
			     amount    = settled(amount, rate, calc_tick) - $1,
			     calc_tick = current_world_tick()
			 WHERE settlement_id=$2 AND good_key=$3
			   AND settled(amount, rate, calc_tick) >= $1`,
			wantQty, destID, wantGood,
		)
		if err != nil || tag.RowsAffected() == 0 {
			// Same hint text as capabilities' trade-accept solvency
			// requirement (temenos_capabilities.md Fas 3 anti-drift) —
			// poleia actions and this 422 can never disagree about what
			// "insolvent" means here.
			writeError(w, http.StatusUnprocessableEntity, capabilities.HintTradeAcceptInsolvent)
			return
		}

		// Step 2: guarded flip to accepted. Guard on status='pending' so that if the
		// offer expired (and its silver was refunded) between the pool-read above and
		// this transaction, the accept aborts cleanly instead of shipping refunded
		// silver to the seller — which would mint silver from nothing.
		tag, err = tx.Exec(r.Context(),
			`UPDATE messengers SET trade_offer = trade_offer || '{"status":"accepted"}'
			  WHERE id=$1 AND trade_offer->>'status'='pending'`,
			messengerID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not update offer status")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusConflict, "trade offer is no longer available (expired or already resolved)")
			return
		}

		// Step 3: dispatch leg 1 as a PHYSICAL caravan (silver buyer→seller), then chain
		// the goods return (leg 2) on arrival. Shadow row = position + interceptability.
		leg1ID, tErr := transport.CreateShadow(r.Context(), tx, transport.DispatchParams{
			WorldID: worldID, OwnerID: originOwner, Kind: "trade",
			OriginID: originID, DestID: destID, Category: "land",
			OriginQ: oQ, OriginR: oR, DestQ: dQ, DestR: dR,
			DepartsAt: h.clk.Now(), ArrivesAt: leg1ArrivesAt, DueTick: tradeAcceptDueTick,
			Manifest: transport.Manifest{"silver": offerSilver}, Interceptable: true,
		})
		if tErr != nil {
			writeError(w, http.StatusInternalServerError, "could not dispatch silver caravan")
			return
		}
		if err = h.scheduler.EnqueueTickTx(r.Context(), tx, worldID, events.ScheduledTradeDelivery,
			map[string]any{
				"destination_id":     destID,           // seller receives silver
				"good_key":           "silver",
				"quantity":           offerSilver,
				"delivered_quantity": offerSilver,
				"transport_id":       leg1ID.String(),
				// Chained: when silver arrives at seller, dispatch goods to buyer (leg 2:
				// destID→originID).
				"then_return": map[string]any{
					"destination_id": originID.String(),
					"good_key":       wantGood,
					"quantity":       wantQty,
					"messenger_id":   messengerID.String(),
					"travel_mins":    float64(dist) * 30.0,
					"owner_id":       playerID.String(), // leg-2 caravan belongs to the acceptor (dest owner)
					"origin_q":       dQ, "origin_r": dR,
					"dest_q":         oQ, "dest_r": oR,
				},
			}, tradeAcceptDueTick); err != nil {
			writeError(w, http.StatusInternalServerError, "could not schedule silver delivery")
			return
		}

		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "commit failed")
			return
		}

		silverArrivesAt := h.clk.Now().Add(messenger.TradeTravelDuration(dist))
		goodsArrivesAt := silverArrivesAt.Add(messenger.TradeTravelDuration(dist))
		writeJSON(w, http.StatusOK, map[string]any{
			"good_key":          wantGood,
			"quantity":          wantQty,
			"silver_paid":       offerSilver,
			"silver_arrives_at": silverArrivesAt,
			"goods_arrives_at":  goodsArrivesAt,
			"distance":          dist,
		})
	}
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

	var offerStatus, kind, offerGood string
	var originID uuid.UUID
	var offerSilver, offerQty float64
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.trade_offer->>'status', m.origin_id,
		        COALESCE(m.trade_offer->>'kind', 'buy'),
		        COALESCE((m.trade_offer->>'offer_silver')::float, 0),
		        COALESCE(m.trade_offer->>'offer_good', ''),
		        COALESCE((m.trade_offer->>'offer_qty')::float, 0)
		 FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 WHERE m.id=$1 AND m.world_id=$2 AND ds.owner_id=$3
		   AND m.status='delivered' AND m.trade_offer IS NOT NULL`,
		messengerID, worldID, playerID,
	).Scan(&offerStatus, &originID, &kind, &offerSilver, &offerGood, &offerQty)
	if err != nil {
		writeError(w, http.StatusNotFound, "trade offer not found")
		return
	}
	if offerStatus != "pending" {
		writeError(w, http.StatusConflict, "trade offer already "+offerStatus)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Flip to declined first, guarded on status='pending'. If a concurrent expiry
	// already resolved+refunded the offer, RowsAffected is 0 and we abort without
	// refunding a second time (which would not preserve mass).
	tag, err := tx.Exec(r.Context(),
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"declined"}'
		  WHERE id=$1 AND trade_offer->>'status'='pending'`,
		messengerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update offer")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "trade offer is no longer available (expired or already resolved)")
		return
	}

	// Refund escrowed value to origin:
	//   buy  → silver to buyer (origin)
	//   sell → goods to seller (origin)
	if kind == "sell" {
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods
			    SET amount    = settled(amount, rate, calc_tick) + $1,
			        calc_tick = current_world_tick()
			  WHERE settlement_id=$2 AND good_key=$3`,
			offerQty, originID, offerGood,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not refund goods to seller")
			return
		}
	} else {
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods
			    SET amount    = settled(amount, rate, calc_tick) + $1,
			        calc_tick = current_world_tick()
			  WHERE settlement_id=$2 AND good_key='silver'`,
			offerSilver, originID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not refund silver to buyer")
			return
		}
	}

	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "declined"})
}

// CancelOffer handles POST /worlds/:worldID/messengers/:messengerID/trade-cancel.
// The SENDER (buyer) cancels a pending outgoing trade offer and reclaims the
// escrowed silver. The ScheduledOfferExpiry event is left in place — its
// guarded flip (status='pending') will no-op once the status is 'cancelled'.
// Idempotent: if the offer is already resolved (accepted/declined/expired/
// cancelled), returns 200 with no silver movement.
func (h *MessengerHandler) CancelOffer(w http.ResponseWriter, r *http.Request) {
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

	// Verify caller owns the ORIGIN settlement (they are the sender).
	// For buy: origin=buyer; for sell: origin=seller.
	var offerStatus, kind, offerGood string
	var originID uuid.UUID
	var offerSilver, offerQty float64
	err = h.pool.QueryRow(r.Context(),
		`SELECT m.trade_offer->>'status', m.origin_id,
		        COALESCE(m.trade_offer->>'kind', 'buy'),
		        COALESCE((m.trade_offer->>'offer_silver')::float, 0),
		        COALESCE(m.trade_offer->>'offer_good', ''),
		        COALESCE((m.trade_offer->>'offer_qty')::float, 0)
		 FROM messengers m
		 JOIN settlements os ON os.id = m.origin_id
		 WHERE m.id=$1 AND m.world_id=$2 AND os.owner_id=$3
		   AND m.trade_offer IS NOT NULL`,
		messengerID, worldID, playerID,
	).Scan(&offerStatus, &originID, &kind, &offerSilver, &offerGood, &offerQty)
	if err != nil {
		writeError(w, http.StatusNotFound, "trade offer not found or not yours")
		return
	}

	// Already resolved — idempotent no-op (escrow already refunded or consumed).
	if offerStatus != "pending" {
		writeJSON(w, http.StatusOK, map[string]any{"status": offerStatus})
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Guarded flip: if a concurrent expiry or accept already resolved the offer,
	// RowsAffected is 0 and we skip the refund (escrow already handled).
	tag, err := tx.Exec(r.Context(),
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"cancelled"}'
		  WHERE id=$1 AND trade_offer->>'status'='pending'`,
		messengerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not cancel offer")
		return
	}
	if tag.RowsAffected() == 0 {
		// Concurrent resolution — treat as already done.
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "commit failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "already_resolved"})
		return
	}

	// Refund escrowed value to origin:
	//   buy  → silver to buyer (origin)
	//   sell → goods to seller (origin)
	if kind == "sell" {
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods
			    SET amount    = settled(amount, rate, calc_tick) + $1,
			        calc_tick = current_world_tick()
			  WHERE settlement_id=$2 AND good_key=$3`,
			offerQty, originID, offerGood,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not refund goods to seller")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "commit failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "cancelled",
			"good_refunded": offerGood,
			"qty_refunded":  offerQty,
		})
	} else {
		if _, err = tx.Exec(r.Context(),
			`UPDATE settlement_goods
			    SET amount    = settled(amount, rate, calc_tick) + $1,
			        calc_tick = current_world_tick()
			  WHERE settlement_id=$2 AND good_key='silver'`,
			offerSilver, originID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not refund silver")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "commit failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "cancelled",
			"silver_refunded": offerSilver,
		})
	}
}

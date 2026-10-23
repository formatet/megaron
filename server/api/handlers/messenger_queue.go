package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The queue intentionally has the same current-recipient authority as Inbox.
// Historical person/case ownership is K2, not inferred from current city owners.
type correspondenceItem struct {
	ID             uuid.UUID       `json:"id"`
	FromID         uuid.UUID       `json:"from_id"`
	FromName       string          `json:"from_name"`
	ToID           uuid.UUID       `json:"to_id"`
	ToName         string          `json:"to_name"`
	Message        string          `json:"message"`
	TradeOffer     json.RawMessage `json:"trade_offer,omitempty"`
	Status         string          `json:"status"`
	ReplyText      *string         `json:"reply_text,omitempty"`
	SentAt         time.Time       `json:"sent_at"`
	ArrivedAt      time.Time       `json:"arrived_at"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	ReadAt         *time.Time      `json:"read_at"`
	Affordable     *bool           `json:"affordable,omitempty"`
	Group          string          `json:"queue_group"`
	OfferState     string          `json:"offer_state"`
	Actions        []string        `json:"actions"`
	MissingPayment string          `json:"missing_payment,omitempty"`
}

type correspondenceCursor struct {
	Epoch    time.Time `json:"e"`
	Snapshot string    `json:"s"`
	Version  int       `json:"v"`
	World    uuid.UUID `json:"w"`
	Player   uuid.UUID `json:"p"`
	Upper    time.Time `json:"u"`
	Before   time.Time `json:"b"`
	ID       uuid.UUID `json:"i"`
}

// Queue lists delivered incoming correspondence without the old 30-row cutoff.
// Keysets use immutable send time/id, never mutable read or offer state.
func (h *MessengerHandler) Queue(w http.ResponseWriter, r *http.Request) {
	world, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	limit := 30
	if raw, exists := r.URL.Query()["limit"]; exists {
		if len(raw) != 1 {
			writeError(w, 400, "invalid limit")
			return
		}
		limit, err = strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, 400, "limit must be between 1 and 100")
			return
		}
	}
	// One read-only connection prevents a DB restart between validating a
	// snapshot epoch and reading its rows from silently accepting an old cursor.
	tx, err := h.pool.BeginTx(r.Context(), pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "could not load correspondence queue")
		return
	}
	defer tx.Rollback(r.Context())
	now := h.clk.Now()
	cursor := correspondenceCursor{Version: 1, World: world, Player: player, Upper: now}
	var before any
	var beforeID any
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		b, decodeErr := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 65536 || decodeErr != nil || json.Unmarshal(b, &cursor) != nil || cursor.Epoch.IsZero() || cursor.Snapshot == "" || cursor.Version != 1 || cursor.World != world || cursor.Player != player || cursor.Upper.IsZero() || cursor.Before.IsZero() || cursor.ID == uuid.Nil || cursor.Before.After(cursor.Upper) {
			writeError(w, 400, "invalid queue cursor")
			return
		}
		before = cursor.Before
		beforeID = cursor.ID
	}
	// Freeze membership as well as the ordering boundary. A later insert with
	// an equal/backdated sent_at must not sneak into subsequent pages.
	if cursor.Snapshot == "" {
		if err := tx.QueryRow(r.Context(), `SELECT pg_current_snapshot()::text,pg_postmaster_start_time()`).Scan(&cursor.Snapshot, &cursor.Epoch); err != nil {
			writeError(w, 500, "could not capture correspondence queue")
			return
		}
	} else {
		var validated string
		var epoch time.Time
		if err := tx.QueryRow(r.Context(), `SELECT $1::pg_snapshot::text,pg_postmaster_start_time()`, cursor.Snapshot).Scan(&validated, &epoch); err != nil || !epoch.Equal(cursor.Epoch) {
			writeError(w, 400, "invalid queue cursor snapshot")
			return
		}
	}
	rows, err := tx.Query(r.Context(), `
 SELECT m.id,COALESCE(m.origin_id,m.origin_unit_id),COALESCE(os.name,'Nomadic Host'),m.destination_id,ds.name,
 m.message_text,m.trade_offer,m.status,m.reply_text,m.sent_at,m.arrives_at,m.expires_at,m.read_at,
 CASE WHEN m.trade_offer IS NULL THEN NULL ELSE COALESCE((SELECT settled(sg.amount,sg.rate,sg.calc_tick)>=
 CASE WHEN COALESCE(m.trade_offer->>'kind','buy')='buy' THEN (m.trade_offer->>'want_qty')::float ELSE (m.trade_offer->>'want_silver')::float END
 FROM settlement_goods sg WHERE sg.settlement_id=m.destination_id AND sg.good_key=
 CASE WHEN COALESCE(m.trade_offer->>'kind','buy')='buy' THEN m.trade_offer->>'want_good' ELSE 'silver' END),false) END
 FROM messengers m
 JOIN settlements ds ON ds.id=m.destination_id
 LEFT JOIN settlements os ON os.id=m.origin_id
 WHERE m.world_id=$1 AND ds.owner_id=$2 AND NOT m.withdrawn AND `+receivedLetterSQL+`
 AND (m.correspondence_created_epoch<>$8 OR pg_visible_in_snapshot(m.correspondence_created_xid,$7::pg_snapshot))
 AND m.sent_at<=$3 AND ($4::timestamptz IS NULL OR (m.sent_at,m.id)<($4::timestamptz,$5::uuid))
 ORDER BY m.sent_at DESC,m.id DESC LIMIT $6`, world, player, cursor.Upper, before, beforeID, limit+1, cursor.Snapshot, cursor.Epoch)
	if err != nil {
		writeError(w, 500, "could not load correspondence queue")
		return
	}
	defer rows.Close()
	items := make([]correspondenceItem, 0, limit+1)
	for rows.Next() {
		var m correspondenceItem
		var offer []byte
		if err := rows.Scan(&m.ID, &m.FromID, &m.FromName, &m.ToID, &m.ToName, &m.Message, &offer, &m.Status, &m.ReplyText, &m.SentAt, &m.ArrivedAt, &m.ExpiresAt, &m.ReadAt, &m.Affordable); err != nil {
			writeError(w, 500, "could not read correspondence queue")
			return
		}
		m.TradeOffer = offer
		if err := m.describe(now); err != nil {
			writeError(w, 500, "could not read trade offer")
			return
		}
		items = append(items, m)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read correspondence queue")
		return
	}
	response := struct {
		Items []correspondenceItem `json:"items"`
		Next  string               `json:"next_cursor,omitempty"`
	}{Items: items}
	if len(items) > limit {
		response.Items = items[:limit]
		last := items[limit-1]
		cursor.Before = last.SentAt
		cursor.ID = last.ID
		raw, _ := json.Marshal(cursor)
		response.Next = base64.RawURLEncoding.EncodeToString(raw)
	}
	writeJSON(w, 200, response)
}

func (m *correspondenceItem) describe(now time.Time) error {
	m.Group = "completed"
	m.Actions = []string{}
	if m.ReadAt == nil {
		m.Actions = append(m.Actions, "read")
	}
	if len(m.TradeOffer) == 0 || string(m.TradeOffer) == "null" {
		if m.Status == "delivered" {
			m.Group = "waiting_you"
			m.Actions = append(m.Actions, "reply")
		}
		return nil
	}
	var offer struct {
		Status     string  `json:"status"`
		Kind       string  `json:"kind"`
		WantGood   string  `json:"want_good"`
		WantQty    float64 `json:"want_qty"`
		WantSilver float64 `json:"want_silver"`
	}
	if err := json.Unmarshal(m.TradeOffer, &offer); err != nil {
		return err
	}
	m.OfferState = offer.Status
	if offer.Status == "pending" && m.ExpiresAt != nil && !m.ExpiresAt.After(now) {
		m.OfferState = "expired"
	}
	if m.OfferState != "pending" || m.Status != "delivered" {
		return nil
	}
	m.Group = "waiting_you"
	m.Actions = append(m.Actions, "trade_decline")
	if m.Affordable != nil && *m.Affordable {
		m.Actions = append(m.Actions, "trade_accept")
	} else {
		good, qty := offer.WantGood, offer.WantQty
		if offer.Kind == "sell" {
			good = "silver"
			qty = offer.WantSilver
		}
		m.MissingPayment = fmt.Sprintf("Requires %g %s at %s; insufficient stock", qty, good, m.ToName)
	}
	return nil
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/tick"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type personLetter struct {
	ID                uuid.UUID          `json:"id"`
	SenderID          uuid.UUID          `json:"sender_id"`
	SenderName        string             `json:"sender_name"`
	Direction         string             `json:"direction"`
	Message           string             `json:"message"`
	ReplyText         *string            `json:"reply_text,omitempty"`
	SentAt            time.Time          `json:"sent_at"`
	Status            string             `json:"status"`
	TradeOffer        json.RawMessage    `json:"trade_offer,omitempty"`
	OfferState        string             `json:"offer_state,omitempty"`
	Actions           []string           `json:"actions"`
	ReplyBefore       *time.Time         `json:"reply_before,omitempty"`
	ExpiresAt         *time.Time         `json:"expires_at,omitempty"`
	Affordable        *bool              `json:"affordable,omitempty"`
	MissingPayment    string             `json:"missing_payment,omitempty"`
	ReadAt            *time.Time         `json:"read_at,omitempty"`
	ReplyWindowClosed bool               `json:"reply_window_closed"`
	Kind              string             `json:"kind"`
	Goods             map[string]float64 `json:"goods,omitempty"`
	counterpart       uuid.UUID
	counterpartName   string
	waiting           bool
}
type letterPerson struct {
	CounterpartID   uuid.UUID  `json:"counterpart_id"`
	CounterpartName string     `json:"counterpart_name"`
	Preview         string     `json:"preview"`
	State           string     `json:"state"`
	ReplyBefore     *time.Time `json:"reply_before,omitempty"`
	LatestAt        time.Time  `json:"latest_at"`
	LetterCount     int        `json:"letter_count"`
}

// Letters and LetterThread are two views of the same physical letters. Reading
// never changes a runner, marks a letter read, or reveals a remote outcome.
func (h *MessengerHandler) Letters(w http.ResponseWriter, r *http.Request) { h.letters(w, r, false) }
func (h *MessengerHandler) LetterThread(w http.ResponseWriter, r *http.Request) {
	h.letters(w, r, true)
}
func (h *MessengerHandler) letters(w http.ResponseWriter, r *http.Request, thread bool) {
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
	var other uuid.UUID
	if thread {
		other, err = uuid.Parse(chi.URLParam(r, "playerID"))
		if err != nil {
			writeError(w, 400, "invalid Wanax ID")
			return
		}
	}
	tx, err := h.pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "could not load letters")
		return
	}
	defer tx.Rollback(r.Context())
	letters, err := h.loadPersonLetters(r.Context(), tx, world, player)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "world not found")
			return
		}
		writeError(w, 500, "could not load letters")
		return
	}
	people := summarizeLetters(letters)
	if !thread {
		writeJSON(w, 200, map[string]any{"items": people})
		return
	}
	for _, p := range people {
		if p.CounterpartID == other {
			items := []personLetter{}
			for _, l := range letters {
				if l.counterpart == other {
					items = append(items, l)
				}
			}
			writeJSON(w, 200, map[string]any{"counterpart_id": p.CounterpartID, "counterpart_name": p.CounterpartName, "state": p.State, "reply_before": p.ReplyBefore, "items": items})
			return
		}
	}
	writeError(w, 404, "no letters with this Wanax")
}

func (h *MessengerHandler) loadPersonLetters(ctx context.Context, tx pgx.Tx, world, player uuid.UUID) ([]personLetter, error) {
	anchor, err := tick.LoadAnchor(ctx, tx, world)
	if err != nil {
		return nil, err
	}
	// Sent letters use frozen recipients where available. Legacy letters retain
	// the existing current-owner fallback. Incoming letters have Inbox's gate.
	// Original trade terms are an allowlist: no new engine field may accidentally
	// disclose acceptance, cargo or rescue before the runner physically returns.
	rows, err := tx.Query(ctx, `SELECT m.id,m.sender_id,COALESCE(a.sender_name,NULLIF(sp.wanax_name,''),sp.username),
 CASE WHEN m.sender_id=$2 THEN COALESCE(a.recipient_id,ds.owner_id) ELSE m.sender_id END,
 CASE WHEN m.sender_id=$2 THEN COALESCE(a.recipient_name,NULLIF(rp.wanax_name,''),rp.username) ELSE COALESCE(a.sender_name,NULLIF(sp.wanax_name,''),sp.username) END,
 m.sender_id=$2,m.message_text,
 CASE WHEN m.sender_id<>$2 OR m.status='arrived' THEN m.reply_text ELSE NULL END,
 m.sent_at,CASE WHEN m.sender_id=$2 AND m.status NOT IN('arrived','lost') THEN 'outbound' ELSE m.status END,
 CASE WHEN m.sender_id=$2 AND m.status<>'arrived' AND m.trade_offer IS NOT NULL THEN
 (SELECT COALESCE(jsonb_object_agg(f.key,f.value),'{}'::jsonb) FROM jsonb_each(m.trade_offer) f WHERE f.key IN('kind','want_good','want_qty','offer_silver','offer_good','offer_qty','want_silver'))
 ||jsonb_build_object('status',CASE WHEN m.trade_offer->>'status'='cancelled' THEN 'cancelled' ELSE 'pending' END)
 ELSE m.trade_offer END,
 m.expires_at,m.read_at,
 CASE WHEN m.sender_id=$2 OR m.trade_offer IS NULL THEN NULL ELSE COALESCE((SELECT settled(sg.amount,sg.rate,sg.calc_tick)>=
 CASE WHEN COALESCE(m.trade_offer->>'kind','buy')='buy' THEN (m.trade_offer->>'want_qty')::float ELSE (m.trade_offer->>'want_silver')::float END
 FROM settlement_goods sg WHERE sg.settlement_id=m.destination_id AND sg.good_key=CASE WHEN COALESCE(m.trade_offer->>'kind','buy')='buy' THEN m.trade_offer->>'want_good' ELSE 'silver' END),false) END,
 `+replyDeadlineSQL+`,ds.name
 FROM messengers m LEFT JOIN correspondence_letters a ON a.id=m.id
 LEFT JOIN settlements ds ON ds.id=m.destination_id
 JOIN players sp ON sp.id=m.sender_id LEFT JOIN players rp ON rp.id=COALESCE(a.recipient_id,ds.owner_id)
 WHERE m.world_id=$1 AND m.kind='diplomatic' AND
 (m.sender_id=$2 OR (ds.owner_id=$2 AND correspondence_recipient(m.id,$2) AND NOT m.withdrawn AND `+receivedLetterSQL+`))
 ORDER BY m.sent_at,m.id`, world, player)
	if err != nil {
		return nil, err
	}
	letters := []personLetter{}
	for rows.Next() {
		var l personLetter
		var out bool
		var deadline *int
		var city *string
		var offer []byte
		if err = rows.Scan(&l.ID, &l.SenderID, &l.SenderName, &l.counterpart, &l.counterpartName, &out, &l.Message, &l.ReplyText, &l.SentAt, &l.Status, &offer, &l.ExpiresAt, &l.ReadAt, &l.Affordable, &deadline, &city); err != nil {
			rows.Close()
			return nil, err
		}
		l.Kind = "letter"
		l.TradeOffer = offer
		l.Direction = "incoming"
		l.Actions = []string{}
		if out {
			l.Direction = "outgoing"
			l.waiting = l.Status != "arrived" && l.Status != "lost"
			var o struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(offer, &o)
			l.OfferState = o.Status
			if o.Status == "cancelled" {
				l.waiting = false
			}
		} else {
			item := correspondenceItem{ID: l.ID, Status: l.Status, TradeOffer: offer, ReplyText: l.ReplyText, ExpiresAt: l.ExpiresAt, ReadAt: l.ReadAt, Affordable: l.Affordable}
			if city != nil {
				item.ToName = *city
			}
			if deadline != nil && len(offer) == 0 {
				at := anchor.WallAt(int64(*deadline) * 1000)
				item.ReplyBefore = &at
				item.ReplyWindowClosed = anchor.Tick >= *deadline
			}
			if err = item.describe(h.clk.Now()); err != nil {
				rows.Close()
				return nil, err
			}
			l.Actions = item.Actions
			l.OfferState = item.OfferState
			l.ReplyBefore = item.ReplyBefore
			l.ReplyWindowClosed = item.ReplyWindowClosed
			l.MissingPayment = item.MissingPayment
			l.waiting = item.Group == "waiting_you"
			if len(offer) > 0 && l.waiting {
				l.ReplyBefore = l.ExpiresAt
			}
		}
		letters = append(letters, l)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	gifts, err := personGifts(ctx, tx, world, player)
	if err != nil {
		return nil, err
	}
	letters = append(letters, gifts...)
	sort.Slice(letters, func(i, j int) bool {
		if letters[i].SentAt.Equal(letters[j].SentAt) {
			return letters[i].ID.String() < letters[j].ID.String()
		}
		return letters[i].SentAt.Before(letters[j].SentAt)
	})
	return letters, nil
}

func summarizeLetters(letters []personLetter) []letterPerson {
	byPerson := map[uuid.UUID]*letterPerson{}
	for _, l := range letters {
		p := byPerson[l.counterpart]
		if p == nil {
			p = &letterPerson{CounterpartID: l.counterpart, State: "done"}
			byPerson[l.counterpart] = p
		}
		p.CounterpartName = l.counterpartName
		p.LetterCount++
		p.LatestAt = l.SentAt
		lines := strings.Split(strings.TrimSpace(l.Message), "\n")
		p.Preview = lines[len(lines)-1]
		if l.ReplyText != nil && *l.ReplyText != "" {
			lines = strings.Split(strings.TrimSpace(*l.ReplyText), "\n")
			p.Preview = lines[len(lines)-1]
		}
		if l.waiting && l.Direction == "incoming" {
			p.State = "waiting_you"
			if l.ReplyBefore != nil && (p.ReplyBefore == nil || l.ReplyBefore.Before(*p.ReplyBefore)) {
				p.ReplyBefore = l.ReplyBefore
			}
		} else if l.waiting && p.State != "waiting_you" {
			p.State = "waiting_them"
		}
	}
	people := make([]letterPerson, 0, len(byPerson))
	for _, p := range byPerson {
		people = append(people, *p)
	}
	sort.Slice(people, func(i, j int) bool {
		a, b := people[i], people[j]
		rank := func(s string) int {
			if s == "waiting_you" {
				return 0
			}
			if s == "waiting_them" {
				return 1
			}
			return 2
		}
		if rank(a.State) != rank(b.State) {
			return rank(a.State) < rank(b.State)
		}
		if a.State == "waiting_you" {
			if a.ReplyBefore == nil && b.ReplyBefore != nil {
				return false
			}
			if a.ReplyBefore != nil && b.ReplyBefore == nil {
				return true
			}
			if a.ReplyBefore != nil && !a.ReplyBefore.Equal(*b.ReplyBefore) {
				return a.ReplyBefore.Before(*b.ReplyBefore)
			}
		}
		if !a.LatestAt.Equal(b.LatestAt) {
			return a.LatestAt.After(b.LatestAt)
		}
		return a.CounterpartID.String() < b.CounterpartID.String()
	})
	return people
}

func personGifts(ctx context.Context, tx pgx.Tx, world, player uuid.UUID) ([]personLetter, error) {
	rows, err := tx.Query(ctx, `SELECT event_type,payload,created_at FROM (SELECT DISTINCT ON(payload->>'transport_id') event_type,payload,created_at,id FROM events WHERE world_id=$1 AND ((event_type IN('GiftDispatched','GiftDispatchedV2') AND payload->>'sender_id'=$2) OR (event_type IN('GiftDelivered','GiftLost','GiftDeliveredV2','GiftLostV2') AND (payload->>'sender_id'=$2 OR payload->>'recipient_id'=$2 OR payload->>'actual_recipient_id'=$2))) ORDER BY payload->>'transport_id',id DESC) gifts ORDER BY created_at,id`, world, player.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []personLetter{}
	for rows.Next() {
		var kind string
		var at time.Time
		var raw []byte
		if err = rows.Scan(&kind, &raw, &at); err != nil {
			return nil, err
		}
		var p struct {
			ID            uuid.UUID `json:"transport_id"`
			Sender        uuid.UUID `json:"sender_id"`
			Recipient     uuid.UUID `json:"recipient_id"`
			SenderName    string    `json:"sender_name"`
			RecipientName string    `json:"recipient_name"`
			Good          string    `json:"good_key"`
			Quantity      float64   `json:"quantity"`
			Credited      float64   `json:"credited_quantity"`
			Items         []struct {
				Good     string  `json:"good_key"`
				Quantity float64 `json:"quantity"`
				Credited float64 `json:"credited_quantity"`
			} `json:"items"`
		}
		if err = json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		l := personLetter{ID: p.ID, SenderID: p.Sender, SenderName: p.SenderName, SentAt: at, Kind: "gift", Actions: []string{}, Direction: "incoming", counterpart: p.Sender, counterpartName: p.SenderName, Status: kind, Message: "Gift received", Goods: map[string]float64{p.Good: p.Credited}}
		if p.Sender == player {
			l.Direction = "outgoing"
			l.counterpart = p.Recipient
			l.counterpartName = p.RecipientName
			l.Message = "Gift sent"
			l.Goods = map[string]float64{p.Good: p.Quantity}
		}
		if len(p.Items) > 0 {
			l.Goods = map[string]float64{}
			for _, item := range p.Items {
				amount := item.Credited
				if p.Sender == player {
					amount = item.Quantity
				}
				l.Goods[item.Good] = amount
			}
		}
		if kind == "GiftLost" || kind == "GiftLostV2" {
			l.Message = "Gift lost"
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

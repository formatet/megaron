package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errCorrespondenceCase = errors.New("case must belong to you and the destination's original counterpart; subject must be 1–120 characters")

// attachCorrespondenceTx runs in the departure transaction: a failed append
// cannot dispatch a bearer, reserve payment or create a case at the other end.
func attachCorrespondenceTx(ctx context.Context, tx pgx.Tx, letter, world, sender uuid.UUID, requested, subject string) (uuid.UUID, error) {
	var recipient uuid.UUID
	// Lock the destination through departure so its identity cannot race capture.
	if err := tx.QueryRow(ctx, `SELECT ds.owner_id FROM messengers m JOIN settlements ds ON ds.id=m.destination_id WHERE m.id=$1 AND m.world_id=$2 FOR SHARE OF ds`, letter, world).Scan(&recipient); err != nil {
		return uuid.Nil, errCorrespondenceCase
	}
	var caseID uuid.UUID
	if requested != "" {
		parsed, err := uuid.Parse(requested)
		if err != nil {
			return uuid.Nil, errCorrespondenceCase
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM correspondence_cases WHERE id=$1 AND world_id=$2 AND ((first_player_id=$3 AND second_player_id=$4) OR (first_player_id=$4 AND second_player_id=$3))`, parsed, world, sender, recipient).Scan(&caseID); err != nil {
			return uuid.Nil, errCorrespondenceCase
		}
	} else {
		subject = strings.TrimSpace(subject)
		if subject == "" {
			subject = "Correspondence"
		}
		if len([]rune(subject)) > 120 {
			return uuid.Nil, errCorrespondenceCase
		}
		if err := tx.QueryRow(ctx, `INSERT INTO correspondence_cases(world_id,first_player_id,second_player_id,subject) VALUES($1,$2,$3,$4) RETURNING id`, world, sender, recipient, subject).Scan(&caseID); err != nil {
			return uuid.Nil, err
		}
	}
	_, err := tx.Exec(ctx, `INSERT INTO correspondence_letters(id,case_id,sender_id,recipient_id,sender_name,recipient_name,origin_id,destination_id,origin_name,destination_name,message_text,sent_at,status,trade_offer,delivered,returned)
 SELECT m.id,$2,m.sender_id,$3,COALESCE(sp.wanax_name,sp.username),COALESCE(rp.wanax_name,rp.username),COALESCE(m.origin_id,m.origin_unit_id),m.destination_id,COALESCE(os.name,'Nomadic Host'),ds.name,m.message_text,m.sent_at,m.status,m.trade_offer,m.status='delivered',m.status='arrived'
 FROM messengers m JOIN players sp ON sp.id=m.sender_id JOIN players rp ON rp.id=$3 JOIN settlements ds ON ds.id=m.destination_id LEFT JOIN settlements os ON os.id=m.origin_id WHERE m.id=$1`, letter, caseID, recipient)
	return caseID, err
}

type caseSummary struct {
	ID              uuid.UUID `json:"id"`
	Subject         string    `json:"subject"`
	CounterpartID   uuid.UUID `json:"counterpart_id"`
	CounterpartName string    `json:"counterpart_name"`
	CityName        string    `json:"city_name"`
	CreatedAt       time.Time `json:"created_at"`
	LatestAt        time.Time `json:"latest_at"`
	Preview         string    `json:"preview"`
	LetterCount     int       `json:"letter_count"`
}

type archiveLetter struct {
	ID              uuid.UUID       `json:"id"`
	SenderID        uuid.UUID       `json:"sender_id"`
	RecipientID     uuid.UUID       `json:"recipient_id"`
	SenderName      string          `json:"sender_name"`
	RecipientName   string          `json:"recipient_name"`
	OriginID        uuid.UUID       `json:"origin_id"`
	DestinationID   uuid.UUID       `json:"destination_id"`
	OriginName      string          `json:"origin_name"`
	DestinationName string          `json:"destination_name"`
	Message         string          `json:"message_text"`
	ReplyText       *string         `json:"reply_text,omitempty"`
	SentAt          time.Time       `json:"sent_at"`
	Status          string          `json:"status"`
	TradeOffer      json.RawMessage `json:"trade_offer,omitempty"`
}

type archiveCursor struct {
	World    uuid.UUID `json:"w"`
	Player   uuid.UUID `json:"p"`
	Case     uuid.UUID `json:"c"`
	Query    string    `json:"q"`
	Snapshot string    `json:"s"`
	Epoch    time.Time `json:"e"`
	Before   time.Time `json:"b"`
	ID       uuid.UUID `json:"i"`
}

func (h *MessengerHandler) archiveRequest(w http.ResponseWriter, r *http.Request) (pgx.Tx, archiveCursor, int, bool) {
	var c archiveCursor
	world, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return nil, c, 0, false
	}
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return nil, c, 0, false
	}
	c.World = world
	c.Player = player
	c.Query = strings.TrimSpace(r.URL.Query().Get("q"))
	if len(c.Query) > 200 {
		writeError(w, 400, "search must be at most 200 characters")
		return nil, c, 0, false
	}
	if id := chi.URLParam(r, "caseID"); id != "" {
		c.Case, err = uuid.Parse(id)
		if err != nil {
			writeError(w, 400, "invalid case ID")
			return nil, c, 0, false
		}
	}
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, 400, "limit must be between 1 and 100")
			return nil, c, 0, false
		}
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var saved archiveCursor
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 65536 || e != nil || json.Unmarshal(b, &saved) != nil || saved.World != c.World || saved.Player != c.Player || saved.Case != c.Case || saved.Query != c.Query || saved.Snapshot == "" || saved.Epoch.IsZero() || saved.Before.IsZero() || saved.ID == uuid.Nil {
			writeError(w, 400, "invalid archive cursor")
			return nil, c, 0, false
		}
		c = saved
	}
	tx, err := h.pool.BeginTx(r.Context(), pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "could not load archive")
		return nil, c, 0, false
	}
	if c.Snapshot == "" {
		err = tx.QueryRow(r.Context(), `SELECT pg_current_snapshot()::text,pg_postmaster_start_time()`).Scan(&c.Snapshot, &c.Epoch)
	} else {
		var validated string
		var epoch time.Time
		err = tx.QueryRow(r.Context(), `SELECT $1::pg_snapshot::text,pg_postmaster_start_time()`, c.Snapshot).Scan(&validated, &epoch)
		if err == nil && !epoch.Equal(c.Epoch) {
			err = errors.New("stale cursor")
		}
	}
	if err != nil {
		tx.Rollback(r.Context())
		writeError(w, 400, "invalid archive snapshot")
		return nil, c, 0, false
	}
	return tx, c, limit, true
}

func archiveNext(c archiveCursor, at time.Time, id uuid.UUID) string {
	c.Before = at
	c.ID = id
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Cases is a private person archive. Case membership and ordering use immutable
// creation keys. New letters may change previews but never move cases across pages.
func (h *MessengerHandler) Cases(w http.ResponseWriter, r *http.Request) {
	tx, c, limit, ok := h.archiveRequest(w, r)
	if !ok {
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), `SELECT c.id,c.subject,CASE WHEN c.first_player_id=$2 THEN c.second_player_id ELSE c.first_player_id END,
 CASE WHEN l.sender_id=$2 THEN l.recipient_name ELSE l.sender_name END,
 CASE WHEN l.sender_id=$2 THEN l.destination_name ELSE l.origin_name END,c.created_at,l.sent_at,left(l.message_text,160),
 (SELECT count(*) FROM correspondence_letters n WHERE n.case_id=c.id AND (n.sender_id=$2 OR n.delivered))
 FROM correspondence_cases c JOIN LATERAL(SELECT * FROM correspondence_letters l WHERE l.case_id=c.id AND (l.sender_id=$2 OR l.delivered) ORDER BY l.sent_at DESC,l.id DESC LIMIT 1)l ON true
 WHERE c.world_id=$1 AND ($2=c.first_player_id OR $2=c.second_player_id)
 AND (c.created_epoch<>$4 OR pg_visible_in_snapshot(c.created_xid,$3::pg_snapshot))
 AND ($5::timestamptz IS NULL OR (c.created_at,c.id)<($5,$6::uuid))
 AND ($7='' OR strpos(lower(c.subject||' '||l.sender_name||' '||l.recipient_name||' '||l.origin_name||' '||l.destination_name),lower($7))>0
 OR EXISTS(SELECT 1 FROM correspondence_letters s WHERE s.case_id=c.id AND (s.sender_id=$2 OR s.delivered) AND strpos(lower(s.sender_name||' '||s.recipient_name||' '||s.origin_name||' '||s.destination_name||' '||COALESCE(s.trade_offer->>'want_good','')||' '||COALESCE(s.trade_offer->>'offer_good','')||' '||CASE WHEN s.recipient_id=$2 OR s.returned OR s.trade_offer->>'status'='cancelled' THEN COALESCE(s.trade_offer->>'status','') ELSE 'pending' END),lower($7))>0))
 ORDER BY c.created_at DESC,c.id DESC LIMIT $8`, c.World, c.Player, c.Snapshot, c.Epoch, nullableArchiveTime(c.Before), nullableArchiveID(c.ID), c.Query, limit+1)
	if err != nil {
		writeError(w, 500, "could not load cases")
		return
	}
	defer rows.Close()
	items := make([]caseSummary, 0)
	for rows.Next() {
		var x caseSummary
		if err = rows.Scan(&x.ID, &x.Subject, &x.CounterpartID, &x.CounterpartName, &x.CityName, &x.CreatedAt, &x.LatestAt, &x.Preview, &x.LetterCount); err != nil {
			writeError(w, 500, "could not read cases")
			return
		}
		items = append(items, x)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read cases")
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = archiveNext(c, last.CreatedAt, last.ID)
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next, "legacy_identity": "unavailable"})
}

func nullableArchiveTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
func nullableArchiveID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

func (h *MessengerHandler) Case(w http.ResponseWriter, r *http.Request) {
	var exactID any
	if raw := chi.URLParam(r, "letterID"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil || parsed == uuid.Nil {
			writeError(w, 400, "invalid letter ID")
			return
		}
		if _, exists := r.URL.Query()["cursor"]; exists {
			writeError(w, 400, "exact source letter does not accept a cursor")
			return
		}
		exactID = parsed
	}
	tx, c, limit, ok := h.archiveRequest(w, r)
	if !ok {
		return
	}
	defer tx.Rollback(r.Context())
	var subject string
	var counterpart uuid.UUID
	// Delivery-gate the case itself; knowledge of a UUID cannot reveal outbound mail.
	err := tx.QueryRow(r.Context(), `SELECT subject,CASE WHEN first_player_id=$2 THEN second_player_id ELSE first_player_id END FROM correspondence_cases c WHERE id=$1 AND world_id=$3 AND ($2=first_player_id OR $2=second_player_id) AND EXISTS(SELECT 1 FROM correspondence_letters l WHERE l.case_id=c.id AND (l.sender_id=$2 OR l.delivered))`, c.Case, c.Player, c.World).Scan(&subject, &counterpart)
	if err != nil {
		writeError(w, 404, "case not found")
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT id,sender_id,recipient_id,sender_name,recipient_name,origin_id,destination_id,origin_name,destination_name,message_text,
 CASE WHEN recipient_id=$2 OR returned THEN reply_text ELSE NULL END,sent_at,
 CASE WHEN sender_id=$2 AND NOT returned AND status<>'lost' THEN 'outbound' ELSE status END,
 CASE WHEN sender_id=$2 AND NOT returned AND trade_offer IS NOT NULL AND trade_offer->>'status'<>'cancelled' THEN jsonb_set(trade_offer,'{status}','"pending"') ELSE trade_offer END
 FROM correspondence_letters WHERE case_id=$1 AND (sender_id=$2 OR delivered)
 AND (created_epoch<>$4 OR pg_visible_in_snapshot(created_xid,$3::pg_snapshot))
 AND ($5::timestamptz IS NULL OR (sent_at,id)<($5,$6::uuid)) AND ($8::uuid IS NULL OR id=$8) ORDER BY sent_at DESC,id DESC LIMIT $7`, c.Case, c.Player, c.Snapshot, c.Epoch, nullableArchiveTime(c.Before), nullableArchiveID(c.ID), limit+1, exactID)
	if err != nil {
		writeError(w, 500, "could not load letters")
		return
	}
	defer rows.Close()
	items := make([]archiveLetter, 0)
	for rows.Next() {
		var x archiveLetter
		var offer []byte
		if err = rows.Scan(&x.ID, &x.SenderID, &x.RecipientID, &x.SenderName, &x.RecipientName, &x.OriginID, &x.DestinationID, &x.OriginName, &x.DestinationName, &x.Message, &x.ReplyText, &x.SentAt, &x.Status, &offer); err != nil {
			writeError(w, 500, "could not read letters")
			return
		}
		x.TradeOffer = offer
		items = append(items, x)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not read letters")
		return
	}
	if exactID != nil && len(items) == 0 {
		writeError(w, 404, "visible source letter not found in this case")
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = archiveNext(c, last.SentAt, last.ID)
	}
	writeJSON(w, 200, map[string]any{"id": c.Case, "subject": subject, "counterpart_id": counterpart, "items": items, "next_cursor": next})
}

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type memoryTransaction struct {
	ID              uuid.UUID       `json:"id"`
	CaseID          uuid.UUID       `json:"case_id"`
	Subject         string          `json:"subject"`
	SenderID        uuid.UUID       `json:"sender_id"`
	RecipientID     uuid.UUID       `json:"recipient_id"`
	SenderName      string          `json:"sender_name"`
	RecipientName   string          `json:"recipient_name"`
	OriginName      string          `json:"origin_name"`
	DestinationName string          `json:"destination_name"`
	SentAt          time.Time       `json:"sent_at"`
	AcceptedAt      *time.Time      `json:"accepted_at"`
	Terms           json.RawMessage `json:"terms"`
	Outcome         string          `json:"outcome"`
}
type memoryCitation struct {
	ID         uuid.UUID `json:"id"`
	CaseID     uuid.UUID `json:"case_id"`
	LetterID   uuid.UUID `json:"letter_id"`
	Source     string    `json:"source"`
	Quote      string    `json:"quote"`
	CreatedAt  time.Time `json:"created_at"`
	AuthorID   uuid.UUID `json:"author_id"`
	AuthorName string    `json:"author_name"`
	CityName   string    `json:"city_name"`
}
type memoryPosition struct {
	Before time.Time `json:"b"`
	ID     uuid.UUID `json:"i"`
	Done   bool      `json:"d"`
}
type memoryCursor struct {
	Version      int            `json:"v"`
	World        uuid.UUID      `json:"w"`
	Player       uuid.UUID      `json:"p"`
	Case         uuid.UUID      `json:"c"`
	Counterpart  uuid.UUID      `json:"o"`
	Snapshot     string         `json:"s"`
	Epoch        time.Time      `json:"e"`
	Transactions memoryPosition `json:"t"`
	Citations    memoryPosition `json:"n"`
}

func memoryCounterpart(tx pgx.Tx, r *http.Request, id, world, player uuid.UUID) (uuid.UUID, error) {
	var counterpart uuid.UUID
	err := tx.QueryRow(r.Context(), `SELECT CASE WHEN c.first_player_id=$3 THEN c.second_player_id ELSE c.first_player_id END FROM correspondence_cases c WHERE `+privateCaseAccessSQL+` FOR SHARE OF c`, id, world, player).Scan(&counterpart)
	return counterpart, err
}

// CaseMemory spans this same pair of people across visible cases. It reads
// archive outcomes only; a live remote transport is never a knowledge shortcut.
func (h *MessengerHandler) CaseMemory(w http.ResponseWriter, r *http.Request) {
	world, id, player, ok := privateCaseIdentity(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("q") != "" {
		writeError(w, 400, "memory search is not supported; use the case archive search")
		return
	}
	limit := 30
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, 400, "limit must be between 1 and 100")
			return
		}
	}
	// A regular transaction permits a shared case lock, protecting retention
	// against a concurrent world deletion through both private reads/writes.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not load correspondence memory")
		return
	}
	defer tx.Rollback(r.Context())
	counterpart, err := memoryCounterpart(tx, r, id, world, player)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "case not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not authorize correspondence memory")
		return
	}
	c := memoryCursor{Version: 1, World: world, Player: player, Case: id, Counterpart: counterpart}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var saved memoryCursor
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 65536 || e != nil || json.Unmarshal(b, &saved) != nil || saved.Version != 1 || saved.World != world || saved.Player != player || saved.Case != id || saved.Counterpart != counterpart || saved.Snapshot == "" || saved.Epoch.IsZero() {
			writeError(w, 400, "invalid memory cursor")
			return
		}
		for _, p := range []memoryPosition{saved.Transactions, saved.Citations} {
			if !p.Done && (p.Before.IsZero() || p.ID == uuid.Nil) {
				writeError(w, 400, "invalid memory position")
				return
			}
		}
		c = saved
	}
	if c.Snapshot == "" {
		err = tx.QueryRow(r.Context(), `SELECT pg_current_snapshot()::text,pg_postmaster_start_time()`).Scan(&c.Snapshot, &c.Epoch)
	} else {
		var validated string
		var epoch time.Time
		err = tx.QueryRow(r.Context(), `SELECT $1::pg_snapshot::text,pg_postmaster_start_time()`, c.Snapshot).Scan(&validated, &epoch)
		if err == nil && !epoch.Equal(c.Epoch) {
			err = errors.New("stale memory cursor")
		}
	}
	if err != nil {
		writeError(w, 400, "invalid memory snapshot")
		return
	}
	transactions := make([]memoryTransaction, 0)
	citations := make([]memoryCitation, 0)
	if !c.Transactions.Done {
		rows, e := tx.Query(r.Context(), `SELECT l.id,l.case_id,c.subject,l.sender_id,l.recipient_id,l.sender_name,l.recipient_name,l.origin_name,l.destination_name,l.sent_at,l.accepted_at,l.trade_offer
 FROM correspondence_letters l JOIN correspondence_cases c ON c.id=l.case_id
 WHERE c.world_id=$1 AND ((c.first_player_id=$2 AND c.second_player_id=$3) OR (c.first_player_id=$3 AND c.second_player_id=$2))
 AND l.trade_offer->>'status'='accepted' AND l.accepted_xid IS NOT NULL
 AND ((l.recipient_id=$2 AND l.delivered) OR (l.sender_id=$2 AND l.returned AND (l.returned_epoch<>$5 OR pg_visible_in_snapshot(l.returned_xid,$4::pg_snapshot))))
 AND (l.accepted_epoch<>$5 OR pg_visible_in_snapshot(l.accepted_xid,$4::pg_snapshot))
 AND ($6::timestamptz IS NULL OR (l.sent_at,l.id)<($6,$7::uuid))
 ORDER BY l.sent_at DESC,l.id DESC LIMIT $8`, world, player, counterpart, c.Snapshot, c.Epoch, nullableArchiveTime(c.Transactions.Before), nullableArchiveID(c.Transactions.ID), limit+1)
		if e != nil {
			writeError(w, 500, "could not load accepted transactions")
			return
		}
		for rows.Next() {
			var x memoryTransaction
			var terms []byte
			if e = rows.Scan(&x.ID, &x.CaseID, &x.Subject, &x.SenderID, &x.RecipientID, &x.SenderName, &x.RecipientName, &x.OriginName, &x.DestinationName, &x.SentAt, &x.AcceptedAt, &terms); e != nil {
				break
			}
			x.Terms = terms
			x.Outcome = "unavailable"
			transactions = append(transactions, x)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			writeError(w, 500, "could not read accepted transactions")
			return
		}
		c.Transactions.Done = len(transactions) <= limit
		if len(transactions) > limit {
			transactions = transactions[:limit]
		}
		if len(transactions) > 0 {
			last := transactions[len(transactions)-1]
			c.Transactions.Before = last.SentAt
			c.Transactions.ID = last.ID
		}
	}
	if !c.Citations.Done {
		rows, e := tx.Query(r.Context(), `SELECT n.id,n.case_id,n.letter_id,n.source,n.quote,n.created_at,
 CASE WHEN n.source='reply' THEN l.recipient_id ELSE l.sender_id END,
 CASE WHEN n.source='reply' THEN l.recipient_name ELSE l.sender_name END,
 CASE WHEN n.source='reply' THEN l.destination_name ELSE l.origin_name END
 FROM correspondence_citations n JOIN correspondence_cases c ON c.id=n.case_id JOIN correspondence_letters l ON l.id=n.letter_id
 WHERE n.player_id=$1 AND c.world_id=$2 AND ((c.first_player_id=$1 AND c.second_player_id=$3) OR (c.first_player_id=$3 AND c.second_player_id=$1))
 AND (n.created_epoch<>$5 OR pg_visible_in_snapshot(n.created_xid,$4::pg_snapshot))
 AND ($6::timestamptz IS NULL OR (n.created_at,n.id)<($6,$7::uuid))
 ORDER BY n.created_at DESC,n.id DESC LIMIT $8`, player, world, counterpart, c.Snapshot, c.Epoch, nullableArchiveTime(c.Citations.Before), nullableArchiveID(c.Citations.ID), limit+1)
		if e != nil {
			writeError(w, 500, "could not load private citations")
			return
		}
		for rows.Next() {
			var x memoryCitation
			if e = rows.Scan(&x.ID, &x.CaseID, &x.LetterID, &x.Source, &x.Quote, &x.CreatedAt, &x.AuthorID, &x.AuthorName, &x.CityName); e != nil {
				break
			}
			citations = append(citations, x)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			writeError(w, 500, "could not read private citations")
			return
		}
		c.Citations.Done = len(citations) <= limit
		if len(citations) > limit {
			citations = citations[:limit]
		}
		if len(citations) > 0 {
			last := citations[len(citations)-1]
			c.Citations.Before = last.CreatedAt
			c.Citations.ID = last.ID
		}
	}
	next := ""
	if !c.Transactions.Done || !c.Citations.Done {
		b, _ := json.Marshal(c)
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	writeJSON(w, 200, map[string]any{"counterpart_id": counterpart, "transactions": transactions, "citations": citations, "next_cursor": next})
}

func (h *MessengerHandler) CreateCitation(w http.ResponseWriter, r *http.Request) {
	world, id, player, ok := privateCaseIdentity(w, r)
	if !ok {
		return
	}
	var req struct {
		LetterID string `json:"letter_id"`
		Source   string `json:"source"`
		Quote    string `json:"quote"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil {
		writeError(w, 400, "invalid citation JSON")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeError(w, 400, "one JSON object required")
		return
	}
	letter, err := uuid.Parse(req.LetterID)
	if err != nil || (req.Source != "message" && req.Source != "reply") || len(req.Quote) == 0 || len(req.Quote) > 1000 {
		writeError(w, 400, "letter_id, source (message or reply), and exact nonempty quote up to 1000 bytes are required")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not save private citation")
		return
	}
	defer tx.Rollback(r.Context())
	counterpart, err := memoryCounterpart(tx, r, id, world, player)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "case not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not authorize citation")
		return
	}
	x := memoryCitation{LetterID: letter, Source: req.Source, Quote: req.Quote}
	var text string
	err = tx.QueryRow(r.Context(), `SELECT l.case_id,
 CASE WHEN $5='reply' THEN l.recipient_id ELSE l.sender_id END,
 CASE WHEN $5='reply' THEN l.recipient_name ELSE l.sender_name END,
 CASE WHEN $5='reply' THEN l.destination_name ELSE l.origin_name END,
 CASE WHEN $5='reply' THEN l.reply_text ELSE l.message_text END
 FROM correspondence_letters l JOIN correspondence_cases c ON c.id=l.case_id
 WHERE l.id=$1 AND c.world_id=$2 AND ((c.first_player_id=$3 AND c.second_player_id=$4) OR (c.first_player_id=$4 AND c.second_player_id=$3))
 AND (l.sender_id=$3 OR l.delivered)
 AND ($5='message' OR (l.reply_text IS NOT NULL AND (l.recipient_id=$3 OR l.returned)))`, letter, world, player, counterpart, req.Source).Scan(&x.CaseID, &x.AuthorID, &x.AuthorName, &x.CityName, &text)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "visible source letter not found for this person")
		return
	}
	if err != nil {
		writeError(w, 500, "could not read citation source")
		return
	}
	if !strings.Contains(text, req.Quote) {
		writeError(w, 400, "quote must be an exact passage from the selected message or reply")
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO correspondence_citations(case_id,player_id,letter_id,source,quote) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(player_id,letter_id,source,quote) DO UPDATE SET quote=correspondence_citations.quote RETURNING id,created_at`, x.CaseID, player, letter, req.Source, req.Quote).Scan(&x.ID, &x.CreatedAt)
	if err != nil {
		writeError(w, 500, "could not save private citation")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "could not commit private citation")
		return
	}
	writeJSON(w, 201, x)
}

func (h *MessengerHandler) DeleteCitation(w http.ResponseWriter, r *http.Request) {
	world, id, player, ok := privateCaseIdentity(w, r)
	if !ok {
		return
	}
	citation, err := uuid.Parse(chi.URLParam(r, "citationID"))
	if err != nil {
		writeError(w, 400, "invalid citation ID")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not delete citation")
		return
	}
	defer tx.Rollback(r.Context())
	counterpart, err := memoryCounterpart(tx, r, id, world, player)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "case not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not authorize citation deletion")
		return
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM correspondence_citations n USING correspondence_cases c WHERE n.id=$1 AND n.player_id=$2 AND c.id=n.case_id AND c.world_id=$3
 AND ((c.first_player_id=$2 AND c.second_player_id=$4) OR (c.first_player_id=$4 AND c.second_player_id=$2))`, citation, player, world, counterpart)
	if err != nil {
		writeError(w, 500, "could not delete citation")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "could not commit citation deletion")
		return
	}
	w.WriteHeader(204)
}

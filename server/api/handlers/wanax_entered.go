package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const wanaxEnteredKind = "WanaxEntered"

// wanaxEnteredMaxRunes caps the founder's own message.
const wanaxEnteredMaxRunes = 280

// A new Wanax is heard by all (Timothy 2026-11-02): a deliberate fourth
// exception to the messenger pillar. The payload carries who and what they
// say — NEVER where: no position, no city name, no direction.

// normaliseEnteredMessage trims blanks and enforces the length cap.
func normaliseEnteredMessage(s string) (string, error) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n > wanaxEnteredMaxRunes {
		return "", fmt.Errorf("your message is %d characters — the limit is %d", n, wanaxEnteredMaxRunes)
	}
	return s, nil
}

type wanaxEntered struct {
	worldID uuid.UUID
	ev      *events.Event
	body    map[string]any
	notices []wanaxEnteredNotice
}

type wanaxEnteredNotice struct {
	playerID uuid.UUID
	id       string
}

// announceWanaxEntered writes the WanaxEntered event and one notice per other
// Wanax in the world, inside the founding transaction. It returns nil when this
// is not the founder's FIRST city: a Wanax who rose again after every city fell
// (founder_phase.is_refugee) is not new. Colonies never reach it. The founder
// gets no copy.
func announceWanaxEntered(ctx context.Context, tx pgx.Tx, store *events.Store, worldID, founderID uuid.UUID, message string) (*wanaxEntered, error) {
	var refugee bool
	var name string
	if err := tx.QueryRow(ctx,
		`SELECT fp.is_refugee, COALESCE(NULLIF(p.wanax_name,''), p.username)
		   FROM founder_phase fp JOIN players p ON p.id = fp.owner_id
		  WHERE fp.world_id = $1 AND fp.owner_id = $2`, worldID, founderID,
	).Scan(&refugee, &name); err != nil {
		return nil, fmt.Errorf("load founder for announcement: %w", err)
	}
	if refugee {
		return nil, nil
	}
	body := map[string]any{"wanax_id": founderID, "wanax_name": name, "message": message}
	ev, err := store.AppendTx(ctx, tx, worldID, events.StreamWorld, wanaxEnteredKind, body, worldID, nil)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`INSERT INTO notifications(world_id, player_id, kind, level, body_json, world_tick)
		 SELECT $1, pwr.player_id, $2, 3, $3, (SELECT current_tick FROM worlds WHERE id = $1)
		   FROM player_world_records pwr
		  WHERE pwr.world_id = $1 AND pwr.player_id <> $4
		 ON CONFLICT DO NOTHING
		 RETURNING player_id, id`, worldID, wanaxEnteredKind, raw, founderID)
	if err != nil {
		return nil, fmt.Errorf("write WanaxEntered notices: %w", err)
	}
	defer rows.Close()
	out := &wanaxEntered{worldID: worldID, ev: ev, body: body}
	for rows.Next() {
		var n wanaxEnteredNotice
		if err := rows.Scan(&n.playerID, &n.id); err != nil {
			return nil, err
		}
		out.notices = append(out.notices, n)
	}
	return out, rows.Err()
}

// deliver records the event and pushes each committed notice; safe on nil.
func (w *wanaxEntered) deliver(ctx context.Context, store *events.Store, hub *notify.Hub) {
	if w == nil {
		return
	}
	store.RecordCommitted(ctx, w.ev)
	if hub == nil {
		return
	}
	for _, n := range w.notices {
		hub.DeliverCommittedNotification(ctx, w.worldID, n.playerID, n.id, wanaxEnteredKind, 3, w.body)
	}
}

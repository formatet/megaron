package handlers

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/notify"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/textfilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// textIntn draws the replacement for a coarse word; a test may pin it.
var textIntn = rand.IntN

// cleanLetter and cleanName replace coarse words (Timothy 2026-10-29) and
// return how many there were; each one angers the gods once the write commits.
func cleanLetter(s string) (string, int) { return textfilter.Letter(s, textIntn) }
func cleanName(s string) (string, int)   { return textfilter.Name(s, textIntn) }

// godsAngered is the GodsAngered notice written inside the caller's
// transaction, delivered over the hub only after it commits.
type godsAngered struct {
	worldID, playerID uuid.UUID
	id                string
	body              map[string]any
	ev                *events.Event
}

// angerGodsTx takes the kharis for words coarse words and writes the event and
// notice in tx. It returns nil when there were none. store may be nil.
func angerGodsTx(ctx context.Context, tx pgx.Tx, store *events.Store, worldID, playerID uuid.UUID, words int, where string) (*godsAngered, error) {
	if words == 0 {
		return nil, nil
	}
	before, after, err := religion.TakeProfanityKharisTx(ctx, tx, worldID, playerID, words)
	if err != nil {
		return nil, err
	}
	g := &godsAngered{worldID: worldID, playerID: playerID, body: map[string]any{
		"words": words, "where": where,
		"kharis_lost": before - after, "kharis_before": before, "kharis_after": after,
	}}
	if store != nil {
		if g.ev, err = store.AppendTx(ctx, tx, playerID, events.StreamReligion, "GodsAngered", g.body, worldID, nil); err != nil {
			return nil, err
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
		VALUES($1,$2,'GodsAngered',2,$3,(SELECT current_tick FROM worlds WHERE id = $1)) RETURNING id`, worldID, playerID, g.body).Scan(&g.id)
	return g, err
}

// deliver pushes the committed notice; safe on nil.
func (g *godsAngered) deliver(ctx context.Context, store *events.Store, hub *notify.Hub) {
	if g == nil {
		return
	}
	if g.ev != nil && store != nil {
		store.RecordCommitted(ctx, g.ev)
	}
	if hub != nil {
		hub.DeliverCommittedNotification(ctx, g.worldID, g.playerID, g.id, "GodsAngered", 2, g.body)
	}
}

// angerGodsOwnTx angers the gods in a transaction of its own, for a write
// whose commit happens deeper down than the handler can reach.
func angerGodsOwnTx(ctx context.Context, pool *pgxpool.Pool, store *events.Store, hub *notify.Hub, worldID, playerID uuid.UUID, words int, where string) {
	if words == 0 {
		return
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		slog.Error("gods angered: begin", "err", err)
		return
	}
	defer tx.Rollback(ctx)
	g, err := angerGodsTx(ctx, tx, store, worldID, playerID, words, where)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		slog.Error("gods angered: record", "err", err, "player", playerID)
		return
	}
	g.deliver(ctx, store, hub)
}

// statusWriter remembers the status a handler answered with (0 = 200).
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

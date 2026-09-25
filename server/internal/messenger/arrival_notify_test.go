package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The diplomacy channel was the only one in the game that notified nothing:
// neither ArrivalHandler nor ReturnHandler held a hub, so a messenger standing
// in your court — the asynchronicity gate's sharpest case, "läs + svara" —
// produced no dispatch, no archive row and no badge. You found out by opening
// the diplomacy drawer on a hunch. These tests hold both halves.

// fakeArrivalBroadcaster records NotifyPlayer calls. Mirrors
// fakeRecallBroadcaster in order_delivery_recall_miss_test.go.
type fakeArrivalBroadcaster struct {
	kinds    []string
	players  []uuid.UUID
	levels   []int
	payloads []map[string]any
}

func (f *fakeArrivalBroadcaster) BroadcastEvent(uuid.UUID, string, any) {}

func (f *fakeArrivalBroadcaster) NotifyPlayer(_ context.Context, _, playerID uuid.UUID, kind string, level int, payload any) error {
	f.kinds = append(f.kinds, kind)
	f.players = append(f.players, playerID)
	f.levels = append(f.levels, level)
	m, _ := payload.(map[string]any)
	f.payloads = append(f.payloads, m)
	return nil
}

// messengerScene is two Wanaxes, each with a city, and a messenger in flight
// between them.
type messengerScene struct {
	pool                 *pgxpool.Pool
	worldID              uuid.UUID
	senderID, receiverID uuid.UUID
	// wanax_name is globally unique, so the readable names carry a suffix and
	// the assertions compare against what was actually stored.
	senderWanax, receiverWanax string
	originID, destID           uuid.UUID
	destQ, destR               int
	originQ, originR           int
}

func setupMessengerScene(t *testing.T) *messengerScene {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	sc := &messengerScene{pool: pool, destQ: 4, destR: 5, originQ: 1, originR: 1}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-msg-notify-"+uuid.NewString(),
	).Scan(&sc.worldID); err != nil {
		t.Fatalf("world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, sc.worldID) })

	sc.senderWanax = "Polyidos-" + uuid.NewString()[:8]
	sc.receiverWanax = "Deukalion-" + uuid.NewString()[:8]
	for _, spec := range []struct {
		id     *uuid.UUID
		wanax  string
		player string
	}{
		{&sc.senderID, sc.senderWanax, "sender"},
		{&sc.receiverID, sc.receiverWanax, "receiver"},
	} {
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, password_hash, wanax_name) VALUES ($1, 'x', $2) RETURNING id`,
			spec.player+"-"+uuid.NewString(), spec.wanax,
		).Scan(spec.id); err != nil {
			t.Fatalf("player %s: %v", spec.player, err)
		}
	}

	mk := func(owner uuid.UUID, name string, q, r int) uuid.UUID {
		var provID, setID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
			sc.worldID, q, r,
		).Scan(&provID); err != nil {
			t.Fatalf("province %s: %v", name, err)
		}
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, population)
			 VALUES ($1, $2, $3, 'achaean', $4, 'capital', 500) RETURNING id`,
			sc.worldID, provID, name, owner,
		).Scan(&setID); err != nil {
			t.Fatalf("settlement %s: %v", name, err)
		}
		return setID
	}
	sc.originID = mk(sc.senderID, "Knossos", sc.originQ, sc.originR)
	sc.destID = mk(sc.receiverID, "Phaistos", sc.destQ, sc.destR)
	return sc
}

// sendMessenger inserts an outbound messenger and returns its id. tradeOffer is
// raw JSON or "" for a plain message.
func (sc *messengerScene) sendMessenger(t *testing.T, text, tradeOffer string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	var offerArg any
	if tradeOffer != "" {
		offerArg = tradeOffer
	}
	if err := sc.pool.QueryRow(context.Background(),
		`INSERT INTO messengers (world_id, sender_id, origin_id, destination_id, message_text,
		                         status, hex_q, hex_r, arrives_at, trade_offer)
		 VALUES ($1, $2, $3, $4, $5, 'outbound', $6, $7, now(), $8) RETURNING id`,
		sc.worldID, sc.senderID, sc.originID, sc.destID, text, sc.originQ, sc.originR, offerArg,
	).Scan(&id); err != nil {
		t.Fatalf("send messenger: %v", err)
	}
	return id
}

func (sc *messengerScene) arrivalHandler(hub *fakeArrivalBroadcaster) *ArrivalHandler {
	clk := clock.NewTestClock(time.Now())
	return NewArrivalHandler(sc.pool, events.NewScheduler(sc.pool, clk), events.NewStore(sc.pool), hub)
}

func (sc *messengerScene) deliver(t *testing.T, h *ArrivalHandler, id uuid.UUID) {
	t.Helper()
	if err := h.Handle(context.Background(), events.ScheduledEvent{
		WorldID: sc.worldID, DueTick: 10,
		Payload: mustJSON(t, ArrivalPayload{MessengerID: id}),
	}); err != nil {
		t.Fatalf("handle arrival: %v", err)
	}
}

// TestMessengerArrivalNotifiesRecipientByName: the notice reaches the RECIPIENT
// (never the sender, never a broadcast — FOW), names the sender by Wanax name
// and the city it reached, and carries that city's hex so "⌖ Take me there"
// works. Mutation: drop the notifyDelivered call and this fails on an empty
// notification list — the exact silence that shipped for months.
func TestMessengerArrivalNotifiesRecipientByName(t *testing.T) {
	sc := setupMessengerScene(t)
	hub := &fakeArrivalBroadcaster{}
	id := sc.sendMessenger(t, "Will you trade tin?", "")

	sc.deliver(t, sc.arrivalHandler(hub), id)

	if len(hub.kinds) != 1 || hub.kinds[0] != "MessengerArrival" {
		t.Fatalf("kinds = %v, want exactly [MessengerArrival]", hub.kinds)
	}
	if hub.players[0] != sc.receiverID {
		t.Errorf("notified %v, want the recipient %v", hub.players[0], sc.receiverID)
	}
	p := hub.payloads[0]
	if p["from"] != sc.senderWanax {
		t.Errorf("from = %v, want the sender's Wanax name %s", p["from"], sc.senderWanax)
	}
	if p["name"] != "Phaistos" {
		t.Errorf("name = %v, want the city it reached, Phaistos", p["name"])
	}
	if p["q"] != sc.destQ || p["r"] != sc.destR {
		t.Errorf("q,r = %v,%v want %d,%d — without them there is no destination to jump to",
			p["q"], p["r"], sc.destQ, sc.destR)
	}
	if p["message"] != "Will you trade tin?" {
		t.Errorf("message = %v, want the text carried", p["message"])
	}
}

// TestMessengerArrivalCarriesOfferTermsAtUrgentLevel: an offer can only be
// accepted while its bearer stands there, so the terms are the notification and
// it outranks a plain message.
func TestMessengerArrivalCarriesOfferTermsAtUrgentLevel(t *testing.T) {
	sc := setupMessengerScene(t)
	hub := &fakeArrivalBroadcaster{}
	id := sc.sendMessenger(t, "A bargain",
		`{"kind":"buy","want_good":"tin","want_qty":40,"offer_silver":200,"status":"pending"}`)

	sc.deliver(t, sc.arrivalHandler(hub), id)

	if len(hub.payloads) != 1 {
		t.Fatalf("notifications = %d, want 1", len(hub.payloads))
	}
	if hub.levels[0] != 2 {
		t.Errorf("level = %d, want 2 — an offer with a clock on it is not routine", hub.levels[0])
	}
	offer, ok := hub.payloads[0]["offer"].(map[string]any)
	if !ok {
		t.Fatalf("offer missing from payload: %v", hub.payloads[0])
	}
	if offer["want_good"] != "tin" || offer["offer_silver"] != float64(200) {
		t.Errorf("offer terms = %v, want tin/200 silver", offer)
	}
}

// TestMessengerToOwnCityIsNotNews: a messenger you sent to your own settlement
// is your own errand. Notifying yourself about your own act is exactly what
// megaron_notifikationer.md forbids.
func TestMessengerToOwnCityIsNotNews(t *testing.T) {
	sc := setupMessengerScene(t)
	hub := &fakeArrivalBroadcaster{}
	if _, err := sc.pool.Exec(context.Background(),
		`UPDATE settlements SET owner_id = $1 WHERE id = $2`, sc.senderID, sc.destID); err != nil {
		t.Fatalf("reassign city: %v", err)
	}
	id := sc.sendMessenger(t, "note to self", "")

	sc.deliver(t, sc.arrivalHandler(hub), id)

	if len(hub.kinds) != 0 {
		t.Errorf("notifications = %v, want none for a messenger to your own city", hub.kinds)
	}
}

// TestMessengerArrivalNotifiedOnceOnReplay: the claim (FOR UPDATE + status
// guard) already makes delivery exactly-once; the notice must inherit that and
// not fire again when the worker retries a completed event (G2).
func TestMessengerArrivalNotifiedOnceOnReplay(t *testing.T) {
	sc := setupMessengerScene(t)
	hub := &fakeArrivalBroadcaster{}
	h := sc.arrivalHandler(hub)
	id := sc.sendMessenger(t, "once", "")

	sc.deliver(t, h, id)
	sc.deliver(t, h, id) // replay

	if len(hub.kinds) != 1 {
		t.Errorf("notifications = %d after replay, want 1", len(hub.kinds))
	}
}

// TestMessengerReturnNotifiesSenderWithReply: the reply rides home WITH the
// messenger (command is never instant), so the return is where the exchange
// closes for the sender — and it was silent too.
func TestMessengerReturnNotifiesSenderWithReply(t *testing.T) {
	sc := setupMessengerScene(t)
	ctx := context.Background()
	hub := &fakeArrivalBroadcaster{}
	id := sc.sendMessenger(t, "Will you trade tin?", "")
	if _, err := sc.pool.Exec(ctx,
		`UPDATE messengers SET status = 'returning', reply_text = 'Bring copper and we shall speak' WHERE id = $1`,
		id); err != nil {
		t.Fatalf("set reply: %v", err)
	}

	rh := NewReturnHandler(sc.pool, events.NewStore(sc.pool), hub)
	for i := 0; i < 2; i++ { // second pass proves the replay guard covers the notice
		if err := rh.Handle(ctx, events.ScheduledEvent{
			WorldID: sc.worldID, DueTick: 20,
			Payload: mustJSON(t, ReturnPayload{MessengerID: id}),
		}); err != nil {
			t.Fatalf("handle return: %v", err)
		}
	}

	if len(hub.kinds) != 1 || hub.kinds[0] != "MessengerReturned" {
		t.Fatalf("kinds = %v, want exactly [MessengerReturned]", hub.kinds)
	}
	if hub.players[0] != sc.senderID {
		t.Errorf("notified %v, want the sender %v", hub.players[0], sc.senderID)
	}
	p := hub.payloads[0]
	if p["replied"] != true || p["reply"] != "Bring copper and we shall speak" {
		t.Errorf("reply not carried: %v", p)
	}
	if p["to"] != "Phaistos" {
		t.Errorf("to = %v, want the city it visited", p["to"])
	}
	if p["q"] != sc.originQ || p["r"] != sc.originR {
		t.Errorf("q,r = %v,%v want the HOME city %d,%d", p["q"], p["r"], sc.originQ, sc.originR)
	}
	if hub.levels[0] != 2 {
		t.Errorf("level = %d, want 2 when an answer came back", hub.levels[0])
	}
}

// TestMessengerReturnWithoutReplyIsInformational: no answer is information, not
// a decision — it must not wear the same urgency as one that carries a reply.
func TestMessengerReturnWithoutReplyIsInformational(t *testing.T) {
	sc := setupMessengerScene(t)
	ctx := context.Background()
	hub := &fakeArrivalBroadcaster{}
	id := sc.sendMessenger(t, "Anyone there?", "")
	if _, err := sc.pool.Exec(ctx, `UPDATE messengers SET status = 'returning' WHERE id = $1`, id); err != nil {
		t.Fatalf("set returning: %v", err)
	}

	rh := NewReturnHandler(sc.pool, events.NewStore(sc.pool), hub)
	if err := rh.Handle(ctx, events.ScheduledEvent{
		WorldID: sc.worldID, DueTick: 20,
		Payload: mustJSON(t, ReturnPayload{MessengerID: id}),
	}); err != nil {
		t.Fatalf("handle return: %v", err)
	}

	if len(hub.levels) != 1 || hub.levels[0] != 3 {
		t.Errorf("levels = %v, want [3]", hub.levels)
	}
	if hub.payloads[0]["replied"] != nil {
		t.Errorf("replied set on an unanswered return: %v", hub.payloads[0])
	}
}

// mustJSON marshals a scheduled-event payload for the handler under test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

package handlers

// A4 — GET /worlds/{id}/runners, the Runners tab's one read. Own runners are
// grouped by where they are; foreign runners appear ONLY where their presence
// is physically known to the reader (standing in the reader's city or port,
// aboard the reader's ship) and never with their content.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

type runnersView struct {
	Own     []map[string]any `json:"own"`
	Foreign []map[string]any `json:"foreign"`
}

func (f *navalPlayerFixture) getRunners(t *testing.T, token string) (runnersView, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/runners", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET runners = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var v runnersView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode runners: %v", err)
	}
	return v, rec.Body.String()
}

func runnersIn(items []map[string]any, group string) []map[string]any {
	var out []map[string]any
	for _, it := range items {
		if it["group"] == group {
			out = append(out, it)
		}
	}
	return out
}

func TestRunners_GroupsAndIntegrity(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	ctx := context.Background()
	mh := NewMessengerHandler(f.pool, nil, f.clk, nil)
	f.router.Get("/worlds/{worldID}/runners", mh.Runners)

	// wanax_name is unique database-wide and the suite leaves rows behind.
	sfx := " " + uuid.NewString()[:6]
	dunios, kassandra := "Dunios"+sfx, "Kassandra"+sfx
	for id, name := range map[uuid.UUID]string{f.initiatorID: dunios, f.counterpartyID: kassandra} {
		if _, err := f.pool.Exec(ctx, `UPDATE players SET wanax_name=$2 WHERE id=$1`, id, name); err != nil {
			t.Fatal(err)
		}
	}
	byblos := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	ugarit := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	kassShip := f.ship(t, ugarit, f.counterpartyID, "merchantman")
	if _, err := f.pool.Exec(ctx, `UPDATE units SET name='Sea Swallow' WHERE id=$1`, kassShip); err != nil {
		t.Fatal(err)
	}

	send := func(token string, from, to uuid.UUID, text string) uuid.UUID {
		code, resp := f.post(t, token, "/worlds/"+f.worldID.String()+"/settlements/"+from.String()+"/messengers",
			map[string]any{"destination_id": to.String(), "message": text})
		if code != 201 {
			t.Fatalf("send: %d %v", code, resp)
		}
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `SELECT id FROM messengers WHERE origin_id=$1 AND destination_id=$2 AND message_text=$3`, from, to, text).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	secret := func(id uuid.UUID, word string) {
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET reply_text=$2, trade_offer=$3, order_payload=$4 WHERE id=$1`,
			id, "REPLY-"+word, `{"want_good":"TRADE-`+word+`","status":"pending"}`, `{"note":"ORDER-`+word+`"}`); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Dunios' runner waits in his own port (no ship anywhere).
	waiting := send(f.initiatorToken, byblos, ugarit, "LETTER-ALPHA to Kassandra")
	// 2. Dunios' runner rides Kassandra's ship.
	aboard := send(f.initiatorToken, byblos, ugarit, "LETTER-BRAVO to Kassandra")
	secret(aboard, "BRAVO")
	if _, err := f.pool.Exec(ctx,
		`UPDATE messengers SET passage_status='aboard', carrier_unit_id=$2, carrier_name='Sea Swallow', carrier_owner_id=$3,
		        boarded_at=now(), disembark_at=now()+interval '5 hours', disembark_q=5, disembark_r=0 WHERE id=$1`,
		aboard, kassShip, f.counterpartyID); err != nil {
		t.Fatal(err)
	}
	// 3. On the road and 4. coming home: no passage at all.
	road := send(f.initiatorToken, byblos, ugarit, "LETTER-CHARLIE to Kassandra")
	home := send(f.initiatorToken, byblos, ugarit, "LETTER-DELTA to Kassandra")
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status=NULL, passage_port_id=NULL, arrives_at=now()+interval '3 hours' WHERE id=$1`, road); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status=NULL, passage_port_id=NULL, status='returning', arrives_at=now()+interval '2 hours' WHERE id=$1`, home); err != nil {
		t.Fatal(err)
	}
	// An order runner shows the sender's own one-line summary (commit 60a8abc8).
	order := send(f.initiatorToken, byblos, ugarit, "ORDER-CARRIER text")
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET kind='order', passage_status=NULL, passage_port_id=NULL, arrives_at=now()+interval '1 hour',
	        order_payload=jsonb_build_object('player_id',$2::text,'verb','stance','stance',jsonb_build_object('stance','hold')) WHERE id=$1`, order, f.initiatorID.String()); err != nil {
		t.Fatal(err)
	}
	// 5. Kassandra's runner stands waiting in DUNIOS' port, carrying words only she should know.
	foreignWait := send(f.counterpartyToken, ugarit, byblos, "LETTER-ECHO to Dunios")
	secret(foreignWait, "ECHO")
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_port_id=$2 WHERE id=$1`, foreignWait, byblos); err != nil {
		t.Fatal(err)
	}

	// 6. Kassandra's own runner rides her own ship: Dunios has no business seeing it.
	ownShip := send(f.counterpartyToken, ugarit, byblos, "LETTER-FOXTROT to Dunios")
	if _, err := f.pool.Exec(ctx,
		`UPDATE messengers SET passage_status='aboard', carrier_unit_id=$2, carrier_name='Sea Swallow', carrier_owner_id=$3,
		        boarded_at=now(), disembark_at=now()+interval '5 hours', disembark_q=0, disembark_r=0 WHERE id=$1`,
		ownShip, kassShip, f.counterpartyID); err != nil {
		t.Fatal(err)
	}
	_ = ownShip

	dun, dunBody := f.getRunners(t, f.initiatorToken)
	kass, kassBody := f.getRunners(t, f.counterpartyToken)

	// Own groups, Dunios.
	w := runnersIn(dun.Own, "waiting")
	if len(w) != 1 || w[0]["port_name"] != "Byblos" || w[0]["can_call_back"] != true || w[0]["waiting_since"] == nil ||
		!strings.Contains(w[0]["explanation"].(string), "Byblos") {
		t.Errorf("waiting group wrong: %v", w)
	}
	_ = waiting
	if a := runnersIn(dun.Own, "aboard"); len(a) != 1 || a[0]["ship_name"] != "Sea Swallow" || a[0]["ship_wanax"] != kassandra ||
		a[0]["boarded_at"] == nil || a[0]["lands_at"] == nil || a[0]["to_wanax"] != kassandra {
		t.Errorf("aboard group wrong: %v", a)
	}
	var orderItem map[string]any
	for _, it := range runnersIn(dun.Own, "on_the_way") {
		if it["kind"] == "order" {
			orderItem = it
		}
	}
	if orderItem == nil || orderItem["order_summary"] != "Your order: Set stance: hold" {
		t.Errorf("order runner summary wrong: %v", orderItem)
	}
	if r := runnersIn(dun.Own, "on_the_way"); len(r) != 2 || r[0]["arrives_at"] == nil {
		t.Errorf("on_the_way group wrong: %v", r)
	}
	if r := runnersIn(dun.Own, "homebound"); len(r) != 1 || r[0]["arrives_at"] == nil || r[0]["returning"] != true || r[0]["to_city"] != "Byblos" {
		t.Errorf("homebound group wrong: %v", r)
	}
	// Foreign, seen by Dunios: only Kassandra's runner standing in his port.
	if len(dun.Foreign) != 1 || dun.Foreign[0]["group"] != "in_your_city" || dun.Foreign[0]["sender_wanax"] != kassandra || dun.Foreign[0]["where"] != "Byblos" {
		t.Errorf("Dunios' foreign view wrong: %v", dun.Foreign)
	}
	// Foreign, seen by Kassandra: Dunios' runner aboard HER ship.
	ab := runnersIn(kass.Foreign, "aboard_your_ship")
	if len(kass.Foreign) != 1 || len(ab) != 1 || ab[0]["sender_wanax"] != dunios || ab[0]["ship_name"] != "Sea Swallow" || ab[0]["lands_at"] == nil {
		t.Errorf("Kassandra's aboard view wrong: %v", kass.Foreign)
	}

	// Integrity: nothing sealed ever reaches the other Wanax.
	for _, word := range []string{"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ORDER-CARRIER"} {
		if strings.Contains(kassBody, word) {
			t.Errorf("Dunios' %s content leaked to Kassandra: %s", word, kassBody)
		}
	}
	if strings.Contains(kassBody, "ORDER-") || strings.Contains(kassBody, "TRADE-") || strings.Contains(kassBody, "REPLY-") {
		t.Errorf("sealed payload leaked to ship owner: %s", kassBody)
	}
	for _, word := range []string{"ECHO", "FOXTROT", "ORDER-", "TRADE-", "REPLY-", "message_text", "trade_offer", "order_payload", "reply_text"} {
		if strings.Contains(dunBody, word) {
			t.Errorf("%q leaked to Dunios: %s", word, dunBody)
		}
	}
	_ = foreignWait
	_ = time.Now
}

// A delivered runner stands in the recipient's court until they answer or its
// stay runs out: the sender sees it under awaiting_answer — who, which city,
// since when, and when it heads home unanswered.
func TestRunners_AwaitingAnswer(t *testing.T) {
	f := setupNavalPlayerFixture(t)
	ctx := context.Background()
	mh := NewMessengerHandler(f.pool, nil, f.clk, nil)
	f.router.Get("/worlds/{worldID}/runners", mh.Runners)

	sfx := " " + uuid.NewString()[:6]
	kassandra := "Kassandra" + sfx
	if _, err := f.pool.Exec(ctx, `UPDATE players SET wanax_name=$2 WHERE id=$1`, f.counterpartyID, kassandra); err != nil {
		t.Fatal(err)
	}
	byblos := f.settlement(t, "Byblos", 0, f.initiatorID, true)
	ugarit := f.settlement(t, "Ugarit", 5, f.counterpartyID, true)
	for q := 1; q <= 4; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	code, resp := f.post(t, f.initiatorToken, "/worlds/"+f.worldID.String()+"/settlements/"+byblos.String()+"/messengers",
		map[string]any{"destination_id": ugarit.String(), "message": "LETTER-GOLF to Kassandra"})
	if code != 201 {
		t.Fatalf("send: %d %v", code, resp)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM messengers WHERE origin_id=$1 AND message_text='LETTER-GOLF to Kassandra'`, byblos).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered', passage_status=NULL, passage_port_id=NULL,
	        arrives_at=now()-interval '1 hour' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	// A stay-end already processed (an earlier stay) must not be read.
	if _, err := f.pool.Exec(ctx, `INSERT INTO scheduled_events (world_id, event_type, payload, process_after, due_tick, processed_at)
	        VALUES ($1,'MessengerStayEnd',jsonb_build_object('messenger_id',$2::text),now(),60,NULL),
	               ($1,'MessengerStayEnd',jsonb_build_object('messenger_id',$2::text),now(),1,now())`, f.worldID, id.String()); err != nil {
		t.Fatal(err)
	}

	dun, _ := f.getRunners(t, f.initiatorToken)
	a := runnersIn(dun.Own, "awaiting_answer")
	if len(a) != 1 || a[0]["to_city"] != "Ugarit" || a[0]["to_wanax"] != kassandra ||
		a[0]["waiting_since"] == nil || a[0]["arrives_at"] != nil || a[0]["can_call_back"] != false {
		t.Fatalf("awaiting_answer group wrong: %v (own=%v)", a, dun.Own)
	}
	anchor, err := tick.LoadAnchor(ctx, f.pool, f.worldID)
	if err != nil {
		t.Fatal(err)
	}
	want := anchor.WallAt(60 * 1000).UTC().Format(time.RFC3339)
	if got, _ := a[0]["leaves_at"].(string); got == "" || mustParse(t, got).UTC().Format(time.RFC3339) != want {
		t.Errorf("leaves_at = %v, want %s (tick 60)", a[0]["leaves_at"], want)
	}
	if len(runnersIn(dun.Own, "on_the_way")) != 0 {
		t.Errorf("a delivered runner is not on the way: %v", dun.Own)
	}

	kass, kassBody := f.getRunners(t, f.counterpartyToken)
	if len(kass.Foreign) != 1 || kass.Foreign[0]["waiting_for"] != "your_answer" || kass.Foreign[0]["where"] != "Ugarit" {
		t.Errorf("Kassandra's view of the runner in her court wrong: %v", kass.Foreign)
	}
	if len(kass.Own) != 0 || strings.Contains(kassBody, "leaves_at") {
		t.Errorf("Kassandra sees Dunios' runner as her own: %s", kassBody)
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return at
}

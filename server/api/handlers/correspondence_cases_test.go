package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func caseFixture(t *testing.T) *replyFixture {
	f := setupReplyFixture(t)
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	f.router.Get("/worlds/{worldID}/correspondence/cases", h.Cases)
	f.router.Get("/worlds/{worldID}/correspondence/cases/{caseID}", h.Case)
	f.router.Get("/worlds/{worldID}/messengers/inbox", h.Inbox)
	f.router.Get("/worlds/{worldID}/messengers/queue", h.Queue)
	f.router.Get("/worlds/{worldID}/settlements/{settlementID}/messengers", h.ListSent)
	f.router.Post("/worlds/{worldID}/messengers/{messengerID}/read", h.Read)
	return f
}
func caseGet(t *testing.T, f *replyFixture, token, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var v map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return rec.Code, v
}
func sendCase(t *testing.T, f *replyFixture, subject, caseID string) (string, string) {
	t.Helper()
	code, v := f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers", map[string]any{"destination_id": f.destID.String(), "message": "Sealed winter terms", "subject": subject, "case_id": caseID})
	if code != 201 {
		t.Fatalf("send: %d %v", code, v)
	}
	return v["id"].(string), v["case_id"].(string)
}
func caseItems(t *testing.T, v map[string]any) []any {
	t.Helper()
	a, ok := v["items"].([]any)
	if !ok {
		t.Fatalf("missing items: %v", v)
	}
	return a
}
func TestCorrespondenceCasePrivacySnapshotsAndDeletion(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	letter, caseID := sendCase(t, f, "Tin for winter", "")
	base := "/worlds/" + f.worldID.String()
	path := base + "/correspondence/cases/" + caseID
	if code, _ := caseGet(t, f, f.recvToken, path); code != 404 {
		t.Fatalf("outbound case leaked: %d", code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE players SET wanax_name='K2-'||id::text WHERE id=$1`, f.senderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	if _, err := events.NewStore(f.pool).Append(ctx, f.destID, events.StreamProvince, "MessengerArrived", map[string]any{"messenger_id": letter}, f.worldID, nil); err != nil {
		t.Fatal(err)
	}
	code, v := caseGet(t, f, f.recvToken, path)
	if code != 200 || len(caseItems(t, v)) != 1 {
		t.Fatalf("delivered: %d %v", code, v)
	}
	row := caseItems(t, v)[0].(map[string]any)
	oldOrigin := row["origin_name"]
	oldSender := row["sender_name"]
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='returning',reply_text='A private reply' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	_, v = caseGet(t, f, f.senderToken, path)
	if caseItems(t, v)[0].(map[string]any)["reply_text"] != nil {
		t.Fatalf("reply read before physical return: %v", v)
	}
	// A city successor inherits neither inbox nor old sent letters, but the
	// original participant keeps its private history even after both cities vanish.
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2,name='Renamed' WHERE id=$1`, f.destID, f.senderID); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", base+"/messengers/inbox?history=true", nil)
	req.Header.Set("Authorization", "Bearer "+f.senderToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "[]\n" {
		t.Fatalf("successor inbox leaked: %d %s", rec.Code, rec.Body.String())
	}
	if code, _ := f.post(t, f.senderToken, base+"/messengers/"+letter+"/read", map[string]any{}); code != 404 {
		t.Fatalf("successor read allowed: %d", code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlements WHERE id=$1 OR id=$2`, f.originID, f.destID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{f.senderToken, f.recvToken} {
		code, v = caseGet(t, f, token, path)
		if code != 200 {
			t.Fatalf("archive lost with city: %d %v", code, v)
		}
		row = caseItems(t, v)[0].(map[string]any)
		if row["origin_name"] != oldOrigin || row["sender_name"] != oldSender || row["reply_text"] != "A private reply" {
			t.Fatalf("snapshots changed/lost: %v", row)
		}
	}
}
func TestCorrespondenceChangedOwnerBeforeDeliveryGetsNoPrivateLetter(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	letter, caseID := sendCase(t, f, "Not delivered to a successor", "")
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, f.senderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	path := "/worlds/" + f.worldID.String() + "/correspondence/cases/" + caseID
	if code, _ := caseGet(t, f, f.recvToken, path); code != 404 {
		t.Fatalf("former owner got remote receipt: %d", code)
	}
	var delivered bool
	if err := f.pool.QueryRow(ctx, `SELECT delivered FROM correspondence_letters WHERE id=$1`, letter).Scan(&delivered); err != nil || delivered {
		t.Fatalf("unavailable recipient wrongly marked delivered: %v %v", delivered, err)
	}
	// Later ownership restoration does not backfill an earlier delivery.
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, f.receiverID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET trade_offer=NULL WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	if code, _ := caseGet(t, f, f.recvToken, path); code != 404 {
		t.Fatalf("unrelated outcome update backfilled receipt: %d", code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='returning' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	if code, _ := caseGet(t, f, f.recvToken, path); code != 404 {
		t.Fatalf("late ownership backfilled receipt: %d", code)
	}
}
func TestCorrespondenceSeparateCasesAndAtomicAppend(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	_, first := sendCase(t, f, "First matter", "")
	_, second := sendCase(t, f, "Second matter", "")
	if first == second {
		t.Fatal("independent matters merged")
	}
	_, same := sendCase(t, f, "ignored on append", first)
	if same != first {
		t.Fatal("append changed identity")
	}
	var count int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE world_id=$1`, f.worldID).Scan(&count)
	code, _ := f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers", map[string]any{"destination_id": f.destID.String(), "message": "Do not dispatch", "case_id": uuid.NewString()})
	if code != 400 {
		t.Fatalf("foreign case accepted: %d", code)
	}
	var after int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE world_id=$1`, f.worldID).Scan(&after)
	if count != after {
		t.Fatal("invalid case dispatched messenger")
	}
	base := "/worlds/" + f.worldID.String() + "/correspondence/cases"
	code, v := caseGet(t, f, f.senderToken, base+"?limit=1")
	if code != 200 || len(caseItems(t, v)) != 1 {
		t.Fatalf("page1: %d %v", code, v)
	}
	cursor := v["next_cursor"].(string)
	_, newID := sendCase(t, f, "Third matter", "")
	code, v = caseGet(t, f, f.senderToken, base+"?limit=1&cursor="+url.QueryEscape(cursor))
	if code != 200 || len(caseItems(t, v)) != 1 {
		t.Fatalf("page2: %d %v", code, v)
	}
	if caseItems(t, v)[0].(map[string]any)["id"] == newID {
		t.Fatal("new case slipped into page2")
	}
	_, v = caseGet(t, f, f.senderToken, base+"?q=Second")
	if len(caseItems(t, v)) != 1 || caseItems(t, v)[0].(map[string]any)["id"] != second {
		t.Fatalf("server search failed: %v", v)
	}
	if code, _ := caseGet(t, f, f.recvToken, base+"?limit=1&cursor="+url.QueryEscape(cursor)); code != 400 {
		t.Fatalf("cross-person cursor accepted: %d", code)
	}
}

func TestCorrespondenceCaseRejectsUnrelatedPersonAndWrongWorld(t *testing.T) {
	f := caseFixture(t)
	letter, caseID := sendCase(t, f, "Private matter", "")
	if _, err := f.pool.Exec(context.Background(), `UPDATE messengers SET status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(f.pool, "test-secret")
	token, _, err := svc.Register(context.Background(), "stranger-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	path := "/worlds/" + f.worldID.String() + "/correspondence/cases/" + caseID
	if code, v := caseGet(t, f, token, path); code != 404 {
		t.Fatalf("unrelated person read case: %d %v", code, v)
	}
	if code, v := caseGet(t, f, f.senderToken, "/worlds/"+uuid.NewString()+"/correspondence/cases/"+caseID); code != 404 {
		t.Fatalf("cross-world case read: %d %v", code, v)
	}
}

func TestCorrespondenceArchivePaginationFreezesCaseAndLetterMembership(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	_, first := sendCase(t, f, "Page first", "")
	sendCase(t, f, "Page second", "")
	sendCase(t, f, "Page third", "")
	equal := time.Now().Add(-time.Hour).Truncate(time.Second)
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_cases SET created_at=$2 WHERE world_id=$1`, f.worldID, equal); err != nil {
		t.Fatal(err)
	}
	base := "/worlds/" + f.worldID.String() + "/correspondence/cases"
	code, v := caseGet(t, f, f.senderToken, base+"?limit=1")
	if code != 200 {
		t.Fatalf("first case page: %d %v", code, v)
	}
	seen := map[string]bool{caseItems(t, v)[0].(map[string]any)["id"].(string): true}
	cursor := v["next_cursor"].(string)
	// Updating the latest activity of an existing case must not move its key.
	sendCase(t, f, "", first)
	_, late := sendCase(t, f, "Backdated new case", "")
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_cases SET created_at=$2 WHERE id=$1`, late, equal.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for cursor != "" {
		code, v = caseGet(t, f, f.senderToken, base+"?limit=1&cursor="+url.QueryEscape(cursor))
		if code != 200 {
			t.Fatalf("next case page: %d %v", code, v)
		}
		for _, item := range caseItems(t, v) {
			id := item.(map[string]any)["id"].(string)
			if seen[id] || id == late {
				t.Fatalf("duplicate/late case slipped into snapshot: %v", item)
			}
			seen[id] = true
		}
		cursor = v["next_cursor"].(string)
	}
	if len(seen) != 3 {
		t.Fatalf("missing equal-time cases: %v", seen)
	}
	// The detail uses a separate snapshot of immutable letter membership.
	sendCase(t, f, "", first)
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_letters SET sent_at=$2 WHERE case_id=$1`, first, equal); err != nil {
		t.Fatal(err)
	}
	path := base + "/" + first
	code, v = caseGet(t, f, f.senderToken, path+"?limit=1")
	if code != 200 {
		t.Fatalf("first letter page: %d %v", code, v)
	}
	seen = map[string]bool{caseItems(t, v)[0].(map[string]any)["id"].(string): true}
	cursor = v["next_cursor"].(string)
	newLetter, _ := sendCase(t, f, "", first)
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_letters SET sent_at=$2 WHERE id=$1`, newLetter, equal.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for cursor != "" {
		code, v = caseGet(t, f, f.senderToken, path+"?limit=1&cursor="+url.QueryEscape(cursor))
		if code != 200 {
			t.Fatalf("next letter page: %d %v", code, v)
		}
		for _, item := range caseItems(t, v) {
			id := item.(map[string]any)["id"].(string)
			if seen[id] || id == newLetter {
				t.Fatalf("duplicate/late letter slipped into snapshot: %v", item)
			}
			seen[id] = true
		}
		cursor = v["next_cursor"].(string)
	}
	if len(seen) != 3 {
		t.Fatalf("missing equal-time letters: %v", seen)
	}
}

func TestCorrespondenceArchiveCursorIsBoundToWorldPersonCaseSearchAndEpoch(t *testing.T) {
	f := caseFixture(t)
	_, first := sendCase(t, f, "Binding", "")
	sendCase(t, f, "Binding", first)
	_, second := sendCase(t, f, "Binding", "")
	base := "/worlds/" + f.worldID.String() + "/correspondence/cases"
	path := base + "/" + first
	_, v := caseGet(t, f, f.senderToken, path+"?limit=1&q=Binding")
	cursor := v["next_cursor"].(string)
	for _, x := range []struct{ token, path string }{
		{f.recvToken, path + "?q=Binding"},
		{f.senderToken, "/worlds/" + uuid.NewString() + "/correspondence/cases/" + first + "?q=Binding"},
		{f.senderToken, base + "/" + second + "?q=Binding"},
		{f.senderToken, path + "?q=Different"},
		{f.senderToken, base + "?q=Binding"},
	} {
		if code, res := caseGet(t, f, x.token, x.path+"&cursor="+url.QueryEscape(cursor)); code != 400 {
			t.Fatalf("mismatched cursor accepted: %d %v", code, res)
		}
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	var c archiveCursor
	if err = json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c.Epoch = c.Epoch.Add(-time.Hour)
	raw, _ = json.Marshal(c)
	if code, res := caseGet(t, f, f.senderToken, path+"?q=Binding&cursor="+base64.RawURLEncoding.EncodeToString(raw)); code != 400 {
		t.Fatalf("stale epoch accepted: %d %v", code, res)
	}
}

func TestCorrespondenceSearchDoesNotLeakUnreturnedTradeOutcome(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	letter, caseID := sendCase(t, f, "Terms", "")
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET trade_offer='{"status":"pending","want_good":"tin"}'::jsonb,status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET trade_offer=jsonb_set(trade_offer,'{status}','"accepted"'),status='returning' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	base := "/worlds/" + f.worldID.String() + "/correspondence/cases"
	_, v := caseGet(t, f, f.senderToken, base+"?q=accepted")
	if len(caseItems(t, v)) != 0 {
		t.Fatalf("search leaked unreturned acceptance: %v", v)
	}
	_, v = caseGet(t, f, f.recvToken, base+"?q=accepted")
	if len(caseItems(t, v)) != 1 {
		t.Fatalf("recipient lost known acceptance: %v", v)
	}
	_, v = caseGet(t, f, f.senderToken, base+"/"+caseID)
	row := caseItems(t, v)[0].(map[string]any)
	if row["trade_offer"].(map[string]any)["status"] != "pending" {
		t.Fatalf("detail leaked acceptance: %v", row)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	_, v = caseGet(t, f, f.senderToken, base+"?q=accepted")
	if len(caseItems(t, v)) != 1 {
		t.Fatalf("returned acceptance not found: %v", v)
	}
}

func TestCorrespondenceSenderRetainsOwnCancellationBeforeReturn(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	letter, caseID := sendCase(t, f, "Cancellation", "")
	// A cancellation is the sender's own registered action, not news from abroad.
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET trade_offer='{"status":"cancelled","want_good":"tin"}'::jsonb,status='returning' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	base := "/worlds/" + f.worldID.String() + "/correspondence/cases"
	_, v := caseGet(t, f, f.senderToken, base+"/"+caseID)
	row := caseItems(t, v)[0].(map[string]any)
	if row["trade_offer"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("own cancellation falsely pending: %v", row)
	}
	_, v = caseGet(t, f, f.senderToken, base+"?q=cancelled")
	if len(caseItems(t, v)) != 1 {
		t.Fatalf("own cancellation not searchable: %v", v)
	}
}

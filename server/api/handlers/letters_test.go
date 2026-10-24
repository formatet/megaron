package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func caseFixture(t *testing.T) *replyFixture {
	f := setupReplyFixture(t)
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	f.router.Get("/worlds/{worldID}/letters", h.Letters)
	f.router.Get("/worlds/{worldID}/letters/{playerID}", h.LetterThread)
	f.router.Get("/worlds/{worldID}/messengers/inbox", h.Inbox)
	return f
}
func caseGet(t *testing.T, f *replyFixture, token, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	v := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return rec.Code, v
}
func caseItems(t *testing.T, v map[string]any) []any {
	t.Helper()
	a, ok := v["items"].([]any)
	if !ok {
		t.Fatalf("missing items: %v", v)
	}
	return a
}
func sendCase(t *testing.T, f *replyFixture, subject, caseID string) (string, string) {
	t.Helper()
	code, v := f.post(t, f.senderToken, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers", map[string]any{"destination_id": f.destID.String(), "message": "First line\nLast words", "subject": subject, "case_id": caseID})
	if code != 201 {
		t.Fatalf("send: %d %v", code, v)
	}
	return v["id"].(string), v["case_id"].(string)
}
func memoryDeliver(t *testing.T, f *replyFixture, id string) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(), `UPDATE messengers SET status='delivered' WHERE id=$1`, id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLettersPhysicalPrivacyAndPersonGrouping(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	base := "/worlds/" + f.worldID.String() + "/letters"
	first, _ := sendCase(t, f, "First matter", "")
	second, _ := sendCase(t, f, "Different matter", "")
	code, v := caseGet(t, f, f.senderToken, base)
	if code != 200 || len(caseItems(t, v)) != 1 {
		t.Fatalf("person grouping: %d %v", code, v)
	}
	row := caseItems(t, v)[0].(map[string]any)
	if row["state"] != "waiting_them" || row["letter_count"] != float64(2) || row["preview"] != "Last words" {
		t.Fatalf("sender state: %v", row)
	}
	if code, v = caseGet(t, f, f.recvToken, base); code != 200 || len(caseItems(t, v)) != 0 {
		t.Fatalf("sealed letters leaked: %d %v", code, v)
	}
	if code, _ = caseGet(t, f, f.recvToken, base+"/"+f.senderID.String()); code != 404 {
		t.Fatalf("sealed thread: %d", code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE players SET wanax_name='King Alder' WHERE id=$1`, f.senderID); err != nil {
		t.Fatal(err)
	}
	memoryDeliver(t, f, first)
	memoryDeliver(t, f, second)
	code, v = caseGet(t, f, f.recvToken, base+"/"+f.senderID.String())
	if code != 200 || len(caseItems(t, v)) != 2 {
		t.Fatalf("thread: %d %v", code, v)
	}
	if v["state"] != "waiting_you" {
		t.Fatalf("recipient state: %v", v)
	}
	for _, x := range caseItems(t, v) {
		l := x.(map[string]any)
		for _, key := range []string{"origin_id", "origin_name", "from_id", "from_name", "destination_id", "destination_name", "subject", "case_id"} {
			if _, ok := l[key]; ok {
				t.Fatalf("geography/case exposed %s: %v", key, l)
			}
		}
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE id IN($1,$2) AND read_at IS NOT NULL`, first, second).Scan(&count); err != nil || count != 0 {
		t.Fatalf("listing marked read %d %v", count, err)
	}
	// Freeze recipient authority across conquest: the new city owner is not the addressee.
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.destID, f.senderID); err != nil {
		t.Fatal(err)
	}
	code, v = caseGet(t, f, f.senderToken, base)
	if code != 200 || caseItems(t, v)[0].(map[string]any)["counterpart_id"] != f.receiverID.String() {
		t.Fatalf("frozen recipient changed: %d %v", code, v)
	}
}

func TestLettersWaitingYouFirstNearestDeadlineAndState(t *testing.T) {
	now := time.Now()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	early := now.Add(time.Hour)
	late := now.Add(2 * time.Hour)
	letters := []personLetter{
		{counterpart: a, counterpartName: "Alder", Direction: "incoming", waiting: true, ReplyBefore: &late, SentAt: now, Message: "old"},
		{counterpart: b, counterpartName: "Berenice", Direction: "incoming", waiting: true, ReplyBefore: &early, SentAt: now.Add(time.Minute), Message: "soon"},
		{counterpart: c, counterpartName: "Cedar", Direction: "outgoing", waiting: true, SentAt: now.Add(2 * time.Minute), Message: "newest"},
	}
	people := summarizeLetters(letters)
	if len(people) != 3 || people[0].CounterpartID != b || people[1].CounterpartID != a || people[2].CounterpartID != c {
		t.Fatalf("waiting-on-you first, deadline ascending: %+v", people)
	}
	if people[0].State != "waiting_you" || people[1].State != "waiting_you" || people[2].State != "waiting_them" {
		t.Fatalf("state words: %+v", people)
	}
	letters[0].waiting = false
	letters[1].waiting = false
	letters[2].waiting = false
	for _, p := range summarizeLetters(letters) {
		if p.State != "done" {
			t.Fatalf("completed state: %+v", p)
		}
	}
}

func TestLettersLegacyHistoryAndNoRemoteAcceptance(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	id, _ := sendCase(t, f, "", "")
	if _, err := f.pool.Exec(ctx, `DELETE FROM correspondence_letters WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered',reply_text='secret reply',trade_offer='{"kind":"buy","want_good":"copper","want_qty":2,"offer_silver":5,"status":"accepted","secret":"cargo"}' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	base := "/worlds/" + f.worldID.String() + "/letters/"
	code, v := caseGet(t, f, f.senderToken, base+f.receiverID.String())
	if code != 200 {
		t.Fatalf("legacy thread: %d %v", code, v)
	}
	row := caseItems(t, v)[0].(map[string]any)
	offer := row["trade_offer"].(map[string]any)
	if offer["status"] != "pending" || offer["secret"] != nil || row["reply_text"] != nil || row["status"] != "outbound" {
		t.Fatalf("remote outcome leaked: %v", row)
	}
	code, v = caseGet(t, f, f.recvToken, base+f.senderID.String())
	if code != 200 || caseItems(t, v)[0].(map[string]any)["offer_state"] != "accepted" {
		t.Fatalf("legacy incoming omitted: %d %v", code, v)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, v = caseGet(t, f, f.senderToken, base+f.receiverID.String())
	row = caseItems(t, v)[0].(map[string]any)
	if row["reply_text"] != "secret reply" || row["trade_offer"].(map[string]any)["status"] != "accepted" {
		t.Fatalf("physical return missing: %v", row)
	}
}

func TestMailboxMigrationRetainsRecipientSubstrate(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	id, _ := sendCase(t, f, "", "")
	var retired, substrate bool
	if err := f.pool.QueryRow(ctx, `SELECT to_regclass('correspondence_private') IS NULL AND to_regclass('correspondence_citations') IS NULL,EXISTS(SELECT 1 FROM correspondence_letters WHERE id=$1) AND EXISTS(SELECT 1 FROM pg_proc WHERE proname='correspondence_recipient')`, id).Scan(&retired, &substrate); err != nil || !retired || !substrate {
		t.Fatalf("retired tables=%v frozen substrate=%v err=%v", retired, substrate, err)
	}
}

func TestLetterIdentityAndStrangerBoundary(t *testing.T) {
	f := caseFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE players SET wanax_name='Alder' WHERE id=$1`, f.senderID); err != nil {
		t.Fatal(err)
	}
	id, _ := sendCase(t, f, "", "")
	memoryDeliver(t, f, id)
	req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/messengers/inbox", nil)
	req.Header.Set("Authorization", "Bearer "+f.recvToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["from_name"] != "Alder" || rows[0]["from_id"] != f.senderID.String() {
		t.Fatalf("Inbox author identity: %d %s %v", rec.Code, rec.Body.String(), err)
	}
	base := "/worlds/" + f.worldID.String() + "/letters/"
	if code, _ := caseGet(t, f, f.recvToken, base+uuid.NewString()); code != 404 {
		t.Fatalf("stranger thread=%d", code)
	}
	if code, _ := caseGet(t, f, f.recvToken, "/worlds/"+uuid.NewString()+"/letters/"+f.senderID.String()); code != 404 {
		t.Fatalf("wrong world=%d", code)
	}
}

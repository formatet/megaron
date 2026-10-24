package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func memoryFixture(t *testing.T) *replyFixture {
	f := privateFixture(t)
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	f.router.Get("/worlds/{worldID}/correspondence/cases/{caseID}/memory", h.CaseMemory)
	f.router.Post("/worlds/{worldID}/correspondence/cases/{caseID}/citations", h.CreateCitation)
	f.router.Delete("/worlds/{worldID}/correspondence/cases/{caseID}/citations/{citationID}", h.DeleteCitation)
	return f
}
func memoryPath(f *replyFixture, id string) string {
	return "/worlds/" + f.worldID.String() + "/correspondence/cases/" + id + "/memory"
}
func citationPath(f *replyFixture, id string) string {
	return "/worlds/" + f.worldID.String() + "/correspondence/cases/" + id + "/citations"
}
func memoryRows(t *testing.T, v map[string]any, key string) []any {
	t.Helper()
	x, ok := v[key].([]any)
	if !ok {
		t.Fatalf("missing %s: %v", key, v)
	}
	return x
}
func memoryDeliver(t *testing.T, f *replyFixture, letter string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE messengers SET status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
}
func memoryAccept(t *testing.T, f *replyFixture, letter string) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `UPDATE messengers SET trade_offer='{"kind":"buy","status":"accepted","want_good":"tin","want_qty":12.5,"offer_silver":7}'::jsonb WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
}
func addCitation(t *testing.T, f *replyFixture, token, id, letter, source, quote string) map[string]any {
	t.Helper()
	code, v := f.post(t, token, citationPath(f, id), map[string]any{"letter_id": letter, "source": source, "quote": quote})
	if code != 201 {
		t.Fatalf("citation: %d %v", code, v)
	}
	return v
}
func memoryDelete(f *replyFixture, token, path string) int {
	req := httptest.NewRequest("DELETE", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec.Code
}

func TestCorrespondenceMemoryAcceptedTermsRequireVisibleAcceptance(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	first, id := sendCase(t, f, "First terms", "")
	second, secondCase := sendCase(t, f, "Another matter", "")
	memoryDeliver(t, f, first)
	memoryDeliver(t, f, second)
	memoryAccept(t, f, first)
	memoryAccept(t, f, second)
	_, v := caseGet(t, f, f.senderToken, memoryPath(f, id))
	if len(memoryRows(t, v, "transactions")) != 0 {
		t.Fatalf("sender learned remote acceptance before return: %v", v)
	}
	_, v = caseGet(t, f, f.recvToken, memoryPath(f, id))
	rows := memoryRows(t, v, "transactions")
	if len(rows) != 2 {
		t.Fatalf("accepted memory did not span same person cases: %v", v)
	}
	for _, item := range rows {
		row := item.(map[string]any)
		if row["outcome"] != "unavailable" || row["accepted_at"] == nil || row["terms"].(map[string]any)["want_qty"] != 12.5 {
			t.Fatalf("invented outcome/lost exact terms: %v", row)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	_, v = caseGet(t, f, f.senderToken, memoryPath(f, secondCase))
	if len(memoryRows(t, v, "transactions")) != 1 || memoryRows(t, v, "transactions")[0].(map[string]any)["id"] != first {
		t.Fatalf("return knowledge incorrect: %v", v)
	}
	// A free-text claim creates no registered accepted transaction.
	plain, _ := sendCase(t, f, "I claim that you accepted", id)
	memoryDeliver(t, f, plain)
	_, v = caseGet(t, f, f.recvToken, memoryPath(f, id))
	if len(memoryRows(t, v, "transactions")) != 2 {
		t.Fatalf("prose became accepted contract: %v", v)
	}
}

func TestCorrespondenceMemoryCitationsAreExactPrivateAndHistoricallyAttributed(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	letter, id := sendCase(t, f, "Quote origin", "")
	other, otherCase := sendCase(t, f, "Quote elsewhere", "")
	memoryDeliver(t, f, letter)
	memoryDeliver(t, f, other)
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET reply_text='Äkta <reply> & a promise',status='returning' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	// Sender cannot quote a reply that has not physically returned.
	if code, v := f.post(t, f.senderToken, citationPath(f, id), map[string]any{"letter_id": letter, "source": "reply", "quote": "<reply>"}); code != 404 {
		t.Fatalf("hidden reply citation allowed: %d %v", code, v)
	}
	own := addCitation(t, f, f.recvToken, otherCase, letter, "reply", "Äkta <reply> & a promise")
	if own["case_id"] != id || own["author_id"] != f.receiverID.String() {
		t.Fatalf("citation attribution is not actual source: %v", own)
	}
	repeated := addCitation(t, f, f.recvToken, otherCase, letter, "reply", "Äkta <reply> & a promise")
	if repeated["id"] != own["id"] {
		t.Fatal("retry created duplicate citation")
	}
	addCitation(t, f, f.senderToken, id, other, "message", "winter terms")
	_, v := caseGet(t, f, f.senderToken, memoryPath(f, id))
	if len(memoryRows(t, v, "citations")) != 1 || memoryRows(t, v, "citations")[0].(map[string]any)["source"] != "message" {
		t.Fatalf("counterpart citation leaked: %v", v)
	}
	_, v = caseGet(t, f, f.recvToken, memoryPath(f, otherCase))
	if len(memoryRows(t, v, "citations")) != 1 || memoryRows(t, v, "citations")[0].(map[string]any)["quote"] != "Äkta <reply> & a promise" {
		t.Fatalf("quote changed or missing cross-case citation: %v", v)
	}
	for _, token := range []string{f.senderToken, f.recvToken} {
		_, pub := caseGet(t, f, token, "/worlds/"+f.worldID.String()+"/correspondence/cases/"+id)
		raw, _ := json.Marshal(pub)
		if strings.Contains(string(raw), "citations") {
			t.Fatalf("case detail exports personal bookmarks: %s", raw)
		}
	}
	// Both personal memories survive city deletion and retain historical author.
	oldAuthor := own["author_name"]
	if _, err := f.pool.Exec(ctx, `UPDATE players SET wanax_name='renamed-k4-'||id::text WHERE id=$1`, f.receiverID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlements WHERE id=$1 OR id=$2`, f.originID, f.destID); err != nil {
		t.Fatal(err)
	}
	_, v = caseGet(t, f, f.recvToken, memoryPath(f, otherCase))
	row := memoryRows(t, v, "citations")[0].(map[string]any)
	if row["author_name"] != oldAuthor || row["quote"] != own["quote"] {
		t.Fatalf("city/person rename rewrote citation: %v", row)
	}
	path := citationPath(f, otherCase) + "/" + own["id"].(string)
	if code := memoryDelete(f, f.senderToken, path); code != 204 {
		t.Fatalf("idempotent foreign delete: %d", code)
	}
	_, v = caseGet(t, f, f.recvToken, memoryPath(f, id))
	if len(memoryRows(t, v, "citations")) != 1 {
		t.Fatal("counterpart deleted private citation")
	}
	for i := 0; i < 2; i++ {
		if code := memoryDelete(f, f.recvToken, path); code != 204 {
			t.Fatalf("own/retry delete: %d", code)
		}
	}
	_, v = caseGet(t, f, f.recvToken, memoryPath(f, id))
	if len(memoryRows(t, v, "citations")) != 0 {
		t.Fatal("own citation not deleted")
	}
}

func TestCorrespondenceMemoryRejectsUnauthorizedOrInexactCitation(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	letter, id := sendCase(t, f, "Protected source", "")
	if code, _ := caseGet(t, f, f.recvToken, memoryPath(f, id)); code != 404 {
		t.Fatalf("undelivered memory visible: %d", code)
	}
	if code, _ := f.post(t, f.recvToken, citationPath(f, id), map[string]any{"letter_id": letter, "source": "message", "quote": "Sealed"}); code != 404 {
		t.Fatalf("undelivered quote saved: %d", code)
	}
	memoryDeliver(t, f, letter)
	svc := auth.NewService(f.pool, "test-secret")
	stranger, _, err := svc.Register(ctx, "k4-stranger-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := caseGet(t, f, stranger, memoryPath(f, id)); code != 404 {
		t.Fatalf("stranger memory visible: %d", code)
	}
	for _, body := range []map[string]any{{"letter_id": letter, "source": "message", "quote": "Invented promise"}, {"letter_id": letter, "source": "message", "quote": "sealed"}, {"letter_id": letter, "source": "message", "quote": ""}, {"letter_id": letter, "source": "ambiguous", "quote": "Sealed"}, {"letter_id": letter, "source": "message", "quote": strings.Repeat("x", 1001)}} {
		if code, v := f.post(t, f.recvToken, citationPath(f, id), body); code != 400 {
			t.Fatalf("inexact quote accepted: %d %v", code, v)
		}
	}
	// A different pair of people is not this person's memory source.
	var foreignCase, foreignLetter uuid.UUID
	if err = f.pool.QueryRow(ctx, `INSERT INTO correspondence_cases(world_id,first_player_id,second_player_id,subject) VALUES($1,$2,$3,'Foreign person') RETURNING id`, f.worldID, f.receiverID, uuid.MustParse(strangerPlayerID(t, svc, stranger))).Scan(&foreignCase); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, `INSERT INTO correspondence_letters(id,case_id,sender_id,recipient_id,sender_name,recipient_name,origin_id,destination_id,origin_name,destination_name,message_text,sent_at,status,delivered) VALUES(gen_random_uuid(),$1,$2,$3,'Third','Recipient',$4,$5,'Foreign','Dest','Not this counterpart',now(),'delivered',true) RETURNING id`, foreignCase, uuid.MustParse(strangerPlayerID(t, svc, stranger)), f.receiverID, f.originID, f.destID).Scan(&foreignLetter); err != nil {
		t.Fatal(err)
	}
	if code, v := f.post(t, f.recvToken, citationPath(f, id), map[string]any{"letter_id": foreignLetter.String(), "source": "message", "quote": "counterpart"}); code != 404 {
		t.Fatalf("foreign-counterpart source accepted: %d %v", code, v)
	}
}
func strangerPlayerID(t *testing.T, svc *auth.Service, token string) string {
	t.Helper()
	c, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return c.PlayerID.String()
}

func TestCorrespondenceMemoryTransactionPaginationFreezesAcceptanceAndReturn(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	first, id := sendCase(t, f, "Page transactions", "")
	second, _ := sendCase(t, f, "", id)
	hidden, _ := sendCase(t, f, "", id)
	pending, _ := sendCase(t, f, "", id)
	for _, letter := range []string{first, second, hidden, pending} {
		memoryDeliver(t, f, letter)
	}
	for _, letter := range []string{first, second, hidden} {
		memoryAccept(t, f, letter)
	}
	for _, letter := range []string{first, second} {
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, letter); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	for letter, at := range map[string]time.Time{first: old, second: old, hidden: old.Add(-time.Hour), pending: old.Add(-2 * time.Hour)} {
		if _, err := f.pool.Exec(ctx, `UPDATE correspondence_letters SET sent_at=$2 WHERE id=$1`, letter, at); err != nil {
			t.Fatal(err)
		}
	}
	path := memoryPath(f, id)
	code, v := caseGet(t, f, f.senderToken, path+"?limit=1")
	if code != 200 {
		t.Fatalf("first page: %d %v", code, v)
	}
	seen := map[string]bool{}
	for _, x := range memoryRows(t, v, "transactions") {
		seen[x.(map[string]any)["id"].(string)] = true
	}
	cursor := v["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("two known transactions lost continuation")
	}
	memoryAccept(t, f, pending)
	for _, letter := range []string{hidden, pending} {
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, letter); err != nil {
			t.Fatal(err)
		}
	}
	for cursor != "" {
		code, v = caseGet(t, f, f.senderToken, path+"?limit=1&cursor="+url.QueryEscape(cursor))
		if code != 200 {
			t.Fatalf("transaction page: %d %v", code, v)
		}
		if len(memoryRows(t, v, "citations")) != 0 {
			t.Fatal("empty citation stream restarted")
		}
		for _, x := range memoryRows(t, v, "transactions") {
			key := x.(map[string]any)["id"].(string)
			if seen[key] || key == hidden || key == pending {
				t.Fatalf("newly known acceptance/return slipped into snapshot: %s", key)
			}
			seen[key] = true
		}
		cursor = v["next_cursor"].(string)
	}
	if len(seen) != 2 {
		t.Fatalf("transaction page lost original member: %v", seen)
	}
	_, v = caseGet(t, f, f.senderToken, path)
	if len(memoryRows(t, v, "transactions")) != 4 {
		t.Fatalf("new snapshot did not include now-known transactions: %v", v)
	}
}

func TestCorrespondenceMemoryCitationPaginationNeverRestartsExhaustedStream(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	letter, id := sendCase(t, f, "Page citations", "")
	path := memoryPath(f, id)
	for _, quote := range []string{"Sealed", "winter", "terms"} {
		addCitation(t, f, f.senderToken, id, letter, "message", quote)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_citations SET created_at=$2 WHERE case_id=$1`, id, old); err != nil {
		t.Fatal(err)
	}
	code, v := caseGet(t, f, f.senderToken, path+"?limit=1")
	if code != 200 {
		t.Fatalf("first citation page: %d %v", code, v)
	}
	seen := map[string]bool{}
	for _, x := range memoryRows(t, v, "citations") {
		seen[x.(map[string]any)["id"].(string)] = true
	}
	cursor := v["next_cursor"].(string)
	late := addCitation(t, f, f.senderToken, id, letter, "message", "Sealed winter")
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_citations SET created_at=$2 WHERE id=$1`, late["id"], old.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Retrying an existing save updates no immutable membership field.
	addCitation(t, f, f.senderToken, id, letter, "message", "terms")
	for cursor != "" {
		code, v = caseGet(t, f, f.senderToken, path+"?limit=1&cursor="+url.QueryEscape(cursor))
		if code != 200 {
			t.Fatalf("citation page: %d %v", code, v)
		}
		if len(memoryRows(t, v, "transactions")) != 0 {
			t.Fatal("empty transaction stream restarted")
		}
		for _, x := range memoryRows(t, v, "citations") {
			key := x.(map[string]any)["id"].(string)
			if seen[key] || key == late["id"] {
				t.Fatalf("duplicate/late citation slipped into snapshot: %v", x)
			}
			seen[key] = true
		}
		cursor = v["next_cursor"].(string)
	}
	if len(seen) != 3 {
		t.Fatalf("citation snapshot lost equal-time member: %v", seen)
	}
}

func TestCorrespondenceMemoryMixedCursorBindsContextAndExhaustsBoth(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	letter, id := sendCase(t, f, "Mixed memory", "")
	second, otherCase := sendCase(t, f, "Another linked case", "")
	for _, l := range []string{letter, second} {
		memoryDeliver(t, f, l)
		memoryAccept(t, f, l)
		if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='arrived' WHERE id=$1`, l); err != nil {
			t.Fatal(err)
		}
	}
	for _, quote := range []string{"Sealed", "winter", "terms"} {
		addCitation(t, f, f.senderToken, id, letter, "message", quote)
	}
	path := memoryPath(f, id)
	_, v := caseGet(t, f, f.senderToken, path+"?limit=1")
	cursor := v["next_cursor"].(string)
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	var original memoryCursor
	if err = json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*memoryCursor){func(c *memoryCursor) { c.World = uuid.New() }, func(c *memoryCursor) { c.Player = f.receiverID }, func(c *memoryCursor) { c.Case = uuid.MustParse(otherCase) }, func(c *memoryCursor) { c.Counterpart = uuid.New() }, func(c *memoryCursor) { c.Epoch = c.Epoch.Add(-time.Hour) }, func(c *memoryCursor) { c.Snapshot = "not-a-snapshot" }, func(c *memoryCursor) { c.Version = 2 }} {
		changed := original
		change(&changed)
		b, _ := json.Marshal(changed)
		if code, res := caseGet(t, f, f.senderToken, path+"?cursor="+base64.RawURLEncoding.EncodeToString(b)); code != 400 {
			t.Fatalf("foreign cursor accepted: %d %v", code, res)
		}
	}
	if code, _ := caseGet(t, f, f.senderToken, path+"?cursor="+url.QueryEscape(cursor)+"&q=changed"); code != 400 {
		t.Fatalf("query changed accepted: %d", code)
	}
	txs, cites := 1, 1
	for cursor != "" {
		code, res := caseGet(t, f, f.senderToken, path+"?limit=1&cursor="+url.QueryEscape(cursor))
		if code != 200 {
			t.Fatalf("mixed page: %d %v", code, res)
		}
		txs += len(memoryRows(t, res, "transactions"))
		cites += len(memoryRows(t, res, "citations"))
		if txs > 2 || cites > 3 {
			t.Fatalf("exhausted stream restarted: tx=%d cite=%d", txs, cites)
		}
		cursor = res["next_cursor"].(string)
	}
	if txs != 2 || cites != 3 {
		t.Fatalf("mixed missing members: tx=%d cite=%d", txs, cites)
	}
}

func TestCorrespondenceMemoryRecipientPaginationFreezesAcceptance(t *testing.T) {
	f := memoryFixture(t)
	ctx := context.Background()
	first, id := sendCase(t, f, "Receipt snapshot", "")
	second, _ := sendCase(t, f, "", id)
	late, _ := sendCase(t, f, "", id)
	for _, l := range []string{first, second, late} {
		memoryDeliver(t, f, l)
	}
	memoryAccept(t, f, first)
	memoryAccept(t, f, second)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if _, err := f.pool.Exec(ctx, `UPDATE correspondence_letters SET sent_at=$2 WHERE id=$1`, late, old); err != nil {
		t.Fatal(err)
	}
	path := memoryPath(f, id)
	_, v := caseGet(t, f, f.recvToken, path+"?limit=1")
	cursor := v["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("missing continuation")
	}
	count := 1
	memoryAccept(t, f, late)
	for cursor != "" {
		code, res := caseGet(t, f, f.recvToken, path+"?limit=1&cursor="+url.QueryEscape(cursor))
		if code != 200 {
			t.Fatalf("recipient page: %d %v", code, res)
		}
		for _, item := range memoryRows(t, res, "transactions") {
			if item.(map[string]any)["id"] == late {
				t.Fatalf("later accepted offer slipped into recipient snapshot: %v", item)
			}
			count++
		}
		cursor = res["next_cursor"].(string)
	}
	if count != 2 {
		t.Fatalf("wrong recipient membership: %d", count)
	}
}

func TestCorrespondenceMemoryHistoricalAcceptanceTimeIsExplicitlyUnknown(t *testing.T) {
	f := memoryFixture(t)
	letter, id := sendCase(t, f, "Older accepted record", "")
	memoryDeliver(t, f, letter)
	memoryAccept(t, f, letter)
	// A migrated pre209 acceptance has a proven status/membership, but no recorded
	// original acceptance wall time. Never substitute send time or migration time.
	if _, err := f.pool.Exec(context.Background(), `UPDATE correspondence_letters SET accepted_at=NULL WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	_, v := caseGet(t, f, f.recvToken, memoryPath(f, id))
	row := memoryRows(t, v, "transactions")[0].(map[string]any)
	if row["accepted_at"] != nil {
		t.Fatalf("invented historical acceptance time: %v", row)
	}
}

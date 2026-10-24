package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func privateFixture(t *testing.T) *replyFixture {
	f := caseFixture(t)
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	f.router.Get("/worlds/{worldID}/correspondence/cases/{caseID}/private", h.CasePrivate)
	f.router.Put("/worlds/{worldID}/correspondence/cases/{caseID}/private", h.SaveCasePrivate)
	return f
}
func privatePut(f *replyFixture, token, path string, body any) (int, map[string]any) {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", path, strings.NewReader(string(data)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var result map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	return rec.Code, result
}
func privatePath(f *replyFixture, id string) string {
	return "/worlds/" + f.worldID.String() + "/correspondence/cases/" + id + "/private"
}
func privateBody(draft, note string, version int64) map[string]any {
	return map[string]any{"draft": draft, "note": note, "expected_version": version}
}

func TestCorrespondencePrivatePersistsOnlyForOriginalPerson(t *testing.T) {
	f := privateFixture(t)
	ctx := context.Background()
	letter, id := sendCase(t, f, "Private memory", "")
	path := privatePath(f, id)
	code, v := caseGet(t, f, f.senderToken, path)
	if code != 200 || v["version"] != float64(0) || v["draft"] != "" || v["note"] != "" {
		t.Fatalf("initial state: %d %v", code, v)
	}
	if code, _ = caseGet(t, f, f.recvToken, path); code != 404 {
		t.Fatalf("pre-delivery recipient read: %d", code)
	}
	code, v = privatePut(f, f.senderToken, path, privateBody("DRAFT-ONLY-SENDER", "NOTE-ONLY-SENDER", 0))
	if code != 200 || v["version"] != float64(1) {
		t.Fatalf("first save: %d %v", code, v)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	code, v = caseGet(t, f, f.recvToken, path)
	if code != 200 || v["version"] != float64(0) || v["note"] != "" {
		t.Fatalf("counterpart inherited private data: %d %v", code, v)
	}
	code, v = privatePut(f, f.recvToken, path, privateBody("RECIPIENT-DRAFT", "RECIPIENT-NOTE", 0))
	if code != 200 {
		t.Fatalf("recipient save: %d %v", code, v)
	}
	_, v = caseGet(t, f, f.senderToken, path)
	if v["draft"] != "DRAFT-ONLY-SENDER" || v["note"] != "NOTE-ONLY-SENDER" {
		t.Fatalf("recipient changed sender data: %v", v)
	}
	// Public case detail and list never export either party's private material.
	for _, token := range []string{f.senderToken, f.recvToken} {
		for _, suffix := range []string{"", "/" + id} {
			code, pub := caseGet(t, f, token, "/worlds/"+f.worldID.String()+"/correspondence/cases"+suffix)
			raw, _ := json.Marshal(pub)
			if code != 200 || strings.Contains(string(raw), "ONLY-SENDER") || strings.Contains(string(raw), "RECIPIENT-DRAFT") || strings.Contains(string(raw), "RECIPIENT-NOTE") {
				t.Fatalf("private state leaked through case: %d %s", code, raw)
			}
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.originID, f.receiverID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM settlements WHERE id=$1 OR id=$2`, f.originID, f.destID); err != nil {
		t.Fatal(err)
	}
	_, v = caseGet(t, f, f.senderToken, path)
	if v["draft"] != "DRAFT-ONLY-SENDER" || v["version"] != float64(1) {
		t.Fatalf("city loss/deletion lost person memory: %v", v)
	}
	code, v = privatePut(f, f.senderToken, path, privateBody("Historical draft", "Historical note", 1))
	if code != 200 || v["version"] != float64(2) {
		t.Fatalf("historical private update: %d %v", code, v)
	}
}

func TestCorrespondencePrivateStaleSaveAndConcurrentFirstSave(t *testing.T) {
	f := privateFixture(t)
	_, id := sendCase(t, f, "Versioned", "")
	path := privatePath(f, id)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := privatePut(f, f.senderToken, path, privateBody(fmt.Sprintf("writer-%d", i), "", 0))
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		if code == 200 {
			success++
		}
		if code == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent first-save statuses: success=%d conflict=%d", success, conflict)
	}
	code, v := privatePut(f, f.senderToken, path, privateBody("newest", "retain newest", 1))
	if code != 200 || v["version"] != float64(2) {
		t.Fatalf("second save: %d %v", code, v)
	}
	for _, old := range []int64{0, 1} {
		code, v = privatePut(f, f.senderToken, path, privateBody("stale", "overwrite", old))
		if code != 409 || v["current_version"] != float64(2) {
			t.Fatalf("stale save accepted: %d %v", code, v)
		}
	}
	_, v = caseGet(t, f, f.senderToken, path)
	if v["draft"] != "newest" || v["note"] != "retain newest" || v["version"] != float64(2) {
		t.Fatalf("stale tab overwrote current data: %v", v)
	}
}

func TestCorrespondencePrivateRejectsStrangerWrongWorldAndInvisibleCase(t *testing.T) {
	f := privateFixture(t)
	ctx := context.Background()
	letter, id := sendCase(t, f, "Private access", "")
	path := privatePath(f, id)
	if code, _ := privatePut(f, f.recvToken, path, privateBody("premature", "", 0)); code != 404 {
		t.Fatalf("undelivered recipient wrote: %d", code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET status='delivered' WHERE id=$1`, letter); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(f.pool, "test-secret")
	stranger, _, err := svc.Register(ctx, "k3-stranger-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	paths := []struct{ token, path string }{{stranger, path}, {f.senderToken, "/worlds/" + uuid.NewString() + "/correspondence/cases/" + id + "/private"}, {f.senderToken, privatePath(f, uuid.NewString())}}
	for _, p := range paths {
		if code, v := caseGet(t, f, p.token, p.path); code != 404 {
			t.Fatalf("unauthorized private read: %d %v", code, v)
		}
		if code, v := privatePut(f, p.token, p.path, privateBody("unauthorized", "", 0)); code != 404 {
			t.Fatalf("unauthorized private save: %d %v", code, v)
		}
	}
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM correspondence_private WHERE case_id=$1`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unauthorized save persisted: %d %v", count, err)
	}
}

func TestCorrespondencePrivateValidationAndIndependentCases(t *testing.T) {
	f := privateFixture(t)
	ctx := context.Background()
	_, first := sendCase(t, f, "First", "")
	_, second := sendCase(t, f, "Second", "")
	path := privatePath(f, first)
	for _, body := range []any{
		map[string]any{"draft": "a", "note": "b"}, privateBody("a", "b", -1), privateBody(strings.Repeat("å", 501), "", 0), privateBody("", strings.Repeat("x", 10001), 0), map[string]any{"draft": nil, "note": "b", "expected_version": 0}, map[string]any{"draft": "a", "note": "b", "expected_version": 0, "player_id": f.receiverID.String()},
	} {
		if code, v := privatePut(f, f.senderToken, path, body); code != 400 {
			t.Fatalf("invalid state accepted: %d %v", code, v)
		}
	}
	if code, _ := privatePut(f, f.senderToken, path, privateBody(strings.Repeat("å", 500), strings.Repeat("b", 10000), 0)); code != 200 {
		t.Fatalf("valid byte boundary rejected: %d", code)
	}
	_, v := caseGet(t, f, f.senderToken, privatePath(f, second))
	if v["version"] != float64(0) || v["draft"] != "" || v["note"] != "" {
		t.Fatalf("private state crossed cases: %v", v)
	}
	var cases, bearers, eventsBefore, notificationsBefore int
	for query, target := range map[string]*int{`SELECT count(*) FROM correspondence_cases WHERE world_id=$1`: &cases, `SELECT count(*) FROM messengers WHERE world_id=$1`: &bearers, `SELECT count(*) FROM events WHERE world_id=$1`: &eventsBefore, `SELECT count(*) FROM notifications WHERE world_id=$1`: &notificationsBefore} {
		if err := f.pool.QueryRow(ctx, query, f.worldID).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := privatePut(f, f.senderToken, privatePath(f, second), privateBody("UNSENT", "PRIVATE", 0)); code != 200 {
		t.Fatalf("save failed: %d", code)
	}
	for query, before := range map[string]int{`SELECT count(*) FROM correspondence_cases WHERE world_id=$1`: cases, `SELECT count(*) FROM messengers WHERE world_id=$1`: bearers, `SELECT count(*) FROM events WHERE world_id=$1`: eventsBefore, `SELECT count(*) FROM notifications WHERE world_id=$1`: notificationsBefore} {
		var after int
		if err := f.pool.QueryRow(ctx, query, f.worldID).Scan(&after); err != nil || after != before {
			t.Fatalf("private save created shared side effect: %s %d→%d %v", query, before, after, err)
		}
	}
}

func TestCorrespondencePrivateConcurrentExistingSaveHasOneWinner(t *testing.T) {
	f := privateFixture(t)
	_, id := sendCase(t, f, "Concurrent existing", "")
	path := privatePath(f, id)
	if code, v := privatePut(f, f.senderToken, path, privateBody("initial", "", 0)); code != 200 {
		t.Fatalf("initial save: %d %v", code, v)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := privatePut(f, f.senderToken, path, privateBody(fmt.Sprintf("existing-%d", i), "", 1))
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		if code == 200 {
			success++
		}
		if code == 409 {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent existing statuses: success=%d conflict=%d", success, conflict)
	}
	_, v := caseGet(t, f, f.senderToken, path)
	if v["version"] != float64(2) {
		t.Fatalf("concurrent version advanced twice: %v", v)
	}
}

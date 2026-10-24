package handlers

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestCorrespondenceExactSourceLetterIsReachableBeyondFirstPage(t *testing.T) {
	f := caseFixture(t)
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	f.router.Get("/worlds/{worldID}/correspondence/cases/{caseID}/letters/{letterID}", h.Case)
	first, id := sendCase(t, f, "Long correspondence", "")
	var hidden string
	for i := 0; i < 31; i++ {
		hidden, _ = sendCase(t, f, "", id)
	}
	base := "/worlds/" + f.worldID.String() + "/correspondence/cases/" + id
	code, v := caseGet(t, f, f.senderToken, base)
	if code != 200 || len(caseItems(t, v)) != 30 {
		t.Fatalf("normal first page: %d %v", code, v)
	}
	for _, x := range caseItems(t, v) {
		if x.(map[string]any)["id"] == first {
			t.Fatal("old source unexpectedly in first page")
		}
	}
	exact := base + "/letters/" + first
	code, v = caseGet(t, f, f.senderToken, exact)
	if code != 200 || len(caseItems(t, v)) != 1 || caseItems(t, v)[0].(map[string]any)["id"] != first || v["next_cursor"] != "" {
		t.Fatalf("exact old source not reached: %d %v", code, v)
	}
	if code, _ = caseGet(t, f, f.recvToken, exact); code != 404 {
		t.Fatalf("undelivered source visible to recipient: %d", code)
	}
	memoryDeliver(t, f, first)
	code, v = caseGet(t, f, f.recvToken, exact)
	if code != 200 || len(caseItems(t, v)) != 1 {
		t.Fatalf("visible delivered source unavailable: %d %v", code, v)
	}
	if code, _ = caseGet(t, f, f.recvToken, base+"/letters/"+hidden); code != 404 {
		t.Fatalf("undelivered exact letter leaked inside otherwise visible case: %d", code)
	}
	foreignLetter, otherCase := sendCase(t, f, "Another case", "")
	if code, _ = caseGet(t, f, f.senderToken, base+"/letters/"+foreignLetter); code != 404 {
		t.Fatalf("visible wrong-case letter leaked: %d", code)
	}
	if code, _ = caseGet(t, f, f.senderToken, "/worlds/"+f.worldID.String()+"/correspondence/cases/"+otherCase+"/letters/"+first); code != 404 {
		t.Fatalf("letter resolved outside requested case: %d", code)
	}
	svc := auth.NewService(f.pool, "test-secret")
	stranger, _, err := svc.Register(context.Background(), "source-stranger-"+uuid.NewString(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = caseGet(t, f, stranger, exact); code != 404 {
		t.Fatalf("stranger read historical source: %d", code)
	}
	for _, suffix := range []string{"/letters/not-a-uuid", "/letters/00000000-0000-0000-0000-000000000000", "/letters/" + first + "?cursor=", "/letters/" + first + "?cursor=forged"} {
		if code, _ = caseGet(t, f, f.senderToken, base+suffix); code != 400 {
			t.Fatalf("invalid exact-source request accepted: %s: %d", suffix, code)
		}
	}
	if code, _ = caseGet(t, f, f.senderToken, base+"/letters/"+uuid.NewString()); code != 404 {
		t.Fatalf("missing exact source is not404: %d", code)
	}
}

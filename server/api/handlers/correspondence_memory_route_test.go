package handlers

import (
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"net/http"
	"reflect"
	"testing"
	"time"
)

// Conditional handler binding lets the identical chi HTTP test compile on K3;
// an absent production handler fails as route404 rather than a compiler error.
func TestCorrespondenceMemoryHTTPRouteContract(t *testing.T) {
	f := privateFixture(t)
	_, id := sendCase(t, f, "Memory route", "")
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	method := reflect.ValueOf(h).MethodByName("CaseMemory")
	if method.IsValid() {
		f.router.Get("/worlds/{worldID}/correspondence/cases/{caseID}/memory", method.Interface().(func(http.ResponseWriter, *http.Request)))
	}
	code, v := caseGet(t, f, f.senderToken, "/worlds/"+f.worldID.String()+"/correspondence/cases/"+id+"/memory")
	if code != 200 || len(v["transactions"].([]any)) != 0 || len(v["citations"].([]any)) != 0 {
		t.Fatalf("memory endpoint status=%d, want200 empty streams: %v", code, v)
	}
}

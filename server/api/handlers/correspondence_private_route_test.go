package handlers

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// Reflection lets this exact HTTP contract test compile on K2 as well: a
// missing production handler remains an unmounted route and yields HTTP404,
// rather than a compiler error naming a symbol introduced by the fix.
func TestCorrespondencePrivateHTTPRouteContract(t *testing.T) {
	f := caseFixture(t)
	_, id := sendCase(t, f, "A saved private draft", "")
	clk := clock.NewTestClock(time.Now())
	h := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	method := reflect.ValueOf(h).MethodByName("CasePrivate")
	if method.IsValid() {
		f.router.Get("/worlds/{worldID}/correspondence/cases/{caseID}/private", method.Interface().(func(http.ResponseWriter, *http.Request)))
	}
	code, v := caseGet(t, f, f.senderToken, "/worlds/"+f.worldID.String()+"/correspondence/cases/"+id+"/private")
	if code != 200 || v["version"] != float64(0) {
		t.Fatalf("private draft endpoint status=%d, want200 with initial version0: %v", code, v)
	}
}

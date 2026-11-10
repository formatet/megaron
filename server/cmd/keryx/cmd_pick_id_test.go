package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const (
	idA = "3f2a1b9c-0000-4000-8000-000000000001"
	idB = "3f2a77aa-0000-4000-8000-000000000002"
	idC = "9c0de000-0000-4000-8000-000000000003"
)

func offerMsg(id, from string, qty float64) map[string]any {
	return map[string]any{"id": id, "from_name": from, "message": "deal?", "status": "delivered",
		"trade_offer": map[string]any{"kind": "buy", "status": "pending", "want_good": "grain", "want_qty": qty, "offer_silver": 80.0}}
}

// fake serves the inbox and records every POST/DELETE path.
func fakeWorld(t *testing.T, routes map[string]any) *[]string {
	t.Helper()
	var calls []string
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
			json.NewEncoder(w).Encode(map[string]any{"kind": "buy", "quantity": 1, "good_key": "grain", "silver_paid": 1, "status": "cancelled", "cancelled": "farm"})
			return
		}
		for suffix, body := range routes {
			if strings.HasSuffix(r.URL.Path, suffix) {
				json.NewEncoder(w).Encode(body)
				return
			}
		}
		http.NotFound(w, r)
	})
	return &calls
}

func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	cmd.SetArgs(args)
	return captureStdout(t, cmd.Execute)
}

func TestTradeAcceptNoIdOneTwoNone(t *testing.T) {
	t.Run("one candidate acts and says which", func(t *testing.T) {
		calls := fakeWorld(t, map[string]any{"/messengers/inbox": []any{offerMsg(idA, "Myrine", 20)}})
		out, err := runCmd(t, tradeAcceptCmd())
		if err != nil {
			t.Fatal(err)
		}
		if len(*calls) != 1 || !strings.HasSuffix((*calls)[0], "/messengers/"+idA+"/trade-accept") {
			t.Fatalf("calls=%v", *calls)
		}
		if !strings.Contains(out, "20 grain for 80 silver from Myrine") {
			t.Fatalf("output should name the offer: %s", out)
		}
	})
	t.Run("two candidates print ready command lines and do nothing", func(t *testing.T) {
		calls := fakeWorld(t, map[string]any{"/messengers/inbox": []any{offerMsg(idA, "Myrine", 20), offerMsg(idC, "Korinth", 5)}})
		_, err := runCmd(t, tradeAcceptCmd())
		if err == nil {
			t.Fatal("must not guess between two")
		}
		for _, want := range []string{"keryx trade-accept --id 3f2a1b9c  # 20 grain for 80 silver from Myrine", "keryx trade-accept --id 9c0de000  # 5 grain for 80 silver from Korinth"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("missing %q in %v", want, err)
			}
		}
		if len(*calls) != 0 {
			t.Fatalf("nothing may be sent: %v", *calls)
		}
	})
	t.Run("none says so", func(t *testing.T) {
		fakeWorld(t, map[string]any{"/messengers/inbox": []any{}})
		if _, err := runCmd(t, tradeAcceptCmd()); err == nil || !strings.Contains(err.Error(), "no trade offer to trade-accept") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("full id still works without reading the inbox", func(t *testing.T) {
		calls := fakeWorld(t, map[string]any{})
		if _, err := runCmd(t, tradeAcceptCmd(), "--id", idB); err != nil {
			t.Fatal(err)
		}
		if len(*calls) != 1 || !strings.Contains((*calls)[0], idB) {
			t.Fatalf("calls=%v", *calls)
		}
	})
	t.Run("unique prefix expands, ambiguous prefix lists", func(t *testing.T) {
		calls := fakeWorld(t, map[string]any{"/messengers/inbox": []any{offerMsg(idA, "Myrine", 20), offerMsg(idB, "Knossos", 7), offerMsg(idC, "Korinth", 5)}})
		if _, err := runCmd(t, tradeAcceptCmd(), "--id", "9c0d"); err != nil {
			t.Fatal(err)
		}
		if len(*calls) != 1 || !strings.Contains((*calls)[0], idC) {
			t.Fatalf("prefix should expand: %v", *calls)
		}
		*calls = nil
		_, err := runCmd(t, tradeAcceptCmd(), "--id", "3f2a")
		if err == nil || !strings.Contains(err.Error(), idA) && !strings.Contains(err.Error(), "3f2a1b9c") || len(*calls) != 0 {
			t.Fatalf("ambiguous prefix must list and not act: %v %v", err, *calls)
		}
	})
}

func TestReplyNoIdUsesTheOnlyOpenLetter(t *testing.T) {
	letter := map[string]any{"id": idA, "from_name": "Myrine", "message": "greetings"}
	calls := fakeWorld(t, map[string]any{"/messengers/inbox": []any{letter, offerMsg(idB, "Knossos", 1)}})
	out, err := runCmd(t, replyCmd(), "--text", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || !strings.HasSuffix((*calls)[0], idA+"/reply") || !strings.Contains(out, "greetings") {
		t.Fatalf("calls=%v out=%s", *calls, out)
	}
	fakeWorld(t, map[string]any{"/messengers/inbox": []any{letter, map[string]any{"id": idC, "from_name": "X", "message": "second"}}})
	_, err = runCmd(t, replyCmd(), "--text", "hello")
	if err == nil || !strings.Contains(err.Error(), `keryx reply --id 3f2a1b9c --text "..."`) {
		t.Fatalf("two letters should list ready lines: %v", err)
	}
}

func TestRouteVerbsNoId(t *testing.T) {
	orders := []any{
		map[string]any{"id": idA, "from_name": "Phaistos", "to_name": "Kommos", "status": "active"},
		map[string]any{"id": idB, "from_name": "Phaistos", "to_name": "Tylissos", "status": "paused"},
	}
	calls := fakeWorld(t, map[string]any{"/standing-orders": orders})
	if _, err := runCmd(t, routePauseCmd()); err != nil || !strings.HasSuffix((*calls)[0], idA+"/pause") {
		t.Fatalf("pause: only one active order: %v %v", err, *calls)
	}
	*calls = nil
	if _, err := runCmd(t, routeResumeCmd()); err != nil || !strings.HasSuffix((*calls)[0], idB+"/resume") {
		t.Fatalf("resume: only one paused order: %v %v", err, *calls)
	}
	*calls = nil
	if _, err := runCmd(t, routeDeleteCmd()); err == nil || len(*calls) != 0 || !strings.Contains(err.Error(), "keryx route-delete --id") {
		t.Fatalf("delete with two orders must list: %v %v", err, *calls)
	}
}

func TestOutboxVerbsNoId(t *testing.T) {
	waiting := map[string]any{"id": idA, "destination_name": "Rhodes", "status": "waiting", "passage_status": "awaiting_passage", "passage_port": "Kommos", "can_arrange_passage": true, "can_call_back": true}
	pending := map[string]any{"id": idB, "destination_name": "Myrine", "trade_offer": map[string]any{"kind": "buy", "status": "pending", "want_good": "grain", "want_qty": 20.0, "offer_silver": 80.0}}
	fakeWorldOutbox := func() *[]string {
		return fakeWorld(t, map[string]any{
			"/provinces":                 []any{map[string]any{"own": true, "settlement_id": "s1", "id": "p1"}},
			"/settlements/s1/messengers": []any{waiting, pending},
			"/founding/messengers":       []any{},
		})
	}
	calls := fakeWorldOutbox()
	if _, err := runCmd(t, callBackCmd()); err != nil || !strings.HasSuffix((*calls)[0], idA+"/call-back") {
		t.Fatalf("call-back: %v %v", err, *calls)
	}
	*calls = nil
	if _, err := runCmd(t, tradeCancelCmd()); err != nil || !strings.HasSuffix((*calls)[0], idB+"/trade-cancel") {
		t.Fatalf("trade-cancel: %v %v", err, *calls)
	}
}

func TestCancelBuildNoId(t *testing.T) {
	prov := func(name string, queue ...any) map[string]any {
		return map[string]any{"settlement": map[string]any{"name": name, "build_queue": queue}}
	}
	item := map[string]any{"id": idA, "type": "farm", "hex_q": 12.0, "hex_r": 21.0}
	calls := fakeWorld(t, map[string]any{
		"/provinces":    []any{map[string]any{"own": true, "id": "p1"}, map[string]any{"own": true, "id": "p2"}},
		"/provinces/p1": prov("Phaistos", item),
		"/provinces/p2": prov("Kommos"),
	})
	out, err := runCmd(t, cancelBuildCmd())
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 || !strings.Contains((*calls)[0], "/provinces/p1/build-queue/"+idA) || !strings.Contains(out, "Phaistos") {
		t.Fatalf("calls=%v out=%s", *calls, out)
	}
}

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

// dossierFake serves canned GET replies and records every request. Any
// method other than GET is answered 405 and remembered, so a test can prove
// the dossier never writes (and so never marks anything read).
type dossierFake struct {
	mu       sync.Mutex
	replies  map[string]string
	requests []string
	nonGET   []string
}

func (f *dossierFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	if r.Method != http.MethodGet {
		f.nonGET = append(f.nonGET, r.Method+" "+r.URL.Path)
		http.Error(w, `{"error":"read-only test server"}`, http.StatusMethodNotAllowed)
		return
	}
	body, ok := f.replies[strings.TrimPrefix(r.URL.Path, "/api/v1/worlds/world-1")]
	if !ok {
		http.Error(w, `{"error":"no fixture"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, body)
}

func newDossierFake(t *testing.T, replies map[string]string) *dossierFake {
	t.Helper()
	f := &dossierFake{replies: replies}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	withTestConfig(t, srv.URL)
	return f
}

func notifJSON(id int, kind string, tick int, body string) string {
	return fmt.Sprintf(`{"id":"n%d","kind":%q,"level":1,"body":%s,"created_at":"2026-11-04T10:%02d:00Z","read_at":null,"world_tick":%d}`, id, kind, body, id%60, tick)
}

const dossierWorld = `{"name":"Alfa","tick_seconds":1800,"current_tick":100,"tick_anchor_at":"2026-11-04T10:00:00Z"}`

func fullDossierReplies(notifs string) map[string]string {
	return map[string]string{
		"/me":                             `{"wanax_name":"Minos"}`,
		"":                                dossierWorld,
		"/notifications":                  `{"notifications":[` + notifs + `],"unread":0}`,
		"/provinces":                      `[{"id":"p1","settlement_id":"aaaaaaaa-0001","name":"Aigina","q":7,"r":46,"own":true,"is_capital":true}]`,
		"/provinces/p1":                   `{"settlement":{"name":"Aigina","population":1005,"resources":{"silver":{"amount":120},"grain":{"amount":1194},"copper":{"amount":0}},"food":{"coverage_ticks":237.6,"food_net_per_tick":0.8},"build_queue":[],"temples":[{"name":"Poseidon","level":1,"staffed":2,"capacity":3}]}}`,
		"/provinces/p1/placement-options": `{"hexes":[],"buildings":[],"total_gubbar":10,"pool_size":4}`,
		"/provinces/p1/actions":           `[{"name":"build","category":"province","available":true},{"name":"march","category":"military","available":true},{"name":"gift","category":"trade","available":false}]`,
		"/units":                          `{"units":[{"id":"u1","type":"spearman","category":"land","size":100,"status":"garrison","display_name":"1st Spearmen of Aigina","settlement_id":"aaaaaaaa-0001"},{"id":"u2","type":"galley","category":"naval","size":1,"crew":20,"status":"garrison","display_name":"Potnia, Galley of Aigina","settlement_id":"aaaaaaaa-0001"}]}`,
		"/standing-orders":                `[]`,
		"/runners":                        `{"own":[],"foreign":[]}`,
		"/cities":                         `[{"settlement_id":"aaaaaaaa-0001","name":"Aigina","owner":"Minos","own":true,"coastal":true,"terrain":"plains","q":7,"r":46,"knowledge":"known"},{"settlement_id":"bbbbbbbb-0002","name":"Knossos","owner":"Idomeneus","own":false,"terrain":"hills","q":12,"r":40,"knowledge":"known","copper_deposit":true}]`,
		"/places":                         `{"places":[{"kind":"river","id":"9,44","name":"Kairatos","hexes":[{"q":9,"r":44}]}]}`,
		"/map":                            `[{"q":10,"r":44,"terrain":"hills","visible":true,"tin_deposit":true}]`,
		"/diplomacy":                      `[{"owner":"Idomeneus","kingdom":"","known_cities":1,"rumour_cities":0}]`,
		"/storms":                         `{"storms":[{"id":"abcdef123456","tier":"live","heading":"north","seen_tick":100,"hexes":[{"q":3,"r":50}]}],"tick":100}`,
	}
}

func emptyDossierReplies() map[string]string {
	return map[string]string{
		"":                 dossierWorld,
		"/me":              `{"wanax_name":"Minos"}`,
		"/notifications":   `{"notifications":[],"unread":0}`,
		"/provinces":       `[]`,
		"/units":           `{"units":[]}`,
		"/standing-orders": `[]`,
		"/runners":         `{"own":[],"foreign":[]}`,
		"/cities":          `[]`,
		"/places":          `{"places":[]}`,
		"/map":             `[]`,
		"/diplomacy":       `[]`,
		"/storms":          `{"storms":[],"tick":100}`,
	}
}

var dossierNow = time.Date(2026, 11, 7, 12, 30, 0, 0, keryxTZ)

// sectionBodies splits a dossier on "## " headings.
func sectionBodies(t *testing.T, text string) ([]string, map[string]string) {
	t.Helper()
	var order []string
	bodies := map[string]string{}
	var cur string
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "## ") {
			cur = strings.TrimPrefix(l, "## ")
			order = append(order, cur)
			continue
		}
		if cur != "" {
			bodies[cur] += l + "\n"
		}
	}
	return order, bodies
}

func TestDossierSectionsInFixedOrder(t *testing.T) {
	newDossierFake(t, fullDossierReplies(""))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	order, _ := sectionBodies(t, text)
	want := []string{"Failed orders", "Urgent", "Cities", "Forces and orders on the way", "Known places", "Contacts", "Storms at sea", "Since you last read", "What you can do now"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Fatalf("section order:\n got %v\nwant %v", order, want)
	}
}

func TestDossierHeader(t *testing.T) {
	newDossierFake(t, fullDossierReplies(""))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	for _, want := range []string{
		"# Dossier — Minos in Alfa",
		"Written 2026-11-07 12:30",
		"One game day = 30 minutes of wall-clock time.",
		"This is a snapshot. Write a new one each round; do not trust an old file.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("header lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "# Dossier") != 0 {
		t.Errorf("dossier must open with the title")
	}
}

func TestDossierEmptySectionsSayNone(t *testing.T) {
	newDossierFake(t, emptyDossierReplies())
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	order, bodies := sectionBodies(t, text)
	if len(order) != len(dossierSections) {
		t.Fatalf("every section must be written even when empty; got %v", order)
	}
	for _, name := range order {
		if !strings.HasPrefix(strings.TrimSpace(bodies[name]), "None.") {
			t.Errorf("empty section %q must start with \"None.\"; got %q", name, bodies[name])
		}
	}
}

func TestDossierOrderFailedOnlyInFailedOrders(t *testing.T) {
	notifs := strings.Join([]string{
		notifJSON(1, "OrderFailed", 99, `{"verb":"march","name":"1st Spearmen of Aigina","reason":"the road is blocked"}`),
		notifJSON(2, "BuildComplete", 98, `{"type":"farm"}`),
	}, ",")
	newDossierFake(t, fullDossierReplies(notifs))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	if !strings.Contains(bodies["Failed orders"], "OrderFailed") || !strings.Contains(bodies["Failed orders"], "the road is blocked") {
		t.Errorf("Failed orders lacks the failed order:\n%s", bodies["Failed orders"])
	}
	if strings.Contains(bodies["Since you last read"], "OrderFailed") {
		t.Errorf("OrderFailed must not repeat under Since you last read:\n%s", bodies["Since you last read"])
	}
	if !strings.Contains(bodies["Since you last read"], "BuildComplete") {
		t.Errorf("an ordinary unread notification belongs under Since you last read:\n%s", bodies["Since you last read"])
	}
}

func TestDossierUrgentKindsSeparated(t *testing.T) {
	notifs := notifJSON(1, "FoodRunningLow", 99, `{"name":"Aigina","days":3,"flows":{}}`)
	newDossierFake(t, fullDossierReplies(notifs))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	if !strings.Contains(bodies["Urgent"], "FoodRunningLow") {
		t.Errorf("Urgent lacks FoodRunningLow:\n%s", bodies["Urgent"])
	}
	if strings.Contains(bodies["Since you last read"], "FoodRunningLow") {
		t.Errorf("an urgent kind must not repeat under Since you last read")
	}
}

func TestDossierBattleAndSiegeAreUrgent(t *testing.T) {
	notifs := notifJSON(1, "BattleBegun", 99, `{"opponent_name":"Oledoledoff","place":"Tiryns","q":1,"r":2}`) + "," +
		notifJSON(2, "SiegeStarted", 99, `{"name":"Aigina","q":1,"r":2}`)
	newDossierFake(t, fullDossierReplies(notifs))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	for _, k := range []string{"BattleBegun", "SiegeStarted"} {
		if !strings.Contains(bodies["Urgent"], k) {
			t.Errorf("Urgent lacks %s:\n%s", k, bodies["Urgent"])
		}
	}
}

func TestDossierOnlyReadsAndNeverMarksRead(t *testing.T) {
	notifs := notifJSON(1, "BuildComplete", 98, `{"type":"farm"}`)
	f := newDossierFake(t, fullDossierReplies(notifs))
	buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	if len(f.nonGET) > 0 {
		t.Errorf("dossier sent a non-GET request: %v", f.nonGET)
	}
	for _, r := range f.requests {
		if strings.Contains(r, "read") && !strings.Contains(r, "already") {
			t.Errorf("dossier touched a read endpoint: %s", r)
		}
		if strings.Contains(r, "/admin/") {
			t.Errorf("dossier used an admin endpoint: %s", r)
		}
	}
}

func TestDossierSinceCapsAtThirty(t *testing.T) {
	var list []string
	for i := 1; i <= 35; i++ {
		list = append(list, notifJSON(i, "BuildComplete", 50+i, `{"type":"farm"}`))
	}
	newDossierFake(t, fullDossierReplies(strings.Join(list, ",")))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	rows := 0
	for _, l := range strings.Split(bodies["Since you last read"], "\n") {
		if strings.HasPrefix(l, "- ") {
			rows++
		}
	}
	if rows != 30 {
		t.Errorf("want 30 rows, got %d:\n%s", rows, bodies["Since you last read"])
	}
	if !strings.Contains(bodies["Since you last read"], "…and 5 more — `keryx notifications --unread`") {
		t.Errorf("missing overflow line:\n%s", bodies["Since you last read"])
	}
}

func TestDossierCityBlockAndContent(t *testing.T) {
	newDossierFake(t, fullDossierReplies(""))
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	for _, want := range []string{
		"### Aigina (capital)",
		"Stock: grain 1.2k, silver 120",
		"Food for 238 game days",
		"Citizens: 4 idle of 10",
		"Building: nothing queued.",
		"Temples: Poseidon level 1, staffed 2/3",
	} {
		if !strings.Contains(bodies["Cities"], want) {
			t.Errorf("Cities lacks %q:\n%s", want, bodies["Cities"])
		}
	}
	for _, want := range []string{"City Knossos — hills, deposit of copper — 6 hexes from Aigina", "river Kairatos", "Deposit of tin on hills at (10,44)"} {
		if !strings.Contains(bodies["Known places"], want) {
			t.Errorf("Known places lacks %q:\n%s", want, bodies["Known places"])
		}
	}
	if !strings.Contains(bodies["Contacts"], "Wanax Idomeneus") || !strings.Contains(bodies["Contacts"], "Knossos") {
		t.Errorf("Contacts lacks the Wanax and city:\n%s", bodies["Contacts"])
	}
	if !strings.Contains(bodies["Storms at sea"], "Storm in sight, drifting north") {
		t.Errorf("Storms lacks the live storm:\n%s", bodies["Storms at sea"])
	}
	if !strings.Contains(bodies["What you can do now"], "province: build · 0 locked") ||
		!strings.Contains(bodies["What you can do now"], "trade: none available · 1 locked") {
		t.Errorf("Actions short form wrong:\n%s", bodies["What you can do now"])
	}
	if !strings.Contains(bodies["Forces and orders on the way"], "1st Spearmen of Aigina — 100 men, garrison — in Aigina") {
		t.Errorf("Forces lacks a unit with a place name:\n%s", bodies["Forces and orders on the way"])
	}
}

func TestDossierStormsNeedShipOrCoast(t *testing.T) {
	replies := fullDossierReplies("")
	replies["/units"] = `{"units":[{"id":"u1","type":"spearman","category":"land","size":100,"status":"garrison","display_name":"1st Spearmen of Aigina","settlement_id":"aaaaaaaa-0001"}]}`
	replies["/cities"] = `[{"settlement_id":"aaaaaaaa-0001","name":"Aigina","owner":"Minos","own":true,"coastal":false,"terrain":"plains","q":7,"r":46}]`
	newDossierFake(t, replies)
	text := buildDossier(newClient(cfg), cfg.WorldID, dossierNow)
	_, bodies := sectionBodies(t, text)
	if !strings.HasPrefix(strings.TrimSpace(bodies["Storms at sea"]), "None.") {
		t.Errorf("a landlocked realm without ships gets no storms:\n%s", bodies["Storms at sea"])
	}
}

func TestDossierOutWritesFileAndPrintsPathAndSize(t *testing.T) {
	newDossierFake(t, fullDossierReplies(""))
	path := filepath.Join(t.TempDir(), "world.md")
	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := dossierCmd()
	cmd.SetArgs([]string{"--out", path})
	out, err := captureStdout(t, cmd.Execute)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "old content") || !strings.HasPrefix(string(data), "# Dossier — Minos in Alfa") {
		t.Errorf("the file must be overwritten with the dossier:\n%s", data)
	}
	if want := fmt.Sprintf("%s (%d bytes)\n", path, len(data)); out != want {
		t.Errorf("stdout must be only path and size:\n got %q\nwant %q", out, want)
	}
}

func TestDossierRejectsJSON(t *testing.T) {
	newDossierFake(t, fullDossierReplies(""))
	jsonMode = true
	cmd := dossierCmd()
	cmd.SetArgs(nil)
	_, err := captureStdout(t, cmd.Execute)
	if err == nil || !strings.Contains(err.Error(), "dossier is markdown; use the individual commands with --json") {
		t.Errorf("want the markdown-only error, got %v", err)
	}
}

func TestDossierIsRegisteredAndRootHasNoOtherFlags(t *testing.T) {
	root := newRootCommand()
	cmd, _, err := root.Find([]string{"dossier"})
	if err != nil || cmd.Name() != "dossier" {
		t.Fatalf("dossier not registered: %v", err)
	}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "out" && f.Name != "help" {
			t.Errorf("unexpected flag --%s", f.Name)
		}
	})
}

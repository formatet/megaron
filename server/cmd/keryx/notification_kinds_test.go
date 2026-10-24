package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type kindRef struct {
	expr ast.Expr
	file *kindFile
}
type kindFile struct {
	path    string
	parent  *kindFile
	symbols map[string][]kindRef
}
type kindFunction struct {
	file   *kindFile
	name   string
	params []string
}
type kindCall struct {
	file *kindFile
	call *ast.CallExpr
}

func notificationCallName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

// Follow literal/constant/local and forwarded kind arguments, including local
// dispatch closures. Unresolved producers fail instead of shrinking coverage.
func serverNotificationKinds(root string) (map[string][]string, error) {
	files := []*kindFile{}
	functions := []kindFunction{}
	calls := []kindCall{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || path == filepath.Join(root, "cmd", "keryx") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		node, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		f := &kindFile{path: path, symbols: map[string][]kindRef{}}
		files = append(files, f)
		var inspect func(ast.Node, *kindFile)
		inspect = func(node ast.Node, f *kindFile) {
			ast.Inspect(node, func(n ast.Node) bool {
				var fnName string
				var params *ast.FieldList
				var body *ast.BlockStmt
				switch n := n.(type) {
				case *ast.FuncDecl:
					fnName = n.Name.Name
					params = n.Type.Params
					body = n.Body
				case *ast.AssignStmt:
					if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
						if lit, ok := n.Rhs[0].(*ast.FuncLit); ok {
							if name, ok := n.Lhs[0].(*ast.Ident); ok {
								fnName = name.Name
								params = lit.Type.Params
								body = lit.Body
							}
						}
					}
				}
				if fnName != "" {
					scope := &kindFile{path: f.path + "#" + fnName, parent: f, symbols: map[string][]kindRef{}}
					files = append(files, scope)
					names := []string{}
					for _, p := range params.List {
						for _, n := range p.Names {
							names = append(names, n.Name)
						}
					}
					functions = append(functions, kindFunction{scope, fnName, names})
					if body != nil {
						inspect(body, scope)
					}
					return false
				}
				switch n := n.(type) {
				case *ast.ValueSpec:
					for i, name := range n.Names {
						if i < len(n.Values) {
							f.symbols[name.Name] = append(f.symbols[name.Name], kindRef{n.Values[i], f})
						}
					}
				case *ast.AssignStmt:
					for i, lhs := range n.Lhs {
						if name, ok := lhs.(*ast.Ident); ok && i < len(n.Rhs) {
							f.symbols[name.Name] = append(f.symbols[name.Name], kindRef{n.Rhs[i], f})
						}
					}
				case *ast.CallExpr:
					calls = append(calls, kindCall{f, n})
				}
				return true
			})
		}
		inspect(node, f)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, fn := range functions {
		for _, c := range calls {
			if notificationCallName(c.call.Fun) != fn.name {
				continue
			}
			for i, name := range fn.params {
				if i < len(c.call.Args) {
					fn.file.symbols[name] = append(fn.file.symbols[name], kindRef{c.call.Args[i], c.file})
				}
			}
		}
	}
	var resolve func(kindRef, map[string]bool) []string
	resolve = func(r kindRef, seen map[string]bool) []string {
		switch e := r.expr.(type) {
		case *ast.BasicLit:
			if e.Kind == token.STRING {
				s, _ := strconv.Unquote(e.Value)
				return []string{s}
			}
		case *ast.Ident:
			key := r.file.path + ":" + e.Name
			if seen[key] {
				return nil
			}
			seen[key] = true
			out := []string{}
			refs := r.file.symbols[e.Name]
			for p := r.file.parent; len(refs) == 0 && p != nil; p = p.parent {
				refs = p.symbols[e.Name]
			}
			if len(refs) == 0 {
				for _, f := range files {
					if f.parent == nil && filepath.Dir(f.path) == filepath.Dir(r.file.path) {
						refs = append(refs, f.symbols[e.Name]...)
					}
				}
			}
			for _, ref := range refs {
				out = append(out, resolve(ref, seen)...)
			}
			return out
		case *ast.SelectorExpr:
			key := "selector:" + e.Sel.Name
			if seen[key] {
				return nil
			}
			seen[key] = true
			out := []string{}
			for _, f := range files {
				if f.parent != nil {
					continue
				}
				for _, ref := range f.symbols[e.Sel.Name] {
					out = append(out, resolve(ref, seen)...)
				}
			}
			return out
		}
		return nil
	}
	kinds := map[string][]string{}
	kindPattern := regexp.MustCompile(`^(?:[A-Z][A-Za-z0-9]+|agora_ready)$`)
	add := func(f *kindFile, e ast.Expr) error {
		values := resolve(kindRef{e, f}, map[string]bool{})
		valid := false
		for _, v := range values {
			if !kindPattern.MatchString(v) {
				continue
			}
			valid = true
			kinds[v] = append(kinds[v], f.path)
		}
		if !valid {
			return fmt.Errorf("unresolved notification kind producer in %s (%T)", f.path, e)
		}
		return nil
	}
	sqlPattern := regexp.MustCompile(`(?is)INSERT\s+INTO\s+notifications\s*\(([^)]+)\)\s*VALUES\s*\(([^)]+)\)`)
	for _, c := range calls {
		name := notificationCallName(c.call.Fun)
		args := c.call.Args
		if name == "NotifyPlayer" && len(args) >= 6 {
			if err := add(c.file, args[3]); err != nil {
				return nil, err
			}
		}
		// DeliverCommittedNotification forwards an already archived row; its
		// producer is the INSERT inspected below, not outcome.kind.

		if (name == "QueryRow" || name == "Exec") && len(args) > 1 {
			lit, ok := args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			sql, _ := strconv.Unquote(lit.Value)
			m := sqlPattern.FindStringSubmatch(sql)
			if m == nil {
				continue
			}
			columns, values := strings.Split(m[1], ","), strings.Split(m[2], ",")
			for i, col := range columns {
				if strings.TrimSpace(col) != "kind" {
					continue
				}
				v := strings.TrimSpace(values[i])
				if strings.HasPrefix(v, "$") {
					pos, err := strconv.Atoi(v[1:])
					if err != nil || pos+1 >= len(args) {
						return nil, fmt.Errorf("unresolved SQL kind %s", c.file.path)
					}
					if err := add(c.file, args[pos+1]); err != nil {
						return nil, err
					}
				} else {
					kind := strings.Trim(v, "'")
					kinds[kind] = append(kinds[kind], c.file.path)
				}
			}
		}
	}
	return kinds, nil
}
func knownNotificationKinds(t *testing.T) []string {
	t.Helper()
	kinds, err := serverNotificationKinds("../..")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../../web/static/js/megaron/ui/format.js")
	if err != nil {
		t.Fatal(err)
	}
	// Same icon keys and four-space case arms as codex.test.mjs.
	s := string(src)
	icons := s[strings.Index(s, "export function notifIcon"):strings.Index(s, "return icons[kind]")]
	text := s[strings.Index(s, "export function notifText"):strings.Index(s, "// Mirrors keryx")]
	for _, m := range regexp.MustCompile(`(?m)^\s+([A-Za-z][A-Za-z0-9_]*):`).FindAllStringSubmatch(icons, -1) {
		kinds[m[1]] = append(kinds[m[1]], "Codex/client icons")
	}
	for _, m := range regexp.MustCompile(`(?m)^    case '([A-Za-z][A-Za-z0-9_]*)'`).FindAllStringSubmatch(text, -1) {
		kinds[m[1]] = append(kinds[m[1]], "Codex/client text")
	}
	t.Logf("Known notification kinds: %d", len(kinds))
	out := []string{}
	for k := range kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	for _, k := range out {
		t.Logf("%s: %s", k, strings.Join(kinds[k], ", "))
	}
	return out
}
func TestEveryNotificationKindHasReadableDetail(t *testing.T) {
	raw := json.RawMessage(`{"name":"Bronze Guard","from":"Nestor","to":"Agamemnon","owner":"Nestor","type":"divine_recruits","unit_type":"spearman","building_type":"farm","good_key":"grain","quantity":12,"amount":12,"count":3,"q":6,"r":7,"target_q":8,"target_r":9,"net_per_tick":-2,"ticks_left":12,"grain_net_per_tick":3,"grain_amount":100,"grain_ticks":12,"tier":"yellow","message":"Come home","reason":"grain_shortage","outcome":"attacker_wins","role":"defender","opponent_name":"Nestor","own_unit":{"type":"spearman","size_before":100,"size_after":70},"enemy_unit":{"type":"spearman","size_before":100,"size_after":80},"goods":[{"good_key":"grain","quantity":12}],"ticks_out":12,"hexes_seen":4,"foreign_owner":"Nestor","foreign_type":"spearman","port_name":"Tiryns","crew_after":60}`)
	for _, kind := range knownNotificationKinds(t) {
		t.Run(kind, func(t *testing.T) {
			body := raw
			if kind == "DivinePunishment" {
				body = json.RawMessage(`{"type":"harvest_failure","amount":12,"name":"Tiryns"}`)
			}
			if kind == "FoodShortfall" {
				body = json.RawMessage(`{"unmet":12,"name":"Tiryns"}`)
			}
			out, err := captureStdout(t, func() error { printNotificationDetail(nil, notificationItem{Kind: kind, Body: body}); return nil })
			if err != nil {
				t.Fatal(err)
			}
			for _, view := range []string{"row", "watch"} {
				rendered := capturePrint(t, func() {
					if view == "row" {
						printNotificationRow(nil, notificationItem{Kind: kind, Body: body})
					} else {
						old := jsonMode
						jsonMode = false
						defer func() { jsonMode = old }()
						printWatchMessage(nil, wsMsg{Kind: kind, Payload: body})
					}
				})
				if strings.Contains(rendered, `"name":`) || strings.Contains(rendered, `{"`) {
					t.Fatalf("%s %s leaks raw JSON: %s", kind, view, rendered)
				}
			}
			if strings.TrimSpace(out) == "" {
				t.Fatalf("notification kind %s has no readable detail", kind)
			}
			if strings.Contains(out, `{"`) || strings.Contains(out, `"name":`) {
				t.Fatalf("notification kind %s prints raw JSON: %s", kind, out)
			}
			if regexp.MustCompile(`(?i)\bticks?\b`).MatchString(out) {
				t.Fatalf("notification kind %s leaks internal time: %s", kind, out)
			}
		})
	}
}
func TestNotificationSourceDiscovery(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "new.go")
	source := `package producer
 const kindName="NewNotice"
 func wrap(h Hub, kind string){ h.NotifyPlayer(ctx,w,p,kind,3,body) }
 func emit(h Hub){ wrap(h,kindName); h.NotifyPlayer(ctx,w,p,"AnotherNotice",3,body); db.QueryRow(ctx,"INSERT INTO notifications(world_id,player_id,kind) VALUES($1,$2,'SQLNotice')",w,p) }
 `
	if err := os.WriteFile(p, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	kinds, err := serverNotificationKinds(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"NewNotice", "AnotherNotice", "SQLNotice"} {
		if len(kinds[k]) == 0 {
			t.Errorf("new producer %s silently omitted", k)
		}
	}
}

func TestNotificationHumanFeedGroupingAndJSON(t *testing.T) {
	// Historical only: archived food-support rows still group without leaking payloads.
	raw := `{"notifications":[{"kind":"UnitArrived","body":{"name":"Bronze Guard","q":6,"r":7}}, {"kind":"SubsistenceWarning","body":{"name":"Tiryns","tier":"critical","pop_loss":3,"net_per_tick":-2}}, {"kind":"SitosIntervention","body":{"secret":"machine-only"}}, {"kind":"SitosIntervention","body":{"secret":"machine-only"}}],"unread":4,"future_field":{"retained":true}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected mutation: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, raw)
	}))
	defer server.Close()
	oldCfg, oldJSON := cfg, jsonMode
	defer func() { cfg, jsonMode = oldCfg, oldJSON }()
	cfg = &Config{Server: server.URL, WorldID: "world"}
	run := func(asJSON bool) string {
		jsonMode = asJSON
		cmd := notificationsCmd()
		cmd.SetArgs([]string{"--exclude", "none"})
		out, err := captureStdout(t, cmd.Execute)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	human := run(false)
	for _, part := range []string{"Bronze Guard arrived at (6, 7)", "STARVING", "×2 (latest)", "Food support was given", "per game day"} {
		if !strings.Contains(human, part) {
			t.Errorf("missing %q: %s", part, human)
		}
	}
	if strings.Index(human, "STARVING") > strings.Index(human, "Bronze Guard") {
		t.Fatal("critical warning lost priority")
	}
	if strings.Contains(human, "machine-only") || strings.Contains(human, `{"`) {
		t.Fatalf("human feed leaked JSON: %s", human)
	}
	var expected, actual any
	json.Unmarshal([]byte(raw), &expected)
	if err := json.Unmarshal([]byte(run(true)), &actual); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("--json discarded payload or response fields")
	}
	msg := wsMsg{Kind: "UnitArrived", Payload: json.RawMessage(`{"name":"Bronze Guard","q":6,"r":7,"opaque":{"retained":true}}`)}
	jsonMode = true
	out := capturePrint(t, func() { printWatchMessage(nil, msg) })
	var pushed wsMsg
	if err := json.Unmarshal([]byte(out), &pushed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pushed.Payload), `"opaque"`) {
		t.Fatal("watch --json lost payload fields")
	}
}

func TestNotificationWordsAndArchiveSemantics(t *testing.T) {
	cases := []struct {
		kind, body string
		want       []string
		avoid      []string
	}{
		{"SubsistenceWarning", `{"name":"Tiryns","net_per_tick":-3,"net_per_day":999,"ticks_left":12,"days_left":999}`, []string{"-3 per game day", "12 game days"}, []string{"999", "tick"}},
		{"SubsistenceWarning", `{"name":"Tiryns","net_per_day":-3,"days_left":12}`, []string{"-3 per game day", "12 game days"}, []string{"tick"}},
		// Historical only: frozen archive payloads.
		{"SitosGranaryRelease", `{"food_released":50,"coverage_days":12,"per_good":{"grain":30,"fish":20},"granary_empty":true}`, []string{"20 fish, 30 grain", "12.0 game days", "granary now empty"}, []string{"50 grain"}},
		// Historical only: frozen archive payloads.
		{"SitosGranaryRelease", `{"food_released":0.3,"coverage_days":12,"per_good":{"grain":0.3}}`, []string{"0.3 grain"}, []string{"released 0 grain"}},
		{"OfferDeclined", `{"kind":"buy","quantity":0,"silver":15}`, []string{"15 silver", "back in your stores"}, nil},
		{"OfferExpired", `{"kind":"sell","quantity":12,"good_key":"tin"}`, []string{"12 tin", "back in your stores"}, nil},
		{"OfferAccepted", `{"kind":"sell","quantity":12,"good_key":"tin","silver":15,"silver_arrival_tick":130}`, []string{"12 tin", "15 silver", "silver arrives " + testTickTime(130)}, []string{"tick"}},
		{"UnitAttrition", `{"name":"Bronze Guard","lost":7,"disbanded":true}`, []string{"Bronze Guard starved to nothing"}, nil},
		{"UnitDeserted", `{"name":"Bronze Guard","lost":7}`, []string{"Bronze Guard", "7 left unpaid"}, nil},
		{"CityCollapsed", `{"name":"Tiryns","last_settlement":true}`, []string{"last settlement"}, nil},
		{"ColonyFounded", `{"name":"Tiryns"}`, []string{"Colony founded: Tiryns"}, nil},
		{"OrderFailed", `{"name":"Bronze Guard","verb":"stance_pursuit","reason":"target_lost"}`, []string{"Bronze Guard", "pursuit", "target lost"}, []string{"stance_pursuit"}},
	}
	for _, tc := range cases {
		t.Run(tc.kind+tc.body, func(t *testing.T) {
			out := capturePrint(t, func() {
				printNotificationDetail(testTimeClient(), notificationItem{Kind: tc.kind, Body: json.RawMessage(tc.body)})
			})
			for _, s := range tc.want {
				if !strings.Contains(out, s) {
					t.Errorf("missing %q in %s", s, out)
				}
			}
			for _, s := range tc.avoid {
				if strings.Contains(out, s) {
					t.Errorf("unexpected %q in %s", s, out)
				}
			}
		})
	}
	for _, tc := range []struct{ kind, body, person, place string }{
		{"SentryAlerted", `{"foreign_owner":"Nestor","foreign_type":"spearman","q":6,"r":7}`, "Nestor", "(6, 7)"},
		{"MessengerArrival", `{"from":"Nestor","name":"Tiryns"}`, "Nestor", "Tiryns"},
		{"SiegeStarted", `{"besieger_name":"Nestor","name":"Tiryns"}`, "Nestor", "Tiryns"},
		{"BattleWon", `{"opponent_name":"Nestor","place":"Tiryns"}`, "Nestor", "Tiryns"},
	} {
		out := capturePrint(t, func() {
			printNotificationDetail(testTimeClient(), notificationItem{Kind: tc.kind, Body: json.RawMessage(tc.body)})
		})
		if strings.Index(out, tc.person) < 0 || strings.Index(out, tc.person) > strings.Index(out, tc.place) {
			t.Errorf("%s should name person before place: %s", tc.kind, out)
		}
	}
}

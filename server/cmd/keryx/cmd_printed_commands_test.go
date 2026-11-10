package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// shellSplit splits a printed command line the way a shell would (quotes only).
func shellSplit(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	started := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == ' ':
			if started || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if started || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// printedCommands returns every "→ keryx ..." line of an output.
func printedCommands(out string) []string {
	var cmds []string
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, "→ keryx "); i >= 0 {
			cmds = append(cmds, strings.TrimSpace(l[i+len("→ "):]))
		}
	}
	return cmds
}

// A printed line must parse back through the very cobra command it names.
func assertParsesBack(t *testing.T, line string) {
	t.Helper()
	words := shellSplit(line)
	if len(words) < 2 || words[0] != "keryx" {
		t.Fatalf("not a keryx line: %q", line)
	}
	root := newRootCommand()
	cmd, rest, err := root.Find(words[1:])
	if err != nil || cmd == root {
		t.Fatalf("%q does not name a command: %v", line, err)
	}
	if err := cmd.ParseFlags(rest); err != nil {
		t.Fatalf("%q does not parse: %v", line, err)
	}
	if cmd.Args != nil {
		if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
			t.Fatalf("%q rejected by its own command: %v", line, err)
		}
	}
	for _, name := range []string{"city", "type", "unit", "prayer"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed && f.Value.String() == "" {
			t.Fatalf("%q: --%s parsed empty", line, name)
		}
	}
}

func TestBuildListPrintsCommands(t *testing.T) {
	q := func(typ string, mut func(m map[string]any)) map[string]any {
		m := map[string]any{"type": typ, "current_level": 1, "next_level": 2, "costs": map[string]any{"timber": 5}, "cost_silver": 0, "duration_game_days": 3}
		mut(m)
		return m
	}
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/buildings") {
			json.NewEncoder(w).Encode([]any{
				map[string]any{"type": "farm", "purpose": "grain"}, map[string]any{"type": "temple", "purpose": "gods"},
				map[string]any{"type": "storehouse", "purpose": "store"}, map[string]any{"type": "harbour", "purpose": "ships"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{"name": "Old Phaistos", "build_quotes": []any{
			q("farm", func(m map[string]any) { m["hex_q"], m["hex_r"] = 12, 21 }),
			q("farm", func(m map[string]any) { m["new_site"] = true; m["current_level"] = 0; m["next_level"] = 1 }),
			q("temple", func(m map[string]any) { m["god_id"] = "potnia" }),
			q("storehouse", func(m map[string]any) { m["next_level"] = nil; m["at_max_level"] = true }),
			q("harbour", func(m map[string]any) {}),
		}}})
	})
	out, err := captureStdout(t, func() error { return listCityBuilds(newClient(cfg), "world", "p") })
	if err != nil {
		t.Fatal(err)
	}
	cmds := printedCommands(out)
	want := []string{
		"keryx build --city 'Old Phaistos' --type farm --hex 12,21",
		"keryx build --city 'Old Phaistos' --type farm --hex <q,r>",
		"keryx build --city 'Old Phaistos' --type temple --god potnia",
		"keryx build --city 'Old Phaistos' --type harbour",
	}
	if strings.Join(cmds, "\n") != strings.Join(want, "\n") {
		t.Fatalf("printed:\n%s\nwant:\n%s\n--- output\n%s", strings.Join(cmds, "\n"), strings.Join(want, "\n"), out)
	}
	for _, line := range cmds {
		assertParsesBack(t, line)
	}
}

func TestRecruitListPrintsCommandsOnlyForAffordableRows(t *testing.T) {
	round3Client(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/units") {
			json.NewEncoder(w).Encode([]any{
				map[string]any{"type": "spearman", "costs": map[string]any{"bronze": 5}, "batch_men": 100, "duration_game_days": 2},
				map[string]any{"type": "war_chariot", "costs": map[string]any{"bronze": 9}, "batch_men": 100, "duration_game_days": 3, "requires_stable": true},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"settlement": map[string]any{"name": "Phaistos", "can_recruit": []any{
			map[string]any{"unit": "spearman", "can_recruit": true},
			map[string]any{"unit": "war_chariot", "can_recruit": false, "reasons": []any{"needs a stable"}},
		}}})
	})
	out, err := captureStdout(t, func() error { return printRecruitCatalogue(newClient(cfg), "world", "p") })
	if err != nil {
		t.Fatal(err)
	}
	cmds := printedCommands(out)
	if len(cmds) != 1 || cmds[0] != "keryx recruit --city Phaistos --unit spearman" {
		t.Fatalf("cmds=%v\n%s", cmds, out)
	}
	if !strings.Contains(out, "needs a stable") {
		t.Fatalf("locked row keeps its reason:\n%s", out)
	}
	assertParsesBack(t, cmds[0])
}

func TestRiteCommandLines(t *testing.T) {
	p := cultPrayer{CityName: "Old Phaistos"}
	p.ID = "minoan_potnia_blessing"
	line := riteCommandLine(p)
	if line != "keryx rite --city 'Old Phaistos' --prayer minoan_potnia_blessing --yes" {
		t.Fatalf("plain prayer: %q", line)
	}
	assertParsesBack(t, line)
	p.Targets = &riteTargets{Kind: "hex"}
	assertParsesBack(t, riteCommandLine(p))
	p.Targets = &riteTargets{Kind: "world"}
	if l := riteCommandLine(p); !strings.Contains(l, "--scope city") || !strings.HasSuffix(l, "--yes") {
		t.Fatalf("scoped prayer: %q", l)
	}
	assertParsesBack(t, riteCommandLine(p))
}

func TestActionsDetailPrintsCommandsForAvailableVerbsOnly(t *testing.T) {
	verbs := []actionVerb{
		{Name: "build", Category: "province", Available: true, Purpose: "build"},
		{Name: "recruit", Category: "province", Available: false, Purpose: "recruit", Requirements: []actionRequirement{{Text: "a barracks", Detail: "none", Hint: "build one"}}},
		{Name: "cancel-build", Category: "province", Available: true, Purpose: "cancel"},
		{Name: "abandon", Category: "province", Available: true, Purpose: "abandon"},
	}
	out, _ := captureStdout(t, func() error { printCategoryDetail(verbs, "province", "Old Phaistos"); return nil })
	cmds := printedCommands(out)
	if len(cmds) != 3 {
		t.Fatalf("want 3 commands (locked row has none): %v\n%s", cmds, out)
	}
	for _, c := range cmds {
		assertParsesBack(t, c)
	}
	if strings.Contains(out, "keryx recruit") {
		t.Fatalf("locked verb must print no command:\n%s", out)
	}
}

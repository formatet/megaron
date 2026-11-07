package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// dossierSections is the fixed order of the report (order-keryx-dossier-20261104):
// failures first, as in a PBEM turn report. Every section is always written;
// an empty one says "None." so silence is never ambiguous.
var dossierSections = []string{
	"Failed orders", "Urgent", "Cities", "Forces and orders on the way", "Known places",
	"Contacts", "Storms at sea", "Since you last read", "What you can do now",
}

// dossierUrgentKinds are the unread notification kinds that go under "Urgent".
// OrderFailed has its own section above and is never repeated here.
var dossierUrgentKinds = map[string]bool{
	"FoodRunningLow": true, "SubsistenceWarning": true,
	"GarrisonStarvationStarted": true, "CivilianStarvationStarted": true,
	"ForeignMarchSighted": true, "ForeignMarchSightedV2": true,
	"ShipStormDamaged": true, "ShipFoundered": true,
	"MessengerLostAtSea": true,
}

const (
	dossierSinceCap  = 30
	dossierPlacesCap = 15
	dossierDepCap    = 10
)

func dossierCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "dossier",
		Short: "Write a markdown report of everything you know — read it first each round",
		Long: `Writes one markdown report from your own endpoints (so the fog of war applies):
failed orders, urgent news, your cities, forces and orders on the way, known
places, contacts, storms, unread news and what you can do now — always in that
order, an empty section reading "None.".

It only reads. It does not mark anything read. It is a snapshot: write a new one
each round and do not trust an old file.`,
		Example: `  keryx dossier
  keryx dossier --out world.md`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonMode {
				return fmt.Errorf("dossier is markdown; use the individual commands with --json")
			}
			text := buildDossier(newClient(cfg), cfg.WorldID, time.Now())
			if out == "" {
				fmt.Print(text)
				return nil
			}
			if err := os.WriteFile(out, []byte(text), 0o644); err != nil {
				return err
			}
			fmt.Printf("%s (%d bytes)\n", out, len(text))
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write the report to this file (overwritten) instead of stdout")
	return cmd
}

// grabPrinted captures what the shared notification printers write to stdout,
// so the dossier reuses them instead of keeping a second copy of every sentence.
func grabPrinted(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return ""
	}
	os.Stdout = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func dossierGet(c *Client, format string, args ...any) ([]byte, error) {
	return c.get(fmt.Sprintf(format, args...))
}

// dayLength says how long one game day is in wall-clock time.
func dayLength(seconds float64) string {
	switch {
	case seconds <= 0:
		return ""
	case seconds >= 3600 && int(seconds)%3600 == 0:
		h := int(seconds) / 3600
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	case seconds >= 60 && int(seconds)%60 == 0:
		m := int(seconds) / 60
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	}
	return fmt.Sprintf("%g seconds", seconds)
}

type dossierCity struct {
	ID, ProvinceID, Name string
	Q, R                 int
	Capital              bool
}

func buildDossier(c *Client, worldID string, now time.Time) string {
	wanax, world, dayLine := "unknown Wanax", "unknown world", ""
	if data, err := dossierGet(c, "/api/v1/worlds/%s/me", worldID); err == nil {
		var me struct {
			WanaxName string `json:"wanax_name"`
		}
		if json.Unmarshal(data, &me) == nil && me.WanaxName != "" {
			wanax = me.WanaxName
		}
	}
	if data, err := dossierGet(c, "/api/v1/worlds/%s", worldID); err == nil {
		var w struct {
			Name        string  `json:"name"`
			TickSeconds float64 `json:"tick_seconds"`
		}
		if json.Unmarshal(data, &w) == nil {
			if w.Name != "" {
				world = w.Name
			}
			if l := dayLength(w.TickSeconds); l != "" {
				dayLine = fmt.Sprintf("One game day = %s of wall-clock time.", l)
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Dossier — %s in %s\n\n", wanax, world)
	fmt.Fprintf(&b, "Written %s\n", now.In(keryxTZ).Format("2006-01-02 15:04"))
	if dayLine != "" {
		fmt.Fprintf(&b, "%s\n", dayLine)
	}
	b.WriteString("This is a snapshot. Write a new one each round; do not trust an old file.\n")

	cities, citiesErr := dossierOwnCities(c, worldID)
	failed, urgent, since, notifErr := dossierNotifications(c, worldID)

	body := map[string][]string{}
	body["Failed orders"] = orNone(failed, notifErr)
	body["Urgent"] = orNone(urgent, notifErr)
	body["Cities"] = dossierCitiesSection(c, worldID, cities, citiesErr)
	body["Forces and orders on the way"] = dossierForces(c, worldID)
	body["Known places"] = dossierKnownPlaces(c, worldID, cities)
	body["Contacts"] = dossierContacts(c, worldID)
	body["Storms at sea"] = dossierStorms(c, worldID)
	body["Since you last read"] = orNone(since, notifErr)
	body["What you can do now"] = dossierActions(c, worldID, cities)

	for _, title := range dossierSections {
		fmt.Fprintf(&b, "\n## %s\n\n", title)
		for _, l := range body[title] {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// orNone turns an empty section into "None." and a failed read into a visible error.
func orNone(lines []string, err error) []string {
	if err != nil {
		return []string{fmt.Sprintf("Unavailable: %v", err)}
	}
	if len(lines) == 0 {
		return []string{"None."}
	}
	return lines
}

func dossierOwnCities(c *Client, worldID string) ([]dossierCity, error) {
	data, err := dossierGet(c, "/api/v1/worlds/%s/provinces", worldID)
	if err != nil {
		return nil, err
	}
	var markers []struct {
		ID           string `json:"id"`
		SettlementID string `json:"settlement_id"`
		Name         string `json:"name"`
		Q            int    `json:"q"`
		R            int    `json:"r"`
		Own          bool   `json:"own"`
		IsCapital    bool   `json:"is_capital"`
	}
	if err := json.Unmarshal(data, &markers); err != nil {
		return nil, err
	}
	var out []dossierCity
	for _, m := range markers {
		if m.Own && m.SettlementID != "" {
			out = append(out, dossierCity{ID: m.SettlementID, ProvinceID: m.ID, Name: m.Name, Q: m.Q, R: m.R, Capital: m.IsCapital})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// nearestOwn returns the own city nearest to (q,r) and the distance in hexes.
func nearestOwn(cities []dossierCity, q, r int) (string, int, bool) {
	best, name := -1, ""
	for _, ct := range cities {
		if d := hexDist(ct.Q, ct.R, q, r); best < 0 || d < best {
			best, name = d, ct.Name
		}
	}
	return name, best, best >= 0
}

func hexesFrom(d int, name string) string {
	if d == 0 {
		return "at " + name
	}
	if d == 1 {
		return "1 hex from " + name
	}
	return fmt.Sprintf("%d hexes from %s", d, name)
}

// ---- notifications ------------------------------------------------------------

func dossierNotifications(c *Client, worldID string) (failed, urgent, since []string, err error) {
	q := url.Values{}
	q.Set("unread", "true")
	q.Set("exclude", strings.Join(noisyNotificationKinds, ","))
	data, err := dossierGet(c, "/api/v1/worlds/%s/notifications?%s", worldID, q.Encode())
	if err != nil {
		return nil, nil, nil, err
	}
	var resp struct {
		Notifications []notificationItem `json:"notifications"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, nil, err
	}
	notifs := resp.Notifications
	sort.SliceStable(notifs, func(i, j int) bool {
		a, b := notificationTick(notifs[i]), notificationTick(notifs[j])
		if a != nil && b != nil && *a != *b {
			return *a > *b
		}
		return notifs[i].CreatedAt > notifs[j].CreatedAt
	})
	var rest []notificationItem
	for _, n := range notifs {
		switch {
		case n.Kind == "OrderFailed":
			failed = append(failed, dossierNotifBlock(c, n)...)
		case dossierUrgentKinds[n.Kind]:
			urgent = append(urgent, dossierNotifBlock(c, n)...)
		default:
			rest = append(rest, n)
		}
	}
	for i, n := range rest {
		if i == dossierSinceCap {
			since = append(since, fmt.Sprintf("…and %d more — `keryx notifications --unread`", len(rest)-dossierSinceCap))
			break
		}
		since = append(since, dossierNotifLine(c, n))
	}
	return failed, urgent, since, nil
}

func dossierDetailLines(c *Client, n notificationItem) []string {
	var lines []string
	for _, l := range strings.Split(grabPrinted(func() { printNotificationDetail(c, n) }), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func dossierNotifBlock(c *Client, n notificationItem) []string {
	out := []string{fmt.Sprintf("- %s · %s", notificationDay(n, c), n.Kind)}
	for _, l := range dossierDetailLines(c, n) {
		out = append(out, "  "+l)
	}
	return out
}

func dossierNotifLine(c *Client, n notificationItem) string {
	line := fmt.Sprintf("- %s · %s", notificationDay(n, c), n.Kind)
	if d := dossierDetailLines(c, n); len(d) > 0 {
		text := d[0]
		if r := []rune(text); len(r) > 220 {
			text = string(r[:220]) + "…"
		}
		line += " — " + text
	}
	return line
}

// ---- cities -------------------------------------------------------------------

func dossierCitiesSection(c *Client, worldID string, cities []dossierCity, err error) []string {
	if err != nil {
		return []string{fmt.Sprintf("Unavailable: %v", err)}
	}
	if len(cities) == 0 {
		return []string{"None. Your people are still on the road — see `keryx status`."}
	}
	var out []string
	for _, ct := range cities {
		out = append(out, dossierCityBlock(c, worldID, ct)...)
	}
	return out
}

func dossierCityBlock(c *Client, worldID string, ct dossierCity) []string {
	head := "### " + ct.Name
	if ct.Capital {
		head += " (capital)"
	}
	data, err := dossierGet(c, "/api/v1/worlds/%s/provinces/%s", worldID, ct.ProvinceID)
	if err != nil {
		return []string{head, "- Unavailable: " + err.Error()}
	}
	var p map[string]any
	if err := json.Unmarshal(data, &p); err != nil {
		return []string{head, "- Unavailable: " + err.Error()}
	}
	sett, _ := p["settlement"].(map[string]any)
	if sett == nil {
		return []string{head, "- Unavailable: no settlement in the reply."}
	}
	pop, _ := sett["population"].(float64)
	out := []string{head + fmt.Sprintf(" — %s people", resource(pop))}
	if besieged, _ := sett["besieged"].(bool); besieged {
		out = append(out, "- BESIEGED — an enemy holds an access road.")
	}

	var stock []string
	if res, ok := sett["resources"].(map[string]any); ok {
		keys := make([]string, 0, len(res))
		for k := range res {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if rd, ok := res[k].(map[string]any); ok {
				if amt, _ := rd["amount"].(float64); amt >= 0.5 {
					stock = append(stock, fmt.Sprintf("%s %s", k, resource(amt)))
				}
			}
		}
	}
	if len(stock) == 0 {
		stock = []string{"nothing"}
	}
	out = append(out, "- Stock: "+strings.Join(stock, ", "))

	food, _ := sett["food"].(map[string]any)
	if cov, ok := food["coverage_ticks"].(float64); ok {
		line := fmt.Sprintf("- Food for %s at current consumption", formatDays(cov, "%.0f"))
		if net, ok := food["food_net_per_tick"].(float64); ok {
			line += fmt.Sprintf(" (net %s per game day)", formatNetPerTick(net))
		}
		out = append(out, line)
	} else {
		out = append(out, "- Food stores not reported.")
	}

	if opts, err := fetchPlacementOptions(c, worldID, ct.ProvinceID); err == nil {
		out = append(out, fmt.Sprintf("- Citizens: %d idle of %d (1 citizen = 100 people)", opts.PoolSize, opts.TotalGubbar))
	}

	if bq, _ := sett["build_queue"].([]any); len(bq) > 0 {
		var parts []string
		for _, it := range bq {
			if m, ok := it.(map[string]any); ok {
				parts = append(parts, fmt.Sprintf("%s (%s)", buildingLabel(m), buildQueueState(c, m)))
			}
		}
		out = append(out, "- Building: "+strings.Join(parts, "; "))
	} else {
		out = append(out, "- Building: nothing queued.")
	}

	if temples, _ := sett["temples"].([]any); len(temples) > 0 {
		var parts []string
		for _, it := range temples {
			if m, ok := it.(map[string]any); ok {
				name, _ := m["name"].(string)
				lvl, _ := m["level"].(float64)
				staffed, _ := m["staffed"].(float64)
				capacity, _ := m["capacity"].(float64)
				parts = append(parts, fmt.Sprintf("%s level %.0f, staffed %.0f/%.0f", name, lvl, staffed, capacity))
			}
		}
		out = append(out, "- Temples: "+strings.Join(parts, "; "))
	} else {
		out = append(out, "- Temples: none.")
	}
	return out
}

// ---- forces and orders --------------------------------------------------------

func dossierForces(c *Client, worldID string) []string {
	var out []string
	udata, err := dossierGet(c, "/api/v1/worlds/%s/units", worldID)
	if err != nil {
		out = append(out, "Units unavailable: "+err.Error())
	} else {
		var resp struct {
			Units []unitRow `json:"units"`
		}
		if err := json.Unmarshal(udata, &resp); err != nil {
			out = append(out, "Units unavailable: "+err.Error())
		} else {
			homes := fetchOwnSettlementPositions(c, worldID)
			var own []dossierCity
			for id, h := range homes {
				own = append(own, dossierCity{ID: id, Name: h.Name, Q: h.Q, R: h.R})
			}
			sort.Slice(resp.Units, func(i, j int) bool { return resp.Units[i].DisplayName < resp.Units[j].DisplayName })
			for _, u := range resp.Units {
				name := u.DisplayName
				if name == "" {
					name = u.Type + shipNameSuffix(u.Name)
				}
				loc := locationStr(c, u, homes)
				if u.SettlementID != nil {
					if h, ok := homes[*u.SettlementID]; ok && strings.HasPrefix(loc, "settlement ") {
						loc = "in " + h.Name
					}
				}
				if strings.HasPrefix(loc, "hex (") && u.Q != nil && u.R != nil {
					if n, d, ok := nearestOwn(own, *u.Q, *u.R); ok {
						loc += " — " + hexesFrom(d, n)
					}
				}
				stance := ""
				if u.Stance != nil && *u.Stance != "" {
					stance = ", stance " + *u.Stance
				}
				out = append(out, fmt.Sprintf("- %s — %s, %s%s — %s", name, formatSize(c, u), u.Status, stance, loc))
				for _, o := range u.OrdersEnRoute {
					out = append(out, "  - "+ordersEnRouteLine(c, o))
				}
			}
		}
	}

	if data, err := dossierGet(c, "/api/v1/worlds/%s/standing-orders", worldID); err != nil {
		out = append(out, "Standing trade routes unavailable: "+err.Error())
	} else {
		var orders []map[string]any
		if json.Unmarshal(data, &orders) == nil {
			for _, o := range orders {
				status, _ := o["status"].(string)
				line := fmt.Sprintf("- Standing trade route %v → %v [%s]", o["from_name"], o["to_name"], status)
				if reason, _ := o["pause_reason"].(string); reason != "" {
					line += " — " + reason
				}
				out = append(out, line)
			}
		}
	}

	if data, err := dossierGet(c, "/api/v1/worlds/%s/runners", worldID); err != nil {
		out = append(out, "Runners unavailable: "+err.Error())
	} else {
		var rr runnersResp
		if json.Unmarshal(data, &rr) == nil && (len(rr.Own) > 0 || len(rr.Foreign) > 0) {
			if text, err := renderRunners(data); err == nil {
				for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
					if strings.TrimSpace(l) == "" {
						continue
					}
					if strings.HasPrefix(l, " ") {
						out = append(out, "  "+strings.TrimSpace(l))
					} else {
						out = append(out, "- Runners — "+strings.ToLower(l))
					}
				}
			}
		}
	}
	return orNone(out, nil)
}

// ---- known places and contacts ------------------------------------------------

type dossierKnown struct {
	dist int
	line string
}

func dossierKnownPlaces(c *Client, worldID string, own []dossierCity) []string {
	if len(own) == 0 {
		return []string{"None. Without a city there is no distance to measure from — see `keryx status`."}
	}
	var out []string
	var named, unnamed, deposits, towns []dossierKnown

	if data, err := dossierGet(c, "/api/v1/worlds/%s/cities", worldID); err != nil {
		out = append(out, "Cities unavailable: "+err.Error())
	} else {
		var entries []map[string]any
		if json.Unmarshal(data, &entries) == nil {
			for _, e := range entries {
				if isOwn, _ := e["own"].(bool); isOwn {
					continue
				}
				if k, _ := e["knowledge"].(string); k == "rumour" {
					continue
				}
				name, _ := e["name"].(string)
				terrain, _ := e["terrain"].(string)
				fq, qok := e["q"].(float64)
				fr, rok := e["r"].(float64)
				if !qok || !rok {
					continue
				}
				n, d, _ := nearestOwn(own, int(fq), int(fr))
				line := fmt.Sprintf("- City %s — %s", name, terrain)
				if dep := dossierDepositText(e); dep != "" {
					line += ", " + dep
				}
				towns = append(towns, dossierKnown{d, line + " — " + hexesFrom(d, n)})
			}
		}
	}

	if data, err := dossierGet(c, "/api/v1/worlds/%s/places", worldID); err != nil {
		out = append(out, "Places unavailable: "+err.Error())
	} else {
		var resp struct {
			Places []struct {
				Kind  string `json:"kind"`
				Name  string `json:"name"`
				Hexes []struct {
					Q int `json:"q"`
					R int `json:"r"`
				} `json:"hexes"`
			} `json:"places"`
		}
		if json.Unmarshal(data, &resp) == nil {
			for _, p := range resp.Places {
				best, bn := -1, ""
				for _, h := range p.Hexes {
					if n, d, ok := nearestOwn(own, h.Q, h.R); ok && (best < 0 || d < best) {
						best, bn = d, n
					}
				}
				if best < 0 {
					continue
				}
				label := placeKindLabel(p.Kind)
				if p.Name != "" {
					named = append(named, dossierKnown{best, fmt.Sprintf("- %s %s — %s", label, p.Name, hexesFrom(best, bn))})
				} else {
					unnamed = append(unnamed, dossierKnown{best, fmt.Sprintf("- Unnamed %s — %s", label, hexesFrom(best, bn))})
				}
			}
		}
	}

	if data, err := dossierGet(c, "/api/v1/worlds/%s/map", worldID); err != nil {
		out = append(out, "Map unavailable: "+err.Error())
	} else {
		var tiles []struct {
			Q       int    `json:"q"`
			R       int    `json:"r"`
			Terrain string `json:"terrain"`
			Visible bool   `json:"visible"`
			Copper  bool   `json:"copper_deposit"`
			Tin     bool   `json:"tin_deposit"`
			Cedar   bool   `json:"cedar_deposit"`
			Silver  bool   `json:"silver_deposit"`
			Incense bool   `json:"incense_deposit"`
		}
		if json.Unmarshal(data, &tiles) == nil {
			for _, t := range tiles {
				if !t.Visible || t.Terrain == "fog" {
					continue
				}
				var dep []string
				for _, d := range []struct {
					on   bool
					name string
				}{{t.Cedar, "cedar"}, {t.Copper, "copper"}, {t.Incense, "incense"}, {t.Silver, "silver"}, {t.Tin, "tin"}} {
					if d.on {
						dep = append(dep, d.name)
					}
				}
				if len(dep) == 0 {
					continue
				}
				n, d, _ := nearestOwn(own, t.Q, t.R)
				deposits = append(deposits, dossierKnown{d, fmt.Sprintf("- Deposit of %s on %s at (%d,%d) — %s", strings.Join(dep, " and "), t.Terrain, t.Q, t.R, hexesFrom(d, n))})
			}
		}
	}

	byDist := func(s []dossierKnown) {
		sort.SliceStable(s, func(i, j int) bool {
			if s[i].dist != s[j].dist {
				return s[i].dist < s[j].dist
			}
			return s[i].line < s[j].line
		})
	}
	for _, s := range [][]dossierKnown{towns, named, deposits, unnamed} {
		byDist(s)
	}
	appendCapped := func(s []dossierKnown, limit int, more string) {
		for i, k := range s {
			if i == limit {
				out = append(out, fmt.Sprintf("…and %d more %s", len(s)-limit, more))
				return
			}
			out = append(out, k.line)
		}
	}
	appendCapped(towns, dossierPlacesCap, "cities — `keryx cities`")
	appendCapped(named, dossierPlacesCap, "named places — `keryx places`")
	appendCapped(deposits, dossierDepCap, "deposits — `keryx map`")
	appendCapped(unnamed, dossierPlacesCap, "unnamed places — `keryx places`")
	return orNone(out, nil)
}

func dossierDepositText(e map[string]any) string {
	var dep []string
	for _, d := range []struct{ key, label string }{
		{"cedar_deposit", "cedar"}, {"copper_deposit", "copper"}, {"incense_deposit", "incense"},
		{"silver_deposit", "silver"}, {"tin_deposit", "tin"},
	} {
		if on, _ := e[d.key].(bool); on {
			dep = append(dep, d.label)
		}
	}
	if len(dep) == 0 {
		return ""
	}
	return "deposit of " + strings.Join(dep, " and ")
}

func dossierContacts(c *Client, worldID string) []string {
	var out []string
	data, err := dossierGet(c, "/api/v1/worlds/%s/diplomacy", worldID)
	if err != nil {
		return []string{"Unavailable: " + err.Error()}
	}
	var entries []map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		return []string{"Unavailable: " + err.Error()}
	}
	citiesByOwner := map[string][]string{}
	if cdata, err := dossierGet(c, "/api/v1/worlds/%s/cities", worldID); err == nil {
		var cities []map[string]any
		if json.Unmarshal(cdata, &cities) == nil {
			for _, e := range cities {
				if isOwn, _ := e["own"].(bool); isOwn {
					continue
				}
				owner, _ := e["owner"].(string)
				name, _ := e["name"].(string)
				label := name
				switch k, _ := e["knowledge"].(string); k {
				case "heard":
					label += " (word only)"
				case "rumour":
					label += " (rumour)"
				}
				citiesByOwner[owner] = append(citiesByOwner[owner], label)
			}
		}
	}
	for _, e := range entries {
		owner, _ := e["owner"].(string)
		known, _ := e["known_cities"].(float64)
		rumour, _ := e["rumour_cities"].(float64)
		rumourOnly, _ := e["rumour_only"].(bool)
		line := "- Wanax " + owner
		if rumourOnly {
			line += " (rumour only)"
		}
		if kingdom, _ := e["kingdom"].(string); kingdom != "" {
			line += ", kingdom " + kingdom
		}
		line += fmt.Sprintf(" — %.0f known cities, %.0f by rumour", known, rumour)
		if names := citiesByOwner[owner]; len(names) > 0 {
			sort.Strings(names)
			line += ": " + strings.Join(names, ", ")
		}
		out = append(out, line)
	}
	return orNone(out, nil)
}

// ---- storms -------------------------------------------------------------------

func dossierStorms(c *Client, worldID string) []string {
	ships, coastal := false, false
	if data, err := dossierGet(c, "/api/v1/worlds/%s/units", worldID); err == nil {
		var resp struct {
			Units []unitRow `json:"units"`
		}
		if json.Unmarshal(data, &resp) == nil {
			for _, u := range resp.Units {
				if u.Category == "naval" {
					ships = true
				}
			}
		}
	}
	if data, err := dossierGet(c, "/api/v1/worlds/%s/cities", worldID); err == nil {
		var cities []map[string]any
		if json.Unmarshal(data, &cities) == nil {
			for _, e := range cities {
				if isOwn, _ := e["own"].(bool); isOwn {
					if cs, _ := e["coastal"].(bool); cs {
						coastal = true
					}
				}
			}
		}
	}
	if !ships && !coastal {
		return []string{"None. You have no ship and no coastal city."}
	}
	data, err := dossierGet(c, "/api/v1/worlds/%s/storms", worldID)
	if err != nil {
		return []string{"Unavailable: " + err.Error()}
	}
	var probe struct {
		Storms []json.RawMessage `json:"storms"`
	}
	if json.Unmarshal(data, &probe) == nil && len(probe.Storms) == 0 {
		return []string{"None. A storm shows only inside your sight; unseen storms may still be out at sea."}
	}
	text, err := formatStorms(data, c)
	if err != nil {
		return []string{"Unavailable: " + err.Error()}
	}
	var out []string
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		out = append(out, "- "+l)
	}
	return out
}

// ---- actions ------------------------------------------------------------------

func dossierActions(c *Client, worldID string, cities []dossierCity) []string {
	if len(cities) == 0 {
		return []string{"None. Found your city first: `keryx founding settle`."}
	}
	capital := cities[0]
	for _, ct := range cities {
		if ct.Capital {
			capital = ct
			break
		}
	}
	data, err := dossierGet(c, "/api/v1/worlds/%s/provinces/%s/actions", worldID, capital.ProvinceID)
	if err != nil {
		return []string{"Unavailable: " + err.Error()}
	}
	var verbs []actionVerb
	if err := json.Unmarshal(data, &verbs); err != nil {
		return []string{"Unavailable: " + err.Error()}
	}
	out := []string{fmt.Sprintf("From %s. Details: `keryx actions <category>`.", capital.Name)}
	for _, cat := range categoryOrder {
		var avail []string
		locked := 0
		for _, v := range verbs {
			if v.Category != cat {
				continue
			}
			if v.Available {
				avail = append(avail, v.Name)
			} else {
				locked++
			}
		}
		sort.Strings(avail)
		if len(avail) == 0 && locked == 0 {
			continue
		}
		list := "none available"
		if len(avail) > 0 {
			list = strings.Join(avail, ", ")
		}
		out = append(out, fmt.Sprintf("- %s: %s · %d locked", cat, list, locked))
	}
	return out
}

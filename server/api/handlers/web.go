package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"path/filepath"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// kharisToMood maps the 0-100 kharis level to its English display mood.
// Same canonical thresholds as internal/kharis.deriveMood (religion.MoodFavorable/
// MoodIndifferent/MoodSuspicious, 60/30/5) — kept as two functions (Go/Swedish
// label sets, different packages per the G1 dependency order) but ONE
// threshold table, per the 2026-07-09 kharis omdesign's "Ta bort ev. dubbel
// skala" instruction.
func kharisToMood(k float64) string {
	switch {
	case k >= religion.MoodFavorable:
		return "Favorable"
	case k >= religion.MoodIndifferent:
		return "Indifferent"
	case k >= religion.MoodSuspicious:
		return "Suspicious"
	default:
		return "Wrathful"
	}
}

// WebHandler renders HTMX-powered HTML pages.
type WebHandler struct {
	pool        *pgxpool.Pool
	authSvc     *auth.Service
	base        *template.Template // base.html only, cloned per request
	templateDir string
	mapFile     string // static/map.html — served directly, no Go templating (FAS 1 frikoppling)
	clk         clock.Clock
}

// NewWebHandler creates a WebHandler. Only base.html is pre-parsed; page
// templates are parsed fresh per request so each gets its own "content" block.
func NewWebHandler(pool *pgxpool.Pool, authSvc *auth.Service, templateDir string, staticDir string, clk clock.Clock) (*WebHandler, error) {
	buildingNames := map[string]string{
		"farm":        "Farm",
		"lumbermill":  "Lumbermill",
		"stonequarry": "Stone Quarry",
		"mine":        "Mine",
		"barracks":    "Barracks",
		"market":      "Market",
		"wall":        "Wall",
		"harbour":     "Harbour",
		"foundry":     "Foundry",
		"stable":      "Stable",
		"temple":      "Temple",
		"olive_press": "Olive Press",
		"winery":      "Winery",
	}
	funcs := template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04")
		},
		"formatISO": func(t time.Time) string {
			return t.UTC().Format(time.RFC3339)
		},
		"resource": func(v float64) string {
			if v >= 1000 {
				return fmt.Sprintf("%.1fk", v/1000) //nolint
			}
			return fmt.Sprintf("%.0f", v)
		},
		"rate": func(v float64) string {
			if v == 0 {
				return "—"
			}
			return fmt.Sprintf("+%.1f/m", v)
		},
		"buildingName": func(key string) string {
			if n, ok := buildingNames[key]; ok {
				return n
			}
			return key
		},
		"unitName": unit.DisplayName,
		"mul":      func(a, b float64) float64 { return a * b },
		"now": func() string {
			return clk.Now().UTC().Format(time.RFC3339)
		},
		// fmtSilver formats a silver amount as DECIMAL silver (Timothy
		// 2026-08-13: sexagesimal shekel/mina/talang retired — count in silver
		// and decimal fractions). Matches the client's ui/format.js fmtSilver:
		// one decimal, trailing ".0" dropped. Presentation only — the model was
		// always plain float64 silver.
		"fmtSilver": func(v float64) string {
			r := math.Round(v*10) / 10
			if r == math.Trunc(r) {
				return fmt.Sprintf("%d silver", int64(r))
			}
			return fmt.Sprintf("%.1f silver", r)
		},
	}
	// Parse base into the base set. Page templates are parsed per-request via
	// Clone so each gets its own "content" block.
	base, err := template.New("").Funcs(funcs).ParseFiles(
		filepath.Join(templateDir, "base.html"),
	)
	if err != nil {
		return nil, err
	}
	return &WebHandler{pool: pool, authSvc: authSvc, base: base, templateDir: templateDir, mapFile: filepath.Join(staticDir, "map.html"), clk: clk}, nil
}

// resolveWorldID returns the world this server currently hosts, looked up per
// request rather than frozen at boot (megaron_plan_varldsid_resolver.md): a
// reseed replaces the world row, and the web surface must follow it without a
// restart. ORDER BY created_at DESC makes the pick deterministic and prefers the
// freshest world if more than one row ever exists (reseed TRUNCATEs, so there is
// normally exactly one).
func (h *WebHandler) resolveWorldID(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := h.pool.QueryRow(ctx, `SELECT id FROM worlds ORDER BY created_at DESC LIMIT 1`).Scan(&id)
	return id, err
}

// render renders a full-page template that extends base.html.
func (h *WebHandler) render(w http.ResponseWriter, name string, data any) {
	t, err := h.base.Clone()
	if err != nil {
		slog.Error("template clone", "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if _, err = t.ParseFiles(filepath.Join(h.templateDir, name)); err != nil {
		slog.Error("template parse", "template", name, "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		slog.Error("template render error", "template", name, "err", err)
	}
}

// Index serves the login/register page.
func (h *WebHandler) Index(w http.ResponseWriter, r *http.Request) {
	h.render(w, "index.html", nil)
}

// Play is the post-login landing. Redirects to the map if the player has a
// settlement, or to the join page if they haven't entered the world yet.
func (h *WebHandler) Play(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	wid, err := h.resolveWorldID(r.Context())
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	var exists bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM settlements WHERE owner_id = $1 AND world_id = $2)
		     OR EXISTS (SELECT 1 FROM founder_phase WHERE owner_id = $1 AND world_id = $2 AND active)`,
		playerID, wid,
	).Scan(&exists)
	if exists {
		http.Redirect(w, r, "/world/"+wid.String()+"/map", http.StatusSeeOther)
		return
	}
	// No settlement. A dispossessed Wanax (lost their last city) is shown their
	// epitaph, where they rise again or leave; one who left sees it too, without
	// the choice. A Wanax who never joined goes to the join page.
	var dispossessed bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM player_world_records
		   WHERE player_id = $1 AND world_id = $2 AND status IN ('dispossessed', 'departed'))`,
		playerID, wid,
	).Scan(&dispossessed)
	if dispossessed {
		http.Redirect(w, r, "/world/"+wid.String()+"/epitaph", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/world/"+wid.String()+"/join", http.StatusSeeOther)
}

// JoinView serves the world join page — shown to new players before they have a settlement.
func (h *WebHandler) JoinView(w http.ResponseWriter, r *http.Request) {
	wid, err := h.resolveWorldID(r.Context())
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	var name, state string
	var players int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT w.name, w.state,
		        (SELECT COUNT(*) FROM settlements WHERE world_id = w.id AND owner_id IS NOT NULL)
		 FROM worlds w WHERE w.id = $1`,
		wid,
	).Scan(&name, &state, &players)

	h.render(w, "join.html", map[string]any{
		"WorldID":   wid,
		"WorldName": name,
		"State":     state,
		"Players":   players,
	})
}

// MapView serves the hex map page. map.html is a static file (FAS 1
// frikoppling — no Go templating); the client bootstraps its own state via
// the API. This handler keeps only what the redirect logic needs: 404 on an
// unknown world, and the /play redirect for an authenticated Wanax with no
// settlement here.
func (h *WebHandler) MapView(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		http.Error(w, "invalid world ID", http.StatusBadRequest)
		return
	}

	var exists bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM worlds WHERE id = $1)`, worldID,
	).Scan(&exists); err != nil || !exists {
		http.Error(w, "world not found", http.StatusNotFound)
		return
	}

	var settlementID string
	var playerIDStr string
	if playerID, ok := auth.PlayerIDFromContext(r.Context()); ok {
		playerIDStr = playerID.String()
		var sid uuid.UUID
		if err := h.pool.QueryRow(r.Context(),
			`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
			worldID, playerID,
		).Scan(&sid); err == nil {
			settlementID = sid.String()
		}
	}

	// An authenticated Wanax with no settlement here (never joined, or lost their
	// last city) must not land on a fog-only map — route them through /play, which
	// sends them to the join page or their epitaph as appropriate. EXCEPT a
	// founder: an active founder phase has no settlement by design (the host IS
	// the player's presence), and the map is where its whole surface lives —
	// without this the join → map redirect loops back to /join forever.
	if playerIDStr != "" && settlementID == "" {
		var founder bool
		_ = h.pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM founder_phase WHERE world_id = $1 AND owner_id = $2 AND active)`,
			worldID, playerIDStr,
		).Scan(&founder)
		if !founder {
			http.Redirect(w, r, "/play", http.StatusSeeOther)
			return
		}
	}

	http.ServeFile(w, r, h.mapFile)
}

// EpitaphView renders a fallen Wanax's reign as a scrolling crawl. Only a
// dispossessed player (one who lost their last settlement) has an epitaph; anyone
// else is bounced back through /play.
func (h *WebHandler) EpitaphView(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	wid, err := h.resolveWorldID(r.Context())
	if err != nil {
		http.Redirect(w, r, "/play", http.StatusSeeOther)
		return
	}

	var status string
	var lastSettlementID *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT status, last_settlement_id FROM player_world_records
		 WHERE player_id = $1 AND world_id = $2`,
		playerID, wid,
	).Scan(&status, &lastSettlementID)
	if err != nil || (status != "dispossessed" && status != "departed") {
		http.Redirect(w, r, "/play", http.StatusSeeOther)
		return
	}

	var wanax string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(wanax_name, username) FROM players WHERE id = $1`, playerID).Scan(&wanax)
	if wanax == "" {
		wanax = "an unknown Wanax"
	}

	var cityName, culture string
	if lastSettlementID != nil {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT name, culture_id FROM settlements WHERE id = $1`, *lastSettlementID,
		).Scan(&cityName, &culture)
	}

	h.render(w, "epitaph.html", map[string]any{
		"Wanax":    wanax,
		"City":     cityName,
		"Culture":  culture,
		"Lines":    h.epitaphLines(r.Context(), playerID, wid, lastSettlementID),
		"WorldID":  wid,
		"Departed": status == "departed",
		"MapMode":  true, // suppress the site nav/footer for a full-screen crawl
	})
}

// epitaphLines tells the whole reign of a fallen Wanax in this world as short
// English prose lines (Timothy 2026-10-10: the WHOLE Wanax's history, every
// reign, not only the last city's). Sources, ordered by game tick:
//   - every city the Wanax founded (settlements.founder_id), with a founding
//     line, then that city's own events — but only while the Wanax held it:
//     the stream is cut at its first fall, so a conqueror's later works never
//     enter the wrong epitaph;
//   - cities the Wanax took or burned, and those it lost without having
//     founded them (the payloads name the Wanax);
//   - the battles its units fought (battle_participants).
// A city from before founder_id existed falls back to the last city alone.
func (h *WebHandler) epitaphLines(ctx context.Context, playerID, worldID uuid.UUID, lastSettlementID *uuid.UUID) []string {
	var lines []string

	// wanaxName names the rival in a fall line; "" when the id is unknown.
	wanaxName := func(id string) string {
		var name string
		_ = h.pool.QueryRow(ctx,
			`SELECT COALESCE(wanax_name, username) FROM players WHERE id::text = $1`, id).Scan(&name)
		return name
	}

	rows, err := h.pool.Query(ctx,
		`WITH founded AS (
		     SELECT id, name, COALESCE(founded_tick, 0) AS ft, founded_from IS NOT NULL AS colony
		     FROM settlements
		     WHERE world_id = $2 AND (founder_id = $1 OR (founder_id IS NULL AND id = $3))
		 ),
		 held_until AS (
		     SELECT f.id, min(e.id) AS end_id
		     FROM founded f
		     JOIN events e ON e.stream_id = f.id
		      AND e.event_type IN ('SettlementCaptured', 'SettlementBurned', 'CityCollapsed', 'SettlementAbandoned')
		     GROUP BY f.id
		 ),
		 fought AS (
		     SELECT DISTINCT bp.battle_id, bp.side
		     FROM battle_participants bp JOIN battles b ON b.id = bp.battle_id
		     WHERE bp.owner_id = $1 AND b.world_id = $2
		 )
		 SELECT kind, payload, city FROM (
		     SELECT f.ft AS tick, 0::bigint AS seq, 'EpitaphFounded' AS kind,
		            jsonb_build_object('colony', f.colony) AS payload, f.name AS city
		     FROM founded f
		   UNION ALL
		     SELECT e.world_tick, e.id, e.event_type, e.payload, f.name
		     FROM founded f
		     JOIN events e ON e.stream_id = f.id
		     LEFT JOIN held_until hu ON hu.id = f.id
		     WHERE e.event_type = ANY($4) AND (hu.end_id IS NULL OR e.id <= hu.end_id)
		   UNION ALL
		     SELECT e.world_tick, e.id, e.event_type, e.payload, s.name
		     FROM events e JOIN settlements s ON s.id = e.stream_id
		     WHERE e.world_id = $2 AND s.id NOT IN (SELECT id FROM founded) AND (
		           (e.event_type = 'SettlementCaptured' AND $1::text IN (e.payload->>'new_owner', e.payload->>'former_owner'))
		        OR (e.event_type = 'SettlementBurned' AND $1::text IN (e.payload->>'raider_id', e.payload->>'former_owner'))
		        OR (e.event_type = 'CityCollapsed' AND e.payload->>'owner_id' = $1::text))
		   UNION ALL
		     SELECT e.world_tick, e.id, 'EpitaphBattle',
		            jsonb_build_object('outcome', CASE WHEN COALESCE(e.payload->>'winner', '') = '' THEN 'none'
		                                               WHEN e.payload->>'winner' = f.side THEN 'won' ELSE 'lost' END),
		            COALESCE(s.name, '')
		     FROM fought f
		     JOIN events e ON e.stream_id = f.battle_id AND e.event_type = 'BattleEnded'
		     LEFT JOIN provinces p ON p.world_id = $2
		      AND p.map_q = (e.payload->>'q')::int AND p.map_r = (e.payload->>'r')::int
		     LEFT JOIN settlements s ON s.province_id = p.id
		 ) reign
		 ORDER BY tick, seq
		 LIMIT 500`,
		playerID, worldID, lastSettlementID, epitaphEventTypes,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var kind, city string
			var payload []byte
			if rows.Scan(&kind, &payload, &city) != nil {
				continue
			}
			if line := epitaphLine(kind, payload, city, wanaxName); line != "" {
				lines = append(lines, line)
			}
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "A Wanax rose on the shore of the Thalassa.")
	}
	lines = append(lines, "So ended a Wanax's reign.")
	return lines
}

// epitaphEventTypes are the event types epitaphLine renders; epitaphLines reads
// only these from the stream. Keep the two in step.
var epitaphEventTypes = []string{
	"BuildComplete", "TrainComplete", "CombatResolved", "UnitCombatResolved",
	"DivineBlessing", "DivinePunishment", "CityCollapsed",
	"SettlementCaptured", "SettlementBurned",
}

// epitaphLine renders one reign-worthy event into an English crawl line, or "" to
// skip it. Deliberately narrow — only the beats that read as a story (buildings
// raised, armies mustered, battles, divine favour, the fall) earn a line.
// wanaxName resolves a player id from the payload to a name for the fall lines.
func epitaphLine(eventType string, payload []byte, city string, wanaxName func(string) string) string {
	var p map[string]any
	_ = json.Unmarshal(payload, &p)
	str := func(k string) string {
		if v, ok := p[k].(string); ok {
			return v
		}
		return ""
	}
	switch eventType {
	case "EpitaphFounded":
		if p["colony"] == true {
			return "Founded " + city + "."
		}
		return city + " rose on the shore of the Thalassa."
	case "EpitaphBattle":
		place := " in the field."
		if city != "" {
			place = " at " + city + "."
		}
		switch str("outcome") {
		case "won":
			return "Won a battle" + place
		case "lost":
			return "Lost a battle" + place
		default:
			return "A battle" + place[:len(place)-1] + " left no one standing."
		}
	case "BuildComplete":
		if b := str("building_type"); b != "" {
			return "Raised " + b + " in " + city + "."
		}
		return "A building was raised in " + city + "."
	case "TrainComplete":
		if u := str("unit_type"); u != "" {
			return "Mustered " + u + " from " + city + "."
		}
		return "Mustered a host from " + city + "."
	case "CombatResolved", "UnitCombatResolved":
		return "The winds of war swept over " + city + "."
	case "DivineBlessing":
		return "The gods smiled upon " + city + "."
	case "DivinePunishment":
		return "The gods turned away from " + city + "."
	case "SettlementCaptured":
		if n := wanaxName(str("new_owner")); n != "" {
			return "Wanax " + n + " took " + city + "."
		}
		return "A rival Wanax took " + city + "."
	case "SettlementBurned":
		if n := wanaxName(str("raider_id")); n != "" {
			return "Wanax " + n + " burned " + city + "."
		}
		return city + " was burned."
	case "CityCollapsed":
		switch str("cause") {
		case "starvation":
			return "Hunger came. " + city + " fell."
		case "overmobilisation":
			return "The armies emptied " + city + ". The city fell."
		default:
			return city + " fell."
		}
	default:
		return ""
	}
}

// Logout clears the auth cookie and returns to the start screen. The epitaph's
// "Begin anew" button hits this after clearing localStorage client-side.
func (h *WebHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "poleia_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

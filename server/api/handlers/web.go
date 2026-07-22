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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/unit"
)

// kharisToMood maps the 0-100 kharis level to its English display mood.
// Same canonical thresholds as internal/kharis.deriveMood (60/30/10, strawman —
// temenos_balans_spakar.md §9) — kept as two functions (Go/Swedish label sets,
// different packages per the G1 dependency order) but ONE threshold table, per
// the 2026-07-09 kharis omdesign's "Ta bort ev. dubbel skala" instruction.
func kharisToMood(k float64) string {
	switch {
	case k >= 60:
		return "Favorable"
	case k >= 30:
		return "Indifferent"
	case k >= 10:
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
	worldID     uuid.UUID // the single world this server hosts
}

// NewWebHandler creates a WebHandler. Only base.html is pre-parsed; page
// templates are parsed fresh per request so each gets its own "content" block.
func NewWebHandler(pool *pgxpool.Pool, authSvc *auth.Service, templateDir string, staticDir string, clk clock.Clock, worldID uuid.UUID) (*WebHandler, error) {
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
		// fmtSilver formats a silver amount as shekel / mina / talang
		// (1 mina = 60 shekel, 1 talang = 60 mina = 3600 shekel)
		"fmtSilver": func(v float64) string {
			n := int(math.Round(v))
			if n <= 0 {
				return "0 shekel"
			}
			if n < 60 {
				return fmt.Sprintf("%d shekel", n)
			}
			if n < 3600 {
				mina, shekel := n/60, n%60
				if shekel == 0 {
					return fmt.Sprintf("%d mina", mina)
				}
				return fmt.Sprintf("%d mina %d shekel", mina, shekel)
			}
			talang, rest := n/3600, n%3600
			mina := rest / 60
			if mina == 0 {
				return fmt.Sprintf("%d talang", talang)
			}
			return fmt.Sprintf("%d talang %d mina", talang, mina)
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
	return &WebHandler{pool: pool, authSvc: authSvc, base: base, templateDir: templateDir, mapFile: filepath.Join(staticDir, "map.html"), clk: clk, worldID: worldID}, nil
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
	var exists bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM settlements WHERE owner_id = $1 AND world_id = $2)
		     OR EXISTS (SELECT 1 FROM founder_phase WHERE owner_id = $1 AND world_id = $2 AND active)`,
		playerID, h.worldID,
	).Scan(&exists)
	if exists {
		http.Redirect(w, r, "/world/"+h.worldID.String()+"/map", http.StatusSeeOther)
		return
	}
	// No settlement. A dispossessed Wanax (lost their last city) is shown their
	// epitaph; a Wanax who never joined goes to the join page.
	var dispossessed bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM player_world_records
		   WHERE player_id = $1 AND world_id = $2 AND status = 'dispossessed')`,
		playerID, h.worldID,
	).Scan(&dispossessed)
	if dispossessed {
		http.Redirect(w, r, "/world/"+h.worldID.String()+"/epitaph", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/world/"+h.worldID.String()+"/join", http.StatusSeeOther)
}

// JoinView serves the world join page — shown to new players before they have a settlement.
func (h *WebHandler) JoinView(w http.ResponseWriter, r *http.Request) {
	var name, state string
	var players int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT w.name, w.state,
		        (SELECT COUNT(*) FROM settlements WHERE world_id = w.id AND owner_id IS NOT NULL)
		 FROM worlds w WHERE w.id = $1`,
		h.worldID,
	).Scan(&name, &state, &players)

	h.render(w, "join.html", map[string]any{
		"WorldID":   h.worldID,
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

	var status string
	var lastSettlementID *uuid.UUID
	err := h.pool.QueryRow(r.Context(),
		`SELECT status, last_settlement_id FROM player_world_records
		 WHERE player_id = $1 AND world_id = $2`,
		playerID, h.worldID,
	).Scan(&status, &lastSettlementID)
	if err != nil || status != "dispossessed" {
		http.Redirect(w, r, "/play", http.StatusSeeOther)
		return
	}

	var wanax string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT username FROM players WHERE id = $1`, playerID).Scan(&wanax)
	if wanax == "" {
		wanax = "en okänd Wanax"
	}

	var cityName, culture string
	if lastSettlementID != nil {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT name, culture_id FROM settlements WHERE id = $1`, *lastSettlementID,
		).Scan(&cityName, &culture)
	}

	h.render(w, "epitaph.html", map[string]any{
		"Wanax":   wanax,
		"City":    cityName,
		"Culture": culture,
		"Lines":   h.epitaphLines(r.Context(), lastSettlementID, cityName),
		"WorldID": h.worldID,
		"MapMode": true, // suppress the site nav/footer for a full-screen crawl
	})
}

// epitaphLines reconstructs a fallen Wanax's reign as short Swedish prose lines,
// drawn from the fallen capital's own event stream (stream_id = settlementID). The
// founding and closing lines are synthesized — the event log carries no explicit
// "settlement founded" event — so the crawl always has a beginning and an end even
// for a very short reign.
func (h *WebHandler) epitaphLines(ctx context.Context, settlementID *uuid.UUID, cityName string) []string {
	city := cityName
	if city == "" {
		city = "staden"
	}
	lines := []string{city + " restes vid Thalassas kust."}

	if settlementID != nil {
		rows, err := h.pool.Query(ctx,
			`SELECT event_type, payload FROM events
			 WHERE stream_id = $1
			 ORDER BY id ASC
			 LIMIT 200`,
			*settlementID,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var eventType string
				var payload []byte
				if rows.Scan(&eventType, &payload) != nil {
					continue
				}
				if line := epitaphLine(eventType, payload, city); line != "" {
					lines = append(lines, line)
				}
			}
		}
	}

	lines = append(lines, "Så slutade en Wanax' regeringstid.")
	return lines
}

// epitaphLine renders one reign-worthy event into a Swedish crawl line, or "" to
// skip it. Deliberately narrow — only the beats that read as a story (buildings
// raised, armies mustered, battles, divine favour, the fall) earn a line.
func epitaphLine(eventType string, payload []byte, city string) string {
	var p map[string]any
	_ = json.Unmarshal(payload, &p)
	str := func(k string) string {
		if v, ok := p[k].(string); ok {
			return v
		}
		return ""
	}
	switch eventType {
	case "BuildComplete":
		if b := str("building_type"); b != "" {
			return "Reste " + b + " i " + city + "."
		}
		return "Ett byggnadsverk restes i " + city + "."
	case "TrainComplete":
		if u := str("unit_type"); u != "" {
			return "Mönstrade " + u + " ur " + city + "."
		}
		return "Mönstrade en här ur " + city + "."
	case "CombatResolved", "UnitCombatResolved":
		return "Krigets vindar drog över " + city + "."
	case "DivineBlessing":
		return "Gudarna log mot " + city + "."
	case "DivinePunishment":
		return "Gudarna vände sig från " + city + "."
	case "CityCollapsed":
		switch str("cause") {
		case "starvation":
			return "Hungern kom. " + city + " föll."
		case "overmobilisation":
			return "Härarna tömde " + city + ". Staden föll."
		default:
			return city + " föll."
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

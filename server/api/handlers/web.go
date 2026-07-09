package handlers

import (
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
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/world"
)

func kharisToMood(k float64) string {
	switch {
	case k >= 800:
		return "Favorable"
	case k >= 400:
		return "Indifferent"
	case k >= 100:
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
	clk         clock.Clock
	worldID     uuid.UUID // the single world this server hosts
}

// NewWebHandler creates a WebHandler. Only base.html is pre-parsed; page
// templates are parsed fresh per request so each gets its own "content" block.
func NewWebHandler(pool *pgxpool.Pool, authSvc *auth.Service, templateDir string, clk clock.Clock, worldID uuid.UUID) (*WebHandler, error) {
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
	unitNames := map[string]string{
		"spearman":       "Hoplites",
		"war_chariot":    "War Chariot",
		"priest":         "Hiereus",
		"ship":           "Galley",
		"elite_infantry": "Agema",
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
		"unitName": func(key string) string {
			if n, ok := unitNames[key]; ok {
				return n
			}
			return key
		},
		"mul": func(a, b float64) float64 { return a * b },
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
	return &WebHandler{pool: pool, authSvc: authSvc, base: base, templateDir: templateDir, clk: clk, worldID: worldID}, nil
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
		`SELECT EXISTS (SELECT 1 FROM settlements WHERE owner_id = $1 AND world_id = $2)`,
		playerID, h.worldID,
	).Scan(&exists)
	if !exists {
		http.Redirect(w, r, "/world/"+h.worldID.String()+"/join", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/world/"+h.worldID.String()+"/map", http.StatusSeeOther)
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

// MapView serves the hex map page.
func (h *WebHandler) MapView(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		http.Error(w, "invalid world ID", http.StatusBadRequest)
		return
	}

	var wld world.World
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, state, map_width, map_height, prestige, era_number, created_at FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wld.ID, &wld.Name, &wld.State, &wld.MapWidth, &wld.MapHeight, &wld.Prestige, &wld.EraNumber, &wld.CreatedAt)
	if err != nil {
		http.Error(w, "world not found", http.StatusNotFound)
		return
	}

	var settlementID string
	var playerIDStr string
	var unreadCount int
	if playerID, ok := auth.PlayerIDFromContext(r.Context()); ok {
		playerIDStr = playerID.String()
		var sid uuid.UUID
		if err := h.pool.QueryRow(r.Context(),
			`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
			worldID, playerID,
		).Scan(&sid); err == nil {
			settlementID = sid.String()
		}
		_ = h.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM messengers m
			 JOIN settlements s ON s.id = m.destination_id
			 WHERE m.world_id = $1 AND s.owner_id = $2 AND m.status = 'delivered'
			   AND (m.trade_offer IS NULL OR m.trade_offer->>'status' = 'pending')`,
			worldID, playerID,
		).Scan(&unreadCount)
	}

	h.render(w, "map.html", map[string]any{
		"World":          wld,
		"WorldID":        worldID,
		"WorldCreatedAt": wld.CreatedAt.UTC().Format(time.RFC3339),
		"TimeScale":      1,
		"SettlementID":   settlementID,
		"PlayerID":       playerIDStr,
		"UnreadCount":    unreadCount,
		"MapMode":        true,
	})
}

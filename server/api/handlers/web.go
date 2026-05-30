package handlers

import (
	"fmt"
	"html/template"
	"log/slog"
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
}

// NewWebHandler creates a WebHandler. Only base.html is pre-parsed; page
// templates are parsed fresh per request so each gets its own "content" block.
func NewWebHandler(pool *pgxpool.Pool, authSvc *auth.Service, templateDir string, clk clock.Clock) (*WebHandler, error) {
	buildingNames := map[string]string{
		"farm":        "Farm",
		"lumbermill":  "Lumbermill",
		"stonequarry": "Stone Quarry",
		"mine":        "Mine",
		"barracks":    "Barracks",
		"market":      "Market",
		"wall":        "Wall",
		"tower":       "Tower",
		"harbour":     "Harbour",
		"foundry":     "Foundry",
		"stable":      "Stable",
		"bronze_wall": "Bronze Wall",
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
	}
	// Parse base + all partials (named templates used across pages) into the base set.
	// Page templates are parsed per-request via Clone so each gets its own "content" block.
	base, err := template.New("").Funcs(funcs).ParseFiles(
		filepath.Join(templateDir, "base.html"),
		filepath.Join(templateDir, "resource_bar.html"),
	)
	if err != nil {
		return nil, err
	}
	return &WebHandler{pool: pool, authSvc: authSvc, base: base, templateDir: templateDir, clk: clk}, nil
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

// renderPartial renders a named partial (no base wrapper — used for HTMX fragments).
func (h *WebHandler) renderPartial(w http.ResponseWriter, name string, data any) {
	t, err := h.base.Clone()
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if _, err = t.ParseFiles(filepath.Join(h.templateDir, name)); err != nil {
		slog.Error("template parse", "template", name, "err", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("template render error", "template", name, "err", err)
	}
}

// Index serves the login/register page.
func (h *WebHandler) Index(w http.ResponseWriter, r *http.Request) {
	h.render(w, "index.html", nil)
}

// Play is the post-login landing: redirects to the player's map if they have a
// settlement, or to the worlds list if they haven't joined one yet.
func (h *WebHandler) Play(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	var worldID uuid.UUID
	err := h.pool.QueryRow(r.Context(),
		`SELECT world_id FROM settlements WHERE owner_id = $1 ORDER BY created_at DESC LIMIT 1`,
		playerID,
	).Scan(&worldID)
	if err != nil {
		http.Redirect(w, r, "/worlds", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/world/"+worldID.String()+"/map", http.StatusSeeOther)
}

// WorldList serves the list of active worlds.
func (h *WebHandler) WorldList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, state, prestige, era_number,
		        (SELECT COUNT(*) FROM settlements WHERE world_id = worlds.id AND owner_id IS NOT NULL) AS players,
		        created_at
		 FROM worlds ORDER BY created_at DESC`,
	)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type row struct {
		ID        uuid.UUID
		Name      string
		State     string
		Prestige  int
		EraNumber int
		Players   int
		CreatedAt time.Time
	}
	var worlds []row
	for rows.Next() {
		var wr row
		if err := rows.Scan(&wr.ID, &wr.Name, &wr.State, &wr.Prestige, &wr.EraNumber, &wr.Players, &wr.CreatedAt); err != nil {
			continue
		}
		worlds = append(worlds, wr)
	}
	h.render(w, "worlds.html", map[string]any{"Worlds": worlds})
}

// Province serves the main province view (resources, army, build queue).
// The template uses Province.ID as the province ID for URL construction.
func (h *WebHandler) Province(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		http.Error(w, "invalid world ID", http.StatusBadRequest)
		return
	}

	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s, err := loadPlayerCapital(r.Context(), h.pool, playerID, worldID)
	if err != nil {
		h.render(w, "no_province.html", map[string]any{"WorldID": worldID})
		return
	}

	now := h.clk.Now()
	resources := s.Resources.Snapshot(now)
	resources["gold_rate"] = s.Resources.Gold.RatePerMinute
	resources["food_rate"] = s.Resources.Food.RatePerMinute
	resources["lumber_rate"] = s.Resources.Lumber.RatePerMinute
	resources["stone_rate"] = s.Resources.Stone.RatePerMinute
	resources["iron_rate"] = s.Resources.Iron.RatePerMinute
	resources["kharis_rate"] = s.Resources.Kharis.RatePerMinute

	divineMood := kharisToMood(resources["kharis"])

	// province.html uses .Province.ID in URLs — pass province_id as the ID.
	// Province is the settlement struct, but with ID = province tile ID.
	var copperDeposit, tinDeposit bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT copper_deposit, tin_deposit FROM provinces WHERE id = $1`,
		s.ProvinceID,
	).Scan(&copperDeposit, &tinDeposit)

	var kingdomName string
	if s.KingdomID != nil {
		_ = h.pool.QueryRow(r.Context(), `SELECT name FROM kingdoms WHERE id = $1`, s.KingdomID).Scan(&kingdomName)
	}

	type provinceView struct {
		ID            uuid.UUID // province_id for URL routing
		SettlementID  uuid.UUID // settlement UUID for cult-level and settlement API calls
		WorldID       uuid.UUID
		Name          string
		CultureID     string
		Army          any
		Population    int
		Walls         int
		Loyalty       int
		KingdomID     *uuid.UUID
		KingdomName   string
		CopperDeposit bool
		TinDeposit    bool
	}
	pv := provinceView{
		ID:            s.ProvinceID,
		SettlementID:  s.ID,
		WorldID:       worldID,
		Name:          s.Name,
		CultureID:     string(s.CultureID),
		Army:          s.Army,
		Population:    s.Population,
		Walls:         s.WallLevel,
		Loyalty:       s.Loyalty,
		KingdomID:     s.KingdomID,
		KingdomName:   kingdomName,
		CopperDeposit: copperDeposit,
		TinDeposit:    tinDeposit,
	}

	// Load build queue.
	qrows, _ := h.pool.Query(r.Context(),
		`SELECT building_type, complete_at FROM build_queue
		 WHERE settlement_id = $1 ORDER BY complete_at`,
		s.ID,
	)
	defer qrows.Close()
	type buildItem struct {
		Type       string
		CompleteAt time.Time
	}
	var queue []buildItem
	for qrows.Next() {
		var bi buildItem
		_ = qrows.Scan(&bi.Type, &bi.CompleteAt)
		queue = append(queue, bi)
	}

	// Load marching armies — join settlements so we can show the target name.
	mrows, _ := h.pool.Query(r.Context(),
		`SELECT ma.id, ma.target_id, COALESCE(s.name, ma.target_id::text),
		        ma.infantry, ma.cavalry, ma.catapult, ma.priest, ma.ship, ma.intent, ma.arrives_at
		 FROM marching_armies ma
		 LEFT JOIN settlements s ON s.province_id = ma.target_id
		 WHERE ma.origin_id = $1 AND ma.resolved = false ORDER BY ma.arrives_at`,
		s.ProvinceID,
	)
	defer mrows.Close()
	type marchItem struct {
		ID         uuid.UUID
		TargetID   uuid.UUID
		TargetName string
		Infantry   int
		Cavalry    int
		Intent     string
		ArrivesAt  time.Time
	}
	var marches []marchItem
	for mrows.Next() {
		var mi marchItem
		var cat, pri, ship int
		_ = mrows.Scan(&mi.ID, &mi.TargetID, &mi.TargetName, &mi.Infantry, &mi.Cavalry, &cat, &pri, &ship, &mi.Intent, &mi.ArrivesAt)
		marches = append(marches, mi)
	}

	// Load incoming armies — join through settlements for name.
	irows, _ := h.pool.Query(r.Context(),
		`SELECT ma.origin_id, s.name, ma.infantry+ma.cavalry+ma.catapult+ma.priest AS total, ma.intent, ma.arrives_at
		 FROM marching_armies ma
		 JOIN settlements s ON s.province_id = ma.origin_id
		 WHERE ma.target_id = $1 AND ma.resolved = false ORDER BY ma.arrives_at`,
		s.ProvinceID,
	)
	defer irows.Close()
	type incomingItem struct {
		OriginID   uuid.UUID
		Name       string
		TotalUnits int
		Intent     string
		ArrivesAt  time.Time
	}
	var incoming []incomingItem
	for irows.Next() {
		var ii incomingItem
		_ = irows.Scan(&ii.OriginID, &ii.Name, &ii.TotalUnits, &ii.Intent, &ii.ArrivesAt)
		incoming = append(incoming, ii)
	}

	// Load completed buildings.
	bldRows, _ := h.pool.Query(r.Context(),
		`SELECT building_type, level FROM buildings WHERE settlement_id = $1 ORDER BY building_type`,
		s.ID,
	)
	defer bldRows.Close()
	type buildingItem struct {
		Type  string
		Level int
	}
	var buildings []buildingItem
	for bldRows.Next() {
		var bi buildingItem
		_ = bldRows.Scan(&bi.Type, &bi.Level)
		buildings = append(buildings, bi)
	}

	built := make(map[string]bool)
	for _, b := range buildings {
		built[b.Type] = true
	}

	h.render(w, "province.html", map[string]any{
		"Province":   pv,
		"Resources":  resources,
		"Queue":      queue,
		"Marches":    marches,
		"Incoming":   incoming,
		"Buildings":  buildings,
		"Built":      built,
		"WorldID":    worldID,
		"Now":        now,
		"DivineMood": divineMood,
	})
}

// ResourceBar handles HTMX partial refresh of the resource bar.
func (h *WebHandler) ResourceBar(w http.ResponseWriter, r *http.Request) {
	worldID, _ := uuid.Parse(chi.URLParam(r, "worldID"))
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s, err := loadPlayerCapital(r.Context(), h.pool, playerID, worldID)
	if err != nil {
		http.Error(w, "no settlement", http.StatusNotFound)
		return
	}
	resources := s.Resources.Snapshot(h.clk.Now())
	resources["gold_rate"] = s.Resources.Gold.RatePerMinute
	resources["food_rate"] = s.Resources.Food.RatePerMinute
	resources["lumber_rate"] = s.Resources.Lumber.RatePerMinute
	resources["stone_rate"] = s.Resources.Stone.RatePerMinute
	resources["iron_rate"] = s.Resources.Iron.RatePerMinute
	resources["kharis_rate"] = s.Resources.Kharis.RatePerMinute

	h.renderPartial(w, "resource_bar.html", map[string]any{"Resources": resources, "Province": s, "DivineMood": kharisToMood(resources["kharis"])})
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
		`SELECT id, name, state, map_width, map_height FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wld.ID, &wld.Name, &wld.State, &wld.MapWidth, &wld.MapHeight)
	if err != nil {
		http.Error(w, "world not found", http.StatusNotFound)
		return
	}

	var settlementID string
	if playerID, ok := auth.PlayerIDFromContext(r.Context()); ok {
		var sid uuid.UUID
		if err := h.pool.QueryRow(r.Context(),
			`SELECT id FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
			worldID, playerID,
		).Scan(&sid); err == nil {
			settlementID = sid.String()
		}
	}

	h.render(w, "map.html", map[string]any{
		"World":        wld,
		"WorldID":      worldID,
		"SettlementID": settlementID,
	})
}

// KingdomView serves the kingdom overview page.
func (h *WebHandler) KingdomView(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		http.Error(w, "invalid world ID", http.StatusBadRequest)
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var kingdomID *uuid.UUID
	var kingdomName, playerRole string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT k.id, k.name, km.role FROM kingdoms k
		 JOIN kingdom_members km ON km.kingdom_id = k.id
		 WHERE k.world_id = $1 AND km.player_id = $2`,
		worldID, playerID,
	).Scan(&kingdomID, &kingdomName, &playerRole)

	var settlementID uuid.UUID
	var settlementName string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT id, name FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, playerID,
	).Scan(&settlementID, &settlementName)

	h.render(w, "kingdom.html", map[string]any{
		"WorldID":        worldID,
		"KingdomID":      kingdomID,
		"KingdomName":    kingdomName,
		"PlayerRole":     playerRole,
		"HasKingdom":     kingdomID != nil,
		"SettlementID":   settlementID,
		"SettlementName": settlementName,
	})
}

// WanaxesView serves the world leaderboard — all settlements sorted by army strength.
func (h *WebHandler) WanaxesView(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		http.Error(w, "invalid world ID", http.StatusBadRequest)
		return
	}
	playerID, _ := auth.PlayerIDFromContext(r.Context())

	var worldName string
	_ = h.pool.QueryRow(r.Context(), `SELECT name FROM worlds WHERE id = $1`, worldID).Scan(&worldName)

	rows, err := h.pool.Query(r.Context(),
		`SELECT s.name, p.username, s.culture_id,
		        (SELECT k.name FROM kingdoms k
		         JOIN kingdom_members km ON km.kingdom_id = k.id
		         WHERE km.player_id = s.owner_id AND k.world_id = $1 LIMIT 1),
		        s.wall_level, s.population,
		        s.infantry + s.cavalry*3 + s.elite_infantry*2 + s.priest*2,
		        s.owner_id
		 FROM settlements s
		 LEFT JOIN players p ON p.id = s.owner_id
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		 ORDER BY s.infantry + s.cavalry*3 + s.elite_infantry*2 + s.priest*2 DESC`,
		worldID,
	)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type row struct {
		Name       string
		Owner      string
		Culture    string
		Kingdom    string
		Walls      int
		Population int
		ArmyTotal  int
		Own        bool
	}
	var settlements []row
	for rows.Next() {
		var r row
		var ownerID *uuid.UUID
		var kingdom *string
		if err := rows.Scan(&r.Name, &r.Owner, &r.Culture, &kingdom, &r.Walls, &r.Population, &r.ArmyTotal, &ownerID); err != nil {
			continue
		}
		if kingdom != nil {
			r.Kingdom = *kingdom
		}
		if ownerID != nil && *ownerID == playerID {
			r.Own = true
		}
		settlements = append(settlements, r)
	}

	h.render(w, "wanaxes.html", map[string]any{
		"WorldID":     worldID,
		"WorldName":   worldName,
		"Settlements": settlements,
	})
}

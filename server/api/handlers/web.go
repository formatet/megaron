package handlers

import (
	"context"
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
	"github.com/poleia/server/internal/settlement"
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
		"tower":       "Tower",
		"harbour":     "Harbour",
		"foundry":     "Foundry",
		"stable":      "Stable",
		"bronze_wall": "Bronze Wall",
		"olive_press": "Olive Press",
		"winery":      "Winery",
	}
	unitNames := map[string]string{
		"infantry":       "Hoplites",
		"cavalry":        "Hippeis",
		"priest":         "Hiereus",
		"ship":           "Trireme",
		"elite_infantry": "Agema",
		"catapult":       "Siege",
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
			return time.Now().UTC().Format(time.RFC3339)
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
	// Parse base + all partials (named templates used across pages) into the base set.
	// Page templates are parsed per-request via Clone so each gets its own "content" block.
	base, err := template.New("").Funcs(funcs).ParseFiles(
		filepath.Join(templateDir, "base.html"),
		filepath.Join(templateDir, "resource_bar.html"),
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

// Play is the post-login landing. Redirects to the megaron hub if the player
// has a settlement, or to the join page if they haven't entered the world yet.
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
	http.Redirect(w, r, "/world/"+h.worldID.String()+"/megaron", http.StatusSeeOther)
}

// MegaronView serves the hub page — the great hall from which all rooms are entered.
func (h *WebHandler) MegaronView(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/world/"+worldID.String()+"/join", http.StatusSeeOther)
		return
	}

	now := h.clk.Now()
	resources := s.Resources.Snapshot(now)
	loadSettlementGoodsIntoResources(r.Context(), h.pool, s.ID, now, resources, "grain", "silver")

	// Kharis is per-Wanax, not per-settlement.
	if kh, err := loadPlayerKharis(r.Context(), h.pool, playerID, worldID); err == nil {
		resources["kharis"] = kh.Amount
	}

	var wld world.World
	_ = h.pool.QueryRow(r.Context(),
		`SELECT id, name, state, map_width, map_height, prestige, era_number FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wld.ID, &wld.Name, &wld.State, &wld.MapWidth, &wld.MapHeight, &wld.Prestige, &wld.EraNumber)

	var kingdomName, kingdomState string
	var memberCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT k.name, COALESCE(k.state,'forming'),
		        (SELECT count(*) FROM kingdom_members WHERE kingdom_id = k.id)
		 FROM kingdoms k JOIN kingdom_members km ON km.kingdom_id = k.id
		 WHERE k.world_id = $1 AND km.player_id = $2`,
		worldID, playerID,
	).Scan(&kingdomName, &kingdomState, &memberCount)

	var unreadCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT count(*) FROM messengers m
		 JOIN settlements s ON s.id = m.destination_id
		 WHERE m.world_id = $1 AND s.owner_id = $2 AND m.status = 'delivered'
		   AND (m.trade_offer IS NULL OR m.trade_offer->>'status' = 'pending')`,
		worldID, playerID,
	).Scan(&unreadCount)

	var marchCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT count(*) FROM marching_armies WHERE origin_id = $1 AND resolved = false`,
		s.ProvinceID,
	).Scan(&marchCount)

	var tradeCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT count(*) FROM trade_routes WHERE origin_settlement_id = $1 AND resolved = false`,
		s.ID,
	).Scan(&tradeCount)

	armyDP := s.Army.Infantry + s.Army.EliteInfantry*2 + s.Army.Cavalry*3

	h.render(w, "megaron.html", map[string]any{
		"WorldID":      worldID,
		"World":        wld,
		"Settlement":   s,
		"ArmyDP":       armyDP,
		"Grain":        resources["grain"],
		"GrainRate":    resources["grain_rate"],
		"Silver":       resources["silver"],
		"DivineMood":   kharisToMood(resources["kharis"]),
		"KingdomName":  kingdomName,
		"KingdomState": kingdomState,
		"MemberCount":  memberCount,
		"UnreadCount":  unreadCount,
		"MarchCount":   marchCount,
		"TradeCount":   tradeCount,
	})
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

	// ?sid= lets the player view any settlement they own (conquered, colony, capital).
	var s *settlement.Settlement
	if sidStr := r.URL.Query().Get("sid"); sidStr != "" {
		if sid, parseErr := uuid.Parse(sidStr); parseErr == nil {
			if cand, loadErr := loadSettlement(r.Context(), h.pool, sid, worldID); loadErr == nil {
				if cand.OwnerID != nil && *cand.OwnerID == playerID {
					s = cand
				}
			}
		}
	}
	if s == nil {
		var err error
		s, err = loadPlayerCapital(r.Context(), h.pool, playerID, worldID)
		if err != nil {
			h.render(w, "no_province.html", map[string]any{"WorldID": worldID})
			return
		}
	}

	// List all settlements the player owns (for the switcher bar).
	type settSwitchItem struct {
		ID          uuid.UUID
		Name        string
		IsCapital   bool
		ControlType string
	}
	var ownSettlements []settSwitchItem
	switchRows, _ := h.pool.Query(r.Context(),
		`SELECT id, name, is_capital, control_type FROM settlements WHERE world_id = $1 AND owner_id = $2 ORDER BY is_capital DESC, name`,
		worldID, playerID,
	)
	if switchRows != nil {
		for switchRows.Next() {
			var ss settSwitchItem
			_ = switchRows.Scan(&ss.ID, &ss.Name, &ss.IsCapital, &ss.ControlType)
			ownSettlements = append(ownSettlements, ss)
		}
		switchRows.Close()
	}

	now := h.clk.Now()
	resources := s.Resources.Snapshot(now)
	resources["gold_rate"] = s.Resources.Gold.RatePerMinute
	resources["gold_cap"] = s.Resources.Gold.Cap

	// Load grain, cedar, stone from settlement_goods for the resource bar.
	loadSettlementGoodsIntoResources(r.Context(), h.pool, s.ID, now, resources,
		"grain", "cedar", "stone")

	// Kharis belongs to the Wanax, not the settlement.
	if s.OwnerID != nil {
		if kh, err2 := loadPlayerKharis(r.Context(), h.pool, *s.OwnerID, worldID); err2 == nil {
			resources["kharis"]      = kh.Amount
			resources["kharis_rate"] = kh.Rate
		}
	}

	divineMood := kharisToMood(resources["kharis"])

	// province.html uses .Province.ID in URLs — pass province_id as the ID.
	// Province is the settlement struct, but with ID = province tile ID.
	var copperDeposit, tinDeposit, silverDeposit, cedarDeposit bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT copper_deposit, tin_deposit,
		        COALESCE(silver_deposit, false), COALESCE(cedar_deposit, false)
		 FROM provinces WHERE id = $1`,
		s.ProvinceID,
	).Scan(&copperDeposit, &tinDeposit, &silverDeposit, &cedarDeposit)

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
		ArmyDP        int
		Population    int
		Walls         int
		Loyalty       int
		KingdomID     *uuid.UUID
		KingdomName   string
		CopperDeposit bool
		TinDeposit    bool
		SilverDeposit bool
		CedarDeposit  bool
	}
	armyDP := s.Army.Infantry + s.Army.EliteInfantry*2 + s.Army.Cavalry*3
	pv := provinceView{
		ID:            s.ProvinceID,
		SettlementID:  s.ID,
		WorldID:       worldID,
		Name:          s.Name,
		CultureID:     string(s.CultureID),
		Army:          s.Army,
		ArmyDP:        armyDP,
		Population:    s.Population,
		Walls:         s.WallLevel,
		Loyalty:       s.Loyalty,
		KingdomID:     s.KingdomID,
		KingdomName:   kingdomName,
		CopperDeposit: copperDeposit,
		TinDeposit:    tinDeposit,
		SilverDeposit: silverDeposit,
		CedarDeposit:  cedarDeposit,
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

	// Load pending recruit/training jobs from the scheduled-events queue.
	// process_after is UTC in the DB — we render in template via the user's locale.
	trrows, _ := h.pool.Query(r.Context(),
		`SELECT
		    (payload->>'unit_type')::text AS unit_type,
		    (payload->>'count')::int      AS count,
		    process_after
		 FROM scheduled_events
		 WHERE world_id = $1
		   AND event_type = 'TrainComplete'
		   AND processed_at IS NULL
		   AND (payload->>'settlement_id')::uuid = $2
		 ORDER BY process_after`,
		s.WorldID, s.ID,
	)
	defer trrows.Close()
	type recruitItem struct {
		Unit       string
		Count      int
		CompleteAt time.Time
	}
	var recruitQueue []recruitItem
	for trrows.Next() {
		var ri recruitItem
		_ = trrows.Scan(&ri.Unit, &ri.Count, &ri.CompleteAt)
		recruitQueue = append(recruitQueue, ri)
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
		"Province":         pv,
		"Resources":        resources,
		"Queue":            queue,
		"RecruitQueue":     recruitQueue,
		"Marches":          marches,
		"Incoming":         incoming,
		"Buildings":        buildings,
		"Built":            built,
		"WorldID":          worldID,
		"Now":              now,
		"DivineMood":       divineMood,
		"OwnSettlements":   ownSettlements,
		"ActiveSettlement": s.ID,
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
	var s *settlement.Settlement
	var err error
	if sidStr := r.URL.Query().Get("sid"); sidStr != "" {
		if sid, parseErr := uuid.Parse(sidStr); parseErr == nil {
			if cand, loadErr := loadSettlement(r.Context(), h.pool, sid, worldID); loadErr == nil {
				if cand.OwnerID != nil && *cand.OwnerID == playerID {
					s = cand
				}
			}
		}
	}
	if s == nil {
		s, err = loadPlayerCapital(r.Context(), h.pool, playerID, worldID)
		if err != nil {
			http.Error(w, "no settlement", http.StatusNotFound)
			return
		}
	}
	now := h.clk.Now()
	resources := s.Resources.Snapshot(now)
	resources["gold_rate"] = s.Resources.Gold.RatePerMinute
	resources["gold_cap"] = s.Resources.Gold.Cap
	loadSettlementGoodsIntoResources(r.Context(), h.pool, s.ID, now, resources,
		"grain", "cedar", "stone")

	if s.OwnerID != nil {
		if kh, err2 := loadPlayerKharis(r.Context(), h.pool, *s.OwnerID, worldID); err2 == nil {
			resources["kharis"]      = kh.Amount
			resources["kharis_rate"] = kh.Rate
		}
	}

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
		`SELECT id, name, state, map_width, map_height, prestige, era_number FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wld.ID, &wld.Name, &wld.State, &wld.MapWidth, &wld.MapHeight, &wld.Prestige, &wld.EraNumber)
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
	var kingdomName, playerRole, kingdomState string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT k.id, k.name, km.role, COALESCE(k.state,'forming') FROM kingdoms k
		 JOIN kingdom_members km ON km.kingdom_id = k.id
		 WHERE k.world_id = $1 AND km.player_id = $2`,
		worldID, playerID,
	).Scan(&kingdomID, &kingdomName, &playerRole, &kingdomState)

	var settlementID uuid.UUID
	var settlementName string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT id, name FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, playerID,
	).Scan(&settlementID, &settlementName)

	var memberCount int
	var treasuryGold float64
	var tributeRate float64
	if kingdomID != nil {
		_ = h.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM kingdom_members WHERE kingdom_id = $1`, kingdomID,
		).Scan(&memberCount)
		_ = h.pool.QueryRow(r.Context(),
			`SELECT gold_amount + (EXTRACT(EPOCH FROM (now()-gold_calc_at))/60 * gold_rate)
			 FROM kingdoms WHERE id = $1`, kingdomID,
		).Scan(&treasuryGold)
		_ = h.pool.QueryRow(r.Context(),
			`SELECT tribute_rate FROM settlements WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
			worldID, playerID,
		).Scan(&tributeRate)
	}
	h.render(w, "kingdom.html", map[string]any{
		"WorldID":        worldID,
		"KingdomID":      kingdomID,
		"KingdomName":    kingdomName,
		"KingdomState":   kingdomState,
		"KingdomForming": kingdomState == "forming",
		"MemberCount":    memberCount,
		"PlayerRole":     playerRole,
		"HasKingdom":     kingdomID != nil,
		"SettlementID":   settlementID,
		"SettlementName": settlementName,
		"TreasuryGold":   treasuryGold,
		"TributeRate":    tributeRate,
	})
}

// MarketView serves the goods market overview.
// Own and allied settlements: live prices (you have current intel).
// Others: prices from last caravan delivery or messenger arrival (snapshot).
func (h *WebHandler) MarketView(w http.ResponseWriter, r *http.Request) {
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

	type goodRow struct {
		Key         string
		Name        string
		Stock       float64
		Price       float64
		ObservedAt  time.Time
		HasObserved bool
	}
	type settlRow struct {
		ID      string
		Name    string
		Owner   string
		Own     bool
		Allied  bool
		Live    bool // own or allied — prices are current
		Goods   []goodRow
	}

	var worldName string
	_ = h.pool.QueryRow(r.Context(), `SELECT name FROM worlds WHERE id = $1`, worldID).Scan(&worldName)

	// ── Live data: own and allied settlements ─────────────────────────────────
	liveRows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.name, pl.username,
		        s.owner_id = $2 AS own,
		        sg.good_key, g.name,
		        GREATEST(0, LEAST(sg.cap,
		            sg.amount + (EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate))),
		        sg.cap, g.base_value
		 FROM settlements s
		 JOIN provinces p ON p.id = s.province_id
		 LEFT JOIN players pl ON pl.id = s.owner_id
		 JOIN settlement_goods sg ON sg.settlement_id = s.id
		 JOIN goods g ON g.key = sg.good_key
		 WHERE p.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND (
		       s.owner_id = $2
		       OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		           SELECT km.kingdom_id FROM kingdom_members km WHERE km.player_id = $2
		       ))
		   )
		   AND (sg.amount + (EXTRACT(EPOCH FROM (now()-sg.calc_at))/60 * sg.rate) > 0
		        OR sg.rate > 0)
		 ORDER BY s.name, sg.good_key`,
		worldID, playerID,
	)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer liveRows.Close()

	byID := map[string]*settlRow{}
	order := []string{}
	for liveRows.Next() {
		var sid, sName, ownerName, goodKey, goodName string
		var own bool
		var stock, cap, baseValue float64
		if err := liveRows.Scan(&sid, &sName, &ownerName, &own, &goodKey, &goodName, &stock, &cap, &baseValue); err != nil {
			continue
		}
		if _, exists := byID[sid]; !exists {
			byID[sid] = &settlRow{ID: sid, Name: sName, Owner: ownerName, Own: own, Allied: !own, Live: true}
			order = append(order, sid)
		}
		price := baseValue * clampF(cap*0.3/max(stock, 0.001), 0.5, 3.0)
		byID[sid].Goods = append(byID[sid].Goods, goodRow{Key: goodKey, Name: goodName, Stock: stock, Price: price})
	}

	// ── Snapshot data: other settlements visited by caravan or messenger ──────
	snapRows, _ := h.pool.Query(r.Context(),
		`SELECT DISTINCT ON (ms.settlement_id)
		        ms.settlement_id, s.name, pl.username, ms.observed_at
		 FROM market_snapshots ms
		 JOIN settlements s ON s.id = ms.settlement_id
		 LEFT JOIN players pl ON pl.id = s.owner_id
		 WHERE ms.player_id = $1 AND s.world_id = $2
		   AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		 ORDER BY ms.settlement_id, ms.observed_at DESC`,
		playerID, worldID,
	)
	if snapRows != nil {
		for snapRows.Next() {
			var sid, sName, ownerName string
			var observedAt time.Time
			if snapRows.Scan(&sid, &sName, &ownerName, &observedAt) != nil {
				continue
			}
			if _, exists := byID[sid]; exists {
				continue // already have live data
			}
			byID[sid] = &settlRow{ID: sid, Name: sName, Owner: ownerName, Live: false}
			order = append(order, sid)

			// Fetch all good rows for this snapshot.
			gRows, _ := h.pool.Query(r.Context(),
				`SELECT ms.good_key, g.name, ms.stock, ms.price, ms.observed_at
				 FROM market_snapshots ms JOIN goods g ON g.key = ms.good_key
				 WHERE ms.player_id = $1 AND ms.settlement_id = $2
				 ORDER BY ms.good_key`,
				playerID, sid,
			)
			if gRows != nil {
				for gRows.Next() {
					var gKey, gName string
					var stock, price float64
					var obs time.Time
					if gRows.Scan(&gKey, &gName, &stock, &price, &obs) == nil {
						byID[sid].Goods = append(byID[sid].Goods, goodRow{
							Key: gKey, Name: gName, Stock: stock, Price: price,
							ObservedAt: obs, HasObserved: true,
						})
					}
				}
				gRows.Close()
			}
		}
		snapRows.Close()
	}

	result := make([]settlRow, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}

	h.render(w, "market.html", map[string]any{
		"WorldID":   worldID,
		"WorldName": worldName,
		"Markets":   result,
	})
}

// loadSettlementGoodsIntoResources queries the named goods from settlement_goods
// and adds amount/rate/cap entries to the resources map for template rendering.
// Keys written: "grain", "grain_rate", "grain_cap", etc.
func loadSettlementGoodsIntoResources(ctx context.Context, pool *pgxpool.Pool,
	settlementID uuid.UUID, now time.Time, resources map[string]float64, keys ...string,
) {
	if len(keys) == 0 {
		return
	}
	rows, err := pool.Query(ctx,
		`SELECT good_key, amount, rate, cap, calc_at
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = ANY($2)`,
		settlementID, keys,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var amount, rate, cap float64
		var calcAt time.Time
		if rows.Scan(&key, &amount, &rate, &cap, &calcAt) != nil {
			continue
		}
		elapsed := now.Sub(calcAt).Minutes()
		current := amount + elapsed*rate
		if current < 0 {
			current = 0
		}
		if current > cap {
			current = cap
		}
		resources[key] = current
		resources[key+"_rate"] = rate
		resources[key+"_cap"] = cap
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// MessagesView serves the standalone messages/diplomacy page.
func (h *WebHandler) MessagesView(w http.ResponseWriter, r *http.Request) {
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

	// Find player's capital settlement for sending messengers.
	var mySettlementID, mySettlementName string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT id, name FROM settlements WHERE world_id=$1 AND owner_id=$2 AND is_capital=true`,
		worldID, playerID,
	).Scan(&mySettlementID, &mySettlementName)

	// Unread count for display.
	var unread int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT count(*) FROM messengers m
		 JOIN settlements s ON s.id = m.destination_id
		 WHERE m.world_id=$1 AND s.owner_id=$2 AND m.status='delivered'
		   AND (m.trade_offer IS NULL OR m.trade_offer->>'status' = 'pending')`,
		worldID, playerID,
	).Scan(&unread)

	h.render(w, "messages.html", map[string]any{
		"WorldID":           worldID,
		"MySettlementID":    mySettlementID,
		"MySettlementName":  mySettlementName,
		"UnreadCount":       unread,
	})
}

package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/religion"
	"github.com/poleia/server/internal/world"
)

var cultureSettlementNames = map[string][]string{
	"akhaier": {
		"Mykene", "Tiryns", "Argos", "Pylos", "Sparta", "Iolkos",
		"Orchomenos", "Asine", "Midea", "Nauplia", "Epidauros",
		"Korinthos", "Megara", "Athenai", "Thebai", "Plataia",
		"Eretria", "Chalkis", "Pharsalos", "Larissa", "Phaistos",
		"Knossos", "Gortyn", "Kalydon", "Ithake", "Dodona",
		"Amyklai", "Arne", "Oikhalia", "Pherai", "Eleusis",
	},
	"khemetiu": {
		"Waset", "Memphis", "Iunu", "Avaris", "Khemenu", "Nekhen",
		"Per-Neith", "Sapi-Res", "Tjaru", "Ineb-hedj", "Akhetaten",
		"Edfu", "Abydos", "Esna", "Dendara", "Sais",
		"Bubastis", "Tanis", "Koptos", "Hermopolis", "Per-Ramesses",
		"Khmun", "Mennefer", "Thinis", "Akhmin", "Djanet",
		"Khent-Abt", "Imet", "Per-Bastet", "Sekhemkhet",
	},
	"knaani": {
		"Tyros", "Sidon", "Byblos", "Akko", "Megiddo", "Ashdod",
		"Ashkelon", "Gaza", "Hazor", "Shechem", "Jericho", "Jaffa",
		"Dor", "Ugarit", "Alalakh", "Qatna", "Gezer",
		"Lachish", "Timna", "Arwad", "Berytus", "Acre",
		"Beit-Shean", "Taanach", "Megiddu", "Arvad", "Ullaza",
		"Sumur", "Irqata", "Tunip",
	},
	"thrakes": {
		"Seuthopolis", "Kabyle", "Kypsela", "Maroneia", "Abdera",
		"Ainos", "Samothrake", "Doriskos", "Perinthos", "Byzantion",
		"Anchialos", "Odessus", "Apollonia", "Mesambria", "Istros",
		"Tomis", "Kallatis", "Bizone", "Dionysopolis", "Tyras",
		"Olbia", "Kardia", "Lysimacheia", "Amadokos", "Teres",
		"Odrysai", "Philippoi", "Eion", "Amphipolis", "Abros",
	},
	"pelasger": {
		"Larisa", "Antron", "Pteleon", "Halos", "Larymna",
		"Aulis", "Gla", "Eleusis", "Brauron", "Tanagra",
		"Thisbe", "Koroneia", "Arne", "Itonos", "Meliboia",
		"Krannon", "Gomphoi", "Pelinnai", "Olosson", "Alos",
		"Gynaikopolis", "Pelinnaion", "Achilleion", "Phthia", "Alope",
		"Halai", "Mopsos", "Peiros", "Enipeus", "Titarisios",
	},
	"hatti": {
		"Hattusa", "Kanesh", "Nesa", "Washukanni", "Kussara",
		"Zalpa", "Ankuwa", "Zippalanda", "Katapa", "Arinna",
		"Nerik", "Tarhuntassa", "Kumani", "Alisar", "Samuha",
		"Apasas", "Puranda", "Millawanda", "Arzawa", "Karabel",
		"Kizzuwatna", "Wilusa", "Lazpa", "Seha", "Hapalla",
		"Pitassa", "Lukka", "Pahhuwa", "Tegarama", "Ishuwa",
	},
}

func settlementNameForCulture(culture string) string {
	names := cultureSettlementNames[culture]
	if len(names) == 0 {
		return "Unknown Settlement"
	}
	return names[rand.Intn(len(names))]
}

// JoinHandler handles POST /worlds/:worldID/join.
type JoinHandler struct {
	pool *pgxpool.Pool
}

// NewJoinHandler creates a JoinHandler.
func NewJoinHandler(pool *pgxpool.Pool) *JoinHandler {
	return &JoinHandler{pool: pool}
}

// Join creates a province + settlement for the authenticated player in the given world.
// If a settlement already exists, returns the existing one.
func (h *JoinHandler) Join(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Already has a settlement in this world?
	var existingProvID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT province_id FROM settlements WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&existingProvID); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"province_id": existingProvID, "existing": true})
		return
	}

	// Verify world is in a joinable state.
	var wState string
	var maxProvinces int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT state, max_provinces FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wState, &maxProvinces); err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	if wState != "forming" && wState != "active" {
		writeError(w, http.StatusConflict, "world is not accepting new players")
		return
	}

	// Count current players via settlements.
	var playerCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM settlements WHERE world_id = $1 AND owner_id IS NOT NULL`,
		worldID,
	).Scan(&playerCount)
	if playerCount >= maxProvinces {
		writeError(w, http.StatusConflict, "world is full — you are queued")
		return
	}

	// Decode optional preferences.
	var req struct {
		ProvinceName string `json:"province_name"`
		Culture      string `json:"culture"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.ProvinceName == "" {
		req.ProvinceName = settlementNameForCulture(req.Culture)
	}
	if req.Culture == "" {
		req.Culture = string(province.CultureAkhaier)
	}

	// Find an unclaimed tile (no province row exists yet for this tile).
	var q, r2 int
	var terrainType string
	var copperDeposit, tinDeposit bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT mt.q, mt.r, mt.terrain, mt.copper_deposit, mt.tin_deposit
		 FROM map_tiles mt
		 LEFT JOIN provinces p ON p.world_id = mt.world_id AND p.map_q = mt.q AND p.map_r = mt.r
		 WHERE mt.world_id = $1 AND p.id IS NULL AND mt.terrain IN ('plains','coast','hills')
		 ORDER BY RANDOM() LIMIT 1`,
		worldID,
	).Scan(&q, &r2, &terrainType, &copperDeposit, &tinDeposit)
	if err != nil {
		writeError(w, http.StatusConflict, "no available tiles — try again")
		return
	}

	// Seed resource rates from terrain and pantheon.
	regions := religion.DefaultPantheonRegions()
	var maxPower float64
	for _, reg := range regions {
		if p := religion.LocalPower(reg, q, r2); p > maxPower {
			maxPower = p
		}
	}
	kharisRate := maxPower * 0.05

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Create the province tile row — copy deposit flags from map_tiles.
	var provinceID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state, copper_deposit, tin_deposit)
		 VALUES ($1, $2, $3, $4, 'controlled', $5, $6) RETURNING id`,
		worldID, q, r2, terrainType, copperDeposit, tinDeposit,
	).Scan(&provinceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create province")
		return
	}

	// Create the settlement (capital). gold and kharis are settlement columns;
	// all other goods (grain, cedar, stone, etc.) live in settlement_goods.
	var settlementID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital,
		  kharis_rate, kharis_calc_at)
		 VALUES ($1,$2,$3,$4,$5,'capital',true,$6,now())
		 RETURNING id`,
		worldID, provinceID, req.ProvinceName, req.Culture, playerID,
		kharisRate,
	).Scan(&settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create settlement")
		return
	}

	// Link province back to its controlling settlement.
	_, err = tx.Exec(r.Context(),
		`UPDATE provinces SET controller_id = $1 WHERE id = $2`,
		settlementID, provinceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not link province")
		return
	}

	// Seed a zero row for every good first so the settlement always has full
	// inventory schema regardless of terrain. The production-rule UPSERT below
	// only adds rate for goods the terrain actually produces.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, g.key,
		        CASE g.key
		            WHEN 'grain' THEN 150
		            WHEN 'cedar' THEN 120
		            WHEN 'stone' THEN 120
		            ELSE 0
		        END,
		        0,
		        CASE g.key
		            WHEN 'grain'  THEN 1000
		            WHEN 'cedar'  THEN 500
		            WHEN 'stone'  THEN 1000
		            WHEN 'copper' THEN 300
		            WHEN 'tin'    THEN 300
		            ELSE 200
		        END,
		        now()
		 FROM goods g
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not seed goods")
		return
	}

	// Init settlement_goods from terrain-only production rules.
	// Cap is chosen per good: staples (grain) get 1000, bulk (cedar, stone) get 500-1000,
	// other goods default to 200.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, pr.good_key, 0, pr.rate_per_min,
		        CASE pr.good_key
		            WHEN 'grain'  THEN 1000
		            WHEN 'cedar'  THEN 500
		            WHEN 'stone'  THEN 1000
		            WHEN 'copper' THEN 300
		            WHEN 'tin'    THEN 300
		            ELSE 200
		        END,
		        now()
		 FROM production_rules pr
		 WHERE pr.building_type IS NULL
		   AND pr.terrain_type = $2
		   AND (pr.requires_deposit IS NULL
		        OR (pr.requires_deposit = 'copper' AND $3)
		        OR (pr.requires_deposit = 'tin'    AND $4))
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     rate = settlement_goods.rate + EXCLUDED.rate`,
		settlementID, terrainType, copperDeposit, tinDeposit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not init goods")
		return
	}

	// Record in player_world_records.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO player_world_records (player_id, world_id, settlement_id, status)
		 VALUES ($1, $2, $3, 'active')
		 ON CONFLICT (player_id, world_id) DO UPDATE SET settlement_id = EXCLUDED.settlement_id, status = 'active'`,
		playerID, worldID, settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record join")
		return
	}

	// Transition world to active if still forming.
	if wState == "forming" {
		_, _ = tx.Exec(r.Context(),
			`UPDATE worlds SET state = 'active' WHERE id = $1 AND state = 'forming'`,
			worldID,
		)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"province_id": provinceID,
		"tile":        world.MapTile{Q: q, R: r2},
		"culture":     req.Culture,
	})
}

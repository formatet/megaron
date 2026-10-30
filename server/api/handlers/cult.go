package handlers

import (
	"errors"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/settlement"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
	"sort"
)

// CultView is an owner-private realm catalogue; every action retains its city.
type CultView struct {
	Kharis         map[string]any      `json:"kharis"`
	Temples        []CultTemple        `json:"temples"`
	Prayers        []CultPrayer        `json:"prayers"`
	UnbuiltTemples []CultUnbuiltTemple `json:"unbuilt_temples"`
}
type CultPlace struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	ProvinceID   uuid.UUID `json:"province_id"`
	CityName     string    `json:"city_name"`
}
type CultTemple struct {
	CultPlace
	ID               uuid.UUID                `json:"id"`
	GodID            string                   `json:"god_id"`
	Name             string                   `json:"name"`
	Level            int                      `json:"level"`
	Staffed          int                      `json:"staffed"`
	Capacity         int                      `json:"capacity"`
	Offer            kharis.TempleOffer       `json:"offer"`
	DemolitionKharis *kharis.TempleDemolition `json:"demolition_kharis"`
}
type CultPrayer struct {
	prayerRow
	CultPlace
	HasTemple bool `json:"has_temple"`
}
type CultUnbuiltTemple struct {
	CultPlace
	GodID string `json:"god_id"`
	Name  string `json:"name"`
}

// Cult assembles existing projection owners without changing prayer or temple rules.
func (h *ProvinceHandler) Cult(w http.ResponseWriter, r *http.Request) {
	player, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, 401, "not authenticated")
		return
	}
	world, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, 400, "invalid world ID")
		return
	}
	ctx := r.Context()
	k, err := loadPlayerKharis(ctx, h.pool, player, world)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "not joined to this world")
		} else {
			writeError(w, 500, "could not load realm standing")
		}
		return
	}
	fail := func() { writeError(w, 500, "could not load realm cult") }
	wrath, err := kharis.LoadWrathStatus(ctx, h.pool, player, world)
	if err != nil {
		fail()
		return
	}
	decay, err := kharis.ProjectDecay(ctx, h.pool, player, world)
	if err != nil {
		fail()
		return
	}
	net, has, dev, cap, err := kharis.ProjectDailyNet(ctx, h.pool, player, world)
	if err != nil {
		fail()
		return
	}
	staffed, capacity, err := economy.RealmTempleStaffing(ctx, h.pool, player, world)
	if err != nil {
		fail()
		return
	}
	values, err := religion.LoadDivineValues(ctx, h.pool, world)
	if err != nil {
		fail()
		return
	}
	offers, err := kharis.LoadTempleOffers(ctx, h.pool, player, world)
	if err != nil {
		fail()
		return
	}
	offerByID := map[uuid.UUID]kharis.TempleOffer{}
	for _, o := range offers {
		offerByID[o.BuildingID] = o
	}
	view := CultView{Temples: []CultTemple{}, Prayers: []CultPrayer{}, UnbuiltTemples: []CultUnbuiltTemple{}, Kharis: map[string]any{
		"kharis": k.Amount, "kharis_cap": k.Cap, "max_temple_level": k.MaxTempleLevel, "kharis_per_tick": k.Rate,
		"kharis_mood": kharisToMood(k.Amount), "kharis_standing": kharis.DescribeStanding(k.Amount, has, wrath.ProtectionRemainingDays),
		"kharis_wrath": wrath, "kharis_blessing_threshold": kharis.BlessThreshold, "kharis_city_count": decay.CityCount,
		"kharis_population": decay.Population, "kharis_base_decay": decay.Base, "kharis_city_decay": decay.Cities,
		"kharis_population_decay": decay.Residents, "kharis_daily_decay": decay.Total, "kharis_net_per_tick": net,
		"kharis_net_known": true, "kharis_production_per_tick": net + decay.Total, "kharis_demand": kharis.RealmDemand(decay.CityCount), "kharis_given_weight": kharis.GivenWeight(offers),
		"kharis_devotion_idle": has && cap > dev, "temple_staffed": staffed, "temple_capacity": capacity, "temple_offers": offers, "divine_values": values,
	}}
	rows, err := h.pool.Query(ctx, `SELECT id,province_id,name,culture_id FROM settlements WHERE world_id=$1 AND owner_id=$2 AND state NOT IN ('sunk','collapsed','razed') ORDER BY name,id`, world, player)
	if err != nil {
		fail()
		return
	}
	cities := []settlement.Settlement{}
	for rows.Next() {
		c := settlement.Settlement{WorldID: world, OwnerID: &player}
		if err := rows.Scan(&c.ID, &c.ProvinceID, &c.Name, &c.CultureID); err != nil {
			rows.Close()
			fail()
			return
		}
		cities = append(cities, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail()
		return
	}
	for _, city := range cities {
		place := CultPlace{city.ID, city.ProvinceID, city.Name}
		staff, err := economy.LoadTempleStaffing(ctx, h.pool, city.ID)
		if err != nil {
			fail()
			return
		}
		gods := []string{}
		for _, t := range staff.Temples {
			gods = append(gods, t.GodID)
			quote, err := kharis.ProjectTempleDemolition(ctx, h.pool, player, world, t.BuildingID)
			if err != nil {
				fail()
				return
			}
			offer, ok := offerByID[t.BuildingID]
			if !ok {
				fail()
				return
			}
			view.Temples = append(view.Temples, CultTemple{CultPlace: place, ID: t.BuildingID, GodID: t.GodID, Name: religion.TempleName(t.GodID), Level: t.Level, Staffed: t.Staffed, Capacity: t.Capacity, Offer: offer, DemolitionKharis: quote})
		}
		goods := map[string]float64{}
		rows, err := h.pool.Query(ctx, `SELECT good_key,GREATEST(0,settled(amount,rate,calc_tick)) FROM settlement_goods WHERE settlement_id=$1`, city.ID)
		if err != nil {
			fail()
			return
		}
		for rows.Next() {
			var good string
			var n float64
			if err := rows.Scan(&good, &n); err != nil {
				rows.Close()
				fail()
				return
			}
			goods[good] = n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			fail()
			return
		}
		prayers, _, err := h.loadPrayerRows(ctx, &city, world, k.Amount, goods)
		if err != nil {
			fail()
			return
		}
		for _, p := range prayers {
			view.Prayers = append(view.Prayers, CultPrayer{prayerRow: p, CultPlace: place, HasTemple: (p.GodID == "" && len(gods) > 0) || slices.Contains(gods, p.GodID)})
		}
		for _, god := range religion.TempleGods {
			if slices.Contains(god.Cultures, string(city.CultureID)) && !slices.Contains(gods, god.ID) {
				view.UnbuiltTemples = append(view.UnbuiltTemples, CultUnbuiltTemple{CultPlace: place, GodID: god.ID, Name: religion.TempleName(god.ID)})
			}
		}
	}
	// Stable city/god order is retained within each readiness group.
	sort.SliceStable(view.Prayers, func(i, j int) bool {
		rank := func(p CultPrayer) int {
			if p.Affordable {
				return 0
			}
			if p.CanPrepareOffering {
				return 1
			}
			if p.HasTemple {
				return 2
			}
			return 3
		}
		return rank(view.Prayers[i]) < rank(view.Prayers[j])
	})
	writeJSON(w, 200, view)
}

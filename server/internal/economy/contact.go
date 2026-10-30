package economy

import (
	"context"
	"encoding/json"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Contact sources (megaron_kontrakt_frammande_stad): what of the player's own
// reached the city. Mirrors the CHECK on player_contacted_settlements.source.
const (
	ContactMessenger   = "messenger"
	ContactCaravan     = "caravan"
	ContactTrade       = "trade"
	ContactMarketBoard = "market_board"
)

type contactDB interface {
	boardDB
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ContactGoodsMax is how many goods a contact snapshot names.
const ContactGoodsMax = 3

// ContactGood is one good as a visitor saw it: a word, never a number.
type ContactGood struct {
	Good string `json:"good"`
	Word string `json:"word"` // "plenty of" | "some"
}

// ContactBuilding is one building as a visitor saw it, level included.
type ContactBuilding struct {
	Type  string `json:"type"`
	Level int    `json:"level"`
	GodID string `json:"god_id,omitempty"`
}

// FoodWord is how a city's table reads to a visitor, from the same inputs as
// GrowthState: unmet food is starving, eating into stock is hungry.
func FoodWord(unmet, foodNet float64) string {
	switch {
	case unmet > 0:
		return "starving"
	case foodNet < 0:
		return "hungry"
	default:
		return "well fed"
	}
}

// GoodsWords names the largest stocks (silver never), "plenty of" at half the
// store's cap or more, else "some". stock and cap are keyed by good.
func GoodsWords(stock, cap map[string]float64) []ContactGood {
	type kv struct {
		good string
		n    float64
	}
	var all []kv
	for g, n := range stock {
		if g == "silver" || n < 1 {
			continue
		}
		all = append(all, kv{g, n})
	}
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && (all[j].n > all[j-1].n || all[j].n == all[j-1].n && all[j].good < all[j-1].good); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	out := []ContactGood{}
	for i := 0; i < len(all) && i < ContactGoodsMax; i++ {
		word := "some"
		if c := cap[all[i].good]; c > 0 && all[i].n >= c/2 {
			word = "plenty of"
		}
		out = append(out, ContactGood{Good: all[i].good, Word: word})
	}
	return out
}

// RecordContact is the one owner of player_contacted_settlements: playerID's
// own messenger, caravan, trade or board reading has reached settlementID at
// tick. Call it in the arrival's transaction (an outcome, never an intention).
// The snapshot is taken now and stored coarse; an older contact never
// replaces a newer one, and a replay writes the same row. A player's own city
// is never recorded.
func RecordContact(ctx context.Context, db contactDB, worldID, playerID, settlementID uuid.UUID, source string, tick int) error {
	var ownerID *uuid.UUID
	var name, ownerName string
	var population int
	var unmet float64
	if err := db.QueryRow(ctx,
		`SELECT s.name, s.owner_id, COALESCE(pl.wanax_name, pl.username, ''), s.population, s.food_unmet_amount
		   FROM settlements s LEFT JOIN players pl ON pl.id = s.owner_id
		  WHERE s.id = $1 AND s.world_id = $2`,
		settlementID, worldID,
	).Scan(&name, &ownerID, &ownerName, &population, &unmet); err != nil {
		return err
	}
	if ownerID != nil && *ownerID == playerID {
		return nil
	}
	stock, cap := map[string]float64{}, map[string]float64{}
	var foodRate float64
	rows, err := db.Query(ctx,
		`SELECT good_key, GREATEST(0, LEAST(cap, settled(amount, rate, calc_tick))), cap, rate
		   FROM settlement_goods WHERE settlement_id = $1`, settlementID)
	if err != nil {
		return err
	}
	food := map[string]bool{}
	for _, g := range FoodGoods {
		food[g] = true
	}
	for rows.Next() {
		var g string
		var n, c, rate float64
		if err := rows.Scan(&g, &n, &c, &rate); err != nil {
			rows.Close()
			return err
		}
		stock[g], cap[g] = n, c
		if food[g] {
			foodRate += rate
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	buildings := []ContactBuilding{}
	brows, err := db.Query(ctx,
		`SELECT building_type, level, COALESCE(god_id, '') FROM buildings
		  WHERE settlement_id = $1 AND level >= 1 AND hex_q IS NULL ORDER BY building_type, god_id, level DESC`, settlementID)
	if err != nil {
		return err
	}
	for brows.Next() {
		var b ContactBuilding
		if err := brows.Scan(&b.Type, &b.Level, &b.GodID); err != nil {
			brows.Close()
			return err
		}
		buildings = append(buildings, b)
	}
	brows.Close()
	if err := brows.Err(); err != nil {
		return err
	}
	goodsJSON, _ := json.Marshal(GoodsWords(stock, cap))
	buildingsJSON, _ := json.Marshal(buildings)
	_, err = db.Exec(ctx,
		`INSERT INTO player_contacted_settlements
		        (world_id, player_id, settlement_id, contact_tick, source, name, owner_id, owner_name,
		         size, food, goods, buildings)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (player_id, settlement_id) DO UPDATE SET
		        contact_tick = EXCLUDED.contact_tick, source = EXCLUDED.source, name = EXCLUDED.name,
		        owner_id = EXCLUDED.owner_id, owner_name = EXCLUDED.owner_name, size = EXCLUDED.size,
		        food = EXCLUDED.food, goods = EXCLUDED.goods, buildings = EXCLUDED.buildings
		  WHERE player_contacted_settlements.contact_tick <= EXCLUDED.contact_tick`,
		worldID, playerID, settlementID, tick, source, name, ownerID, ownerName,
		province.SizeWord(population), FoodWord(unmet, FoodNet(foodRate, population)),
		goodsJSON, buildingsJSON)
	return err
}

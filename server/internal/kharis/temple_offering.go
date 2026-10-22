package kharis

import (
	"context"
	"fmt"
	"maps"
	"math"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TempleOfferingGrain is the standing reference gift per temple and tick.
// Early local substitutes have the same divine worth; silver is never required.
const TempleOfferingGrain = 0.25

// Grain first, then other early goods, with silver last. This order is stable
// even when two goods have the same divine value, and is shown in the Codex.
var templeOfferingGoods = [...]string{"grain", "fish", "timber", "silver"}

func templeOfferingRecipe() map[string]float64 {
	return map[string]float64{"grain": TempleOfferingGrain}
}

// TempleOffer is the actual next-tick plan, shared by status and maintenance.
// Offering is empty if the city cannot fund the entire gift. Every temple has
// its own gift (ORDER 15); a city's temples draw on one store in god-ID order,
// each planned against what the earlier ones left.
type TempleOffer struct {
	SettlementID uuid.UUID          `json:"settlement_id"`
	Name         string             `json:"name"`
	BuildingID   uuid.UUID          `json:"building_id"`
	GodID        string             `json:"god_id"`
	Temple       string             `json:"temple"`
	Required     map[string]float64 `json:"required_goods"`
	Alternatives map[string]float64 `json:"alternatives"`
	Stock        map[string]float64 `json:"stock"`
	Offering     map[string]float64 `json:"offering"`
	Fed          bool               `json:"fed"`
}

func positiveFinite(v float64) bool { return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) }

func planTempleOffering(stock, values map[string]float64) (map[string]float64, bool) {
	// The reference gift works even before a world's valuation index is ready.
	if stock["grain"] >= TempleOfferingGrain {
		return templeOfferingRecipe(), true
	}
	due := TempleOfferingGrain * values["grain"]
	if !positiveFinite(due) {
		return map[string]float64{}, false
	}
	take := map[string]float64{}
	remaining := due
	for _, good := range templeOfferingGoods {
		amount, value := stock[good], values[good]
		if !positiveFinite(amount) || !positiveFinite(value) {
			continue
		}
		want := remaining / value
		if amount >= want {
			take[good] = want
			return take, true
		}
		take[good] = amount
		remaining -= amount * value
	}
	return map[string]float64{}, false
}

func newTempleOffer(id uuid.UUID, name string, stock, values map[string]float64) TempleOffer {
	o := TempleOffer{SettlementID: id, Name: name, Required: templeOfferingRecipe(),
		Alternatives: map[string]float64{}, Stock: maps.Clone(stock)}
	if positiveFinite(values["grain"]) {
		for _, good := range templeOfferingGoods[1:] {
			if positiveFinite(values[good]) {
				o.Alternatives[good] = TempleOfferingGrain * values["grain"] / values[good]
			}
		}
	}
	o.Offering, o.Fed = planTempleOffering(stock, values)
	return o
}

// LoadTempleOffers reads the same local stocks and plan that the tick charges.
func LoadTempleOffers(ctx context.Context, db religion.Execer, playerID, worldID uuid.UUID) ([]TempleOffer, error) {
	values, err := religion.LoadDivineValues(ctx, db, worldID)
	if err != nil {
		return nil, err
	}
	return loadTempleOffers(ctx, db, playerID, worldID, values)
}

type templeRef struct {
	id  uuid.UUID
	god string
}

// planCityOffers plans each temple's gift in turn against one shared store.
func planCityOffers(id uuid.UUID, name string, temples []templeRef, stock, values map[string]float64) []TempleOffer {
	left := maps.Clone(stock)
	out := make([]TempleOffer, 0, len(temples))
	for _, t := range temples {
		o := newTempleOffer(id, name, left, values)
		o.BuildingID, o.GodID, o.Temple = t.id, t.god, religion.TempleName(t.god)
		for good, amount := range o.Offering {
			left[good] -= amount
		}
		out = append(out, o)
	}
	return out
}

func loadTempleOffers(ctx context.Context, db religion.Execer, playerID, worldID uuid.UUID, values map[string]float64) ([]TempleOffer, error) {
	rows, err := db.Query(ctx, `SELECT s.id,s.name,
	 (SELECT array_agg(b.id ORDER BY b.god_id) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),
	 (SELECT array_agg(b.god_id ORDER BY b.god_id) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),
	 (SELECT coalesce(jsonb_object_agg(sg.good_key,GREATEST(0,settled(sg.amount,sg.rate,sg.calc_tick))),'{}')
	   FROM settlement_goods sg WHERE sg.settlement_id=s.id AND sg.good_key=ANY($3::text[]))
	 FROM settlements s
	 WHERE s.owner_id=$1 AND s.world_id=$2 AND s.state NOT IN ('sunk','collapsed','razed')
	 AND EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple')
	 ORDER BY s.name,s.id`, playerID, worldID, templeOfferingGoods[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	offers := []TempleOffer{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		var ids []uuid.UUID
		var gods []string
		var stored map[string]float64
		if err := rows.Scan(&id, &name, &ids, &gods, &stored); err != nil {
			return nil, err
		}
		stock := map[string]float64{}
		for _, key := range templeOfferingGoods {
			stock[key] = stored[key]
		}
		temples := make([]templeRef, len(ids))
		for i := range ids {
			temples[i] = templeRef{ids[i], gods[i]}
		}
		offers = append(offers, planCityOffers(id, name, temples, stock, values)...)
	}
	return offers, rows.Err()
}

// chargeTempleOffering replans the city's temples under row locks and commits
// every local gift together. A failed later debit cannot leave an earlier good
// partially burned. Returns which temples were fed.
func (h *TickHandler) chargeTempleOffering(ctx context.Context, settlementID uuid.UUID, values map[string]float64) (map[uuid.UUID]bool, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, `SELECT state NOT IN ('sunk','collapsed','razed')
	 FROM settlements s WHERE id=$1 FOR UPDATE`, settlementID).Scan(&active)
	if err != nil || !active {
		return nil, err
	}
	var temples []templeRef
	trows, err := tx.Query(ctx, `SELECT id,god_id FROM buildings WHERE settlement_id=$1 AND building_type='temple' ORDER BY god_id`, settlementID)
	if err != nil {
		return nil, err
	}
	for trows.Next() {
		var t templeRef
		if err := trows.Scan(&t.id, &t.god); err != nil {
			trows.Close()
			return nil, err
		}
		temples = append(temples, t)
	}
	err = trows.Err()
	trows.Close()
	if err != nil || len(temples) == 0 {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT good_key,GREATEST(0,settled(amount,rate,calc_tick))
	 FROM settlement_goods WHERE settlement_id=$1 AND good_key=ANY($2::text[])
	 ORDER BY good_key FOR UPDATE`, settlementID, templeOfferingGoods[:])
	if err != nil {
		return nil, err
	}
	stock := map[string]float64{}
	for rows.Next() {
		var good string
		var amount float64
		if err := rows.Scan(&good, &amount); err != nil {
			rows.Close()
			return nil, err
		}
		stock[good] = amount
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	fed := map[uuid.UUID]bool{}
	take := map[string]float64{}
	for _, o := range planCityOffers(settlementID, "", temples, stock, values) {
		if !o.Fed {
			continue
		}
		fed[o.BuildingID] = true
		for good, amount := range o.Offering {
			take[good] += amount
		}
	}
	for _, good := range templeOfferingGoods {
		amount := take[good]
		if amount <= 0 {
			continue
		}
		tag, err := tx.Exec(ctx, `UPDATE settlement_goods SET
		 amount=GREATEST(0,settled(amount,rate,calc_tick)-$3),calc_tick=current_world_tick()
		 WHERE settlement_id=$1 AND good_key=$2`, settlementID, good, amount)
		if err != nil {
			return nil, fmt.Errorf("temple offering %s: %w", good, err)
		}
		if tag.RowsAffected() != 1 {
			return nil, pgx.ErrNoRows
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return fed, nil
}

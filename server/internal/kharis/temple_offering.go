package kharis

import (
	"context"
	"fmt"
	"maps"
	"math"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TempleOfferingGrain is the standing reference gift per temple and tick.
// The level-one grain part also sets the scarcity reference.
const TempleOfferingGrain = 0.25

// Only food/wood may substitute for the grain part. Other parts are independent.
var grainOfferingGoods = [...]string{"grain", "fish", "timber"}
var templeOfferingGoods = [...]string{"grain", "fish", "timber", "silver", "incense"}

func templeOfferingRecipe() map[string]float64 {
	return map[string]float64{"grain": TempleOfferingGrain}
}

// TempleOffer is the actual next-tick plan, shared by status and maintenance.
// Offering is empty if the city cannot fund the entire gift. Every temple has
// its own gift (ORDER 15); a city's temples draw on one store in god-ID order,
// each planned against what the earlier ones left.
type TithePart struct {
	Good   string  `json:"good"`
	Amount float64 `json:"amount"`
	Share  float64 `json:"share"`
	Paid   bool    `json:"paid"`
}

func TempleWeight(level int) float64 { return float64(level * level) }
func RealmDemand(cities int) float64 { return float64(max(0, cities) * max(0, cities)) }

const KharisPerTempleWeight = 2.1
const KharisPerDemand = 1.2

func EffectiveTempleWeight(level, staffed, capacity int, paid float64) float64 {
	if capacity <= 0 {
		return 0
	}
	return TempleWeight(level) * float64(min(staffed, capacity)) / float64(capacity) * paid
}
func titheParts(level int) []TithePart {
	switch level {
	case 2:
		return []TithePart{{Good: "grain", Amount: .75, Share: .75}, {Good: "silver", Amount: .1, Share: .25}}
	case 3:
		return []TithePart{{Good: "grain", Amount: 1.75, Share: .7}, {Good: "silver", Amount: .25, Share: .2}, {Good: "incense", Amount: .1, Share: .1}}
	default:
		return []TithePart{{Good: "grain", Amount: .25, Share: 1}}
	}
}

type TempleOffer struct {
	Level           int                `json:"level"`
	Weight          float64            `json:"weight"`
	Staffed         int                `json:"staffed"`
	Capacity        int                `json:"capacity"`
	PaidFraction    float64            `json:"paid_fraction"`
	EffectiveWeight float64            `json:"effective_weight"`
	Parts           []TithePart        `json:"parts"`
	SettlementID    uuid.UUID          `json:"settlement_id"`
	Name            string             `json:"name"`
	BuildingID      uuid.UUID          `json:"building_id"`
	GodID           string             `json:"god_id"`
	Temple          string             `json:"temple"`
	Required        map[string]float64 `json:"required_goods"`
	Alternatives    map[string]float64 `json:"alternatives"`
	Stock           map[string]float64 `json:"stock"`
	Offering        map[string]float64 `json:"offering"`
	Fed             bool               `json:"fed"`
}

func positiveFinite(v float64) bool { return v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) }

func planTempleOffering(stock, values map[string]float64) (map[string]float64, bool) {
	return planGrainPart(stock, values, TempleOfferingGrain)
}
func planGrainPart(stock, values map[string]float64, grain float64) (map[string]float64, bool) {
	// The reference gift works even before a world's valuation index is ready.
	if stock["grain"] >= grain {
		return map[string]float64{"grain": grain}, true
	}
	due := grain * values["grain"]
	if !positiveFinite(due) {
		return map[string]float64{}, false
	}
	take := map[string]float64{}
	remaining := due
	for _, good := range grainOfferingGoods {
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
	return newTempleOfferForLevel(id, name, 1, 0, stock, values)
}
func newTempleOfferForLevel(id uuid.UUID, name string, level, staffed int, stock, values map[string]float64) TempleOffer {
	o := TempleOffer{SettlementID: id, Name: name, Level: level, Weight: TempleWeight(level), Staffed: staffed, Capacity: economy.TempleWorkplaces(level), Required: map[string]float64{}, Alternatives: map[string]float64{}, Stock: maps.Clone(stock), Offering: map[string]float64{}, Parts: titheParts(level)}
	for i := range o.Parts {
		part := &o.Parts[i]
		o.Required[part.Good] = part.Amount
		take := map[string]float64{}
		if part.Good == "grain" {
			take, part.Paid = planGrainPart(stock, values, part.Amount)
			for _, good := range grainOfferingGoods[1:] {
				if positiveFinite(values["grain"]) && positiveFinite(values[good]) {
					o.Alternatives[good] = part.Amount * values["grain"] / values[good]
				}
			}
		} else if stock[part.Good] >= part.Amount {
			part.Paid = true
			take[part.Good] = part.Amount
		}
		if part.Paid {
			o.PaidFraction += part.Share
			for good, amount := range take {
				o.Offering[good] += amount
			}
		}
	}
	o.Fed = o.PaidFraction >= 1-1e-9
	o.EffectiveWeight = EffectiveTempleWeight(level, staffed, o.Capacity, o.PaidFraction)
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
	id      uuid.UUID
	god     string
	level   int
	staffed int
}

// planCityOffers plans each temple's gift in turn against one shared store.
func planCityOffers(id uuid.UUID, name string, temples []templeRef, stock, values map[string]float64) []TempleOffer {
	left := maps.Clone(stock)
	out := make([]TempleOffer, 0, len(temples))
	for _, t := range temples {
		level := t.level
		if level == 0 {
			level = 1
		}
		o := newTempleOfferForLevel(id, name, level, t.staffed, left, values)
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
	 (SELECT array_agg(b.level ORDER BY b.god_id) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),
 (SELECT array_agg((SELECT count(*)::int FROM settlement_placement p WHERE p.building_id=b.id AND p.good_key='cult') ORDER BY b.god_id) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),
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
		var levels, staffed []int
		var stored map[string]float64
		if err := rows.Scan(&id, &name, &ids, &gods, &levels, &staffed, &stored); err != nil {
			return nil, err
		}
		stock := map[string]float64{}
		for _, key := range templeOfferingGoods {
			stock[key] = stored[key]
		}
		temples := make([]templeRef, len(ids))
		for i := range ids {
			temples[i] = templeRef{id: ids[i], god: gods[i], level: levels[i], staffed: staffed[i]}
		}
		offers = append(offers, planCityOffers(id, name, temples, stock, values)...)
	}
	return offers, rows.Err()
}

// chargeTempleOffering replans the city's temples under row locks and commits
// every local gift together. A failed later debit cannot leave an earlier good
// partially burned. Returns which temples were fed.
func (h *TickHandler) chargeTempleOffering(ctx context.Context, settlementID uuid.UUID, values map[string]float64) (map[uuid.UUID]TempleOffer, error) {
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
	trows, err := tx.Query(ctx, `SELECT id,god_id,level,(SELECT count(*) FROM settlement_placement p WHERE p.building_id=buildings.id AND p.good_key='cult') FROM buildings WHERE settlement_id=$1 AND building_type='temple' ORDER BY god_id`, settlementID)
	if err != nil {
		return nil, err
	}
	for trows.Next() {
		var t templeRef
		if err := trows.Scan(&t.id, &t.god, &t.level, &t.staffed); err != nil {
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
	fed := map[uuid.UUID]TempleOffer{}
	take := map[string]float64{}
	for _, o := range planCityOffers(settlementID, "", temples, stock, values) {
		if o.PaidFraction <= 0 {
			continue
		}
		fed[o.BuildingID] = o
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

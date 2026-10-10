package capabilities

// Live-DB unit tests, one satisfied/unsatisfied pair per non-trivial checker.
// Mirrors the testPool skip pattern used across the repo (see
// internal/kharis/grain_growth_test.go): skips (not fails) when DATABASE_URL
// isn't set, so `go test ./...` stays green without a database.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// fixture is a minimal world + player + capital settlement, ready for a
// checker to be pointed at. Created with world state='forming' so it never
// collides with the single-active-world partial unique index.
type fixture struct {
	pool         *pgxpool.Pool
	worldID      uuid.UUID
	playerID     uuid.UUID
	provinceID   uuid.UUID
	settlementID uuid.UUID
}

func newFixture(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()

	var worldID uuid.UUID
	must(t, pool.QueryRow(ctx,
		`INSERT INTO worlds (name, state, status, map_width, map_height)
		 VALUES ($1, 'forming', 'archived', 40, 30) RETURNING id`,
		"capabilities-test-"+uuid.NewString(),
	).Scan(&worldID))

	var playerID uuid.UUID
	must(t, pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"cap-test-"+uuid.NewString(),
	).Scan(&playerID))

	var provinceID uuid.UUID
	must(t, pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID))

	var settlementID uuid.UUID
	must(t, pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, is_capital, state, population)
		 VALUES ($1, $2, 'Testopolis', 'akhaier', $3, true, 'active', 500) RETURNING id`,
		worldID, provinceID, playerID,
	).Scan(&settlementID))

	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM worlds WHERE id = $1`, worldID) })

	return fixture{pool: pool, worldID: worldID, playerID: playerID, provinceID: provinceID, settlementID: settlementID}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("fixture setup: %v", err)
	}
}

func (f fixture) cc(clk clock.Clock) checkContext {
	return checkContext{
		ctx: context.Background(), pool: f.pool, clk: clk,
		worldID: f.worldID, provinceID: f.provinceID, playerID: f.playerID, settlementID: f.settlementID,
	}
}

func (f fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("fixture exec: %v (%s)", err, sql)
	}
}

func fakeClock(t time.Time) clock.Clock {
	return clock.NewTestClock(t)
}

// ---- colonize (the spec's worked example: "0/1 deployable") -----------------

func TestCanColonize_LockedWithoutDeployableUnit(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	v := canColonize(f.cc(fakeClock(time.Now())))
	if v.Available {
		t.Fatal("colonize must be locked with no deployable land unit")
	}
	if v.Requirements[0].Satisfied {
		t.Fatal("deployable-unit requirement must be unsatisfied")
	}
	if got, want := v.Requirements[0].Detail, "0/1 deployable"; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestCanColonize_UnlockedWithDeployableUnit(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
	           VALUES ($1, $2, 'spearman', 'land', 100, 0, 'garrison', $3)`,
		f.worldID, f.playerID, f.settlementID)

	v := canColonize(f.cc(fakeClock(time.Now())))
	if !v.Available {
		t.Fatalf("colonize must be unlocked with a deployable unit: %+v", v.Requirements)
	}
}

// A battle-worn garrison cohort (size < 100 after combat/desertion losses) is a
// finished, deployable unit — status is the gate, not size. Regression for the
// bug where the size>=100 filter treated a bloody veteran as an unfinished recruit.
func TestCanColonize_UnlockedWithBattleWornUnit(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
	           VALUES ($1, $2, 'spearman', 'land', 60, 0, 'garrison', $3)`,
		f.worldID, f.playerID, f.settlementID)

	v := canColonize(f.cc(fakeClock(time.Now())))
	if !v.Available {
		t.Fatalf("colonize must be unlocked with a battle-worn (size 60) garrison unit: %+v", v.Requirements)
	}
}

// ---- recruit (population + affordability) ------------------------------------

func TestCanRecruit_LockedWithoutPopulation(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `UPDATE settlements SET population = 0 WHERE id = $1`, f.settlementID)

	v := canRecruit(f.cc(fakeClock(time.Now())))
	if v.Available {
		t.Fatal("recruit must be locked with zero population")
	}
	if v.Requirements[0].Satisfied {
		t.Fatal("population requirement must be unsatisfied")
	}
}

func TestCanRecruit_UnlockedWithBarracksAndGoods(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'barracks')`, f.settlementID)
	f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
	           VALUES ($1, 'grain', 1000, 0, 5000, 0), ($1, 'silver', 1000, 0, 5000, 0)`, f.settlementID)

	v := canRecruit(f.cc(fakeClock(time.Now())))
	if !v.Available {
		t.Fatalf("recruit must be unlocked with population + barracks + goods: %+v", v.Requirements)
	}
}

// Independent whole-vessel costs catch accidentally pricing ships as 100-man cohorts.
func TestCanRecruit_NavalCrewAffordability(t *testing.T) {
	pool := testPool(t)
	for _, ship := range []struct {
		typ, material  string
		amount, silver float64
		foundry        bool
	}{
		{"galley", "timber", 30, 6, false},
		{"merchantman", "timber", 16, 2, false},
		{"war_galley", "cedar", 100, 30, true},
	} {
		t.Run(ship.typ, func(t *testing.T) {
			f := newFixture(t, pool)
			f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'shipyard')`, f.settlementID)
			if ship.foundry {
				f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'foundry')`, f.settlementID)
			}
			f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1, $2, $3, 0, 5000, 0), ($1, 'silver', $4, 0, 5000, 0)`, f.settlementID, ship.material, ship.amount, ship.silver)
			cc := f.cc(fakeClock(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)))
			check := func(want bool) {
				t.Helper()
				v := CanRecruit(cc)
				_, names, _ := strings.Cut(v.Requirements[1].Detail, "affordable now: ")
				listed := false
				for _, name := range strings.Split(names, ", ") {
					listed = listed || name == ship.typ
				}
				if listed != want || (want && !v.Available) {
					t.Fatalf("%s listed=%v available=%v, want listed=%v: %+v", ship.typ, listed, v.Available, want, v.Requirements)
				}
			}
			check(true)
			f.exec(t, `UPDATE settlement_goods SET amount = $3 WHERE settlement_id = $1 AND good_key = $2`, f.settlementID, ship.material, ship.amount-0.01)
			check(false)
			f.exec(t, `UPDATE settlement_goods SET amount = $3 WHERE settlement_id = $1 AND good_key = $2`, f.settlementID, ship.material, ship.amount)
			f.exec(t, `UPDATE settlement_goods SET amount = $2 WHERE settlement_id = $1 AND good_key = 'silver'`, f.settlementID, ship.silver-0.01)
			check(false)
			f.exec(t, `UPDATE settlement_goods SET amount = $2 WHERE settlement_id = $1 AND good_key = 'silver'`, f.settlementID, ship.silver)
			f.exec(t, `DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'shipyard'`, f.settlementID)
			check(false)
			if ship.foundry {
				f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'shipyard')`, f.settlementID)
				f.exec(t, `DELETE FROM buildings WHERE settlement_id = $1 AND building_type = 'foundry'`, f.settlementID)
				check(false)
			}
		})
	}
}

func TestCanRecruit_LandRequiresFullCohortGoods(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'barracks')`, f.settlementID)
	f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1, 'grain', 12, 0, 5000, 0), ($1, 'silver', 20, 0, 5000, 0)`, f.settlementID)
	cc := f.cc(fakeClock(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)))
	for _, stock := range []struct {
		grain, silver float64
		available     bool
	}{{12, 20, true}, {11.99, 20, false}, {12, 19.99, false}} {
		f.exec(t, `UPDATE settlement_goods SET amount = CASE good_key WHEN 'grain' THEN $2::double precision ELSE $3::double precision END WHERE settlement_id = $1`, f.settlementID, stock.grain, stock.silver)
		if v := CanRecruit(cc); v.Available != stock.available {
			t.Fatalf("grain=%v silver=%v: available=%v, want %v: %+v", stock.grain, stock.silver, v.Available, stock.available, v.Requirements)
		}
	}
}

// ---- rite (temple + kharis) ---------------------------------------------------

func TestCanRite_LockedWithoutTemple(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate, kharis_calc_tick)
	           VALUES ($1, $2, 1000, 0, 0)`, f.playerID, f.worldID)

	v := canRite(f.cc(fakeClock(time.Now())))
	if v.Available {
		t.Fatal("rite must be locked without a temple")
	}
}

func TestCanRite_UnlockedWithTempleAndKharis(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'temple')`, f.settlementID)
	f.exec(t, `INSERT INTO player_world_records (player_id, world_id, kharis_amount, kharis_rate, kharis_calc_tick)
	           VALUES ($1, $2, 1000, 0, 0)`, f.playerID, f.worldID)

	v := canRite(f.cc(fakeClock(time.Now())))
	if !v.Available {
		t.Fatalf("rite must be unlocked with temple + kharis: %+v", v.Requirements)
	}
}

// ---- trade-offer / sell / message (FOW visibility) -----------------------------

func TestCanTradeOfferAndMessage_LockedWithNoVisibleForeignCity(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)

	if v := canTradeOffer(f.cc(fakeClock(time.Now()))); v.Available {
		t.Fatal("trade-offer must be locked with no contacted foreign settlement")
	}
	if v := canMessage(f.cc(fakeClock(time.Now()))); v.Available {
		t.Fatal("message must be locked with no contacted foreign settlement")
	}
}

func TestCanTradeOfferAndSell_UnlockedWithVisibleForeignCity(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)

	// A second Wanax's capital one hex away — inside the FOW radius (6).
	var otherPlayer uuid.UUID
	must(t, pool.QueryRow(context.Background(),
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"cap-test-neighbour-"+uuid.NewString(),
	).Scan(&otherPlayer))
	var otherProvince uuid.UUID
	must(t, pool.QueryRow(context.Background(),
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&otherProvince))
	f.exec(t, `INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, is_capital, state, population)
	           VALUES ($1, $2, 'Neighbourtown', 'akhaier', $3, true, 'active', 500)`,
		f.worldID, otherProvince, otherPlayer)

	if v := canTradeOffer(f.cc(fakeClock(time.Now()))); !v.Requirements[0].Satisfied {
		t.Fatalf("trade-offer's visibility requirement must be satisfied: %+v", v.Requirements[0])
	}

	// sell additionally needs stock of a non-silver good.
	f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
	           VALUES ($1, 'copper', 50, 0, 1000, 0)`, f.settlementID)
	v := canSell(f.cc(fakeClock(time.Now())))
	if !v.Available {
		t.Fatalf("sell must be unlocked with a visible foreign city + goods in stock: %+v", v.Requirements)
	}
}

// ---- trade-accept (Fas 3.5 — pending offer + solvency) ------------------------

func TestCanTradeAccept_LockedWithNoPendingOffer(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)

	v := canTradeAccept(f.cc(fakeClock(time.Now())))
	if v.Available {
		t.Fatal("trade-accept must be locked with no pending inbound offer")
	}
	if len(v.Requirements) != 1 {
		t.Fatalf("with no pending offer, solvency must not be evaluated yet — got %d requirements, want 1", len(v.Requirements))
	}
}

// insertPendingBuyOffer creates a "buy" trade offer (a foreign Wanax wants
// wantGood, in exchange for offer_silver) delivered to f's settlement,
// pending acceptance — the seller side, which must hold wantGood to accept.
func (f fixture) insertPendingBuyOffer(t *testing.T, wantGood string, wantQty float64) {
	t.Helper()
	var senderID, originID uuid.UUID
	must(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"cap-test-buyer-"+uuid.NewString(),
	).Scan(&senderID))
	var otherProvince uuid.UUID
	must(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 1, 0, 'plains') RETURNING id`,
		f.worldID,
	).Scan(&otherProvince))
	must(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, is_capital, state, population)
		 VALUES ($1, $2, 'Buyertown', 'akhaier', $3, true, 'active', 500) RETURNING id`,
		f.worldID, otherProvince, senderID,
	).Scan(&originID))

	tradeOffer := fmt.Sprintf(`{"kind":"buy","want_good":%q,"want_qty":%v,"offer_silver":10,"status":"pending"}`, wantGood, wantQty)
	f.exec(t, `INSERT INTO messengers
	             (world_id, sender_id, origin_id, destination_id, message_text, trade_offer, hex_q, hex_r, status, arrives_at)
	           VALUES ($1, $2, $3, $4, 'trade', $5::jsonb, 1, 0, 'delivered', now())`,
		f.worldID, senderID, originID, f.settlementID, tradeOffer)
}

func TestCanTradeAccept_LockedWhenPendingButInsolvent(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.insertPendingBuyOffer(t, "copper", 100)
	// No copper in stock at all — the accepting seller cannot cover the offer.

	v := canTradeAccept(f.cc(fakeClock(time.Now())))
	if v.Available {
		t.Fatal("trade-accept must be locked when the pending offer cannot be afforded")
	}
	if len(v.Requirements) != 2 {
		t.Fatalf("with a pending offer, solvency must be evaluated too — got %d requirements, want 2", len(v.Requirements))
	}
	if v.Requirements[1].Satisfied {
		t.Fatal("solvency requirement must be unsatisfied with zero copper in stock")
	}
	if got := v.Requirements[1].Hint; got != HintTradeAcceptInsolvent {
		t.Errorf("solvency hint = %q, want %q (HintTradeAcceptInsolvent)", got, HintTradeAcceptInsolvent)
	}
}

func TestCanTradeAccept_UnlockedWhenPendingAndSolvent(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.insertPendingBuyOffer(t, "copper", 100)
	f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
	           VALUES ($1, 'copper', 200, 0, 1000, 0)`, f.settlementID)

	v := canTradeAccept(f.cc(fakeClock(time.Now())))
	if !v.Available {
		t.Fatalf("trade-accept must be unlocked with a pending, affordable offer: %+v", v.Requirements)
	}
}

// TestTradeOfferAffordable_MatchesTradeAcceptHandlerGate verifies the exact
// math api/handlers/messenger.go's TradeAccept uses to deduct a "buy" offer's
// want_qty of want_good from the accepting seller (and, for a "sell" offer,
// want_silver of silver from the accepting buyer) — TradeOfferAffordable is
// the single function both TradeAccept's precondition and this checker's
// solvency requirement are built from (Fas 3 anti-drift).
func TestTradeOfferAffordable_MatchesTradeAcceptHandlerGate(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
	           VALUES ($1, 'copper', 50, 0, 1000, 0)`, f.settlementID)
	cc := f.cc(fakeClock(time.Now()))

	if ok, have := TradeOfferAffordable(cc, "buy", "copper", 50, 0); !ok || have != 50 {
		t.Errorf("buy offer for exactly 50 copper: ok=%v have=%v, want ok=true have=50", ok, have)
	}
	if ok, _ := TradeOfferAffordable(cc, "buy", "copper", 51, 0); ok {
		t.Error("buy offer for 51 copper must be unaffordable with only 50 in stock")
	}
	if ok, have := TradeOfferAffordable(cc, "sell", "", 0, 1); ok || have != 0 {
		t.Errorf("sell offer wanting 1 silver with 0 silver in stock: ok=%v have=%v, want ok=false have=0", ok, have)
	}
}

// ---- FirstUnsatisfied (the 422 <-> actions hint contract, Fas 3) --------------

func TestFirstUnsatisfied_ReturnsFirstUnsatisfiedHint(t *testing.T) {
	v := verb("x", CategoryProvince, "p", []Requirement{
		{Satisfied: true, Hint: "irrelevant — already satisfied"},
		{Satisfied: false, Hint: "fix this first"},
		{Satisfied: false, Hint: "unreachable — never checked once an earlier one is unsatisfied"},
	})
	if got := FirstUnsatisfied(v); got != "fix this first" {
		t.Errorf("FirstUnsatisfied = %q, want %q", got, "fix this first")
	}

	allSatisfied := verb("x", CategoryProvince, "p", []Requirement{{Satisfied: true, Hint: "n/a"}})
	if got := FirstUnsatisfied(allSatisfied); got != "" {
		t.Errorf("FirstUnsatisfied on an available verb = %q, want \"\"", got)
	}
}

// ---- settlement-cap / population requirement split (Fas 3, shared with handlers) --

func TestSettlementCapRequirement_MatchesColonizesSecondRequirement(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	cc := f.cc(fakeClock(time.Now()))

	standalone := SettlementCapRequirement(cc)
	fromVerb := canColonize(cc).Requirements[1]
	if standalone != fromVerb {
		t.Errorf("SettlementCapRequirement() = %+v, want identical to canColonize's second requirement %+v", standalone, fromVerb)
	}
}

func TestPopulationRequirement_MatchesRecruitsFirstRequirement(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	cc := f.cc(fakeClock(time.Now()))

	standalone := PopulationRequirement(cc)
	fromVerb := canRecruit(cc).Requirements[0]
	if standalone != fromVerb {
		t.Errorf("PopulationRequirement() = %+v, want identical to canRecruit's first requirement %+v", standalone, fromVerb)
	}
}

// ---- trivial verbs (F3 — always listed, always available) ---------------------

func TestTrivialVerbs_AlwaysAvailable(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	cc := f.cc(fakeClock(time.Now()))

	for _, v := range []Verb{canBuild(cc), canCancelBuild(cc), canAllocate(cc)} {
		if !v.Available {
			t.Errorf("%s must be trivially available, got requirements %+v", v.Name, v.Requirements)
		}
		if len(v.Requirements) != 0 {
			t.Errorf("%s must have no requirements, got %+v", v.Name, v.Requirements)
		}
	}
}

// ---- registry shape -------------------------------------------------------------

func TestList_CoversAllSixCategories(t *testing.T) {
	// Kingdoms are gated behind KINGDOMS_ENABLED (post-MVP, Timothy 2026-07-08)
	// — set it here so this test still verifies every checker is registered.
	// The gate itself is covered by TestList_KingdomCategoryGatedByEnv.
	t.Setenv("KINGDOMS_ENABLED", "1")
	pool := testPool(t)
	f := newFixture(t, pool)
	verbs := List(context.Background(), pool, fakeClock(time.Now()), f.worldID, f.provinceID, f.playerID, f.settlementID)

	seen := map[string]bool{}
	for _, v := range verbs {
		seen[v.Category] = true
	}
	for _, cat := range []string{CategoryProvince, CategoryMilitary, CategoryTrade, CategoryDiplomacy, CategoryKingdom, CategoryCult} {
		if !seen[cat] {
			t.Errorf("no verb registered for category %q", cat)
		}
	}
	if len(verbs) != len(checkers) {
		t.Errorf("List returned %d verbs, want %d (one per checker)", len(verbs), len(checkers))
	}
}

// TestList_KingdomCategoryGatedByEnv verifies kingdoms are post-MVP by default
// (Timothy 2026-07-08): with KINGDOMS_ENABLED unset, the category must not
// appear in List's output at all — keryx and agent.py read only this list.
func TestList_KingdomCategoryGatedByEnv(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	verbs := List(context.Background(), pool, fakeClock(time.Now()), f.worldID, f.provinceID, f.playerID, f.settlementID)

	for _, v := range verbs {
		if v.Category == CategoryKingdom {
			t.Errorf("kingdom verb %q present in List() output with KINGDOMS_ENABLED unset", v.Name)
		}
	}
}

// ---- pure logic (no DB) ----------------------------------------------------------

func TestVerb_AvailableIsANDOfRequirements(t *testing.T) {
	allSatisfied := verb("x", CategoryProvince, "p", []Requirement{
		{Satisfied: true}, {Satisfied: true},
	})
	if !allSatisfied.Available {
		t.Error("Available must be true when every requirement is satisfied")
	}

	oneUnsatisfied := verb("x", CategoryProvince, "p", []Requirement{
		{Satisfied: true}, {Satisfied: false},
	})
	if oneUnsatisfied.Available {
		t.Error("Available must be false when any requirement is unsatisfied")
	}

	noReqs := verb("x", CategoryProvince, "p", nil)
	if !noReqs.Available {
		t.Error("Available must default true with no requirements (F3 trivial verbs)")
	}
	if noReqs.Requirements == nil {
		t.Error("Requirements must never be nil (JSON should encode [], not null)")
	}
}

// Cedar covers missing timber 1:1 (megaron_plan_cedar_virke.md): a Wanax with cedar and
// no timber must see a galley (30 timber) as affordable, and not one hair below.
func TestCanRecruit_CedarCoversTimberForGalley(t *testing.T) {
	pool := testPool(t)
	f := newFixture(t, pool)
	f.exec(t, `INSERT INTO buildings (settlement_id, building_type) VALUES ($1, 'shipyard')`, f.settlementID)
	f.exec(t, `INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick) VALUES ($1, 'cedar', 30, 0, 5000, 0), ($1, 'silver', 6, 0, 5000, 0)`, f.settlementID)
	cc := f.cc(fakeClock(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)))
	galleyListed := func() bool {
		v := CanRecruit(cc)
		_, names, _ := strings.Cut(v.Requirements[1].Detail, "affordable now: ")
		for _, name := range strings.Split(names, ", ") {
			if name == "galley" {
				return true
			}
		}
		return false
	}
	if !galleyListed() {
		t.Fatal("30 cedar and no timber must afford a galley")
	}
	f.exec(t, `UPDATE settlement_goods SET amount = 29.99 WHERE settlement_id = $1 AND good_key = 'cedar'`, f.settlementID)
	if galleyListed() {
		t.Fatal("29.99 cedar must not afford a 30-timber galley")
	}
}

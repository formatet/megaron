package economy

import (
	"os"
	"strconv"
	"strings"
)

// SitosConfig holds the tunables for the Sitos granary (σῖτος, Linear B
// "grain-watcher"): the food a city sets aside in good days and eats in lean
// ones. See temenos_sitos_magasin_plan.md for the model and the decisions
// behind it.
//
// It replaced a silver fund on 2026-08-02 (migration 106). The fund's tunables
// — SITOS_TAX_RATE, SITOS_REF_PRICE_FLOOR/CEILING, SITOS_FUND_CAP_MULT,
// SITOS_STARTING_FUND_DAYS, SITOS_PRICE_SMOOTHING_TICKS — are gone with it and
// are ignored if still set in the environment.
//
// All values are start-of-day guesses, not calibrated constants — override
// via env + `systemctl restart poleia` on the live server, no redeploy
// needed (mirrors internal/tick/worker.go's TICK_MINUTES pattern).
type SitosConfig struct {
	// GranaryCapDays: the granary holds at most this many days of the city's
	// food need. Bounding it in DAYS rather than in absolute units keeps a
	// pop=100 colony and a pop=20000 capital equally covered.
	GranaryCapDays float64
	// LowDays: below this coverage the granary releases, up to this level, as
	// far as it reaches. It may reach nothing — that is the only limit on the
	// granary's help (B2).
	LowDays float64
	// HighDays: above this coverage the granary takes its tithe, and only of
	// what is above it. That is what keeps it from ever biting a city that is
	// already short — no separate rule is needed for that case.
	HighDays float64
	// TithePct: the share of the surplus above HighDays taken per tick (B4 —
	// a tithe, not spill capture).
	TithePct float64
	// SubsistenceGoods lists the good keys the granary stores. Defaults to the
	// food basket, grain and fish (B6). Both feed civilians (grain first, fish
	// for the remainder — see FoodConsumptionSplit), so the granary holds what
	// the city actually eats.
	SubsistenceGoods []string
	// SilverLiquidCapDays: a settlement's liquid silver-good cap =
	// dailyGrainNeedInSilver × SilverLiquidCapDays (see GenesisSilverLiquid).
	SilverLiquidCapDays float64
	// SilverStartDays: a settlement's genesis liquid silver seed =
	// dailyGrainNeedInSilver × SilverStartDays (see GenesisSilverLiquid).
	SilverStartDays float64
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// LoadSitosConfig reads SITOS_* env vars once. Call at startup (cmd/server/
// main.go) and thread the returned value through every constructor that
// needs it — do not call this per-request.
func LoadSitosConfig() SitosConfig {
	goods := "grain,fish"
	if v := os.Getenv("SITOS_SUBSISTENCE_GOODS"); v != "" {
		goods = v
	}
	var list []string
	for _, g := range strings.Split(goods, ",") {
		if g = strings.TrimSpace(g); g != "" {
			list = append(list, g)
		}
	}
	return SitosConfig{
		// Defaults chosen so the granary fills in GAME DAYS, not months — that
		// was the whole SI4 problem with the fund (0 → cap took ~150 real days
		// at 60 min/tick). A tenth of the surplus per tick converges on the
		// high threshold inside a game day when there is a real surplus, and
		// the cap stops it there.
		GranaryCapDays:      envFloat("SITOS_GRANARY_CAP_DAYS", 60),
		LowDays:             envFloat("SITOS_LOW_DAYS", 10),
		HighDays:            envFloat("SITOS_HIGH_DAYS", 30),
		TithePct:            envFloat("SITOS_TITHE_PCT", 0.1),
		SubsistenceGoods:    list,
		SilverLiquidCapDays: envFloat("SITOS_SILVER_CAP_DAYS", 10),
		SilverStartDays:     envFloat("SITOS_SILVER_START_DAYS", 5),
	}
}

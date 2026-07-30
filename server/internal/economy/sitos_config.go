package economy

import (
	"os"
	"strconv"
	"strings"
)

// SitosConfig holds the tunables for the Sitos fund (σῖτος, Linear B "grain-
// watcher"): a capital-bounded silver pool per settlement that acts as the
// guaranteed last-resort counterparty for subsistence goods. See
// temenos_sitos.md for the full model.
//
// All values are start-of-day guesses, not calibrated constants — override
// via env + `systemctl restart poleia` on the live server, no redeploy
// needed (mirrors internal/tick/worker.go's TICK_MINUTES pattern).
type SitosConfig struct {
	// TaxRate is a per-capita rate, NOT a fraction of the settlement's silver:
	// applyTax (sitos_tick.go) computes desired = population × TaxRate, spread
	// evenly across TicksPerDay ticks, and moves that much silver into the
	// Sitos fund per day — a head tax charged regardless of whether the
	// settlement earned any silver that day. It is guarded so it never taxes
	// more than the settlement's current silver stock (and never overflows the
	// fund's cap), but those are affordability clamps, not the base formula.
	// Corrected 2026-07-30 — this comment previously described a silver-based
	// tax; verify the head-tax-vs-income-tax choice is intended before
	// retuning it (open question, not decided here).
	TaxRate float64
	// RefPriceFloor/Ceiling clamp the fund's smoothed shadow price.
	RefPriceFloor   float64
	RefPriceCeiling float64
	// FundCapMult: fund cap = dailyGrainNeedInSilver × FundCapMult.
	FundCapMult float64
	// StartingFundDays: genesis seed = dailyGrainNeedInSilver × StartingFundDays.
	StartingFundDays float64
	// PriceSmoothingTicks is the moving-average window (in ticks) for RefPrice.
	PriceSmoothingTicks int
	// SubsistenceGoods lists the good keys the fund stabilizes. Defaults to
	// the goods.category='staple' set (grain, fish).
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

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
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
		TaxRate:             envFloat("SITOS_TAX_RATE", 0.01),
		RefPriceFloor:       envFloat("SITOS_REF_PRICE_FLOOR", 0.5),
		RefPriceCeiling:     envFloat("SITOS_REF_PRICE_CEILING", 3.0),
		FundCapMult:         envFloat("SITOS_FUND_CAP_MULT", 1),
		StartingFundDays:    envFloat("SITOS_STARTING_FUND_DAYS", 2),
		PriceSmoothingTicks: envInt("SITOS_PRICE_SMOOTHING_TICKS", 6),
		SubsistenceGoods:    list,
		SilverLiquidCapDays: envFloat("SITOS_SILVER_CAP_DAYS", 10),
		SilverStartDays:     envFloat("SITOS_SILVER_START_DAYS", 5),
	}
}

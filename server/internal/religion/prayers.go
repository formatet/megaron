package religion

import "time"

// PrayerSpec defines a single prayer available to a culture.
// ID is unique across all prayers. EffectType selects the handler in api/handlers.
// MinKharis gates the prayer by divine mood (same thresholds as rite chance: 100/400/800).
// Cooldown is the minimum time between successive casts of the same prayer.
// TargetKind is "" for self-effect prayers, "province" when a target is meaningful.
// God and Name are display strings for UI and keryx voice.
type PrayerSpec struct {
	ID         string
	EffectType string             // "oracle_reveal_deposits" | "battle_frenzy" | "harvest_blessing"
	MinKharis  float64            // tier-gate (required divine standing, NOT a cost)
	Offering   map[string]float64 // good_key→amount, consumed on cast regardless of outcome
	Cooldown   time.Duration      // per (player, prayer)
	TargetKind string             // "" or "province"
	God        string
	Name       string
}

// Offerings are material sacrifices (wine/oil/grain) — the gods take them
// whether or not they answer. This is the deliberate economic sink that makes the
// grandest prayers demand trade goods you must acquire: religion drives trade.
// Kharis is NEVER part of an offering — it is standing, gated by MinKharis.

// EffectOracleRevealDeposits reveals nearby ore deposits to the caster.
const EffectOracleRevealDeposits = "oracle_reveal_deposits"

// EffectBattleFrenzy applies a combat strength buff.
const EffectBattleFrenzy = "battle_frenzy"

// EffectHarvestBlessing boosts grain production temporarily.
const EffectHarvestBlessing = "harvest_blessing"

// PrayerSpecs is the canonical prayer catalogue.
// Pattern mirrors province.UnitSpecs — a plain Go map, not a DB seed.
// New prayers = new map entries, no logic changes.
//
// Kharis is NEVER spent. MinKharis = required standing, not a cost.
var PrayerSpecs = map[string]PrayerSpec{
	// ── Akhaier (Olympians) ────────────────────────────────────────────────
	"akhaier_oracle_deposits": {
		ID: "akhaier_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 100, Offering: map[string]float64{"oil": 20, "wine": 10},
		Cooldown: 24 * time.Hour, TargetKind: "",
		God: "Apollon", Name: "Apollon's Sight",
	},
	"akhaier_harvest_blessing": {
		ID: "akhaier_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 400, Offering: map[string]float64{"wine": 15, "oil": 10},
		Cooldown: 12 * time.Hour, TargetKind: "",
		God: "Demeter", Name: "Demeter's Bounty",
	},
	"akhaier_battle_frenzy": {
		ID: "akhaier_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 100, Offering: map[string]float64{"grain": 10, "wine": 10},
		Cooldown: 6 * time.Hour, TargetKind: "",
		God: "Ares", Name: "Ares' Fury",
	},

	// ── Khemetiu (Egyptian) ───────────────────────────────────────────────
	"khemetiu_oracle_deposits": {
		ID: "khemetiu_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 100, Offering: map[string]float64{"grain": 25, "oil": 15},
		Cooldown: 24 * time.Hour, TargetKind: "",
		God: "Thoth", Name: "Thoth's Revelation",
	},
	"khemetiu_harvest_blessing": {
		ID: "khemetiu_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 400, Offering: map[string]float64{"grain": 20, "oil": 10},
		Cooldown: 12 * time.Hour, TargetKind: "",
		God: "Osiris", Name: "Osiris' Flood",
	},
	"khemetiu_battle_frenzy": {
		ID: "khemetiu_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 100, Offering: map[string]float64{"grain": 10, "wine": 10},
		Cooldown: 6 * time.Hour, TargetKind: "",
		God: "Sekhmet", Name: "Sekhmet's Wrath",
	},

	// ── Kna'ani (Baal / Levant) ───────────────────────────────────────────
	"knaani_oracle_deposits": {
		ID: "knaani_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 100, Offering: map[string]float64{"oil": 20, "wine": 15},
		Cooldown: 24 * time.Hour, TargetKind: "",
		God: "El", Name: "El's Oracle",
	},
	"knaani_harvest_blessing": {
		ID: "knaani_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 400, Offering: map[string]float64{"wine": 15, "oil": 10},
		Cooldown: 12 * time.Hour, TargetKind: "",
		God: "Baal", Name: "Baal's Rain",
	},
	"knaani_battle_frenzy": {
		ID: "knaani_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 100, Offering: map[string]float64{"wine": 10, "grain": 10},
		Cooldown: 6 * time.Hour, TargetKind: "",
		God: "Anat", Name: "Anat's Rage",
	},

	// ── Thrakes ──────────────────────────────────────────────────────────
	"thrakes_oracle_deposits": {
		ID: "thrakes_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 100, Offering: map[string]float64{"wine": 25, "oil": 10},
		Cooldown: 24 * time.Hour, TargetKind: "",
		God: "Sabazios", Name: "Sabazios' Dream",
	},
	"thrakes_harvest_blessing": {
		ID: "thrakes_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 400, Offering: map[string]float64{"wine": 20, "oil": 10},
		Cooldown: 12 * time.Hour, TargetKind: "",
		God: "Bendis", Name: "Bendis' Harvest",
	},
	"thrakes_battle_frenzy": {
		ID: "thrakes_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 100, Offering: map[string]float64{"wine": 25},
		Cooldown: 6 * time.Hour, TargetKind: "",
		God: "Ares Thrakios", Name: "Thrakian Battle Rage",
	},

	// ── Minoan ───────────────────────────────────────────────────────────
	"minoan_oracle_deposits": {
		ID: "minoan_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 100, Offering: map[string]float64{"oil": 20, "wine": 15},
		Cooldown: 24 * time.Hour, TargetKind: "",
		God: "Potnia", Name: "Potnia's Vision",
	},
	"minoan_harvest_blessing": {
		ID: "minoan_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 400, Offering: map[string]float64{"wine": 15, "oil": 10},
		Cooldown: 12 * time.Hour, TargetKind: "",
		God: "Britomartis", Name: "Britomartis' Gift",
	},
	"minoan_battle_frenzy": {
		ID: "minoan_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 100, Offering: map[string]float64{"grain": 10, "wine": 10},
		Cooldown: 6 * time.Hour, TargetKind: "",
		God: "Poseidon", Name: "Poseidon's Storm",
	},

	// ── Hatti (Hittite) ──────────────────────────────────────────────────
	"hatti_oracle_deposits": {
		ID: "hatti_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 100, Offering: map[string]float64{"grain": 20, "wine": 15},
		Cooldown: 24 * time.Hour, TargetKind: "",
		God: "Hepat", Name: "Hepat's Counsel",
	},
	"hatti_harvest_blessing": {
		ID: "hatti_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 400, Offering: map[string]float64{"wine": 15, "oil": 10},
		Cooldown: 12 * time.Hour, TargetKind: "",
		God: "Telipinu", Name: "Telipinu's Return",
	},
	"hatti_battle_frenzy": {
		ID: "hatti_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 100, Offering: map[string]float64{"grain": 15, "wine": 10},
		Cooldown: 6 * time.Hour, TargetKind: "",
		God: "Teshub", Name: "Teshub's Thunder",
	},
}

// CulturePrayers maps a culture key to its allowed prayer IDs.
// A culture only sees its own prayer names — same effect behind the scenes.
var CulturePrayers = map[string][]string{
	"akhaier":  {"akhaier_oracle_deposits", "akhaier_harvest_blessing", "akhaier_battle_frenzy"},
	"khemetiu": {"khemetiu_oracle_deposits", "khemetiu_harvest_blessing", "khemetiu_battle_frenzy"},
	"knaani":   {"knaani_oracle_deposits", "knaani_harvest_blessing", "knaani_battle_frenzy"},
	"thrakes":  {"thrakes_oracle_deposits", "thrakes_harvest_blessing", "thrakes_battle_frenzy"},
	"minoan":   {"minoan_oracle_deposits", "minoan_harvest_blessing", "minoan_battle_frenzy"},
	"hatti":    {"hatti_oracle_deposits", "hatti_harvest_blessing", "hatti_battle_frenzy"},
}

// AllowedForCulture returns true if the prayer is in the culture's allowed list.
func AllowedForCulture(culture, prayerID string) bool {
	for _, id := range CulturePrayers[culture] {
		if id == prayerID {
			return true
		}
	}
	return false
}

// DefaultBattleFrenzyFor returns the battle_frenzy prayer ID for a culture.
// Used for backward-compatible rite calls that omit the prayer parameter.
func DefaultBattleFrenzyFor(culture string) string {
	return culture + "_battle_frenzy"
}

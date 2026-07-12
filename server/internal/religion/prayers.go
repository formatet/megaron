package religion

// PrayerSpec defines a single prayer available to a culture.
// ID is unique across all prayers. EffectType selects the handler in api/handlers.
// MinKharis gates the prayer by divine mood (same tiers as the mood table:
// 5/30/60 on the 0-100 scale, Timothy 2026-07-09 kharis omdesign — was
// 100/400/800 on the old 0-2000 scale. Strawman — temenos_balans_spakar.md §9).
// CooldownTicks is the minimum number of world ticks between successive casts of the same prayer.
// TargetKind is "" for self-effect prayers, "province" when a target is meaningful.
// God and Name are display strings for UI and keryx voice.
// Description is a short human-readable line of what the prayer DOES if the gods
// answer (Plan A / A7, megaron_kult_legibilitet_plan.md) — shown in `available_prayers`
// and keryx `rite --list` so a Wanax knows what they're casting for before they
// commit an offering. Never states an odds — that stays internal to the Rite handler.
type PrayerSpec struct {
	ID            string
	EffectType    string             // "oracle_reveal_deposits" | "battle_frenzy" | "harvest_blessing"
	MinKharis     float64            // tier-gate (required divine standing, NOT a cost)
	Offering      map[string]float64 // good_key→amount, consumed on cast regardless of outcome
	CooldownTicks int                // minimum world ticks between casts; 0 = no cooldown
	TargetKind    string             // "" or "province"
	God           string
	Name          string
	Description   string // human-readable effect line — no odds, ever
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
		MinKharis: 5, Offering: map[string]float64{"oil": 20, "wine": 10},
		CooldownTicks: 24, TargetKind: "",
		God: "Apollon", Name: "Apollon's Sight",
		Description: "Reveals nearby ore deposits (tin, copper, or silver) for colonisation.",
	},
	"akhaier_harvest_blessing": {
		ID: "akhaier_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 30, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Demeter", Name: "Demeter's Bounty",
		Description: "Blesses the harvest — grain stores swell by a quarter.",
	},
	"akhaier_battle_frenzy": {
		ID: "akhaier_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 5, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Ares", Name: "Ares' Fury",
		Description: "Grants your garrison battle frenzy — a temporary combat-strength boost.",
	},

	// ── Khemetiu (Egyptian) ───────────────────────────────────────────────
	"khemetiu_oracle_deposits": {
		ID: "khemetiu_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 5, Offering: map[string]float64{"grain": 25, "oil": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "Thoth", Name: "Thoth's Revelation",
		Description: "Reveals nearby ore deposits (tin, copper, or silver) for colonisation.",
	},
	"khemetiu_harvest_blessing": {
		ID: "khemetiu_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 30, Offering: map[string]float64{"grain": 20, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Osiris", Name: "Osiris' Flood",
		Description: "Blesses the harvest — grain stores swell by a quarter.",
	},
	"khemetiu_battle_frenzy": {
		ID: "khemetiu_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 5, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Sekhmet", Name: "Sekhmet's Wrath",
		Description: "Grants your garrison battle frenzy — a temporary combat-strength boost.",
	},

	// ── Kna'ani (Baal / Levant) ───────────────────────────────────────────
	"knaani_oracle_deposits": {
		ID: "knaani_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 5, Offering: map[string]float64{"oil": 20, "wine": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "El", Name: "El's Oracle",
		Description: "Reveals nearby ore deposits (tin, copper, or silver) for colonisation.",
	},
	"knaani_harvest_blessing": {
		ID: "knaani_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 30, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Baal", Name: "Baal's Rain",
		Description: "Blesses the harvest — grain stores swell by a quarter.",
	},
	"knaani_battle_frenzy": {
		ID: "knaani_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 5, Offering: map[string]float64{"wine": 10, "grain": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Anat", Name: "Anat's Rage",
		Description: "Grants your garrison battle frenzy — a temporary combat-strength boost.",
	},

	// ── Thrakes ──────────────────────────────────────────────────────────
	"thrakes_oracle_deposits": {
		ID: "thrakes_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 5, Offering: map[string]float64{"wine": 25, "oil": 10},
		CooldownTicks: 24, TargetKind: "",
		God: "Sabazios", Name: "Sabazios' Dream",
		Description: "Reveals nearby ore deposits (tin, copper, or silver) for colonisation.",
	},
	"thrakes_harvest_blessing": {
		ID: "thrakes_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 30, Offering: map[string]float64{"wine": 20, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Bendis", Name: "Bendis' Harvest",
		Description: "Blesses the harvest — grain stores swell by a quarter.",
	},
	"thrakes_battle_frenzy": {
		ID: "thrakes_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 5, Offering: map[string]float64{"wine": 25},
		CooldownTicks: 6, TargetKind: "",
		God: "Ares Thrakios", Name: "Thrakian Battle Rage",
		Description: "Grants your garrison battle frenzy — a temporary combat-strength boost.",
	},

	// ── Minoan ───────────────────────────────────────────────────────────
	"minoan_oracle_deposits": {
		ID: "minoan_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 5, Offering: map[string]float64{"oil": 20, "wine": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "Potnia", Name: "Potnia's Vision",
		Description: "Reveals nearby ore deposits (tin, copper, or silver) for colonisation.",
	},
	"minoan_harvest_blessing": {
		ID: "minoan_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 30, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Britomartis", Name: "Britomartis' Gift",
		Description: "Blesses the harvest — grain stores swell by a quarter.",
	},
	"minoan_battle_frenzy": {
		ID: "minoan_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 5, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Poseidon", Name: "Poseidon's Storm",
		Description: "Grants your garrison battle frenzy — a temporary combat-strength boost.",
	},

	// ── Hatti (Hittite) ──────────────────────────────────────────────────
	"hatti_oracle_deposits": {
		ID: "hatti_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: 5, Offering: map[string]float64{"grain": 20, "wine": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "Hepat", Name: "Hepat's Counsel",
		Description: "Reveals nearby ore deposits (tin, copper, or silver) for colonisation.",
	},
	"hatti_harvest_blessing": {
		ID: "hatti_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: 30, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Telipinu", Name: "Telipinu's Return",
		Description: "Blesses the harvest — grain stores swell by a quarter.",
	},
	"hatti_battle_frenzy": {
		ID: "hatti_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: 5, Offering: map[string]float64{"grain": 15, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Teshub", Name: "Teshub's Thunder",
		Description: "Grants your garrison battle frenzy — a temporary combat-strength boost.",
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

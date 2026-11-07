package religion

// PrayerSpec defines a single prayer available to a culture.
// ID is unique across all prayers. EffectType selects the handler in api/handlers.
// MinKharis gates the prayer by divine mood — always one of the three
// exported tier constants in this package (MoodSuspicious/MoodIndifferent/
// MoodFavorable, 5/30/60 on the 0-100 scale, Timothy 2026-07-09 kharis
// omdesign — was 100/400/800 on the old 0-2000 scale. Strawman —
// temenos_balans_spakar.md §9). Never restate the numbers as a new literal —
// that is exactly how mood.go's table and this one drifted apart
// (megaron_plan_kultbrunnen.md §6).
// CooldownTicks is the minimum number of world ticks between successive casts of the same prayer.
// TargetKind is "" for self-effect prayers, "province" when a target is meaningful.
// God and Name are display strings for UI and keryx voice.
// Description is a short human-readable line of what the prayer DOES if the gods
// answer (Plan A / A7, megaron_kult_legibilitet_plan.md) — shown in `available_prayers`
// and keryx `rite --list` so a Wanax knows what they're casting for before they
// commit an offering. Never states an odds — that stays internal to the Rite handler.
type PrayerSpec struct {
	ID            string
	GodID         string             // stable cooldown key; empty preserves legacy per-temple prayers
	EffectType    string             // "oracle_reveal_deposits" | "battle_frenzy" | "harvest_blessing"
	MinKharis     float64            // tier-gate (required divine standing, NOT a cost)
	Offering      map[string]float64 // good_key→amount, consumed on cast regardless of outcome
	CooldownTicks int                // minimum world ticks between casts; 0 = no cooldown
	TargetKind    string             // "" or "province"
	God           string
	Name          string
	Description   string // human-readable effect line — no odds, ever
	// Favours is this god's taste: good_key→weight over the offering. Goods
	// absent weigh affinityDefault — an unfavoured gift is a lesser gift, never
	// an insult. Empty means "use the effect's archetype", see FavoursFor.
	Favours map[string]float64
}

// FavoursFor returns the god's taste for a prayer, falling back to the archetype
// of its effect. Gods who do the same work want the same kinds of things: the
// one who finds ore wants what is rare and worked, the one who feeds a people
// wants what grows, the one who wins battles wants what is slaughtered and drunk.
// Per-prayer Favours override this when a god needs a character of their own.
func FavoursFor(spec PrayerSpec) map[string]float64 {
	if len(spec.Favours) > 0 {
		return spec.Favours
	}
	return favoursByEffect[spec.EffectType]
}

// favoursByEffect keeps the catalogue honest: 18 hand-written taste maps would
// drift apart within a month. Weights are strawman — tune against soak data.
var favoursByEffect = map[string]map[string]float64{
	EffectOracleRevealDeposits: {
		// The seer wants what is dug and worked — like calls to like.
		"purple": 2.0, "oil": 1.5, "silver": 1.4, "bronze": 1.3, "pottery": 1.1,
		"grain": 0.6, "fish": 0.5, "stone": 0.4, "incense": 2.1, // plan said 3.0; held under the sea-god fish weight (2.2) so fish stays the favourite
	},
	EffectHarvestBlessing: {
		// The one who feeds a people wants what grows and what is poured out.
		"wine": 2.0, "oil": 1.6, "grain": 1.3, "livestock": 1.3, "fish": 0.9,
		"stone": 0.3, "timber": 0.4, "tin": 0.5, "copper": 0.5, "incense": 2.0,
	},
	EffectBattleFrenzy: {
		// The war god wants blood, drink and the means of war.
		"livestock": 2.0, "wine": 1.7, "horses": 1.6, "bronze": 1.5, "grain": 1.0,
		"pottery": 0.5, "stone": 0.4, "fish": 0.4, "incense": 1.5,
	},
}

// Sea gods prize fish above all else (Timothy 2026-08-07,
// megaron_plan_varukatalogen.md §S5: "fish → favoritoffer hos vissa gudar
// (Havsdaimonerna/Potnia Theron); mer kharis per enhet, aldrig ett krav").
// A tabellutvidgning, not a new mechanic — fish becomes worth more in
// OfferingWorth for these two prayers specifically, never required.
//
// Per-prayer Favours REPLACES the archetype's whole taste map (FavoursFor
// never merges), so each override restates its archetype's other weights
// unchanged and raises only fish above the archetype's previous top weight.
var (
	potniaTheronOracleFavours = map[string]float64{
		"purple": 2.0, "oil": 1.5, "silver": 1.4, "bronze": 1.3, "pottery": 1.1,
		"grain": 0.6, "fish": 2.2, "stone": 0.4, "incense": 2.1, // just under fish: the sea gods prize fish above everything (TestSeaGods_FavourFishAboveEverything)
	}
	havsdaimonernaBattleFrenzyFavours = map[string]float64{
		"livestock": 2.0, "wine": 1.7, "horses": 1.6, "bronze": 1.5, "grain": 1.0,
		"pottery": 0.5, "stone": 0.4, "fish": 2.2, "incense": 1.5,
	}
)

// Offerings are material sacrifices (wine/oil/grain) — the gods take them
// whether or not they answer. This is the deliberate economic sink that makes the
// grandest prayers demand trade goods you must acquire: religion drives trade.
// Kharis is NEVER part of an offering — it is standing, gated by MinKharis.

// EffectOracleRevealDeposits reveals an unseen ore deposit anywhere in the world to the caster.
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
		MinKharis: MoodSuspicious, Offering: map[string]float64{"oil": 20, "wine": 10},
		CooldownTicks: 24, TargetKind: "",
		God: "Apollon", Name: "Apollon's Sight",
		Description: "Reveals one ore deposit you have never seen — copper, tin or silver, somewhere in the world.",
	},
	"akhaier_harvest_blessing": {
		ID: "akhaier_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: MoodIndifferent, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Demeter", Name: "Demeter's Bounty",
		Description: "Blesses the harvest — grain, oil and wine yield a quarter more for 25 game days.",
	},
	"akhaier_battle_frenzy": {
		ID: "akhaier_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Ares", Name: "Ares' Fury",
		Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.",
	},

	// ── Khemetiu (Egyptian) ───────────────────────────────────────────────
	"khemetiu_oracle_deposits": {
		ID: "khemetiu_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 25, "oil": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "Thoth", Name: "Thoth's Revelation",
		Description: "Reveals one ore deposit you have never seen — copper, tin or silver, somewhere in the world.",
	},
	"khemetiu_harvest_blessing": {
		ID: "khemetiu_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: MoodIndifferent, Offering: map[string]float64{"grain": 20, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Osiris", Name: "Osiris' Flood",
		Description: "Blesses the harvest — grain, oil and wine yield a quarter more for 25 game days.",
	},
	"khemetiu_battle_frenzy": {
		ID: "khemetiu_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Sekhmet", Name: "Sekhmet's Wrath",
		Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.",
	},

	// ── Kna'ani (Baal / Levant) ───────────────────────────────────────────
	"knaani_oracle_deposits": {
		ID: "knaani_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"oil": 20, "wine": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "El", Name: "El's Oracle",
		Description: "Reveals one ore deposit you have never seen — copper, tin or silver, somewhere in the world.",
	},
	"knaani_harvest_blessing": {
		ID: "knaani_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: MoodIndifferent, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Baal", Name: "Baal's Rain",
		Description: "Blesses the harvest — grain, oil and wine yield a quarter more for 25 game days.",
	},
	"knaani_battle_frenzy": {
		ID: "knaani_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"wine": 10, "grain": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Anat", Name: "Anat's Rage",
		Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.",
	},

	// ── Thrakes ──────────────────────────────────────────────────────────
	"thrakes_oracle_deposits": {
		ID: "thrakes_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"wine": 25, "oil": 10},
		CooldownTicks: 24, TargetKind: "",
		God: "Sabazios", Name: "Sabazios' Dream",
		Description: "Reveals one ore deposit you have never seen — copper, tin or silver, somewhere in the world.",
	},
	"thrakes_harvest_blessing": {
		ID: "thrakes_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: MoodIndifferent, Offering: map[string]float64{"wine": 20, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Bendis", Name: "Bendis' Harvest",
		Description: "Blesses the harvest — grain, oil and wine yield a quarter more for 25 game days.",
	},
	"thrakes_battle_frenzy": {
		ID: "thrakes_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"wine": 25},
		CooldownTicks: 6, TargetKind: "",
		God: "Ares Thrakios", Name: "Thrakian Battle Rage",
		Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.",
	},

	// ── Minoan ───────────────────────────────────────────────────────────
	"minoan_oracle_deposits": {
		ID: "minoan_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"oil": 20, "wine": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "Potnia", Name: "Potnia's Vision",
		Description: "Reveals one ore deposit you have never seen — copper, tin or silver, somewhere in the world.",
		Favours:     potniaTheronOracleFavours,
	},
	"minoan_harvest_blessing": {
		ID: "minoan_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: MoodIndifferent, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Britomartis", Name: "Britomartis' Gift",
		Description: "Blesses the harvest — grain, oil and wine yield a quarter more for 25 game days.",
	},
	"minoan_battle_frenzy": {
		ID: "minoan_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Poseidon", Name: "Poseidon's Storm",
		Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.",
		Favours:     havsdaimonernaBattleFrenzyFavours,
	},

	// ── Hatti (Hittite) ──────────────────────────────────────────────────
	"hatti_oracle_deposits": {
		ID: "hatti_oracle_deposits", EffectType: EffectOracleRevealDeposits,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 20, "wine": 15},
		CooldownTicks: 24, TargetKind: "",
		God: "Hepat", Name: "Hepat's Counsel",
		Description: "Reveals one ore deposit you have never seen — copper, tin or silver, somewhere in the world.",
	},
	"hatti_harvest_blessing": {
		ID: "hatti_harvest_blessing", EffectType: EffectHarvestBlessing,
		MinKharis: MoodIndifferent, Offering: map[string]float64{"wine": 15, "oil": 10},
		CooldownTicks: 12, TargetKind: "",
		God: "Telipinu", Name: "Telipinu's Return",
		Description: "Blesses the harvest — grain, oil and wine yield a quarter more for 25 game days.",
	},
	"hatti_battle_frenzy": {
		ID: "hatti_battle_frenzy", EffectType: EffectBattleFrenzy,
		MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 15, "wine": 10},
		CooldownTicks: 6, TargetKind: "",
		God: "Teshub", Name: "Teshub's Thunder",
		Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.",
	},
}

// Alpha IDs never reinterpret a historical RiteCast prayer/effect.
const (
	PrayerPotniaVision = "minoan_potnia_vision_v2"
	PrayerPotniaWalls  = "minoan_potnia_walls"
	PrayerGreenShoots  = "minoan_velchanos_green_shoots"
	// Retired 2b (replaced by the two prayers of speed): the ids stay so an old RiteCast is still readable
	// and a departure_prayers row left pending is still honoured at departure.
	PrayerAsterionThread    = "minoan_asterion_thread"
	PrayerStillnessPassage  = "minoan_stillness_passage"
	EffectOracleMemory      = "oracle_memory_v2"
	EffectGarrisonDefense   = "garrison_defense"
	EffectNextLandDeparture = "next_land_departure"
	EffectNextShipDeparture = "next_ship_departure"

	// Stormbönerna (steg 3b): the Sea Daimons of Stillness still a named storm or wake an ordinary one.
	// Not blessings: no scope, no duration; the effect is done by combat.CalmStorm / combat.WakeStorm.
	PrayerStillnessCalm = "minoan_stillness_calm"
	PrayerStillnessWake = "minoan_stillness_wake"
	EffectStormCalm     = "storm_calm"
	EffectStormWake     = "storm_wake"

	// Murarna faller (steg 5c): Potnia's offensive prayer. The effect is done by a scheduled
	// WallsFall event WallsFallTicks after the cast (combat.WallsFallHandler).
	PrayerPotniaWallsFall = "minoan_potnia_walls_fall"
	EffectWallsFall       = "walls_fall"
	WallsFallTicks        = 3
)

func init() {
	PrayerSpecs[PrayerPotniaVision] = PrayerSpec{ID: PrayerPotniaVision, GodID: "potnia", EffectType: EffectOracleMemory, MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 10, "silver": 3}, CooldownTicks: 24, God: "Potnia", Name: "Potnia's Vision", Description: "Marks an unseen source of copper, tin, silver or incense and the unknown ground within two hexes as memory; no live activity.", Favours: potniaTheronOracleFavours}
	PrayerSpecs[PrayerPotniaWalls] = PrayerSpec{ID: PrayerPotniaWalls, GodID: "potnia", EffectType: EffectGarrisonDefense, MinKharis: MoodSuspicious, Offering: map[string]float64{"grain": 8, "timber": 4}, CooldownTicks: 24, God: "Potnia", Name: "Mistress of the Walls", Description: "Your own garrison defending this city fights a quarter stronger for 6 game days.", Favours: favoursByEffect[EffectBattleFrenzy]}
	PrayerSpecs[PrayerGreenShoots] = PrayerSpec{ID: PrayerGreenShoots, GodID: "velchanos", EffectType: EffectHarvestBlessing, MinKharis: MoodIndifferent, Offering: map[string]float64{"grain": 15, "silver": 2}, CooldownTicks: 25, God: "Velchanos", Name: "Green Shoots", Description: "Grain, oil and wine yield a quarter more for 25 game days; fish and the herd are unchanged."}
	PrayerSpecs[PrayerAsterionSwift] = PrayerSpec{ID: PrayerAsterionSwift, GodID: "asterion", EffectType: EffectBlessedSpeedLand, MinKharis: MoodSuspicious, Offering: map[string]float64{"timber": 4, "silver": 3}, CooldownTicks: 12, God: "Asterion", Name: "Swift Feet", Description: "Runners and land caravans you name take a quarter less time over what is left of their road, also when already under way, for 12 game days: one Runner or caravan, the city's, or all of them. A wider blessing costs more."}
	PrayerSpecs[PrayerVelchanosStrength] = PrayerSpec{ID: PrayerVelchanosStrength, GodID: "velchanos", EffectType: EffectBlessedStrength, MinKharis: MoodSuspicious, Offering: map[string]float64{"livestock": 6, "wine": 4}, CooldownTicks: 12, God: "Velchanos", Name: "The Young God's Arm", Description: "Land troops you name fight a quarter stronger for 6 game days, wherever they stand: one unit, the city's units, or the whole realm. A wider blessing costs more.", Favours: favoursByEffect[EffectBattleFrenzy]}
	PrayerSpecs[PrayerAsterionHorns] = PrayerSpec{ID: PrayerAsterionHorns, GodID: "asterion", EffectType: EffectBlessedHorns, MinKharis: MoodSuspicious, Offering: map[string]float64{"livestock": 5, "silver": 2}, CooldownTicks: 12, God: "Asterion", Name: "The Bull's Horns", Description: "Land troops you name strike half as hard again in the first round of a battle, for 12 game days, wherever they stand: one unit, the city's units, or the whole realm. Later rounds are as usual, and so is any troop that joins a battle once it has begun. A wider blessing costs more.", Favours: favoursByEffect[EffectBattleFrenzy]}
	PrayerSpecs[PrayerAsterionThreadV2] = PrayerSpec{ID: PrayerAsterionThreadV2, GodID: "asterion", EffectType: EffectBlessedThread, MinKharis: MoodSuspicious, Offering: map[string]float64{"wine": 3, "silver": 2}, CooldownTicks: 12, God: "Asterion", Name: "The Thread", Description: "Land caravans you name can only be taken by a sentry standing within one hex, not two, for 12 game days, also when already on the road: one caravan, the city's, or all of them. Runners are never taken, and ships are not helped. A wider blessing costs more."}
	PrayerSpecs[PrayerStillnessHull] = PrayerSpec{ID: PrayerStillnessHull, GodID: "sea_daimons_stillness", EffectType: EffectBlessedHull, MinKharis: MoodSuspicious, Offering: map[string]float64{"fish": 8, "oil": 4}, CooldownTicks: 12, God: "The Sea Daimons of Stillness", Name: "Stout Hulls", Description: "Each ship you name takes the next storm it meets unharmed, for 12 game days: one ship, the city's ships, or the whole fleet. A wider blessing costs more.", Favours: havsdaimonernaBattleFrenzyFavours}
	PrayerSpecs[PrayerStillnessSwift] = PrayerSpec{ID: PrayerStillnessSwift, GodID: "sea_daimons_stillness", EffectType: EffectBlessedSpeedSea, MinKharis: MoodSuspicious, Offering: map[string]float64{"fish": 6, "silver": 3}, CooldownTicks: 12, God: "The Sea Daimons of Stillness", Name: "Following Seas", Description: "Ships you name take a quarter less time over what is left of their voyage, also when already at sea, for 12 game days: one ship, the city's ships, or the whole fleet. A wider blessing costs more."}
	PrayerSpecs[PrayerPotniaFertility] = PrayerSpec{ID: PrayerPotniaFertility, GodID: "potnia", EffectType: EffectBlessedFertility, MinKharis: MoodSuspicious, Offering: map[string]float64{"livestock": 4, "grain": 4}, CooldownTicks: 12, God: "Potnia", Name: "Fruitful Season", Description: "People and herds grow half as fast again for 20 game days, in this city or in the whole realm. A fed city only: hunger still thins a people. A wider blessing costs more.", Favours: favoursByEffect[EffectHarvestBlessing]}
	PrayerSpecs[PrayerVelchanosBuilding] = PrayerSpec{ID: PrayerVelchanosBuilding, GodID: "velchanos", EffectType: EffectBlessedBuilding, MinKharis: MoodSuspicious, Offering: map[string]float64{"timber": 3, "wine": 2}, CooldownTicks: 12, God: "Velchanos", Name: "The Builder's Blessing", Description: "What is left of a building or a ship under construction takes three quarters of the time, once for each, and so does each one begun while the blessing lasts, for 12 game days: in this city or in the whole realm. A wider blessing costs more."}
	PrayerSpecs[PrayerPotniaScapegoat] = PrayerSpec{ID: PrayerPotniaScapegoat, GodID: "potnia", EffectType: EffectScapegoat, MinKharis: MoodSuspicious, Offering: map[string]float64{"livestock": 3}, CooldownTicks: 12, God: "Potnia", Name: "The Scapegoat", Description: "The next divine punishment that would strike any of your cities within 30 game days is borne by a goat instead: nothing is lost, and the gods' mercy holds as after any punishment. It does not lower the chance of a punishment. The realm only; one goat at a time.", Favours: favoursByEffect[EffectHarvestBlessing]}
	PrayerSpecs[PrayerPotniaWallsFall] = PrayerSpec{ID: PrayerPotniaWallsFall, GodID: "potnia", EffectType: EffectWallsFall, MinKharis: MoodSuspicious, Offering: map[string]float64{"livestock": 12, "silver": 8}, CooldownTicks: 24, God: "Potnia", Name: "The Walls Fall", Description: "A city of another Wanax that you know and remember with a wall loses one wall level 3 game days after the prayer. You hear only that Potnia listened; you learn the result by seeing the city again. A battle already under way at the walls is not changed. If that Wanax stands well with the gods they feel the wrath gather, never from whom. A richer gift does no more.", Favours: favoursByEffect[EffectBattleFrenzy]}
	PrayerSpecs[PrayerStillnessCalm] = PrayerSpec{ID: PrayerStillnessCalm, GodID: "sea_daimons_stillness", EffectType: EffectStormCalm, MinKharis: MoodSuspicious, Offering: map[string]float64{"fish": 10, "oil": 6}, CooldownTicks: 12, God: "The Sea Daimons of Stillness", Name: "Still the Storm", Description: "A storm you have named and keep on your storm map shrinks by one hex at once, for everyone's good; the hex grows back after 24 game days. Three such prayers within 24 game days kill a storm of three hexes. A richer gift does no more.", Favours: havsdaimonernaBattleFrenzyFavours}
	PrayerSpecs[PrayerStillnessWake] = PrayerSpec{ID: PrayerStillnessWake, GodID: "sea_daimons_stillness", EffectType: EffectStormWake, MinKharis: MoodSuspicious, Offering: map[string]float64{"fish": 10, "wine": 6}, CooldownTicks: 12, God: "The Sea Daimons of Stillness", Name: "Wake the Storm", Description: "A new storm is born on a sea hex of your own map. It lives as long as any storm and strikes every ship it meets, yours too. Nobody can tell it was woken. A richer gift does no more.", Favours: havsdaimonernaBattleFrenzyFavours}
}

// CulturePrayers maps a culture key to its allowed prayer IDs.
// A culture only sees its own prayer names — same effect behind the scenes.
var CulturePrayers = map[string][]string{
	"akhaier":  {"akhaier_oracle_deposits", "akhaier_harvest_blessing", "akhaier_battle_frenzy"},
	"khemetiu": {"khemetiu_oracle_deposits", "khemetiu_harvest_blessing", "khemetiu_battle_frenzy"},
	"knaani":   {"knaani_oracle_deposits", "knaani_harvest_blessing", "knaani_battle_frenzy"},
	"thrakes":  {"thrakes_oracle_deposits", "thrakes_harvest_blessing", "thrakes_battle_frenzy"},
	"minoan":   {PrayerPotniaVision, PrayerPotniaWalls, PrayerGreenShoots, PrayerPotniaFertility, PrayerPotniaScapegoat, PrayerPotniaWallsFall, PrayerVelchanosBuilding, PrayerAsterionSwift, PrayerAsterionHorns, PrayerAsterionThreadV2, PrayerStillnessSwift, PrayerVelchanosStrength, PrayerStillnessHull, PrayerStillnessCalm, PrayerStillnessWake},
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

// DefaultBattleFrenzyFor returns the default cultural prayer (legacy function name).
// Minoan defaults to Walls; historical cultures retain battle frenzy.
// Used for backward-compatible rite calls that omit the prayer parameter.
func DefaultBattleFrenzyFor(culture string) string {
	if culture == "minoan" {
		return PrayerPotniaWalls
	}
	return culture + "_battle_frenzy"
}

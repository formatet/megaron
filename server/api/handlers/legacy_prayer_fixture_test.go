package handlers

import "formatet/megaron/server/internal/religion"

// The five-culture prayer catalogue was removed from production (no Wanax can
// be anything but minoan). The rite handler still carries the legacy
// (godless, dice-rolled, offer_multiplier) path, and these tests pin it, so
// they register the two legacy prayers they use. Remove this file together
// with that path.
func init() {
	religion.PrayerSpecs["akhaier_oracle_deposits"] = religion.PrayerSpec{
		ID: "akhaier_oracle_deposits", EffectType: religion.EffectOracleRevealDeposits,
		MinKharis: religion.MoodSuspicious, Offering: map[string]float64{"oil": 20, "wine": 10},
		CooldownTicks: 24, God: "Apollon", Name: "Apollon's Sight",
	}
	religion.PrayerSpecs["akhaier_battle_frenzy"] = religion.PrayerSpec{
		ID: "akhaier_battle_frenzy", EffectType: religion.EffectBattleFrenzy,
		MinKharis: religion.MoodSuspicious, Offering: map[string]float64{"grain": 10, "wine": 10},
		CooldownTicks: 6, God: "Ares", Name: "Ares' Fury",
	}
	religion.CulturePrayers["akhaier"] = []string{"akhaier_oracle_deposits", "akhaier_battle_frenzy"}
}

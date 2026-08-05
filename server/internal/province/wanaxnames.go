package province

import (
	"context"
	"math/rand"
	"strings"
)

// WanaxNamePool is the minoan/Aegean personal-name pool for public Wanax
// identities (mvp/minoisk-identitet, Timothy 2026-08-05: "vi måste börja dela
// ut minoiska namn random på spelare så att folk inte använder deras
// logins"). Mostly attested Linear B anthroponyms.
var WanaxNamePool = []string{
	"Alkeus", "Anokwotas", "Areios", "Aigiwalos", "Aithalos", "Aukewas",
	"Amphimedes", "Daigwotas", "Dikonaros", "Dunios", "Erita", "Eumedes",
	"Gwasileus", "Hektor", "Italaios", "Karpathia", "Kokidas", "Komawes",
	"Koturos", "Knidia", "Kwerekwotas", "Lawodokos", "Lykoros", "Makhawon",
	"Mikharios", "Nedawatas", "Okunawos", "Perimos", "Philona", "Philonetas",
	"Plouteus", "Sakereus", "Thalamikas", "Theseus", "Turios", "Uwamia",
	"Widowoios", "Wedaneus", "Amnisia", "Rhadamas", "Sarpedas", "Deukalas",
	"Katreus", "Idomas",
}

// UniqueWanaxName picks a name from WanaxNamePool no player in the database
// already holds. Mirrors UniqueSettlementName's shape: a name lookup must
// never block a join, so a DB error falls back to a plain random pick from
// the full pool — a duplicate name is a nuisance, a failed join is not.
func UniqueWanaxName(ctx context.Context, db Queryer) (string, error) {
	taken, err := takenWanaxNames(ctx, db)
	if err != nil {
		return WanaxNamePool[rand.Intn(len(WanaxNamePool))], nil
	}
	return wanaxNameExcluding(taken), nil
}

// takenWanaxNames returns every wanax_name already assigned, keyed
// lower-cased.
func takenWanaxNames(ctx context.Context, db Queryer) (map[string]bool, error) {
	rows, err := db.Query(ctx, `SELECT wanax_name FROM players WHERE wanax_name IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taken := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		taken[strings.ToLower(strings.TrimSpace(n))] = true
	}
	return taken, rows.Err()
}

// wanaxNameExcluding is the pure half of UniqueWanaxName: the pool minus
// names already spoken for. The pool is smaller than 100 players, so once it
// is exhausted names suffix with a roman ordinal — Alkeus, then Alkeus II,
// Alkeus III — same fallback as settlementNameExcluding (names.go). It never
// returns an empty string: the loop terminates because `taken` is finite and
// every ordinal adds len(WanaxNamePool) fresh candidates.
func wanaxNameExcluding(taken map[string]bool) string {
	free := make([]string, 0, len(WanaxNamePool))
	for _, n := range WanaxNamePool {
		if !taken[strings.ToLower(n)] {
			free = append(free, n)
		}
	}
	if len(free) > 0 {
		return free[rand.Intn(len(free))]
	}
	for ord := 2; ; ord++ {
		suffix := " " + roman(ord)
		for _, i := range rand.Perm(len(WanaxNamePool)) {
			if cand := WanaxNamePool[i] + suffix; !taken[strings.ToLower(cand)] {
				return cand
			}
		}
	}
}

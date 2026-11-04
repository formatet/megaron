package ledger

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writers is the whitelist of production files that may write settlement_goods.
// A new writer must be added HERE, on purpose, together with a ledger category
// (ledger.Reason / ledger.ExecAs) — otherwise its writes show up as "Other".
// value = true: the file labels its own writes; false: its callers label them
// (or it only settles accrual, which the trigger books as production).
var writers = map[string]bool{
	"api/handlers/create_metropolis.go":  true,
	"api/handlers/helpers.go":            true,
	"api/handlers/kingdom.go":            true,
	"api/handlers/settlement.go":         true,
	"internal/combat/march_start.go":     true,
	"internal/combat/occupation.go":      true,
	"internal/combat/road.go":            true,
	"internal/combat/silver_audit_v2.go": false, // accrual only
	"internal/combat/unit_arrival.go":    true,
	"internal/combat/upkeep.go":          true,
	"internal/economy/food_basket.go":    false, // TakeFood: callers label
	"internal/economy/food_tick.go":      true,
	"internal/economy/recompute.go":      true,
	"internal/kharis/temple_offering.go": true,
	"internal/kharis/tick.go":            true,
	"internal/ledger/ledger.go":          false,
	"internal/province/goods_cost.go":    false, // DeductGood: callers label
}

var writeRE = regexp.MustCompile(`(UPDATE|INSERT INTO) settlement_goods`)

func TestOnlyWhitelistedFilesWriteSettlementGoods(t *testing.T) {
	root := "../.."
	found := map[string]bool{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if !writeRE.Match(b) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		found[rel] = true
		labels, listed := writers[rel]
		if !listed {
			t.Errorf("%s writes settlement_goods but is not in the ledger whitelist (guard_test.go) — name its category with ledger.Reason", rel)
			return nil
		}
		if labels && !strings.Contains(string(b), "ledger.") {
			t.Errorf("%s is whitelisted as labelling its writes but never calls the ledger package", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel := range writers {
		if !found[rel] {
			t.Errorf("whitelisted %s no longer writes settlement_goods — remove it from the list", rel)
		}
	}
}

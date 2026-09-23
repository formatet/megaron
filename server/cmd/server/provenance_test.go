package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"testing"
)

func TestCommitFromBuildInfo(t *testing.T) {
	vcs := func(rev, modified string) *debug.BuildInfo {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: rev}, {Key: "vcs.modified", Value: modified},
		}}
	}
	cases := []struct {
		name, stamped string
		info          *debug.BuildInfo
		ok            bool
		want          string
	}{
		{"ldflags stamp wins over vcs", "abc1234", vcs("ffffffffffff", "false"), true, "abc1234"},
		{"vcs revision is shortened", "unknown", vcs("186ceb02e0ee3972", "false"), true, "186ceb0"},
		{"modified tree says so", "unknown", vcs("186ceb02e0ee3972", "true"), true, "186ceb0+dirty"},
		{"no vcs stamp is unknown, never guessed", "unknown", &debug.BuildInfo{}, true, "unknown"},
		{"no build info at all", "unknown", nil, false, "unknown"},
	}
	for _, c := range cases {
		if got := commitFromBuildInfo(c.stamped, c.info, c.ok); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// healthz must report the migration the database is actually on — read from
// schema_migrations, compared here against the highest migration in the repo
// (tools/gotest.sh migrates a fresh DB to exactly that).
func TestHealthz_ReportsCommitAndMigration(t *testing.T) {
	pool := retentionTestPool(t)
	rec := httptest.NewRecorder()
	healthz(pool, "abc1234")(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Status    string `json:"status"`
		Commit    string `json:"commit"`
		Migration *int   `json:"migration"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Commit != "abc1234" {
		t.Errorf("status/commit = %q/%q", body.Status, body.Commit)
	}
	files, _ := filepath.Glob("../../db/migrations/*.up.sql")
	re := regexp.MustCompile(`^(\d+)_`)
	var nums []int
	for _, f := range files {
		if m := re.FindStringSubmatch(filepath.Base(f)); m != nil {
			n, _ := strconv.Atoi(m[1])
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	if len(nums) == 0 {
		t.Fatal("no migrations found")
	}
	if body.Migration == nil || *body.Migration != nums[len(nums)-1] {
		t.Errorf("migration = %v, want %d", body.Migration, nums[len(nums)-1])
	}
}

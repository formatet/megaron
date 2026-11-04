package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// claimAt runs ClaimFoundingPlaces for a pretend settlement at (q,0) and commits.
func (f reachFixture) claimAt(t *testing.T, q int) *FoundingPlaces {
	t.Helper()
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	fp, err := ClaimFoundingPlaces(ctx, tx, events.NewStore(f.pool), f.world, f.owner, uuid.New(), "Iolkos", q, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return fp
}

func (f reachFixture) countKind(t *testing.T, kind string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind=$2`, f.world, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func placesOf(t *testing.T, fp *FoundingPlaces) []map[string]any {
	t.Helper()
	if fp == nil {
		t.Fatal("expected a PlacesInNewCity notice, got none")
	}
	return fp.body["places"].([]map[string]any)
}

// Both fords lie inside radius 2 of q=4: one notice, both claimed for the founder.
func TestFoundingPlacesOneNoticeBothClaimed(t *testing.T) {
	f := reachWorld(t)
	fp := f.claimAt(t, 4)
	if got := len(placesOf(t, fp)); got != 2 {
		t.Fatalf("places listed=%d want 2 (two rivers)", got)
	}
	if n := f.countKind(t, "PlacesInNewCity"); n != 1 {
		t.Fatalf("notices=%d want exactly 1", n)
	}
	var owned int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM place_names WHERE world_id=$1 AND claimed_by=$2`, f.world, f.owner).Scan(&owned); err != nil || owned != 2 {
		t.Fatalf("claimed for founder=%d err=%v want 2", owned, err)
	}
}

// The ford at q=6 is at distance 3 from q=3, just outside the catchment: unclaimed, unlisted.
func TestFoundingPlacesOutsideCatchmentUntouched(t *testing.T) {
	f := reachWorld(t)
	fp := f.claimAt(t, 3)
	places := placesOf(t, fp)
	if len(places) != 1 || places[0]["place_id"] != "2,0" {
		t.Fatalf("listed=%v want only the ford at 2,0", places)
	}
	var claimed *uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `SELECT claimed_by FROM place_names WHERE world_id=$1 AND anchor_q=6`, f.world).Scan(&claimed); err == nil && claimed != nil {
		t.Fatal("place outside the catchment was claimed")
	}
}

// A place someone already holds is left out; with nothing left, no notice at all.
func TestFoundingPlacesAlreadyClaimedOmittedAndEmptyIsSilent(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	var other uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "other-"+uuid.New().String()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,claimed_by,claimed_tick) VALUES($1,'river',2,0,$2,1)`, f.world, other); err != nil {
		t.Fatal(err)
	}
	places := placesOf(t, f.claimAt(t, 4))
	if len(places) != 1 || places[0]["place_id"] != "6,0" {
		t.Fatalf("listed=%v want only the free ford at 6,0", places)
	}
	// A second founding finds nothing unclaimed: empty list, no notice.
	if fp := f.claimAt(t, 4); fp != nil {
		t.Fatalf("rerun produced a notice: %v", fp.body)
	}
	if n := f.countKind(t, "PlacesInNewCity"); n != 1 {
		t.Fatalf("notices=%d want 1", n)
	}
}

// After the founding claim, a march through the catchment gives no PlaceFirstReached.
func TestFoundingPlacesLaterMarchGivesNoFirstReached(t *testing.T) {
	f := reachWorld(t)
	f.claimAt(t, 4)
	f.mover(t, "infantry", f.owner, reachRoute(0, 6, 0, 6))
	f.scan(t, 7)
	if n := f.countKind(t, "PlaceFirstReached"); n != 0 {
		t.Fatalf("PlaceFirstReached=%d want 0 after founding claim", n)
	}
	if f.claims(t) != 2 {
		t.Fatalf("claims=%d want 2", f.claims(t))
	}
}

// foundColony claims and queues the same single notice for post-commit delivery.
func TestFoundColonyAnnouncesCatchmentPlacesOnce(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	var capitalProv uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 20, 20, 'plains') RETURNING id`, f.world).Scan(&capitalProv); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital) VALUES ($1,$2,'Capital City','achaean',$3,'capital',true)`, f.world, capitalProv, f.owner); err != nil {
		t.Fatal(err)
	}
	var colonyProv, unitID uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 4, 0, 'plains') RETURNING id`, f.world).Scan(&colonyProv); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units (world_id, owner_id, type, category, size, status, q, r) VALUES ($1,$2,'spearman','land',1000,'marching',4,0) RETURNING id`, f.world, f.owner).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	name := "Newhaven"
	var outcomes []expeditionOutcome
	h := &UnitArrivalHandler{pool: f.pool, eventStore: events.NewStore(f.pool), hub: &fakeBroadcaster{}, clk: f.clk, expeditionOutcomes: &outcomes}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	u := unitRow{id: unitID, ownerID: f.owner, utype: "spearman", category: "land", size: 1000, status: "marching", q: 4, r: 0, colonyName: &name}
	if err := h.foundColony(ctx, tx, u, colonyProv, 4, 0, f.world); err != nil {
		t.Fatalf("foundColony: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.countKind(t, "PlacesInNewCity"); n != 1 {
		t.Fatalf("notices=%d want 1", n)
	}
	if len(outcomes) != 1 || outcomes[0].kind != "PlacesInNewCity" {
		t.Fatalf("outcomes=%v want one PlacesInNewCity queued for delivery", outcomes)
	}
	var raw []byte
	if err := f.pool.QueryRow(ctx, `SELECT body_json FROM notifications WHERE world_id=$1 AND kind='PlacesInNewCity'`, f.world).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Name   string `json:"settlement_name"`
		Places []struct {
			Kind string `json:"kind"`
		} `json:"places"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Name != "Newhaven" || len(body.Places) != 2 {
		t.Fatalf("body=%s err=%v", raw, fmt.Sprint(err))
	}
}

package gossip

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool connects to a real Postgres instance for integration tests — gossip
// propagation is pure SQL orchestration (ON CONFLICT dedup, recency gates,
// hex-distance joins) that a mock can't meaningfully stand in for. Skips (not
// fails) when DATABASE_URL isn't set, so `go test ./...` stays green in
// environments without a database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testWorld(t *testing.T, pool *pgxpool.Pool, playerNames ...string) (worldID uuid.UUID, playerIDs []uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'archived') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}

	for _, name := range playerNames {
		var pid uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
			name+"-"+uuid.New().String(), name+"-"+uuid.New().String()+"@test.invalid",
		).Scan(&pid); err != nil {
			t.Fatalf("create test player %s: %v", name, err)
		}
		playerIDs = append(playerIDs, pid)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM worlds WHERE id = $1`, worldID)
		for _, pid := range playerIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id = $1`, pid)
		}
	})
	return worldID, playerIDs
}

func testSettlement(t *testing.T, pool *pgxpool.Pool, worldID, ownerID uuid.UUID, q, r int, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, $2, $3, 'plains') RETURNING id`,
		worldID, q, r,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create test province: %v", err)
	}
	var settlementID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, $3, 'achaean', $4, 'capital', true) RETURNING id`,
		worldID, provinceID, name, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create test settlement: %v", err)
	}
	return settlementID
}

// TestBroadcast verifies a fresh rumor reaches every settlement owner within
// radius, all sharing one rumor_id at hops=0.
func TestBroadcast(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := testWorld(t, pool, "source-owner", "nearby-owner", "far-owner")
	sourceOwner, nearbyOwner, farOwner := players[0], players[1], players[2]

	source := testSettlement(t, pool, worldID, sourceOwner, 0, 0, "Sourceton")
	testSettlement(t, pool, worldID, nearbyOwner, 1, 0, "Nearville")
	testSettlement(t, pool, worldID, farOwner, 50, 50, "Farhaven")

	if err := Broadcast(ctx, pool, worldID, source, "political", "Big news", 3,
		ImportanceMinor, uuid.Nil, ""); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	var nearbyRumorID uuid.UUID
	var nearbyHops int
	if err := pool.QueryRow(ctx,
		`SELECT rumor_id, hops FROM gossip_events WHERE world_id = $1 AND recipient_id = $2`,
		worldID, nearbyOwner,
	).Scan(&nearbyRumorID, &nearbyHops); err != nil {
		t.Fatalf("expected nearby owner to receive gossip: %v", err)
	}
	if nearbyHops != 0 {
		t.Errorf("expected hops=0 for a freshly broadcast rumor, got %d", nearbyHops)
	}

	var farCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2`,
		worldID, farOwner,
	).Scan(&farCount); err != nil {
		t.Fatalf("count far owner gossip: %v", err)
	}
	if farCount != 0 {
		t.Errorf("far owner (outside radius) should not receive the rumor, got %d rows", farCount)
	}
}

// TestPropagateOnContact verifies mechanism 2: a rumor known to a settlement
// owner (hops=0) reaches a contact at hops=1 with the same rumor_id; repeat
// contact does not duplicate it; and a rumor already at the hop ceiling does
// not propagate further. Both seeded rumors default to importance='minor'.
func TestPropagateOnContact(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := testWorld(t, pool, "learner-a", "teacher-b")
	learnerA, teacherB := players[0], players[1]
	settlementB := testSettlement(t, pool, worldID, teacherB, 0, 0, "Teacherton")

	freshRumor := uuid.New()
	deadRumor := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops)
		 VALUES ($1, $2, 'Somewhere', 'political', 'Big news', $3, 0)`,
		worldID, teacherB, freshRumor,
	); err != nil {
		t.Fatalf("seed fresh rumor: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops)
		 VALUES ($1, $2, 'Elsewhere', 'political', 'Old news', $3, 3)`,
		worldID, teacherB, deadRumor,
	); err != nil {
		t.Fatalf("seed dead rumor: %v", err)
	}

	if err := PropagateOnContact(ctx, pool, learnerA, settlementB, worldID); err != nil {
		t.Fatalf("PropagateOnContact: %v", err)
	}

	var hops int
	var region string
	if err := pool.QueryRow(ctx,
		`SELECT hops, source_region FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerA, freshRumor,
	).Scan(&hops, &region); err != nil {
		t.Fatalf("expected learner to receive the fresh rumor at hops+1: %v", err)
	}
	if hops != 1 {
		t.Errorf("expected hops=1, got %d", hops)
	}
	if region != "Teacherton" {
		t.Errorf("expected source_region=Teacherton (the contact point), got %q", region)
	}

	var deadCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerA, deadRumor,
	).Scan(&deadCount); err != nil {
		t.Fatalf("count dead rumor propagation: %v", err)
	}
	if deadCount != 0 {
		t.Errorf("a minor rumor already at hops>=1 must not propagate further, got %d rows", deadCount)
	}

	// A second contact with the same source must not duplicate the rumor.
	if err := PropagateOnContact(ctx, pool, learnerA, settlementB, worldID); err != nil {
		t.Fatalf("PropagateOnContact (2nd contact): %v", err)
	}
	var freshCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerA, freshRumor,
	).Scan(&freshCount); err != nil {
		t.Fatalf("count fresh rumor rows after 2nd contact: %v", err)
	}
	if freshCount != 1 {
		t.Errorf("expected exactly one copy of the rumor at the learner, got %d", freshCount)
	}
}

// TestMinorRumorPropagatesOnlyFromWitness verifies the PASS 2b gate: a MINOR
// rumor only travels from a holder at hops=0 (a firsthand witness). Once a
// learner receives it at hops=1, they hold it but cannot pass it on again —
// the classic "I heard it from a friend, but I'm not spreading it further"
// rule (temenos_gossip.md PASS 2b).
func TestMinorRumorPropagatesOnlyFromWitness(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := testWorld(t, pool, "witness-b", "learner-a", "third-c")
	witnessB, learnerA, thirdC := players[0], players[1], players[2]
	settlementB := testSettlement(t, pool, worldID, witnessB, 0, 0, "Witnessville")
	settlementA := testSettlement(t, pool, worldID, learnerA, 10, 10, "Learnerton")

	rumorID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops, importance)
		 VALUES ($1, $2, 'Somewhere', 'economy', 'A copper mine has opened.', $3, 0, 'minor')`,
		worldID, witnessB, rumorID,
	); err != nil {
		t.Fatalf("seed witness rumor: %v", err)
	}

	// A contacts B (the witness) — A should receive the rumor at hops=1.
	if err := PropagateOnContact(ctx, pool, learnerA, settlementB, worldID); err != nil {
		t.Fatalf("PropagateOnContact A<-B: %v", err)
	}
	var aHops int
	if err := pool.QueryRow(ctx,
		`SELECT hops FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerA, rumorID,
	).Scan(&aHops); err != nil {
		t.Fatalf("expected A to receive the minor rumor from the witness: %v", err)
	}
	if aHops != 1 {
		t.Errorf("expected A's copy at hops=1, got %d", aHops)
	}

	// C now contacts A. A's copy is at hops=1 (no longer a firsthand witness),
	// so the minor rumor must NOT propagate further.
	if err := PropagateOnContact(ctx, pool, thirdC, settlementA, worldID); err != nil {
		t.Fatalf("PropagateOnContact C<-A: %v", err)
	}
	var cCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, thirdC, rumorID,
	).Scan(&cCount); err != nil {
		t.Fatalf("count C's copies: %v", err)
	}
	if cCount != 0 {
		t.Errorf("a minor rumor held at hops=1 must not propagate to a second hop, got %d rows", cCount)
	}
}

// TestMajorRumorPropagatesAcrossHops verifies the PASS 2b gate: a MAJOR rumor
// (a settlement falling) travels as hearsay while hops < maxMajorHops (4), and
// stops once it reaches the ceiling.
func TestMajorRumorPropagatesAcrossHops(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := testWorld(t, pool, "teacher-b", "learner-a", "learner-d")
	teacherB, learnerA, learnerD := players[0], players[1], players[2]
	settlementB := testSettlement(t, pool, worldID, teacherB, 0, 0, "Teacherton")
	settlementA := testSettlement(t, pool, worldID, learnerA, 10, 10, "Learnerton")

	travelingRumor := uuid.New()
	ceilingRumor := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops, importance)
		 VALUES ($1, $2, 'Somewhere', 'military', 'A city has fallen.', $3, 3, 'major')`,
		worldID, teacherB, travelingRumor,
	); err != nil {
		t.Fatalf("seed hops=3 major rumor: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops, importance)
		 VALUES ($1, $2, 'Somewhere', 'military', 'Old news of a fallen city.', $3, 4, 'major')`,
		worldID, teacherB, ceilingRumor,
	); err != nil {
		t.Fatalf("seed hops=4 major rumor: %v", err)
	}

	if err := PropagateOnContact(ctx, pool, learnerA, settlementB, worldID); err != nil {
		t.Fatalf("PropagateOnContact A<-B: %v", err)
	}

	var aHops int
	if err := pool.QueryRow(ctx,
		`SELECT hops FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerA, travelingRumor,
	).Scan(&aHops); err != nil {
		t.Fatalf("expected A to receive the major rumor at hops<4: %v", err)
	}
	if aHops != 4 {
		t.Errorf("expected A's copy at hops=4, got %d", aHops)
	}

	var ceilingCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerA, ceilingRumor,
	).Scan(&ceilingCount); err != nil {
		t.Fatalf("count ceiling rumor propagation: %v", err)
	}
	if ceilingCount != 0 {
		t.Errorf("a major rumor already at hops=4 (the ceiling) must not propagate further, got %d rows", ceilingCount)
	}

	// D now contacts A, whose copy of travelingRumor sits at hops=4 (the
	// ceiling) — it must not travel a further hop.
	if err := PropagateOnContact(ctx, pool, learnerD, settlementA, worldID); err != nil {
		t.Fatalf("PropagateOnContact D<-A: %v", err)
	}
	var dCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM gossip_events WHERE world_id = $1 AND recipient_id = $2 AND rumor_id = $3`,
		worldID, learnerD, travelingRumor,
	).Scan(&dCount); err != nil {
		t.Fatalf("count D's copies: %v", err)
	}
	if dCount != 0 {
		t.Errorf("a major rumor at the hop ceiling must not travel further, got %d rows", dCount)
	}
}

// TestBroadcastUpsertsKnownSettlement verifies that a rumor naming a subject
// settlement registers it as rumour-known (known_settlements, level='rumour')
// for every recipient, carrying the industry hint.
func TestBroadcastUpsertsKnownSettlement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := testWorld(t, pool, "source-owner", "nearby-owner")
	sourceOwner, nearbyOwner := players[0], players[1]

	source := testSettlement(t, pool, worldID, sourceOwner, 0, 0, "Coppertown")
	testSettlement(t, pool, worldID, nearbyOwner, 1, 0, "Nearville")

	if err := Broadcast(ctx, pool, worldID, source, "economy", "A copper mine has opened.", 3,
		ImportanceMinor, source, "copper"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	var level, hint string
	if err := pool.QueryRow(ctx,
		`SELECT level, industry_hint FROM known_settlements
		 WHERE world_id = $1 AND player_id = $2 AND settlement_id = $3`,
		worldID, nearbyOwner, source,
	).Scan(&level, &hint); err != nil {
		t.Fatalf("expected known_settlements row for the recipient: %v", err)
	}
	if level != "rumour" {
		t.Errorf("expected level=rumour, got %q", level)
	}
	if hint != "copper" {
		t.Errorf("expected industry_hint=copper, got %q", hint)
	}
}

// TestPropagateOnContactUpsertsKnownSettlement verifies that a rumor with a
// subject settlement, propagated via contact, also registers rumour-known
// knowledge for the learner (not just the original broadcast recipients).
func TestPropagateOnContactUpsertsKnownSettlement(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	worldID, players := testWorld(t, pool, "teacher-b", "learner-a", "subject-owner")
	teacherB, learnerA, subjectOwner := players[0], players[1], players[2]
	settlementB := testSettlement(t, pool, worldID, teacherB, 0, 0, "Teacherton")
	subjectSettlement := testSettlement(t, pool, worldID, subjectOwner, 20, 20, "Tinhaven")

	rumorID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO gossip_events
		   (world_id, recipient_id, source_region, category, text, rumor_id, hops,
		    importance, subject_settlement_id, industry_hint)
		 VALUES ($1, $2, 'Somewhere', 'economy', 'A tin mine has opened.', $3, 0, 'minor', $4, 'tin')`,
		worldID, teacherB, rumorID, subjectSettlement,
	); err != nil {
		t.Fatalf("seed subject rumor: %v", err)
	}

	if err := PropagateOnContact(ctx, pool, learnerA, settlementB, worldID); err != nil {
		t.Fatalf("PropagateOnContact: %v", err)
	}

	var level, hint string
	if err := pool.QueryRow(ctx,
		`SELECT level, industry_hint FROM known_settlements
		 WHERE world_id = $1 AND player_id = $2 AND settlement_id = $3`,
		worldID, learnerA, subjectSettlement,
	).Scan(&level, &hint); err != nil {
		t.Fatalf("expected known_settlements row for the learner: %v", err)
	}
	if level != "rumour" {
		t.Errorf("expected level=rumour, got %q", level)
	}
	if hint != "tin" {
		t.Errorf("expected industry_hint=tin, got %q", hint)
	}
}

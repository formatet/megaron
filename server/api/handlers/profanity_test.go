package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fula ord (Timothy 2026-10-29): a coarse word is replaced, never refused, and
// each one costs the writer's realm kharis with a GodsAngered notice.

func setKharis(t *testing.T, pool *pgxpool.Pool, player, world uuid.UUID, k float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO player_world_records(player_id,world_id,kharis_amount,kharis_rate,kharis_calc_tick)
		 VALUES($1,$2,$3,0,(SELECT current_tick FROM worlds WHERE id=$2))
		 ON CONFLICT (player_id,world_id) DO UPDATE SET kharis_amount=$3, kharis_rate=0, kharis_calc_tick=EXCLUDED.kharis_calc_tick`,
		player, world, k); err != nil {
		t.Fatal(err)
	}
}

func wrathState(t *testing.T, pool *pgxpool.Pool, player, world uuid.UUID) (kharis float64, notices int) {
	t.Helper()
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT settled(kharis_amount,kharis_rate,kharis_calc_tick) FROM player_world_records WHERE player_id=$1 AND world_id=$2`, player, world).Scan(&kharis); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND world_id=$2 AND kind='GodsAngered'`, player, world).Scan(&notices); err != nil {
		t.Fatal(err)
	}
	return
}

func TestCoarseLetterIsReplacedAndAngersTheGods(t *testing.T) {
	f := setupReplyFixture(t)
	setKharis(t, f.pool, f.senderID, f.worldID, 50)
	path := "/worlds/" + f.worldID.String() + "/settlements/" + f.originID.String() + "/messengers"

	// A refused letter costs nothing.
	if code, _ := f.post(t, f.senderToken, path, map[string]any{"destination_id": f.destID.String(), "message": strings.Repeat("fuck ", 300)}); code != http.StatusBadRequest {
		t.Fatalf("over-long letter: %d, want 400", code)
	}
	if k, n := wrathState(t, f.pool, f.senderID, f.worldID); k != 50 || n != 0 {
		t.Fatalf("refused letter: kharis %v, %d notices — want 50, 0", k, n)
	}

	code, resp := f.post(t, f.senderToken, path, map[string]any{
		"destination_id": f.destID.String(), "subject": "Fucking tin", "message": "Send the tin, you cunt.",
	})
	if code != http.StatusCreated {
		t.Fatalf("send: %d %v", code, resp)
	}
	var stored string
	if err := f.pool.QueryRow(context.Background(), `SELECT message_text FROM messengers WHERE id=$1`, resp["id"]).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "cunt") || !strings.HasPrefix(stored, "Send the tin, you ") || len([]rune(stored)) != len([]rune("Send the tin, you cunt.")) {
		t.Fatalf("stored letter %q, want the word replaced and the rest intact", stored)
	}
	k, n := wrathState(t, f.pool, f.senderID, f.worldID)
	if want := 50 - 2*religion.ProfanityKharisLoss; k != want || n != 1 {
		t.Fatalf("after two coarse words: kharis %v, %d notices — want %v, 1", k, n, want)
	}
	if resp["gods_angered"] == nil {
		t.Fatalf("response does not say the gods are angered: %v", resp)
	}

	// The floor holds.
	setKharis(t, f.pool, f.senderID, f.worldID, 3)
	if code, resp := f.post(t, f.senderToken, path, map[string]any{"destination_id": f.destID.String(), "message": "fuck fuck fuck"}); code != http.StatusCreated {
		t.Fatalf("send: %d %v", code, resp)
	}
	if k, _ := wrathState(t, f.pool, f.senderID, f.worldID); k != religion.RealmKharisFloor {
		t.Fatalf("kharis %v, want the floor %v", k, religion.RealmKharisFloor)
	}
}

func TestCleanLetterCostsNothing(t *testing.T) {
	f := setupReplyFixture(t)
	setKharis(t, f.pool, f.senderID, f.worldID, 50)
	path := "/worlds/" + f.worldID.String() + "/settlements/" + f.originID.String() + "/messengers"
	if code, resp := f.post(t, f.senderToken, path, map[string]any{"destination_id": f.destID.String(), "message": "Damn this winter; greetings from Scunthorpe."}); code != http.StatusCreated {
		t.Fatalf("send: %d %v", code, resp)
	}
	if k, n := wrathState(t, f.pool, f.senderID, f.worldID); k != 50 || n != 0 {
		t.Fatalf("clean letter: kharis %v, %d notices — want 50, 0", k, n)
	}
}

func TestCoarseStormNameIsReplacedAndAngersTheGods(t *testing.T) {
	n, storm := newNameRig(t)
	setKharis(t, n.pool, n.viewer, n.worldID, 50)
	if c := n.do("POST", storm, `{"name":"Fuck Wind"}`, n.token, ""); c != http.StatusOK {
		t.Fatalf("naming = %d, want 200", c)
	}
	got := n.get().Storms[0].Name
	if strings.Contains(strings.ToLower(got), "fuck") || !strings.HasSuffix(got, " Wind") || len(got) != len("Fuck Wind") || !stormNameRe.MatchString(got) {
		t.Fatalf("stored storm name %q, want a made-up word before Wind", got)
	}
	k, notices := wrathState(t, n.pool, n.viewer, n.worldID)
	if k != 50-religion.ProfanityKharisLoss || notices != 1 {
		t.Fatalf("kharis %v, %d notices — want %v, 1", k, notices, 50-religion.ProfanityKharisLoss)
	}
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Real failing PostgreSQL writes must be observable without exposing their
// internal errors in the existing client response. Exercise both HTTP verbs.
func TestPlacementInternalErrorLogsDatabaseCause(t *testing.T) {
	for _, op := range []string{"place", "unplace"} {
		t.Run(op, func(t *testing.T) {
			f := setupPlacementFixture(t, map[[2]int]string{{1, 0}: "plains"})
			pool := p10TestPool(t)
			ctx := context.Background()
			if op == "unplace" {
				if code, body := f.do(t, "POST", f.placementsPath(), map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"}); code != 201 {
					t.Fatalf("seed %d %v", code, body)
				}
			}
			name := "placement_log_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			reference := "NEW"
			event := "INSERT"
			if op == "unplace" {
				reference = "OLD"
				event = "DELETE"
			}
			statement := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF %s.settlement_id='%s'::uuid THEN RAISE EXCEPTION 'placement observability probe' USING ERRCODE='P0001'; END IF; RETURN %s; END $$; CREATE TRIGGER %s BEFORE %s ON settlement_placement FOR EACH ROW EXECUTE FUNCTION %s()`, name, reference, f.settlementID, reference, name, event, name)
			if _, err := pool.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			defer pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON settlement_placement; DROP FUNCTION IF EXISTS %s()`, name, name))
			var buf bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			defer slog.SetDefault(previous)
			method, path, body := "POST", f.placementsPath(), any(map[string]any{"target_kind": "hex", "hex_q": 1, "hex_r": 0, "good_key": "grain"})
			if op == "unplace" {
				method = "DELETE"
				path += "/1"
				body = nil
			}
			code, response := f.do(t, method, path, body)
			if code != 500 {
				t.Fatalf("expected real PG fault500, got%d %v", code, response)
			}
			if strings.Contains(fmt.Sprint(response), "observability probe") {
				t.Fatalf("internal cause leaked to client: %v", response)
			}
			var entry map[string]any
			for _, line := range strings.Split(buf.String(), "\n") {
				var item map[string]any
				if json.Unmarshal([]byte(line), &item) == nil && item["msg"] == "placement failed" {
					entry = item
				}
			}
			if entry["level"] != "ERROR" || entry["operation"] != op || entry["pg_code"] != "P0001" || !strings.Contains(fmt.Sprint(entry["err"]), "placement observability probe") || entry["stage"] == "" || entry["settlement_id"] != f.settlementID.String() || entry["world_id"] != f.worldID.String() {
				t.Fatalf("missing diagnostic fields: %s", buf.String())
			}
			t.Logf("%s HTTP500 retains generic response; ERROR has stage, err, pg_code=P0001 and IDs", op)
		})
	}
}

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAuthenticatedActivityLifecycle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	token, _, err := svc.Register(ctx, "activity-"+uuid8(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	read := func() (*time.Time, string) {
		t.Helper()
		var seen *time.Time
		var version string
		if err := svc.pool.QueryRow(ctx, `SELECT last_seen_at,xmin::text FROM players WHERE id=$1`, claims.PlayerID).Scan(&seen, &version); err != nil {
			t.Fatal(err)
		}
		return seen, version
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := PlayerIDFromContext(r.Context()); !ok || id != claims.PlayerID {
			t.Error("authenticated identity lost")
		}
		w.WriteHeader(204)
	})
	seen, _ := read()
	if seen != nil {
		t.Fatal("registration must not invent prior authenticated activity")
	}
	for _, mode := range []string{"api", "web", "optional"} {
		t.Run(mode, func(t *testing.T) {
			_, err := svc.pool.Exec(ctx, `UPDATE players SET last_seen_at=NULL WHERE id=$1`, claims.PlayerID)
			if err != nil {
				t.Fatal(err)
			}
			// A new service simulates an independent process; the DB gate must also throttle it.
			s := NewService(svc.pool, "test-secret")
			middleware := Middleware(s)
			if mode == "web" {
				middleware = WebMiddleware(s)
			}
			if mode == "optional" {
				middleware = OptionalMiddleware(s)
			}
			h := middleware(handler)
			request := func(valid bool) {
				req := httptest.NewRequest("GET", "/probe", nil)
				if valid {
					if mode == "web" {
						req.AddCookie(&http.Cookie{Name: "poleia_token", Value: token})
					} else {
						req.Header.Set("Authorization", "Bearer "+token)
					}
				} else {
					req.Header.Set("Authorization", "Bearer invalid")
				}
				rec := httptest.NewRecorder()
				if valid {
					h.ServeHTTP(rec, req)
					if rec.Code != 204 {
						t.Errorf("auth response%d", rec.Code)
					}
				} else {
					middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, req)
				}
			}
			request(false)
			seen, _ := read()
			if seen != nil {
				t.Fatal("invalid auth recorded activity")
			}
			request(true)
			first, v1 := read()
			if first == nil || time.Since(*first) > time.Minute {
				t.Fatal("valid auth did not record recent wall time")
			}
			acquires := s.pool.Stat().AcquireCount()
			request(true)
			if s.pool.Stat().AcquireCount() != acquires {
				t.Fatal("cached activity performed a DB request")
			}
			second, v2 := read()
			if !first.Equal(*second) || v1 != v2 {
				t.Fatal("cached request wrote player again")
			}
			other := Middleware(NewService(svc.pool, "test-secret"))(handler)
			req := httptest.NewRequest("GET", "/probe", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			other.ServeHTTP(httptest.NewRecorder(), req)
			_, v3 := read()
			if v3 != v2 {
				t.Fatal("another process bypassed five minute DB gate")
			}
			// Move only the saved stamp backwards, then use a fresh process to test the persistent gate.
			if _, err := svc.pool.Exec(ctx, `UPDATE players SET last_seen_at=now()-interval '6 minutes' WHERE id=$1`, claims.PlayerID); err != nil {
				t.Fatal(err)
			}
			newer := Middleware(NewService(svc.pool, "test-secret"))(handler)
			newer.ServeHTTP(httptest.NewRecorder(), req)
			last, _ := read()
			if last == nil || !last.After(*first) {
				t.Fatal("activity after five minutes not refreshed")
			}
		})
	}
}

func TestActivityConcurrentRequestsWriteOnce(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	token, _, err := svc.Register(ctx, "activity-race-"+uuid8(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := svc.ValidateAccessToken(token)
	// Count real player UPDATEs, including attempted same-value rewrites.
	_, err = svc.pool.Exec(ctx, `CREATE TABLE auth_activity_probe(id uuid); CREATE FUNCTION auth_activity_probe_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO auth_activity_probe VALUES(NEW.id);RETURN NEW;END $$; CREATE TRIGGER auth_activity_probe AFTER UPDATE ON players FOR EACH ROW EXECUTE FUNCTION auth_activity_probe_fn()`)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.pool.Exec(ctx, `DROP TRIGGER auth_activity_probe ON players;DROP FUNCTION auth_activity_probe_fn();DROP TABLE auth_activity_probe`)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := NewService(svc.pool, "test-secret")
			h := Middleware(s)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
			r := httptest.NewRequest("GET", "/probe", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 204 {
				t.Errorf("response%d", w.Code)
			}
		}()
	}
	wg.Wait()
	var n int
	if err := svc.pool.QueryRow(ctx, `SELECT count(*) FROM auth_activity_probe WHERE id=$1`, claims.PlayerID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("concurrent activity wrote%d times, want1", n)
	}
}

func TestActivityFailureDoesNotRejectAuthentication(t *testing.T) {
	svc := newTestService(t)
	token, _, err := svc.Register(context.Background(), "activity-failure-"+uuid8(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/probe", nil).WithContext(canceled)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	Middleware(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("telemetry failure broke auth%d", rec.Code)
	}
	claims, _ := svc.ValidateAccessToken(token)
	var seen *time.Time
	if err := svc.pool.QueryRow(context.Background(), `SELECT last_seen_at FROM players WHERE id=$1`, claims.PlayerID).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen != nil {
		t.Fatal("canceled observation unexpectedly wrote activity")
	}
	req = httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	Middleware(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(httptest.NewRecorder(), req)
	if err := svc.pool.QueryRow(context.Background(), `SELECT last_seen_at FROM players WHERE id=$1`, claims.PlayerID).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen == nil {
		t.Fatal("failed observation poisoned the retry cache")
	}
}

func TestActivityFiveMinuteGateAndCacheExpiry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	token, _, err := svc.Register(ctx, "activity-boundary-"+uuid8(), "pw")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := svc.ValidateAccessToken(token)
	if _, err := svc.pool.Exec(ctx, `UPDATE players SET last_seen_at=now()-interval '299 seconds' WHERE id=$1`, claims.PlayerID); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	var version string
	if err := svc.pool.QueryRow(ctx, `SELECT last_seen_at,xmin::text FROM players WHERE id=$1`, claims.PlayerID).Scan(&before, &version); err != nil {
		t.Fatal(err)
	}
	svc.recordActivity(ctx, claims.PlayerID)
	var after time.Time
	var afterVersion string
	if err := svc.pool.QueryRow(ctx, `SELECT last_seen_at,xmin::text FROM players WHERE id=$1`, claims.PlayerID).Scan(&after, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) || version != afterVersion {
		t.Fatal("activity before five minute gate wrote player")
	}
	if _, err := svc.pool.Exec(ctx, `UPDATE players SET last_seen_at=now()-interval '301 seconds' WHERE id=$1`, claims.PlayerID); err != nil {
		t.Fatal(err)
	}
	svc.activityMu.Lock()
	svc.activityNext[claims.PlayerID] = time.Now().Add(-time.Second)
	svc.activitySweep = time.Now().Add(-time.Second)
	svc.activityMu.Unlock()
	svc.recordActivity(ctx, claims.PlayerID)
	if err := svc.pool.QueryRow(ctx, `SELECT last_seen_at FROM players WHERE id=$1`, claims.PlayerID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if time.Since(after) > time.Minute {
		t.Fatal("expired cache failed to record after five minutes")
	}
}

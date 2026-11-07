package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Every API response states the newest released keryx commit (the newest
// client_releases row for 'keryx', written by tools/announce-keryx-release.sh);
// keryx compares it with its own and says so on stderr. No row, no header.
const keryxLatestHeader = "X-Keryx-Latest"

type keryxLatest struct {
	load   func() string
	every  time.Duration
	mu     sync.Mutex
	at     time.Time
	value  string
	loaded bool
}

func newKeryxLatest(load func() string) *keryxLatest {
	return &keryxLatest{load: load, every: 15 * time.Second}
}

// poolKeryxLatest reads the newest row; a DB error yields "" (no header).
func poolKeryxLatest(pool *pgxpool.Pool) func() string {
	return func() string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var commit string
		if err := pool.QueryRow(ctx,
			`SELECT commit FROM client_releases WHERE client = 'keryx' ORDER BY released_at DESC, commit LIMIT 1`).Scan(&commit); err != nil {
			return ""
		}
		return commit
	}
}

func (l *keryxLatest) current() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.loaded || time.Since(l.at) >= l.every {
		l.value = l.load()
		l.at = time.Now()
		l.loaded = true
	}
	return l.value
}

func (l *keryxLatest) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if v := l.current(); v != "" {
				w.Header().Set(keryxLatestHeader, v)
			}
		}
		next.ServeHTTP(w, r)
	})
}

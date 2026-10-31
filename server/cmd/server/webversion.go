package main

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A tab left open across a deploy keeps running the old ES modules and
// silently ignores data it does not know (2026-10-31: a "name it" notice for
// a new place kind drew no form until reload). Every response states the
// running version; the client offers a reload when it changes. air rebuilds
// only on .go changes, so the commit alone misses a web-only deploy: the
// version also covers the client code on disk.
const versionHeader = "X-Megaron-Version"

type webVersion struct {
	dir    string
	commit string
	every  time.Duration
	mu     sync.Mutex
	at     time.Time
	value  string
}

func newWebVersion(dir, commit string) *webVersion {
	return &webVersion{dir: dir, commit: commit, every: 15 * time.Second}
}

func (v *webVersion) current() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.value == "" || time.Since(v.at) >= v.every {
		v.value = v.commit + "." + clientCodeHash(v.dir)
		v.at = time.Now()
	}
	return v.value
}

// clientCodeHash fingerprints the client code (path, size, mtime) without
// reading file contents; images and audio are not code and are skipped.
func clientCodeHash(dir string) string {
	h := fnv.New64a()
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".js", ".mjs", ".css", ".html", ".md":
		default:
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s|%d|%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return fmt.Sprintf("%x", h.Sum64())
}

func (v *webVersion) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set(versionHeader, v.current())
		}
		next.ServeHTTP(w, r)
	})
}

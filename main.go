// oxid-e2e-go — Dave's branch: HTTP caching + ETag on a db-backed payload.
package main

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

const (
	BRANCH = "fix/cache-headers"
	FEATURE = "ETag + Cache-Control on /payload (fixes stale-cache bug)"
)

var startedAt = time.Now().UTC()

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "oxid-e2e-go", "branch": BRANCH, "feature": FEATURE,
	})
}

func healthz(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) }

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/healthz", healthz)

	mux.HandleFunc("/payload", func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{"branch": BRANCH, "served_at": time.Now().UTC().Format(time.RFC3339)}
		if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
			db, err := sql.Open("postgres", dsn)
			if err == nil {
				defer db.Close()
				var n int64
				qerr := db.QueryRow(
					`INSERT INTO visits (branch, hits) VALUES ($1, 1)
					 ON CONFLICT (branch) DO UPDATE SET hits = visits.hits + 1
					 RETURNING hits`, BRANCH).Scan(&n)
				if qerr == nil {
					body["pg_visits"] = n
				} else {
					body["pg_error"] = qerr.Error()
				}
			} else {
				body["pg_error"] = err.Error()
			}
		}
		raw, _ := json.Marshal(body)
		sum := sha1.Sum(raw)
		etag := `"` + hex.EncodeToString(sum[:8]) + `"`
		w.Header().Set("Cache-Control", "public, max-age=30")
		w.Header().Set("ETag", etag)
		w.Header().Set("X-Branch", BRANCH)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writeJSON(w, http.StatusOK, body)
	})

	addr := ":" + envOr("PORT", "8080")
	log.Printf("oxid-e2e-go branch=%s listening on %s", fmt.Sprint(BRANCH), addr)
	log.Fatal((&http.Server{Addr: addr, Handler: mux}).ListenAndServe())
}

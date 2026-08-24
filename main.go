// oxid-e2e-go — Alice's branch: versioned API v2 backed by Postgres + Redis.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const (
	BRANCH  = "feat/api-v2"
	FEATURE = "API v2: db-backed counter + redis hits"
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
		"api_version": 1, "v2": "/v2",
	})
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// respDo sends one RESP command and returns the bulk/string reply.
func respDo(addr string, selectDB int, args ...string) (string, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	cmds := args
	if selectDB >= 0 {
		cmds = append([]string{"SELECT", strconv.Itoa(selectDB)}, args...)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(cmds))
	for _, a := range cmds {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	r := bufio.NewReader(conn)
	var last string
	for i := 0; i < len(cmds); i++ {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		switch {
		case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "-"), strings.HasPrefix(line, ":"):
			last = strings.TrimRight(line[1:], "\r\n")
		case strings.HasPrefix(line, "$"):
			n, _ := strconv.Atoi(strings.TrimRight(line[1:], "\r\n"))
			buf := make([]byte, n+2)
			if _, err := io.ReadFull(r, buf); err != nil {
				return "", err
			}
			last = string(buf[:n])
		default:
			return "", fmt.Errorf("unexpected reply %q", line)
		}
	}
	return last, nil
}

func visitsCount(db *sql.DB) (int64, error) {
	var n int64
	err := db.QueryRow(
		`INSERT INTO visits (branch, hits) VALUES ($1, 1)
		 ON CONFLICT (branch) DO UPDATE SET hits = visits.hits + 1
		 RETURNING hits`, BRANCH).Scan(&n)
	return n, err
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/healthz", healthz)

	mux.HandleFunc("/v2", func(w http.ResponseWriter, _ *http.Request) {
		out := map[string]any{"branch": BRANCH, "api_version": 2}
		if raw := os.Getenv("DATABASE_URL"); raw != "" {
			dsn := raw
			if !strings.Contains(dsn, "sslmode=") {
				dsn += "?sslmode=disable"
			}
			db, err := sql.Open("postgres", dsn)
			if err == nil {
				defer db.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if _, err = db.ExecContext(ctx,
					`CREATE TABLE IF NOT EXISTS visits (
						branch text primary key, hits bigint not null default 0)`); err == nil {
					n, err := visitsCount(db)
					if err == nil {
						out["pg_visits"] = n
					} else {
						out["pg_error"] = err.Error()
					}
				} else {
					out["pg_error"] = err.Error()
				}
			} else {
				out["pg_error"] = err.Error()
			}
		} else {
			out["pg"] = "DATABASE_URL not injected"
		}
		if rurl := os.Getenv("REDIS_URL"); rurl != "" {
			raw := strings.TrimPrefix(rurl, "redis://")
			parts := strings.SplitN(raw, "/", 2)
			addr, idx := parts[0], -1
			if len(parts) == 2 {
				idx, _ = strconv.Atoi(parts[1])
			}
			if v, err := respDo(addr, idx, "INCR", "hits:"+BRANCH); err == nil {
				out["redis_hits"] = v
			} else {
				out["redis_error"] = err.Error()
			}
		} else {
			out["redis"] = "REDIS_URL not injected"
		}
		writeJSON(w, http.StatusOK, out)
	})

	addr := ":" + envOr("PORT", "8080")
	log.Printf("oxid-e2e-go branch=%s listening on %s", BRANCH, addr)
	log.Fatal((&http.Server{Addr: addr, Handler: mux}).ListenAndServe())
}

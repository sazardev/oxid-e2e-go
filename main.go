// oxid-e2e-go — Carol's branch: Redis-backed rate limiting (no SQL deps).
package main

import (
	"bufio"
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
)

const (
	BRANCH = "feat/rate-limit"
	LIMIT  = 5 // requests per minute per client
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

// Minimal RESP client: INCR + EXPIRE, enough for a fixed-window limiter.
func respDo(addr string, args ...string) ([]string, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return nil, err
	}
	r := bufio.NewReader(conn)
	var out []string
	for range len(args) {
		line, err := r.ReadString('\n')
		if err != nil {
			return out, err
		}
		switch {
		case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "-"):
			out = append(out, strings.TrimRight(line[1:], "\r\n"))
		case strings.HasPrefix(line, ":"), strings.HasPrefix(line, "$"):
			out = append(out, strings.TrimSpace(strings.TrimRight(line[1:], "\r\n")))
			if strings.HasPrefix(line, "$") {
				n, _ := strconv.Atoi(out[len(out)-1])
				buf := make([]byte, n+2)
				if _, err := io.ReadFull(r, buf); err != nil {
					return out, err
				}
				out[len(out)-1] = string(buf[:n])
			}
		}
	}
	return out, nil
}

func index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "oxid-e2e-go", "branch": BRANCH,
		"feature": "fixed-window redis rate limit", "limit_per_min": LIMIT,
	})
}

func healthz(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) }

// limited returns whether this IP is still under the fixed window limit.
func limited(redisAddr, ip string) (allowed bool, remaining int64, err error) {
	res, err := respDo(redisAddr, "INCR", "rl:"+ip, "EXPIRE", "60")
	if err != nil || len(res) < 2 {
		// Fail open: redis unreachable must not take the API down.
		return true, LIMIT, err
	}
	n, _ := strconv.ParseInt(res[0], 10, 64)
	return n <= LIMIT, LIMIT - n, nil
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/healthz", healthz)

	mux.Handle("/limited", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := strings.SplitN(r.RemoteAddr, ":", 2)[0]
		addr := strings.TrimPrefix(os.Getenv("REDIS_URL"), "redis://")
		if addr == "" {
			writeJSON(w, 503, map[string]string{"error": "REDIS_URL not injected"})
			return
		}
		ok, rem, err := limited(addr, ip)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(LIMIT))
		if rem > 0 {
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(rem, 10))
		} else {
			w.Header().Set("X-RateLimit-Remaining", "0")
		}
		if !ok {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"branch": BRANCH, "error": "rate limited", "retry_after_s": 60,
			})
			return
		}
		if err != nil {
			w.Header().Set("X-RateLimit-Fallback", "fail-open")
		}
		writeJSON(w, 200, map[string]any{"branch": BRANCH, "remaining": rem})
	}))

	addr := ":" + envOr("PORT", "8080")
	log.Printf("oxid-e2e-go branch=%s listening on %s", BRANCH, addr)
	log.Fatal((&http.Server{Addr: addr, Handler: mux}).ListenAndServe())
}

// oxid-e2e-go — tiny service for exercising Oxid end to end.
// Each branch customizes BRANCH/FEATURE and may add its own endpoints.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	BRANCH  = "main"
	FEATURE = "stable API v1 (no extras)"
)

var startedAt = time.Now().UTC()

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":    "oxid-e2e-go",
		"branch":     BRANCH,
		"feature":    FEATURE,
		"pid":        os.Getpid(),
		"started_at": startedAt.Format(time.RFC3339),
		"uptime_s":   time.Since(startedAt).Seconds(),
	})
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/healthz", healthz)

	addr := ":" + envOr("PORT", "8080")
	log.Printf("oxid-e2e-go branch=%s listening on %s", BRANCH, addr)
	server := &http.Server{Addr: addr, Handler: mux}
	log.Fatal(server.ListenAndServe())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return fmt.Sprint(v)
	}
	return fallback
}

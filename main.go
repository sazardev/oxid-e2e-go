// oxid-e2e-go — Bob's branch: Prometheus-style metrics, zero external deps.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

const (
	BRANCH  = "feat/metrics"
	FEATURE = "Prometheus-style /metrics v1.1 (adds go_version gauge)"
)

var (
	startedAt = time.Now().UTC()
	reqTotal  atomic.Uint64
)

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
	reqTotal.Add(1)
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "oxid-e2e-go", "branch": BRANCH, "feature": FEATURE,
		"metrics": "/metrics",
	})
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func metrics(w http.ResponseWriter, req *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP http_requests_total Total requests seen by this instance.\n")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\nhttp_requests_total{branch=%q} %d\n", BRANCH, reqTotal.Load())
	fmt.Fprintf(w, "# TYPE process_uptime_seconds gauge\nprocess_uptime_seconds %f\n", time.Since(startedAt).Seconds())
	fmt.Fprintf(w, "# TYPE go_goroutines gauge\ngo_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "# TYPE go_heap_alloc_bytes gauge\ngo_heap_alloc_bytes %d\n", ms.HeapAlloc)
	fmt.Fprintf(w, "# TYPE go_version_info label\ngo_version_info{version=%q} 1\n", runtime.Version())
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/metrics", metrics)

	addr := ":" + envOr("PORT", "8080")
	log.Printf("oxid-e2e-go branch=%s listening on %s", BRANCH, addr)
	log.Fatal((&http.Server{Addr: addr, Handler: mux}).ListenAndServe())
}

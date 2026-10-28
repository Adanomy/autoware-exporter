package exporter

import (
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewHandler serves cached metrics; healthz reports only HTTP process health.
func NewHandler(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true, Timeout: 10 * time.Second, MaxRequestsInFlight: 4,
	}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><title>Autoware Exporter</title></head><body><h1>Autoware Exporter</h1><p><a href=\"/metrics\">Metrics</a> · <a href=\"/healthz\">Health</a></p></body></html>\n")
	})
	return mux
}

// Package api exposes the ingest and query HTTP surface.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/store"
	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/stream"
	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/telemetry"
)

const (
	maxBodyBytes  = 1 << 20
	maxBatch      = 5_000
	maxSeriesSpan = 31 * 24 * time.Hour
	minBucket     = time.Second
)

type Server struct {
	Store   store.Store
	Hub     *stream.Hub
	Metrics *Metrics
	Log     *slog.Logger
	Now     func() time.Time
	// AllowOrigin is echoed in CORS headers so the dashboard can call us.
	AllowOrigin string
	// Gatherer backs /metrics; defaults to the global registry.
	Gatherer prometheus.Gatherer
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.route(mux, "POST /v1/telemetry", s.ingest)
	s.route(mux, "GET /v1/sites/{site}/latest", s.latest)
	s.route(mux, "GET /v1/robots/{robot}/series", s.series)
	mux.HandleFunc("GET /v1/stream", s.stream) // long-lived: excluded from latency histogram
	s.route(mux, "GET /healthz", s.health)
	g := s.Gatherer
	if g == nil {
		g = prometheus.DefaultGatherer
	}
	mux.Handle("GET /metrics", promhttp.HandlerFor(g, promhttp.HandlerOpts{}))
	return s.cors(mux)
}

type IngestResponse struct {
	Accepted int           `json:"accepted"`
	Rejected []RejectedRow `json:"rejected"`
}

type RejectedRow struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// ingest accepts {"samples": [...]}. Valid rows are stored even if others in
// the batch fail, so one bad sensor reading never loses a whole upload from a
// robot on a flaky field LTE link.
func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Samples []telemetry.Sample `json:"samples"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		s.Metrics.Rejected.WithLabelValues("malformed").Inc()
		writeError(w, http.StatusBadRequest, "malformed body: "+err.Error())
		return
	}
	if len(body.Samples) == 0 || len(body.Samples) > maxBatch {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("batch must contain 1..%d samples", maxBatch))
		return
	}
	s.Metrics.BatchSize.Observe(float64(len(body.Samples)))

	now := s.Now()
	valid := make([]telemetry.Sample, 0, len(body.Samples))
	resp := IngestResponse{Rejected: []RejectedRow{}}
	for i, sample := range body.Samples {
		if err := sample.Validate(now); err != nil {
			resp.Rejected = append(resp.Rejected, RejectedRow{Index: i, Error: err.Error()})
			s.Metrics.Rejected.WithLabelValues("validation").Inc()
			continue
		}
		valid = append(valid, sample)
	}
	if len(valid) > 0 {
		if err := s.Store.Insert(r.Context(), valid); err != nil {
			s.Log.Error("insert failed", "err", err, "n", len(valid))
			writeError(w, http.StatusServiceUnavailable, "storage unavailable")
			return
		}
		s.Hub.Publish(valid)
		for _, v := range valid {
			s.Metrics.LastUplift.WithLabelValues(v.SiteID, v.RobotID).Set(v.SurfaceUpliftMM)
		}
	}
	resp.Accepted = len(valid)
	s.Metrics.Accepted.Add(float64(len(valid)))

	code := http.StatusAccepted
	if len(valid) == 0 {
		code = http.StatusUnprocessableEntity
	}
	writeJSON(w, code, resp)
}

func (s *Server) latest(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.Latest(r.Context(), r.PathValue("site"))
	if err != nil {
		s.Log.Error("latest failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// series: ?from=RFC3339&to=RFC3339&bucket=1m. Defaults to the last hour in
// one-minute buckets.
func (s *Server) series(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	to, from := s.Now(), s.Now().Add(-time.Hour)
	bucket := time.Minute
	var err error
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "to: "+err.Error())
			return
		}
	}
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "from: "+err.Error())
			return
		}
	}
	if v := q.Get("bucket"); v != "" {
		if bucket, err = time.ParseDuration(v); err != nil {
			writeError(w, http.StatusBadRequest, "bucket: "+err.Error())
			return
		}
	}
	switch {
	case !from.Before(to):
		writeError(w, http.StatusBadRequest, "from must be before to")
		return
	case to.Sub(from) > maxSeriesSpan:
		writeError(w, http.StatusBadRequest, "range exceeds 31 days")
		return
	case bucket < minBucket:
		writeError(w, http.StatusBadRequest, "bucket must be >= 1s")
		return
	case to.Sub(from)/bucket > 10_000:
		writeError(w, http.StatusBadRequest, "too many buckets; widen bucket")
		return
	}
	out, err := s.Store.Series(r.Context(), r.PathValue("robot"), from, to, bucket)
	if err != nil {
		s.Log.Error("series failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// stream is a Server-Sent Events feed of live samples, optionally filtered by
// ?site=. SSE over plain HTTP survives corporate proxies and needs no client lib.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, cancel := s.Hub.Subscribe(r.URL.Query().Get("site"), 256)
	defer cancel()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case sample, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(sample)
			fmt.Fprintf(w, "event: sample\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "store": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// route wraps a handler with latency metrics and structured access logs.
func (s *Server) route(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		h(rec, r)
		s.Metrics.Latency.WithLabelValues(pattern, strconv.Itoa(rec.code)).Observe(time.Since(start).Seconds())
		s.Log.Debug("request", "route", pattern, "code", rec.code, "dur", time.Since(start))
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AllowOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.AllowOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

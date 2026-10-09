// Command ingest runs the telemetry ingestion service.
//
//	DATABASE_URL  Postgres/TimescaleDB DSN; omit to use the in-memory store
//	ADDR          listen address (default :8080)
//	CORS_ORIGIN   origin allowed to call the API (default *)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/api"
	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/store"
	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/stream"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store = store.NewMemory()
	if url := os.Getenv("DATABASE_URL"); url != "" {
		pg, err := connectWithRetry(ctx, url, log)
		if err != nil {
			log.Error("database unavailable", "err", err)
			os.Exit(1)
		}
		defer pg.Close()
		st = pg
		log.Info("using postgres store")
	} else {
		log.Warn("DATABASE_URL not set; using in-memory store")
	}

	hub := stream.NewHub()
	metrics := api.NewMetrics(prometheus.DefaultRegisterer, func() float64 { return float64(hub.Subscribers()) })
	hub.OnDrop = metrics.Dropped.Inc

	srv := &api.Server{
		Store: st, Hub: hub, Metrics: metrics, Log: log, Now: time.Now,
		AllowOrigin: envOr("CORS_ORIGIN", "*"),
	}
	httpSrv := &http.Server{
		Addr:              envOr("ADDR", ":8080"),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	log.Info("listening", "addr", httpSrv.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", "err", err)
		os.Exit(1)
	}
}

// connectWithRetry tolerates the database container starting after us.
func connectWithRetry(ctx context.Context, url string, log *slog.Logger) (*store.Postgres, error) {
	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		pg, err := store.NewPostgres(ctx, url)
		if err == nil {
			if err = pg.Ping(ctx); err == nil {
				return pg, nil
			}
			pg.Close()
		}
		lastErr = err
		log.Warn("database not ready", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, lastErr
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

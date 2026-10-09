package store

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/telemetry"
)

//go:embed schema.sql
var schemaSQL string

// Postgres stores telemetry in a TimescaleDB hypertable when the extension is
// available and falls back to a plain indexed table otherwise. All queries use
// core-Postgres functions (date_bin, DISTINCT ON) so they run on either.
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

var columns = []string{
	"ts", "site_id", "robot_id", "state", "x_m", "y_m", "depth_m",
	"injection_pressure_kpa", "flow_rate_lpm", "surface_uplift_mm", "battery_pct",
}

func (p *Postgres) Insert(ctx context.Context, samples []telemetry.Sample) error {
	_, err := p.pool.CopyFrom(ctx, pgx.Identifier{"telemetry"}, columns,
		pgx.CopyFromSlice(len(samples), func(i int) ([]any, error) {
			s := samples[i]
			return []any{s.TS, s.SiteID, s.RobotID, string(s.State), s.X, s.Y, s.DepthM,
				s.InjectionPressureKPa, s.FlowRateLPM, s.SurfaceUpliftMM, s.BatteryPct}, nil
		}))
	return err
}

func (p *Postgres) Latest(ctx context.Context, siteID string) ([]telemetry.Sample, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT DISTINCT ON (robot_id)
		       ts, site_id, robot_id, state, x_m, y_m, depth_m,
		       injection_pressure_kpa, flow_rate_lpm, surface_uplift_mm, battery_pct
		FROM telemetry
		WHERE site_id = $1 AND ts > now() - interval '1 day'
		ORDER BY robot_id, ts DESC`, siteID)
	if err != nil {
		return nil, err
	}
	out := []telemetry.Sample{}
	for rows.Next() {
		var s telemetry.Sample
		var state string
		if err := rows.Scan(&s.TS, &s.SiteID, &s.RobotID, &state, &s.X, &s.Y, &s.DepthM,
			&s.InjectionPressureKPa, &s.FlowRateLPM, &s.SurfaceUpliftMM, &s.BatteryPct); err != nil {
			return nil, err
		}
		s.State = telemetry.State(state)
		s.TS = s.TS.UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) Series(ctx context.Context, robotID string, from, to time.Time, bucket time.Duration) ([]telemetry.Bucket, error) {
	rows, err := p.pool.Query(ctx, `
		WITH s AS (
			SELECT ts, injection_pressure_kpa AS p, flow_rate_lpm AS f, surface_uplift_mm AS u,
			       EXTRACT(EPOCH FROM ts - lag(ts) OVER w) / 60.0 AS dt_min,
			       lag(flow_rate_lpm) OVER w AS f_prev
			FROM telemetry
			WHERE robot_id = $1 AND ts >= $2 AND ts < $3
			WINDOW w AS (ORDER BY ts)
		)
		SELECT date_bin($4::interval, ts, TIMESTAMPTZ '2000-01-01 00:00:00+00') AS b,
		       count(*), avg(p), max(p), avg(f), max(u),
		       COALESCE(sum((f + f_prev) / 2 * dt_min), 0)
		FROM s GROUP BY b ORDER BY b`,
		robotID, from, to, bucket)
	if err != nil {
		return nil, err
	}
	out := []telemetry.Bucket{}
	for rows.Next() {
		var b telemetry.Bucket
		if err := rows.Scan(&b.Start, &b.Count, &b.AvgPressureKPa, &b.MaxPressureKPa,
			&b.AvgFlowLPM, &b.MaxUpliftMM, &b.VolumeInjectedL); err != nil {
			return nil, err
		}
		b.Start = b.Start.UTC()
		out = append(out, b)
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/telemetry"
)

// contract runs the same assertions against every Store implementation.
func contract(t *testing.T, s Store) {
	ctx := context.Background()
	t0 := time.Now().UTC().Truncate(time.Hour).Add(-time.Hour)
	mk := func(robot string, offset time.Duration, flow, pressure, uplift float64) telemetry.Sample {
		return telemetry.Sample{RobotID: robot, SiteID: "site-a", TS: t0.Add(offset), State: telemetry.StateInjecting,
			FlowRateLPM: flow, InjectionPressureKPa: pressure, SurfaceUpliftMM: uplift, BatteryPct: 90}
	}
	// Inserted out of order on purpose.
	err := s.Insert(ctx, []telemetry.Sample{
		mk("r1", 2*time.Minute, 100, 900, 3),
		mk("r1", 0, 100, 800, 1),
		mk("r1", time.Minute, 100, 1000, 2),
		mk("r1", 5*time.Minute+30*time.Second, 0, 0, 3.5),
		mk("r2", 0, 50, 600, 0.5),
		{RobotID: "r3", SiteID: "site-b", TS: t0, State: telemetry.StateIdle},
	})
	if err != nil {
		t.Fatal(err)
	}

	latest, err := s.Latest(ctx, "site-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 || latest[0].RobotID != "r1" || latest[1].RobotID != "r2" {
		t.Fatalf("latest: %+v", latest)
	}
	if !latest[0].TS.Equal(t0.Add(5*time.Minute + 30*time.Second)) {
		t.Fatalf("latest r1 ts = %s", latest[0].TS)
	}

	buckets, err := s.Series(ctx, "r1", t0, t0.Add(time.Hour), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 2 {
		t.Fatalf("want 2 buckets, got %+v", buckets)
	}
	b0, b1 := buckets[0], buckets[1]
	approx := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if b0.Count != 3 || !b0.Start.Equal(t0) {
		t.Errorf("b0 = %+v", b0)
	}
	approx("b0.avg_pressure", b0.AvgPressureKPa, 900)
	approx("b0.max_pressure", b0.MaxPressureKPa, 1000)
	approx("b0.max_uplift", b0.MaxUpliftMM, 3)
	approx("b0.volume", b0.VolumeInjectedL, 200) // 2 min at 100 L/min
	// 3.5 min ramp from 100 to 0 L/min: trapezoid = 50 * 3.5
	approx("b1.volume", b1.VolumeInjectedL, 175)

	empty, err := s.Series(ctx, "nobody", t0, t0.Add(time.Hour), time.Minute)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty series: %v %v", empty, err)
	}
}

func TestMemoryStore(t *testing.T) { contract(t, NewMemory()) }

// TestPostgresStore runs only when TEST_DATABASE_URL points at a scratch DB.
func TestPostgresStore(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	p, err := NewPostgres(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.pool.Exec(ctx, "TRUNCATE telemetry"); err != nil {
		t.Fatal(err)
	}
	contract(t, p)
}

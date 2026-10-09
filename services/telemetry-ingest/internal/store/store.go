// Package store persists telemetry. Two implementations share one contract:
// Memory (tests, local demos) and Postgres (TimescaleDB in production).
package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/telemetry"
)

type Store interface {
	Insert(ctx context.Context, samples []telemetry.Sample) error
	// Latest returns the most recent sample for every robot on a site.
	Latest(ctx context.Context, siteID string) ([]telemetry.Sample, error)
	// Series buckets a robot's samples in [from, to). Injected volume is the
	// trapezoidal integral of flow rate between consecutive samples.
	Series(ctx context.Context, robotID string, from, to time.Time, bucket time.Duration) ([]telemetry.Bucket, error)
	Ping(ctx context.Context) error
}

// binOrigin matches the origin used by the SQL date_bin call so both stores
// produce identical bucket boundaries.
var binOrigin = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func binStart(ts time.Time, bucket time.Duration) time.Time {
	return binOrigin.Add(ts.Sub(binOrigin) / bucket * bucket)
}

// Memory is a goroutine-safe in-process store.
type Memory struct {
	mu      sync.RWMutex
	byRobot map[string][]telemetry.Sample // kept sorted by TS
}

func NewMemory() *Memory { return &Memory{byRobot: map[string][]telemetry.Sample{}} }

func (m *Memory) Insert(_ context.Context, samples []telemetry.Sample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	touched := map[string]bool{}
	for _, s := range samples {
		s.TS = s.TS.UTC()
		m.byRobot[s.RobotID] = append(m.byRobot[s.RobotID], s)
		touched[s.RobotID] = true
	}
	for id := range touched {
		rs := m.byRobot[id]
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].TS.Before(rs[j].TS) })
	}
	return nil
}

func (m *Memory) Latest(_ context.Context, siteID string) ([]telemetry.Sample, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []telemetry.Sample{}
	for _, rs := range m.byRobot {
		for i := len(rs) - 1; i >= 0; i-- {
			if rs[i].SiteID == siteID {
				out = append(out, rs[i])
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RobotID < out[j].RobotID })
	return out, nil
}

func (m *Memory) Series(_ context.Context, robotID string, from, to time.Time, bucket time.Duration) ([]telemetry.Bucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var (
		out  []telemetry.Bucket
		cur  *telemetry.Bucket
		prev *telemetry.Sample
		sumP float64
		sumF float64
	)
	flush := func() {
		if cur != nil {
			cur.AvgPressureKPa = sumP / float64(cur.Count)
			cur.AvgFlowLPM = sumF / float64(cur.Count)
			out = append(out, *cur)
		}
	}
	for i := range m.byRobot[robotID] {
		s := m.byRobot[robotID][i]
		if s.TS.Before(from) || !s.TS.Before(to) {
			continue
		}
		start := binStart(s.TS, bucket)
		if cur == nil || !cur.Start.Equal(start) {
			flush()
			cur = &telemetry.Bucket{Start: start, MaxPressureKPa: s.InjectionPressureKPa, MaxUpliftMM: s.SurfaceUpliftMM}
			sumP, sumF = 0, 0
		}
		cur.Count++
		sumP += s.InjectionPressureKPa
		sumF += s.FlowRateLPM
		cur.MaxPressureKPa = max(cur.MaxPressureKPa, s.InjectionPressureKPa)
		cur.MaxUpliftMM = max(cur.MaxUpliftMM, s.SurfaceUpliftMM)
		if prev != nil {
			dtMin := s.TS.Sub(prev.TS).Minutes()
			cur.VolumeInjectedL += (s.FlowRateLPM + prev.FlowRateLPM) / 2 * dtMin
		}
		prev = &s
	}
	flush()
	if out == nil {
		out = []telemetry.Bucket{}
	}
	return out, nil
}

func (m *Memory) Ping(context.Context) error { return nil }

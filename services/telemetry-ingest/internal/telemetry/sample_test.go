package telemetry

import (
	"errors"
	"math"
	"testing"
	"time"
)

func validSample(now time.Time) Sample {
	return Sample{
		RobotID: "tn-01", SiteID: "alameda-1", TS: now, State: StateInjecting,
		X: 12, Y: -4, DepthM: 18, InjectionPressureKPa: 850, FlowRateLPM: 120,
		SurfaceUpliftMM: 3.2, BatteryPct: 77,
	}
}

func TestValidate(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*Sample)
		ok     bool
	}{
		{"valid", func(*Sample) {}, true},
		{"missing robot", func(s *Sample) { s.RobotID = "" }, false},
		{"missing site", func(s *Sample) { s.SiteID = "" }, false},
		{"zero ts", func(s *Sample) { s.TS = time.Time{} }, false},
		{"small skew ok", func(s *Sample) { s.TS = now.Add(time.Minute) }, true},
		{"future ts", func(s *Sample) { s.TS = now.Add(time.Hour) }, false},
		{"bad state", func(s *Sample) { s.State = "dancing" }, false},
		{"nan pressure", func(s *Sample) { s.InjectionPressureKPa = math.NaN() }, false},
		{"inf x", func(s *Sample) { s.X = math.Inf(1) }, false},
		{"negative depth", func(s *Sample) { s.DepthM = -1 }, false},
		{"overpressure", func(s *Sample) { s.InjectionPressureKPa = MaxPressureKPa + 1 }, false},
		{"negative flow", func(s *Sample) { s.FlowRateLPM = -0.1 }, false},
		{"subsidence ok", func(s *Sample) { s.SurfaceUpliftMM = -12 }, true},
		{"battery > 100", func(s *Sample) { s.BatteryPct = 101 }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSample(now)
			tc.mutate(&s)
			err := s.Validate(now)
			if tc.ok && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

// Package telemetry defines the wire format robots use to report state and
// the validation rules applied before anything touches storage.
package telemetry

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// State is the coarse operating mode of an injection robot.
type State string

const (
	StateIdle      State = "idle"
	StateDrilling  State = "drilling"
	StateInjecting State = "injecting"
	StateFault     State = "fault"
)

var validStates = map[State]bool{
	StateIdle: true, StateDrilling: true, StateInjecting: true, StateFault: true,
}

// Sample is a single telemetry reading from one robot.
//
// Positions are in a site-local ENU frame (meters) so the dashboard can render
// a site without a map tile provider.
type Sample struct {
	RobotID string    `json:"robot_id"`
	SiteID  string    `json:"site_id"`
	TS      time.Time `json:"ts"`
	State   State     `json:"state"`

	X float64 `json:"x_m"`
	Y float64 `json:"y_m"`

	DepthM               float64 `json:"depth_m"`
	InjectionPressureKPa float64 `json:"injection_pressure_kpa"`
	FlowRateLPM          float64 `json:"flow_rate_lpm"`
	SurfaceUpliftMM      float64 `json:"surface_uplift_mm"`
	BatteryPct           float64 `json:"battery_pct"`
}

// Physical bounds. Anything outside these is a sensor or firmware bug and is
// rejected rather than silently stored.
const (
	MaxPressureKPa = 10_000
	MaxFlowLPM     = 2_000
	MaxDepthM      = 200
	MaxUpliftMM    = 5_000
	MaxClockSkew   = 5 * time.Minute
)

// ErrInvalid wraps every validation failure so callers can errors.Is on it.
var ErrInvalid = errors.New("invalid sample")

// Validate checks a sample against schema and physical bounds. now is passed
// in so tests are deterministic.
func (s Sample) Validate(now time.Time) error {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	switch {
	case s.RobotID == "":
		return fail("robot_id is required")
	case s.SiteID == "":
		return fail("site_id is required")
	case s.TS.IsZero():
		return fail("ts is required")
	case s.TS.After(now.Add(MaxClockSkew)):
		return fail("ts %s is more than %s in the future", s.TS.Format(time.RFC3339), MaxClockSkew)
	case !validStates[s.State]:
		return fail("unknown state %q", s.State)
	}

	for name, v := range map[string]float64{
		"x_m": s.X, "y_m": s.Y, "depth_m": s.DepthM,
		"injection_pressure_kpa": s.InjectionPressureKPa, "flow_rate_lpm": s.FlowRateLPM,
		"surface_uplift_mm": s.SurfaceUpliftMM, "battery_pct": s.BatteryPct,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fail("%s must be finite", name)
		}
	}

	switch {
	case s.DepthM < 0 || s.DepthM > MaxDepthM:
		return fail("depth_m %.2f outside [0, %d]", s.DepthM, MaxDepthM)
	case s.InjectionPressureKPa < 0 || s.InjectionPressureKPa > MaxPressureKPa:
		return fail("injection_pressure_kpa %.1f outside [0, %d]", s.InjectionPressureKPa, MaxPressureKPa)
	case s.FlowRateLPM < 0 || s.FlowRateLPM > MaxFlowLPM:
		return fail("flow_rate_lpm %.1f outside [0, %d]", s.FlowRateLPM, MaxFlowLPM)
	case math.Abs(s.SurfaceUpliftMM) > MaxUpliftMM:
		return fail("surface_uplift_mm %.1f outside ±%d", s.SurfaceUpliftMM, MaxUpliftMM)
	case s.BatteryPct < 0 || s.BatteryPct > 100:
		return fail("battery_pct %.1f outside [0, 100]", s.BatteryPct)
	}
	return nil
}

// Bucket is a time-bucketed aggregate of one robot's samples.
type Bucket struct {
	Start           time.Time `json:"start"`
	Count           int64     `json:"count"`
	AvgPressureKPa  float64   `json:"avg_pressure_kpa"`
	MaxPressureKPa  float64   `json:"max_pressure_kpa"`
	AvgFlowLPM      float64   `json:"avg_flow_lpm"`
	MaxUpliftMM     float64   `json:"max_uplift_mm"`
	VolumeInjectedL float64   `json:"volume_injected_l"`
}

//! Per-robot state machine: move → drill → inject → retract, with faults and
//! operator commands layered on top.

use std::collections::VecDeque;

use serde::Serialize;

use crate::command::{Ack, Command};
use crate::physics::{self, Injection};
use crate::rng::Rng;

/// A planned injection well.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Well {
    pub x: f64,
    pub y: f64,
    pub depth_m: f64,
    pub volume_m3: f64,
}

#[derive(Debug, Clone, PartialEq)]
pub enum Mode {
    Idle,
    Moving,
    Drilling,
    Injecting,
    Fault(String),
}

/// Telemetry state as understood by the ingest service.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ReportedState {
    Idle,
    Drilling,
    Injecting,
    Fault,
}

pub const MOVE_SPEED_MPS: f64 = 0.4;
pub const DRILL_RATE_MPS: f64 = 0.05;
pub const MAX_FLOW_LPM: f64 = 250.0;
pub const DEFAULT_FLOW_LPM: f64 = 140.0;
/// Pressure trip: above this the robot faults to protect the formation.
pub const PRESSURE_LIMIT_KPA: f64 = 1_200.0;
const FLOW_TAU_S: f64 = 20.0;
const LOW_BATTERY_PCT: f64 = 8.0;

#[derive(Debug, Clone)]
pub struct Robot {
    pub id: String,
    pub x: f64,
    pub y: f64,
    pub depth_m: f64,
    pub mode: Mode,
    pub paused: bool,
    pub flow_setpoint_lpm: f64,
    pub flow_lpm: f64,
    pub pressure_kpa: f64,
    pub battery_pct: f64,
    pub plan: VecDeque<Well>,
    pub current: Option<Well>,
    /// Litres injected into `current` so far.
    pub injected_l: f64,
    /// Mode to return to after a fault is cleared.
    resume_mode: Option<Mode>,
}

impl Robot {
    pub fn new(id: impl Into<String>, x: f64, y: f64, plan: Vec<Well>) -> Self {
        Self {
            id: id.into(),
            x,
            y,
            depth_m: 0.0,
            mode: Mode::Idle,
            paused: false,
            flow_setpoint_lpm: DEFAULT_FLOW_LPM,
            flow_lpm: 0.0,
            pressure_kpa: 0.0,
            battery_pct: 100.0,
            plan: plan.into(),
            current: None,
            injected_l: 0.0,
            resume_mode: None,
        }
    }

    pub fn reported_state(&self) -> ReportedState {
        match self.mode {
            Mode::Fault(_) => ReportedState::Fault,
            _ if self.paused => ReportedState::Idle,
            Mode::Drilling => ReportedState::Drilling,
            Mode::Injecting => ReportedState::Injecting,
            Mode::Idle | Mode::Moving => ReportedState::Idle,
        }
    }

    /// The in-progress injection, so the site can include it in uplift.
    pub fn active_injection(&self) -> Option<Injection> {
        let w = self.current?;
        (self.injected_l > 0.0).then_some(Injection {
            x: w.x,
            y: w.y,
            depth_m: w.depth_m,
            volume_m3: self.injected_l / 1000.0,
        })
    }

    fn fault(&mut self, reason: &str) {
        if !matches!(self.mode, Mode::Fault(_)) {
            self.resume_mode = Some(self.mode.clone());
        }
        self.mode = Mode::Fault(reason.to_string());
        self.flow_lpm = 0.0;
        self.pressure_kpa = 0.0;
    }

    pub fn apply(&mut self, cmd: &Command) -> Ack {
        let reject = |r: &str| Ack::Rejected {
            reason: r.to_string(),
        };
        match cmd {
            Command::Pause => self.paused = true,
            Command::Resume => {
                if matches!(self.mode, Mode::Fault(_)) {
                    return reject("robot is faulted; clear_fault first");
                }
                self.paused = false;
            }
            Command::Estop => self.fault("estop"),
            Command::ClearFault => {
                let Mode::Fault(reason) = &self.mode else {
                    return reject("no active fault");
                };
                if reason == "low_battery" {
                    self.battery_pct = 100.0; // field crew swapped the pack
                }
                if reason == "overpressure" {
                    // Back off so we don't immediately re-trip.
                    self.flow_setpoint_lpm *= 0.8;
                }
                self.mode = self.resume_mode.take().unwrap_or(Mode::Idle);
                self.paused = true; // operator must explicitly resume
            }
            Command::SetFlow { flow_lpm } => {
                if !flow_lpm.is_finite() || *flow_lpm <= 0.0 || *flow_lpm > MAX_FLOW_LPM {
                    return reject(&format!("flow_lpm must be in (0, {MAX_FLOW_LPM}]"));
                }
                self.flow_setpoint_lpm = *flow_lpm;
            }
        }
        Ack::Completed
    }

    /// Advance `dt_s` seconds. Returns a completed injection when one finishes.
    pub fn step(&mut self, dt_s: f64, rng: &mut Rng) -> Option<Injection> {
        if matches!(self.mode, Mode::Fault(_)) {
            return None;
        }
        let drain = match self.mode {
            Mode::Moving => 0.010,
            Mode::Drilling => 0.015,
            Mode::Injecting => 0.006,
            _ => 0.001,
        };
        self.battery_pct = (self.battery_pct - drain * dt_s).max(0.0);
        if self.battery_pct < LOW_BATTERY_PCT {
            self.fault("low_battery");
            return None;
        }

        if self.paused {
            self.ramp_flow(0.0, dt_s);
            self.update_pressure(rng);
            return None;
        }

        match self.mode {
            Mode::Idle => {
                if let Some(w) = self.plan.pop_front() {
                    self.current = Some(w);
                    self.injected_l = 0.0;
                    self.mode = Mode::Moving;
                }
                None
            }
            Mode::Moving => {
                let w = self.current.expect("moving without a target");
                let (dx, dy) = (w.x - self.x, w.y - self.y);
                let dist = (dx * dx + dy * dy).sqrt();
                let step = MOVE_SPEED_MPS * dt_s;
                if dist <= step {
                    (self.x, self.y) = (w.x, w.y);
                    self.mode = Mode::Drilling;
                } else {
                    self.x += dx / dist * step;
                    self.y += dy / dist * step;
                }
                None
            }
            Mode::Drilling => {
                let w = self.current.expect("drilling without a target");
                self.depth_m = (self.depth_m + DRILL_RATE_MPS * dt_s).min(w.depth_m);
                if self.depth_m >= w.depth_m {
                    self.mode = Mode::Injecting;
                }
                None
            }
            Mode::Injecting => {
                let w = self.current.expect("injecting without a target");
                self.ramp_flow(self.flow_setpoint_lpm, dt_s);
                self.injected_l += self.flow_lpm * dt_s / 60.0;
                self.update_pressure(rng);
                if self.pressure_kpa > PRESSURE_LIMIT_KPA {
                    self.fault("overpressure");
                    return None;
                }
                if self.injected_l >= w.volume_m3 * 1000.0 {
                    let done = Injection {
                        x: w.x,
                        y: w.y,
                        depth_m: w.depth_m,
                        volume_m3: self.injected_l / 1000.0,
                    };
                    self.current = None;
                    self.injected_l = 0.0;
                    self.depth_m = 0.0;
                    self.flow_lpm = 0.0;
                    self.pressure_kpa = 0.0;
                    self.mode = Mode::Idle;
                    return Some(done);
                }
                None
            }
            Mode::Fault(_) => unreachable!(),
        }
    }

    fn ramp_flow(&mut self, target: f64, dt_s: f64) {
        let alpha = 1.0 - (-dt_s / FLOW_TAU_S).exp();
        self.flow_lpm += (target - self.flow_lpm) * alpha;
        if self.flow_lpm < 0.5 && target == 0.0 {
            self.flow_lpm = 0.0;
        }
    }

    fn update_pressure(&mut self, rng: &mut Rng) {
        let filled = self
            .current
            .map(|w| self.injected_l / (w.volume_m3 * 1000.0))
            .unwrap_or(0.0);
        let p = physics::injection_pressure_kpa(self.depth_m, self.flow_lpm, filled);
        self.pressure_kpa = if p > 0.0 {
            (p + rng.noise(8.0)).max(0.0)
        } else {
            0.0
        };
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn well() -> Well {
        Well {
            x: 4.0,
            y: 3.0,
            depth_m: 10.0,
            volume_m3: 2.0,
        }
    }

    fn run_until<F: Fn(&Robot) -> bool>(
        r: &mut Robot,
        rng: &mut Rng,
        max_s: usize,
        done: F,
    ) -> Vec<Injection> {
        let mut out = vec![];
        for _ in 0..max_s {
            if done(r) {
                break;
            }
            out.extend(r.step(1.0, rng));
        }
        out
    }

    #[test]
    fn full_cycle_injects_planned_volume() {
        let mut r = Robot::new("tn-01", 0.0, 0.0, vec![well()]);
        let mut rng = Rng::new(1);
        let done = run_until(&mut r, &mut rng, 10_000, |r| {
            r.plan.is_empty() && r.mode == Mode::Idle && r.current.is_none()
        });
        assert_eq!(done.len(), 1);
        let inj = done[0];
        assert!(
            (inj.volume_m3 - 2.0).abs() < 0.01,
            "volume {}",
            inj.volume_m3
        );
        assert_eq!((inj.x, inj.y, inj.depth_m), (4.0, 3.0, 10.0));
        assert_eq!((r.x, r.y), (4.0, 3.0));
        assert_eq!(r.depth_m, 0.0);
    }

    #[test]
    fn passes_through_each_state_in_order() {
        let mut r = Robot::new("tn-01", 0.0, 0.0, vec![well()]);
        let mut rng = Rng::new(1);
        let mut seen = vec![r.reported_state()];
        for _ in 0..5_000 {
            r.step(1.0, &mut rng);
            if *seen.last().unwrap() != r.reported_state() {
                seen.push(r.reported_state());
            }
        }
        use ReportedState::*;
        assert_eq!(seen, vec![Idle, Drilling, Injecting, Idle]);
    }

    #[test]
    fn overpressure_trips_and_clear_fault_backs_off_flow() {
        let mut r = Robot::new("tn-01", 4.0, 3.0, vec![well()]);
        let mut rng = Rng::new(1);
        r.apply(&Command::SetFlow {
            flow_lpm: MAX_FLOW_LPM,
        });
        // Deep well so breakdown pressure + high flow exceeds the limit.
        r.plan[0].depth_m = 40.0;
        run_until(&mut r, &mut rng, 5_000, |r| {
            matches!(r.mode, Mode::Fault(_))
        });
        assert_eq!(r.mode, Mode::Fault("overpressure".into()));
        assert_eq!(r.flow_lpm, 0.0);

        assert!(matches!(r.apply(&Command::Resume), Ack::Rejected { .. }));
        assert_eq!(r.apply(&Command::ClearFault), Ack::Completed);
        assert_eq!(r.mode, Mode::Injecting);
        assert!(r.paused, "clearing a fault should leave the robot paused");
        assert!((r.flow_setpoint_lpm - MAX_FLOW_LPM * 0.8).abs() < 1e-9);
    }

    #[test]
    fn pause_ramps_flow_down_and_stops_progress() {
        let mut r = Robot::new("tn-01", 4.0, 3.0, vec![well()]);
        let mut rng = Rng::new(1);
        run_until(&mut r, &mut rng, 5_000, |r| {
            r.mode == Mode::Injecting && r.flow_lpm > 100.0
        });
        r.apply(&Command::Pause);
        for _ in 0..300 {
            r.step(1.0, &mut rng);
        }
        assert_eq!(r.flow_lpm, 0.0);
        let before = r.injected_l;
        r.step(1.0, &mut rng);
        assert_eq!(r.injected_l, before);
        assert_eq!(r.reported_state(), ReportedState::Idle);
    }

    #[test]
    fn estop_requires_clear_then_resume() {
        let mut r = Robot::new("tn-01", 0.0, 0.0, vec![well()]);
        r.apply(&Command::Estop);
        assert_eq!(r.reported_state(), ReportedState::Fault);
        assert!(r.step(1.0, &mut Rng::new(1)).is_none());
        r.apply(&Command::ClearFault);
        r.apply(&Command::Resume);
        assert!(!r.paused);
        assert_eq!(r.mode, Mode::Idle);
    }

    #[test]
    fn rejects_bad_flow_setpoints() {
        let mut r = Robot::new("tn-01", 0.0, 0.0, vec![]);
        for bad in [0.0, -5.0, MAX_FLOW_LPM + 1.0, f64::NAN] {
            assert!(matches!(
                r.apply(&Command::SetFlow { flow_lpm: bad }),
                Ack::Rejected { .. }
            ));
        }
        assert_eq!(r.flow_setpoint_lpm, DEFAULT_FLOW_LPM);
    }

    #[test]
    fn low_battery_faults_and_swap_restores() {
        let mut r = Robot::new("tn-01", 0.0, 0.0, vec![]);
        r.battery_pct = LOW_BATTERY_PCT + 0.0005;
        r.step(1.0, &mut Rng::new(1));
        assert_eq!(r.mode, Mode::Fault("low_battery".into()));
        r.apply(&Command::ClearFault);
        assert_eq!(r.battery_pct, 100.0);
    }
}

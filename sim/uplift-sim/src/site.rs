//! A site: a well grid shared out across a robot fleet, plus the uplift field
//! produced by every injection so far.

use serde::Serialize;

use crate::physics::{self, Injection};
use crate::rng::Rng;
use crate::robot::{ReportedState, Robot, Well};

#[derive(Debug, Clone)]
pub struct SiteConfig {
    pub site_id: String,
    pub robots: usize,
    /// Wells per side of the square injection grid.
    pub grid: usize,
    pub spacing_m: f64,
    pub volume_m3: f64,
    pub seed: u64,
}

impl Default for SiteConfig {
    fn default() -> Self {
        Self {
            site_id: "alameda-pilot".into(),
            robots: 4,
            grid: 6,
            spacing_m: 8.0,
            volume_m3: 12.0,
            seed: 42,
        }
    }
}

/// One telemetry row; field names match the ingest service's JSON schema.
#[derive(Debug, Clone, Serialize)]
pub struct Sample {
    pub robot_id: String,
    pub site_id: String,
    pub ts: String,
    pub state: ReportedState,
    pub x_m: f64,
    pub y_m: f64,
    pub depth_m: f64,
    pub injection_pressure_kpa: f64,
    pub flow_rate_lpm: f64,
    pub surface_uplift_mm: f64,
    pub battery_pct: f64,
}

pub struct Site {
    pub id: String,
    pub robots: Vec<Robot>,
    pub completed: Vec<Injection>,
    rng: Rng,
}

impl Site {
    pub fn new(cfg: &SiteConfig) -> Self {
        let mut rng = Rng::new(cfg.seed);
        let half = (cfg.grid as f64 - 1.0) * cfg.spacing_m / 2.0;
        let mut wells = Vec::with_capacity(cfg.grid * cfg.grid);
        for row in 0..cfg.grid {
            // Serpentine order keeps each robot's moves short.
            let cols: Vec<usize> = if row % 2 == 0 {
                (0..cfg.grid).collect()
            } else {
                (0..cfg.grid).rev().collect()
            };
            for col in cols {
                wells.push(Well {
                    x: col as f64 * cfg.spacing_m - half,
                    y: row as f64 * cfg.spacing_m - half,
                    depth_m: 12.0 + rng.unit() * 6.0,
                    volume_m3: cfg.volume_m3,
                });
            }
        }
        // Contiguous blocks per robot, so robots work separate strips.
        let n = cfg.robots.max(1);
        let per = wells.len().div_ceil(n);
        let robots = (0..n)
            .map(|i| {
                let plan: Vec<Well> = wells.iter().skip(i * per).take(per).copied().collect();
                let start = plan
                    .first()
                    .map(|w| (w.x, w.y - cfg.spacing_m))
                    .unwrap_or((0.0, 0.0));
                Robot::new(format!("tn-{:02}", i + 1), start.0, start.1, plan)
            })
            .collect();
        Self {
            id: cfg.site_id.clone(),
            robots,
            completed: vec![],
            rng,
        }
    }

    pub fn step(&mut self, dt_s: f64) {
        for r in &mut self.robots {
            if let Some(done) = r.step(dt_s, &mut self.rng) {
                self.completed.push(done);
            }
        }
    }

    /// Every injection contributing to the uplift field right now.
    pub fn injections(&self) -> Vec<Injection> {
        let mut all = self.completed.clone();
        all.extend(self.robots.iter().filter_map(Robot::active_injection));
        all
    }

    pub fn uplift_mm_at(&self, x: f64, y: f64) -> f64 {
        physics::uplift_mm_at(&self.injections(), x, y)
    }

    pub fn total_injected_m3(&self) -> f64 {
        self.injections().iter().map(|i| i.volume_m3).sum()
    }

    pub fn is_finished(&self) -> bool {
        self.robots
            .iter()
            .all(|r| r.plan.is_empty() && r.current.is_none())
    }

    pub fn samples(&mut self, ts: &str) -> Vec<Sample> {
        let inj = self.injections();
        self.robots
            .iter()
            .map(|r| Sample {
                robot_id: r.id.clone(),
                site_id: self.id.clone(),
                ts: ts.to_string(),
                state: r.reported_state(),
                x_m: round(r.x, 2),
                y_m: round(r.y, 2),
                depth_m: round(r.depth_m, 2),
                injection_pressure_kpa: round(r.pressure_kpa, 1),
                flow_rate_lpm: round(r.flow_lpm, 1),
                surface_uplift_mm: round(physics::uplift_mm_at(&inj, r.x, r.y), 3),
                battery_pct: round(r.battery_pct, 1),
            })
            .collect()
    }
}

fn round(v: f64, places: i32) -> f64 {
    let m = 10f64.powi(places);
    (v * m).round() / m
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wells_are_split_across_robots() {
        let site = Site::new(&SiteConfig {
            robots: 4,
            grid: 6,
            ..Default::default()
        });
        let total: usize = site.robots.iter().map(|r| r.plan.len()).sum();
        assert_eq!(total, 36);
        assert!(site.robots.iter().all(|r| r.plan.len() == 9));
    }

    #[test]
    fn same_seed_same_site() {
        let a = Site::new(&SiteConfig::default());
        let b = Site::new(&SiteConfig::default());
        assert_eq!(a.robots[0].plan, b.robots[0].plan);
    }

    #[test]
    fn running_to_completion_lifts_the_centre() {
        let cfg = SiteConfig {
            robots: 2,
            grid: 3,
            volume_m3: 3.0,
            ..Default::default()
        };
        let mut site = Site::new(&cfg);
        let mut secs = 0;
        while !site.is_finished() && secs < 200_000 {
            site.step(1.0);
            secs += 1;
        }
        assert!(site.is_finished(), "did not finish in {secs}s");
        assert_eq!(site.completed.len(), 9);
        assert!((site.total_injected_m3() - 27.0).abs() < 0.1);
        let centre = site.uplift_mm_at(0.0, 0.0);
        let edge = site.uplift_mm_at(40.0, 40.0);
        assert!(centre > 5.0 * edge, "centre={centre} edge={edge}");
        assert!(centre > 1.0);
    }

    #[test]
    fn samples_match_ingest_schema() {
        let mut site = Site::new(&SiteConfig::default());
        site.step(1.0);
        let v = serde_json::to_value(site.samples("2026-10-01T00:00:00Z")).unwrap();
        let row = &v[0];
        for key in [
            "robot_id",
            "site_id",
            "ts",
            "state",
            "x_m",
            "y_m",
            "depth_m",
            "injection_pressure_kpa",
            "flow_rate_lpm",
            "surface_uplift_mm",
            "battery_pct",
        ] {
            assert!(row.get(key).is_some(), "missing {key}");
        }
        assert_eq!(row["state"], "idle");
    }
}

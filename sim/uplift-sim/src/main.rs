//! `uplift-sim`: drive a simulated robot fleet and stream telemetry to the
//! ingest service, taking operator commands from mission-control.

use std::thread;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use clap::Parser;
use serde_json::json;

use uplift_sim::command::{Ack, Envelope};
use uplift_sim::site::{Site, SiteConfig};

#[derive(Parser, Debug)]
#[command(version, about)]
struct Args {
    /// Telemetry ingest base URL, e.g. http://localhost:8080
    #[arg(long, env = "INGEST_URL")]
    ingest_url: Option<String>,
    /// Mission-control base URL for operator commands, e.g. http://localhost:3001
    #[arg(long, env = "CONTROL_URL")]
    control_url: Option<String>,
    /// Fleet API key presented to mission-control.
    #[arg(
        long,
        env = "ROBOT_KEY",
        default_value = "dev-robot",
        hide_env_values = true
    )]
    robot_key: String,
    #[arg(long, env = "SITE_ID", default_value = "alameda-pilot")]
    site: String,
    #[arg(long, default_value_t = 4)]
    robots: usize,
    #[arg(long, default_value_t = 6)]
    grid: usize,
    #[arg(long, default_value_t = 12.0)]
    volume_m3: f64,
    /// Simulated seconds per wall-clock second.
    #[arg(long, env = "SIM_SPEED", default_value_t = 30.0)]
    speed: f64,
    /// Wall-clock milliseconds between telemetry uploads.
    #[arg(long, default_value_t = 1000)]
    tick_ms: u64,
    #[arg(long, default_value_t = 42)]
    seed: u64,
    /// Stop after this many ticks (0 = run until the plan is finished).
    #[arg(long, default_value_t = 0)]
    ticks: u64,
    /// Print NDJSON to stdout instead of POSTing.
    #[arg(long)]
    dry_run: bool,
}

fn main() {
    let args = Args::parse();
    let cfg = SiteConfig {
        site_id: args.site.clone(),
        robots: args.robots,
        grid: args.grid,
        volume_m3: args.volume_m3,
        seed: args.seed,
        ..Default::default()
    };
    let mut site = Site::new(&cfg);
    let agent = ureq::AgentBuilder::new()
        .timeout(Duration::from_secs(5))
        .build();
    let sim_dt = args.tick_ms as f64 / 1000.0 * args.speed;
    eprintln!(
        "uplift-sim: site={} robots={} wells={} speed={}x",
        site.id,
        site.robots.len(),
        args.grid * args.grid,
        args.speed
    );

    let mut tick = 0u64;
    loop {
        let started = Instant::now();
        if let Some(url) = &args.control_url {
            poll_commands(&agent, url, &args.robot_key, &mut site);
        }
        // Sub-step at <= 1 simulated second so fast runs stay stable.
        let steps = sim_dt.ceil().max(1.0) as usize;
        for _ in 0..steps {
            site.step(sim_dt / steps as f64);
        }

        let samples = site.samples(&rfc3339_now());
        if args.dry_run || args.ingest_url.is_none() {
            for s in &samples {
                println!("{}", serde_json::to_string(s).expect("serialize"));
            }
        } else if let Some(url) = &args.ingest_url {
            let res = agent
                .post(&format!("{url}/v1/telemetry"))
                .send_json(json!({ "samples": samples }));
            if let Err(e) = res {
                eprintln!("ingest error (will retry next tick): {e}");
            }
        }

        tick += 1;
        if tick.is_multiple_of(30) {
            eprintln!(
                "tick {tick}: {}/{} wells done, {:.1} m³ injected, centre uplift {:.2} mm",
                site.completed.len(),
                args.grid * args.grid,
                site.total_injected_m3(),
                site.uplift_mm_at(0.0, 0.0)
            );
        }
        if (args.ticks > 0 && tick >= args.ticks) || (args.ticks == 0 && site.is_finished()) {
            eprintln!("done after {tick} ticks");
            break;
        }
        if !args.dry_run {
            thread::sleep(Duration::from_millis(args.tick_ms).saturating_sub(started.elapsed()));
        }
    }
}

/// Fetch at most one pending command per robot and report the outcome.
fn poll_commands(agent: &ureq::Agent, base: &str, key: &str, site: &mut Site) {
    let auth = format!("Bearer {key}");
    for robot in &mut site.robots {
        let url = format!("{base}/v1/robots/{}/commands/next", robot.id);
        let res = match agent.post(&url).set("Authorization", &auth).call() {
            Ok(r) if r.status() == 200 => r,
            Ok(_) => continue, // 204: nothing queued
            Err(e) => {
                eprintln!("control poll failed for {}: {e}", robot.id);
                return; // control plane down; don't hammer it for every robot
            }
        };
        let env: Envelope = match res.into_json() {
            Ok(e) => e,
            Err(e) => {
                eprintln!("bad command payload: {e}");
                continue;
            }
        };
        let ack = robot.apply(&env.command);
        if let Ack::Rejected { reason } = &ack {
            eprintln!("{} rejected {:?}: {reason}", robot.id, env.command);
        }
        let url = format!("{base}/v1/commands/{}/ack", env.id);
        if let Err(e) = agent.post(&url).set("Authorization", &auth).send_json(&ack) {
            eprintln!("ack failed for {}: {e}", env.id);
        }
    }
}

fn rfc3339_now() -> String {
    let d = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("clock before epoch");
    format_rfc3339(d.as_secs() as i64, d.subsec_millis())
}

/// Civil-from-days (Howard Hinnant) to avoid a chrono dependency.
fn format_rfc3339(secs: i64, millis: u32) -> String {
    let days = secs.div_euclid(86_400);
    let sod = secs.rem_euclid(86_400);
    let z = days + 719_468;
    let era = z.div_euclid(146_097);
    let doe = z.rem_euclid(146_097);
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = yoe + era * 400 + i64::from(m <= 2);
    format!(
        "{y:04}-{m:02}-{d:02}T{:02}:{:02}:{:02}.{millis:03}Z",
        sod / 3600,
        sod % 3600 / 60,
        sod % 60
    )
}

#[cfg(test)]
mod tests {
    use super::format_rfc3339;

    #[test]
    fn formats_known_instants() {
        assert_eq!(format_rfc3339(0, 0), "1970-01-01T00:00:00.000Z");
        assert_eq!(format_rfc3339(951_782_400, 5), "2000-02-29T00:00:00.005Z");
        assert_eq!(
            format_rfc3339(1_790_000_000, 123),
            "2026-09-21T14:13:20.123Z"
        );
    }
}

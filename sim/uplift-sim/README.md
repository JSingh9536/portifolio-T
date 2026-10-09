# uplift-sim (Rust)

This simulates a fleet of slurry-injection robots and the ground uplift they cause. It speaks the ingest and mission-control APIs, so it can stand in for real hardware.

- `physics.rs`: Mogi point-source uplift and the injection pressure model.
- `robot.rs`: the move → drill → inject state machine, faults, and how operator commands are applied.
- `site.rs`: the well grid, how wells are assigned to robots, and the superposed uplift field.

```bash
cargo test
cargo run --release -- --dry-run --ticks 5          # NDJSON to stdout
cargo run --release -- --ingest-url http://localhost:8080 --control-url http://localhost:3001 --speed 60
```

Flags: `--robots`, `--grid`, `--volume-m3`, `--speed` (simulated seconds per wall-clock second), `--tick-ms`, `--seed`, `--ticks`, and `--robot-key` (or the `ROBOT_KEY` environment variable).

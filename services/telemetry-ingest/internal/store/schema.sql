-- Idempotent: applied on every service start.
CREATE TABLE IF NOT EXISTS telemetry (
    ts                     TIMESTAMPTZ      NOT NULL,
    site_id                TEXT             NOT NULL,
    robot_id               TEXT             NOT NULL,
    state                  TEXT             NOT NULL,
    x_m                    DOUBLE PRECISION NOT NULL,
    y_m                    DOUBLE PRECISION NOT NULL,
    depth_m                DOUBLE PRECISION NOT NULL,
    injection_pressure_kpa DOUBLE PRECISION NOT NULL,
    flow_rate_lpm          DOUBLE PRECISION NOT NULL,
    surface_uplift_mm      DOUBLE PRECISION NOT NULL,
    battery_pct            DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS telemetry_robot_ts ON telemetry (robot_id, ts DESC);
CREATE INDEX IF NOT EXISTS telemetry_site_robot_ts ON telemetry (site_id, robot_id, ts DESC);

-- Promote to a hypertable with compression + retention when TimescaleDB is
-- installed. Plain Postgres skips this block and still works.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;
        PERFORM create_hypertable('telemetry', 'ts',
            chunk_time_interval => interval '1 day', if_not_exists => true, migrate_data => true);
        ALTER TABLE telemetry SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'site_id, robot_id',
            timescaledb.compress_orderby = 'ts DESC');
        PERFORM add_compression_policy('telemetry', interval '7 days', if_not_exists => true);
        PERFORM add_retention_policy('telemetry', interval '365 days', if_not_exists => true);
    END IF;
END $$;

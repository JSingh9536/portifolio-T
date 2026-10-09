export type RobotState = 'idle' | 'drilling' | 'injecting' | 'fault';

/** Mirrors telemetry-ingest's Sample JSON. */
export interface Sample {
  robot_id: string;
  site_id: string;
  ts: string;
  state: RobotState;
  x_m: number;
  y_m: number;
  depth_m: number;
  injection_pressure_kpa: number;
  flow_rate_lpm: number;
  surface_uplift_mm: number;
  battery_pct: number;
}

export type CommandType = 'pause' | 'resume' | 'estop' | 'clear_fault' | 'set_flow';
export type CommandStatus = 'queued' | 'sent' | 'completed' | 'rejected' | 'cancelled' | 'expired';

/** Mirrors mission-control's RobotCommand. */
export interface RobotCommand {
  id: string;
  robot_id: string;
  type: CommandType;
  params: { flow_lpm?: number } | null;
  status: CommandStatus;
  issued_by: string;
  reason?: string;
  created_at: string;
  finished_at?: string;
}

export interface RobotLink {
  robot_id: string;
  last_poll_at: string | null;
  pending: number;
}

export const PRESSURE_LIMIT_KPA = 1200;
export const MAX_FLOW_LPM = 250;

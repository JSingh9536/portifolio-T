export const COMMAND_TYPES = ['pause', 'resume', 'estop', 'clear_fault', 'set_flow'] as const;
export type CommandType = (typeof COMMAND_TYPES)[number];

/**
 * queued → sent → completed | rejected
 *   ↘ cancelled (operator, or superseded by an e-stop)
 *   ↘ expired   (never picked up, or picked up but never acknowledged)
 */
export type CommandStatus = 'queued' | 'sent' | 'completed' | 'rejected' | 'cancelled' | 'expired';
export const TERMINAL: ReadonlySet<CommandStatus> = new Set(['completed', 'rejected', 'cancelled', 'expired']);

export interface CommandParams {
  flow_lpm?: number;
}

export interface RobotCommand {
  id: string;
  robot_id: string;
  type: CommandType;
  params: CommandParams | null;
  status: CommandStatus;
  issued_by: string;
  reason?: string;
  created_at: string;
  sent_at?: string;
  finished_at?: string;
}

export interface AuditEvent {
  at: string;
  command_id: string;
  robot_id: string;
  type: CommandType;
  from: CommandStatus | null;
  to: CommandStatus;
  actor: string;
  reason?: string;
}

export interface RobotLink {
  robot_id: string;
  last_poll_at: string | null;
  pending: number;
}

/** Robot-side safety limits, mirrored from the firmware/simulator. */
export const MAX_FLOW_LPM = 250;
/** Queued commands go stale; an operator must re-issue rather than surprise a crew. */
export const QUEUE_TTL_MS = 120_000;
/** A robot that took a command but never acknowledged it is presumed to have lost it. */
export const ACK_TIMEOUT_MS = 30_000;
export const MAX_PENDING_PER_ROBOT = 20;

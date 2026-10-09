import {
  BadRequestException,
  ConflictException,
  HttpException,
  HttpStatus,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import { randomUUID } from 'node:crypto';
import { Clock } from '../clock';
import {
  ACK_TIMEOUT_MS,
  AuditEvent,
  CommandParams,
  CommandStatus,
  CommandType,
  MAX_FLOW_LPM,
  MAX_PENDING_PER_ROBOT,
  QUEUE_TTL_MS,
  RobotCommand,
  RobotLink,
  TERMINAL,
} from './command.types';

/**
 * In-memory command queue per robot. A single Node process is the source of
 * truth; swapping the maps for a Postgres table with `SELECT … FOR UPDATE
 * SKIP LOCKED` is the path to running more than one replica.
 */
@Injectable()
export class CommandsService {
  private readonly commands = new Map<string, RobotCommand>();
  private readonly audit: AuditEvent[] = [];
  private readonly lastPoll = new Map<string, Date>();

  constructor(private readonly clock: Clock) {}

  issue(robotId: string, type: CommandType, params: CommandParams | undefined, operator: string): RobotCommand {
    this.sweep();
    const cleanParams = validateParams(type, params);
    const pending = this.forRobot(robotId).filter((c) => !TERMINAL.has(c.status));

    if (type === 'estop') {
      // An e-stop supersedes everything still waiting, so nothing queued
      // behind it can restart the pump after the robot halts.
      for (const c of pending.filter((c) => c.status === 'queued')) {
        this.transition(c, 'cancelled', operator, 'superseded by estop');
      }
    } else if (pending.length >= MAX_PENDING_PER_ROBOT) {
      throw new HttpException(`robot ${robotId} already has ${pending.length} pending commands`, HttpStatus.TOO_MANY_REQUESTS);
    }

    const cmd: RobotCommand = {
      id: randomUUID(),
      robot_id: robotId,
      type,
      params: cleanParams,
      status: 'queued',
      issued_by: operator,
      created_at: this.clock.now().toISOString(),
    };
    this.commands.set(cmd.id, cmd);
    this.record(cmd, null, 'queued', operator);
    return cmd;
  }

  /** Robot pulls its next command. E-stops always jump the queue. */
  next(robotId: string): RobotCommand | null {
    this.sweep();
    this.lastPoll.set(robotId, this.clock.now());
    const queued = this.forRobot(robotId).filter((c) => c.status === 'queued');
    const cmd = queued.find((c) => c.type === 'estop') ?? queued[0];
    if (!cmd) return null;
    cmd.sent_at = this.clock.now().toISOString();
    this.transition(cmd, 'sent', `robot:${robotId}`);
    return cmd;
  }

  ack(id: string, status: 'completed' | 'rejected', reason?: string): RobotCommand {
    this.sweep();
    const cmd = this.get(id);
    if (cmd.status !== 'sent') {
      throw new ConflictException(`command ${id} is ${cmd.status}, not awaiting ack`);
    }
    this.transition(cmd, status, `robot:${cmd.robot_id}`, reason);
    return cmd;
  }

  cancel(id: string, operator: string): RobotCommand {
    this.sweep();
    const cmd = this.get(id);
    if (cmd.status !== 'queued') {
      throw new ConflictException(`only queued commands can be cancelled; ${id} is ${cmd.status}`);
    }
    this.transition(cmd, 'cancelled', operator, 'cancelled by operator');
    return cmd;
  }

  get(id: string): RobotCommand {
    const cmd = this.commands.get(id);
    if (!cmd) throw new NotFoundException(`command ${id} not found`);
    return cmd;
  }

  list(filter: { robotId?: string; status?: CommandStatus; limit?: number }): RobotCommand[] {
    this.sweep();
    return [...this.commands.values()]
      .filter((c) => !filter.robotId || c.robot_id === filter.robotId)
      .filter((c) => !filter.status || c.status === filter.status)
      .sort((a, b) => b.created_at.localeCompare(a.created_at))
      .slice(0, filter.limit ?? 100);
  }

  links(): RobotLink[] {
    this.sweep();
    const ids = new Set([...this.lastPoll.keys(), ...[...this.commands.values()].map((c) => c.robot_id)]);
    return [...ids].sort().map((robot_id) => ({
      robot_id,
      last_poll_at: this.lastPoll.get(robot_id)?.toISOString() ?? null,
      pending: this.forRobot(robot_id).filter((c) => !TERMINAL.has(c.status)).length,
    }));
  }

  auditLog(limit = 200): AuditEvent[] {
    return this.audit.slice(-limit).reverse();
  }

  /** Expire stale work. Run lazily on every access; no timers to leak. */
  private sweep(): void {
    const now = this.clock.now().getTime();
    for (const c of this.commands.values()) {
      if (c.status === 'queued' && c.type !== 'estop' && now - Date.parse(c.created_at) > QUEUE_TTL_MS) {
        this.transition(c, 'expired', 'system', 'not picked up before TTL');
      } else if (c.status === 'sent' && now - Date.parse(c.sent_at!) > ACK_TIMEOUT_MS) {
        this.transition(c, 'expired', 'system', 'robot did not acknowledge');
      }
    }
  }

  private forRobot(robotId: string): RobotCommand[] {
    return [...this.commands.values()]
      .filter((c) => c.robot_id === robotId)
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
  }

  private transition(cmd: RobotCommand, to: CommandStatus, actor: string, reason?: string): void {
    const from = cmd.status;
    cmd.status = to;
    if (reason) cmd.reason = reason;
    if (TERMINAL.has(to)) cmd.finished_at = this.clock.now().toISOString();
    this.record(cmd, from, to, actor, reason);
  }

  private record(cmd: RobotCommand, from: CommandStatus | null, to: CommandStatus, actor: string, reason?: string) {
    this.audit.push({
      at: this.clock.now().toISOString(),
      command_id: cmd.id,
      robot_id: cmd.robot_id,
      type: cmd.type,
      from,
      to,
      actor,
      ...(reason ? { reason } : {}),
    });
    if (this.audit.length > 10_000) this.audit.splice(0, this.audit.length - 10_000);
  }
}

function validateParams(type: CommandType, params: CommandParams | undefined): CommandParams | null {
  if (type === 'set_flow') {
    const flow = params?.flow_lpm;
    if (typeof flow !== 'number' || !(flow > 0) || flow > MAX_FLOW_LPM) {
      throw new BadRequestException(`set_flow requires params.flow_lpm in (0, ${MAX_FLOW_LPM}]`);
    }
    return { flow_lpm: flow };
  }
  if (params && Object.values(params).some((v) => v !== undefined)) {
    throw new BadRequestException(`${type} takes no params`);
  }
  return null;
}

import { BadRequestException, ConflictException, HttpException } from '@nestjs/common';
import { Clock } from '../clock';
import { ACK_TIMEOUT_MS, MAX_PENDING_PER_ROBOT, QUEUE_TTL_MS } from './command.types';
import { CommandsService } from './commands.service';

class FakeClock extends Clock {
  t = Date.parse('2026-10-01T12:00:00Z');
  now() {
    return new Date(this.t);
  }
  advance(ms: number) {
    this.t += ms;
  }
}

describe('CommandsService', () => {
  let clock: FakeClock;
  let svc: CommandsService;

  beforeEach(() => {
    clock = new FakeClock();
    svc = new CommandsService(clock);
  });

  it('delivers commands FIFO and walks the lifecycle', () => {
    const a = svc.issue('tn-01', 'pause', undefined, 'alice');
    clock.advance(1);
    const b = svc.issue('tn-01', 'set_flow', { flow_lpm: 90 }, 'alice');

    expect(svc.next('tn-01')?.id).toBe(a.id);
    expect(svc.get(a.id).status).toBe('sent');
    svc.ack(a.id, 'completed');
    expect(svc.get(a.id).status).toBe('completed');

    const got = svc.next('tn-01')!;
    expect(got.id).toBe(b.id);
    expect(got.params).toEqual({ flow_lpm: 90 });
    svc.ack(b.id, 'rejected', 'pump offline');
    expect(svc.get(b.id)).toMatchObject({ status: 'rejected', reason: 'pump offline' });

    expect(svc.next('tn-01')).toBeNull();
  });

  it('isolates robots', () => {
    svc.issue('tn-01', 'pause', undefined, 'alice');
    expect(svc.next('tn-02')).toBeNull();
  });

  it('estop cancels queued work and jumps the queue', () => {
    const pause = svc.issue('tn-01', 'pause', undefined, 'alice');
    const flow = svc.issue('tn-01', 'set_flow', { flow_lpm: 100 }, 'alice');
    clock.advance(5);
    const stop = svc.issue('tn-01', 'estop', undefined, 'bob');

    expect(svc.get(pause.id)).toMatchObject({ status: 'cancelled', reason: 'superseded by estop' });
    expect(svc.get(flow.id).status).toBe('cancelled');
    expect(svc.next('tn-01')?.id).toBe(stop.id);
  });

  it('estop is accepted even when the queue is full', () => {
    for (let i = 0; i < MAX_PENDING_PER_ROBOT; i++) svc.issue('tn-01', 'pause', undefined, 'alice');
    expect(() => svc.issue('tn-01', 'pause', undefined, 'alice')).toThrow(HttpException);
    expect(svc.issue('tn-01', 'estop', undefined, 'alice').status).toBe('queued');
  });

  it('expires stale queued commands but never an estop', () => {
    const pause = svc.issue('tn-01', 'pause', undefined, 'alice');
    const stop = svc.issue('tn-02', 'estop', undefined, 'alice');
    clock.advance(QUEUE_TTL_MS + 1);
    expect(svc.get(pause.id).status).toBe('queued'); // get() is a pure read
    expect(svc.next('tn-01')).toBeNull();
    expect(svc.get(pause.id).status).toBe('expired');
    expect(svc.next('tn-02')?.id).toBe(stop.id);
  });

  it('expires sent commands that are never acknowledged', () => {
    const c = svc.issue('tn-01', 'resume', undefined, 'alice');
    svc.next('tn-01');
    clock.advance(ACK_TIMEOUT_MS + 1);
    expect(() => svc.ack(c.id, 'completed')).toThrow(ConflictException);
    expect(svc.get(c.id)).toMatchObject({ status: 'expired', reason: 'robot did not acknowledge' });
  });

  it('validates params per command type', () => {
    expect(() => svc.issue('tn-01', 'set_flow', undefined, 'a')).toThrow(BadRequestException);
    expect(() => svc.issue('tn-01', 'set_flow', { flow_lpm: 0 }, 'a')).toThrow(BadRequestException);
    expect(() => svc.issue('tn-01', 'set_flow', { flow_lpm: 251 }, 'a')).toThrow(BadRequestException);
    expect(() => svc.issue('tn-01', 'pause', { flow_lpm: 10 }, 'a')).toThrow(BadRequestException);
  });

  it('only cancels queued commands', () => {
    const c = svc.issue('tn-01', 'pause', undefined, 'alice');
    svc.next('tn-01');
    expect(() => svc.cancel(c.id, 'alice')).toThrow(ConflictException);
  });

  it('records an attributable audit trail', () => {
    const c = svc.issue('tn-01', 'pause', undefined, 'alice');
    svc.next('tn-01');
    svc.ack(c.id, 'completed');
    const trail = svc.auditLog().reverse().map((e) => `${e.from}->${e.to} by ${e.actor}`);
    expect(trail).toEqual(['null->queued by alice', 'queued->sent by robot:tn-01', 'sent->completed by robot:tn-01']);
  });

  it('reports robot link status', () => {
    svc.issue('tn-02', 'pause', undefined, 'alice');
    svc.next('tn-01');
    expect(svc.links()).toEqual([
      { robot_id: 'tn-01', last_poll_at: '2026-10-01T12:00:00.000Z', pending: 0 },
      { robot_id: 'tn-02', last_poll_at: null, pending: 1 },
    ]);
  });
});

import { describe, expect, it } from 'vitest';
import { applySample, emptyFleet, HISTORY_LEN, summarize } from '../app/utils/fleet';
import type { Sample } from '../app/utils/types';

const base: Sample = {
  robot_id: 'tn-01', site_id: 's', ts: '2026-10-01T12:00:00Z', state: 'injecting', x_m: 0, y_m: 0,
  depth_m: 15, injection_pressure_kpa: 700, flow_rate_lpm: 140, surface_uplift_mm: 2, battery_pct: 90,
};
const at = (sec: number, extra: Partial<Sample> = {}): Sample => ({
  ...base, ts: new Date(Date.parse(base.ts) + sec * 1000).toISOString(), ...extra,
});

describe('applySample', () => {
  it('keeps the newest sample as latest even when replays arrive late', () => {
    const f = emptyFleet();
    applySample(f, at(10, { flow_rate_lpm: 1 }));
    applySample(f, at(5, { flow_rate_lpm: 2 }));
    expect(f.latest['tn-01']!.flow_rate_lpm).toBe(1);
    expect(f.history['tn-01']!.map((s) => s.flow_rate_lpm)).toEqual([2, 1]);
  });

  it('bounds history', () => {
    const f = emptyFleet();
    for (let i = 0; i < HISTORY_LEN + 50; i++) applySample(f, at(i));
    expect(f.history['tn-01']).toHaveLength(HISTORY_LEN);
    expect(f.history['tn-01']![0]!.ts).toBe(at(50).ts);
  });

  it('keeps one survey point per metre cell', () => {
    const f = emptyFleet();
    applySample(f, at(0, { x_m: 1.1, surface_uplift_mm: 1 }));
    applySample(f, at(1, { x_m: 0.9, surface_uplift_mm: 3 }));
    applySample(f, at(2, { x_m: 5, surface_uplift_mm: 2 }));
    expect(f.survey.size).toBe(2);
    expect(f.survey.get('1,0')!.uplift).toBe(3);
  });
});

describe('summarize', () => {
  it('aggregates fleet state', () => {
    const f = emptyFleet();
    applySample(f, at(0));
    applySample(f, at(0, { robot_id: 'tn-02', state: 'fault', flow_rate_lpm: 0, x_m: 9, surface_uplift_mm: 5 }));
    expect(summarize(f)).toEqual({ robots: 2, injecting: 1, faulted: 1, totalFlowLpm: 140, maxUpliftMm: 5 });
  });
});

import type { Sample } from './types';

export interface SurveyPoint {
  x: number;
  y: number;
  uplift: number;
}

export interface FleetState {
  latest: Record<string, Sample>;
  history: Record<string, Sample[]>;
  /** Most recent uplift reading per 1 m cell, from wherever robots have been. */
  survey: Map<string, SurveyPoint>;
}

export const HISTORY_LEN = 120;

export function emptyFleet(): FleetState {
  return { latest: {}, history: {}, survey: new Map() };
}

/**
 * Fold one sample into fleet state. Out-of-order samples (a reconnect replay,
 * a robot catching up after an LTE drop) update history but never roll back
 * the "latest" view.
 */
export function applySample(state: FleetState, s: Sample): void {
  const prev = state.latest[s.robot_id];
  if (!prev || Date.parse(s.ts) >= Date.parse(prev.ts)) state.latest[s.robot_id] = s;

  const h = (state.history[s.robot_id] ??= []);
  h.push(s);
  if (h.length > 1 && Date.parse(h[h.length - 2]!.ts) > Date.parse(s.ts)) {
    h.sort((a, b) => Date.parse(a.ts) - Date.parse(b.ts));
  }
  if (h.length > HISTORY_LEN) h.splice(0, h.length - HISTORY_LEN);

  const key = `${Math.round(s.x_m)},${Math.round(s.y_m)}`;
  state.survey.set(key, { x: s.x_m, y: s.y_m, uplift: s.surface_uplift_mm });
}

export interface FleetSummary {
  robots: number;
  injecting: number;
  faulted: number;
  totalFlowLpm: number;
  maxUpliftMm: number;
}

export function summarize(state: FleetState): FleetSummary {
  const all = Object.values(state.latest);
  return {
    robots: all.length,
    injecting: all.filter((s) => s.state === 'injecting').length,
    faulted: all.filter((s) => s.state === 'fault').length,
    totalFlowLpm: all.reduce((a, s) => a + s.flow_rate_lpm, 0),
    maxUpliftMm: Math.max(0, ...[...state.survey.values()].map((p) => p.uplift)),
  };
}

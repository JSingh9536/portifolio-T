import type { SurveyPoint } from './fleet';

/**
 * Inverse-distance-weighted interpolation of sparse robot readings onto a
 * grid. The dashboard deliberately knows no geophysics: it only renders what
 * robots measured.
 */
export function idw(points: SurveyPoint[], x: number, y: number, power = 2, radius = 12): number | null {
  let num = 0;
  let den = 0;
  for (const p of points) {
    const d2 = (p.x - x) ** 2 + (p.y - y) ** 2;
    if (d2 < 1e-9) return p.uplift;
    if (d2 > radius * radius) continue;
    const w = 1 / d2 ** (power / 2);
    num += w * p.uplift;
    den += w;
  }
  return den === 0 ? null : num / den;
}

export interface Bounds {
  minX: number;
  maxX: number;
  minY: number;
  maxY: number;
}

export function boundsOf(points: { x: number; y: number }[], pad = 6): Bounds {
  if (points.length === 0) return { minX: -30, maxX: 30, minY: -30, maxY: 30 };
  const xs = points.map((p) => p.x);
  const ys = points.map((p) => p.y);
  // Square, so metres look the same in both axes.
  const cx = (Math.min(...xs) + Math.max(...xs)) / 2;
  const cy = (Math.min(...ys) + Math.max(...ys)) / 2;
  const half = Math.max(Math.max(...xs) - Math.min(...xs), Math.max(...ys) - Math.min(...ys), 20) / 2 + pad;
  return { minX: cx - half, maxX: cx + half, minY: cy - half, maxY: cy + half };
}

/** Validated sequential blue ramp (light → dark), from the dataviz palette. */
export const SEQUENTIAL_BLUE = [
  '#cde2fb', '#b7d3f6', '#9ec5f4', '#86b6ef', '#6da7ec', '#5598e7', '#3987e5',
  '#2a78d6', '#256abf', '#1c5cab', '#184f95', '#104281', '#0d366b',
];

/**
 * Map t in [0, 1] to a ramp colour. On a dark surface the ramp is reversed so
 * "near zero" recedes into the background in both themes.
 */
export function rampColor(t: number, dark: boolean): string {
  const ramp = dark ? [...SEQUENTIAL_BLUE].reverse() : SEQUENTIAL_BLUE;
  const i = Math.round(Math.min(1, Math.max(0, t)) * (ramp.length - 1));
  return ramp[i]!;
}

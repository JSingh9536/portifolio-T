import { describe, expect, it } from 'vitest';
import { boundsOf, idw, rampColor, SEQUENTIAL_BLUE } from '../app/utils/heatmap';

describe('idw', () => {
  const pts = [
    { x: 0, y: 0, uplift: 10 },
    { x: 10, y: 0, uplift: 0 },
  ];
  it('returns exact readings at sample points', () => expect(idw(pts, 0, 0)).toBe(10));
  it('is the mean at the midpoint', () => expect(idw(pts, 5, 0)).toBeCloseTo(5));
  it('is closer to the nearer reading', () => expect(idw(pts, 2, 0)!).toBeGreaterThan(5));
  it('returns null outside the search radius', () => expect(idw(pts, 100, 100)).toBeNull());
});

describe('boundsOf', () => {
  it('is square and padded', () => {
    const b = boundsOf([{ x: 0, y: 0 }, { x: 40, y: 10 }], 5);
    expect(b.maxX - b.minX).toBeCloseTo(b.maxY - b.minY);
    expect(b.minX).toBe(-5);
    expect(b.maxX).toBe(45);
  });
});

describe('rampColor', () => {
  it('maps light ramp low→light and dark ramp low→dark', () => {
    expect(rampColor(0, false)).toBe(SEQUENTIAL_BLUE[0]);
    expect(rampColor(1, false)).toBe(SEQUENTIAL_BLUE.at(-1));
    expect(rampColor(0, true)).toBe(SEQUENTIAL_BLUE.at(-1));
    expect(rampColor(-3, false)).toBe(SEQUENTIAL_BLUE[0]);
  });
});

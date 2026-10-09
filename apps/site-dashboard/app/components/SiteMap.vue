<script setup lang="ts">
import type { FleetState } from '~/utils/fleet';
import { boundsOf, idw, rampColor, type Bounds } from '~/utils/heatmap';

const props = defineProps<{ fleet: FleetState; version: number; selected: string | null }>();
const emit = defineEmits<{ select: [robotId: string] }>();

const canvas = ref<HTMLCanvasElement | null>(null);
const wrap = ref<HTMLDivElement | null>(null);
const dark = useDarkMode();
const size = ref(480);
const hover = ref<{ px: number; py: number; x: number; y: number; uplift: number | null } | null>(null);
const CELL_M = 1;

const points = computed(() => (props.version, [...props.fleet.survey.values()]));
const robots = computed(() => (props.version, Object.values(props.fleet.latest)));
const bounds = computed<Bounds>(() => boundsOf([...points.value, ...robots.value.map((r) => ({ x: r.x_m, y: r.y_m }))]));
const maxUplift = computed(() => Math.max(1, ...points.value.map((p) => p.uplift)));

const toPx = (b: Bounds, x: number, y: number) => ({
  px: ((x - b.minX) / (b.maxX - b.minX)) * size.value,
  py: (1 - (y - b.minY) / (b.maxY - b.minY)) * size.value, // north up
});
const toM = (b: Bounds, px: number, py: number) => ({
  x: b.minX + (px / size.value) * (b.maxX - b.minX),
  y: b.minY + (1 - py / size.value) * (b.maxY - b.minY),
});

function draw() {
  const c = canvas.value;
  if (!c) return;
  const dpr = window.devicePixelRatio || 1;
  c.width = size.value * dpr;
  c.height = size.value * dpr;
  const ctx = c.getContext('2d')!;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  const css = getComputedStyle(document.documentElement);
  const surface = css.getPropertyValue('--surface-1').trim();
  const ink = css.getPropertyValue('--text-primary').trim();
  const muted = css.getPropertyValue('--text-muted').trim();
  const critical = css.getPropertyValue('--critical').trim();
  ctx.fillStyle = surface;
  ctx.fillRect(0, 0, size.value, size.value);

  const b = bounds.value;
  const pts = points.value;
  const cellPx = (CELL_M / (b.maxX - b.minX)) * size.value;
  for (let x = Math.floor(b.minX); x < b.maxX; x += CELL_M) {
    for (let y = Math.floor(b.minY); y < b.maxY; y += CELL_M) {
      const u = idw(pts, x + CELL_M / 2, y + CELL_M / 2);
      if (u === null) continue;
      const { px, py } = toPx(b, x, y + CELL_M);
      ctx.fillStyle = rampColor(u / maxUplift.value, dark.value);
      ctx.fillRect(px, py, cellPx + 0.5, cellPx + 0.5);
    }
  }

  // 10 m reference grid, recessive.
  ctx.strokeStyle = muted;
  ctx.globalAlpha = 0.18;
  ctx.lineWidth = 1;
  for (let g = Math.ceil(b.minX / 10) * 10; g < b.maxX; g += 10) {
    const { px } = toPx(b, g, 0);
    ctx.beginPath(); ctx.moveTo(px, 0); ctx.lineTo(px, size.value); ctx.stroke();
  }
  for (let g = Math.ceil(b.minY / 10) * 10; g < b.maxY; g += 10) {
    const { py } = toPx(b, 0, g);
    ctx.beginPath(); ctx.moveTo(0, py); ctx.lineTo(size.value, py); ctx.stroke();
  }
  ctx.globalAlpha = 1;

  // Robots: neutral marker with a surface ring; fault uses status red + label.
  ctx.font = '600 11px system-ui, sans-serif';
  for (const r of robots.value) {
    const { px, py } = toPx(b, r.x_m, r.y_m);
    const fault = r.state === 'fault';
    const rad = props.selected === r.robot_id ? 8 : 6;
    ctx.beginPath();
    ctx.arc(px, py, rad + 2, 0, Math.PI * 2);
    ctx.fillStyle = surface;
    ctx.fill();
    ctx.beginPath();
    ctx.arc(px, py, rad, 0, Math.PI * 2);
    ctx.fillStyle = fault ? critical : ink;
    ctx.fill();
    if (r.state === 'injecting') {
      ctx.beginPath();
      ctx.arc(px, py, rad + 5, 0, Math.PI * 2);
      ctx.strokeStyle = ink;
      ctx.lineWidth = 1.5;
      ctx.stroke();
    }
    const label = fault ? `${r.robot_id} ⚠ fault` : r.robot_id;
    ctx.fillStyle = ink;
    ctx.strokeStyle = surface;
    ctx.lineWidth = 3;
    ctx.strokeText(label, px + rad + 6, py + 4);
    ctx.fillText(label, px + rad + 6, py + 4);
  }
}

function onMove(e: MouseEvent) {
  const rect = canvas.value!.getBoundingClientRect();
  const px = e.clientX - rect.left;
  const py = e.clientY - rect.top;
  const { x, y } = toM(bounds.value, px, py);
  hover.value = { px, py, x, y, uplift: idw(points.value, x, y) };
}

function onClick(e: MouseEvent) {
  const rect = canvas.value!.getBoundingClientRect();
  let best: { id: string; d: number } | null = null;
  for (const r of robots.value) {
    const { px, py } = toPx(bounds.value, r.x_m, r.y_m);
    const d = Math.hypot(px - (e.clientX - rect.left), py - (e.clientY - rect.top));
    if (d < 20 && (!best || d < best.d)) best = { id: r.robot_id, d };
  }
  if (best) emit('select', best.id);
}

let ro: ResizeObserver | undefined;
onMounted(() => {
  ro = new ResizeObserver(([entry]) => {
    size.value = Math.max(240, Math.floor(entry!.contentRect.width));
  });
  ro.observe(wrap.value!);
});
onBeforeUnmount(() => ro?.disconnect());
watch([() => props.version, () => props.selected, size, dark], () => nextTick(draw));

const legendStops = computed(() => Array.from({ length: 13 }, (_, i) => rampColor(i / 12, dark.value)));
</script>

<template>
  <section class="panel map">
    <h2>Surface uplift · interpolated from robot readings</h2>
    <div ref="wrap" class="canvas-wrap">
      <canvas
        ref="canvas"
        :style="{ width: `${size}px`, height: `${size}px` }"
        role="img"
        :aria-label="`Site map with ${robots.length} robots; peak uplift ${maxUplift.toFixed(1)} millimetres`"
        @mousemove="onMove"
        @mouseleave="hover = null"
        @click="onClick"
      />
      <div v-if="hover" class="tooltip" :style="{ left: `${hover.px + 14}px`, top: `${hover.py + 14}px` }">
        <div class="num">{{ hover.uplift === null ? 'no data' : `${hover.uplift.toFixed(2)} mm` }}</div>
        <div class="sub num">x {{ hover.x.toFixed(1) }} m · y {{ hover.y.toFixed(1) }} m</div>
      </div>
      <p v-if="robots.length === 0" class="empty">Waiting for telemetry…</p>
    </div>
    <div class="legend">
      <span class="num">0</span>
      <span class="bar" :style="{ background: `linear-gradient(90deg, ${legendStops.join(',')})` }" />
      <span class="num">{{ maxUplift.toFixed(1) }} mm</span>
      <span class="key"><i class="dot" /> robot</span>
      <span class="key"><i class="dot ring" /> injecting</span>
      <span class="key"><i class="dot crit" /> fault</span>
    </div>
  </section>
</template>

<style scoped>
.canvas-wrap { position: relative; width: 100%; }
canvas { display: block; border-radius: 6px; cursor: crosshair; }
.tooltip {
  position: absolute;
  pointer-events: none;
  background: var(--surface-1);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 6px 8px;
  font-size: 12px;
  box-shadow: 0 2px 8px rgb(0 0 0 / 0.15);
  white-space: nowrap;
}
.tooltip .sub { color: var(--text-secondary); font-size: 11px; }
.empty { position: absolute; inset: 0; display: grid; place-items: center; margin: 0; color: var(--text-muted); }
.legend { display: flex; align-items: center; gap: 8px; margin-top: 10px; font-size: 12px; color: var(--text-secondary); flex-wrap: wrap; }
.bar { width: 140px; height: 8px; border-radius: 4px; }
.key { display: inline-flex; align-items: center; gap: 4px; margin-left: 6px; }
.dot { width: 9px; height: 9px; border-radius: 50%; background: var(--text-primary); display: inline-block; }
.dot.ring { box-shadow: 0 0 0 2px var(--surface-1), 0 0 0 3.5px var(--text-primary); }
.dot.crit { background: var(--critical); }
</style>

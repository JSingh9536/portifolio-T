<script setup lang="ts">
const props = defineProps<{
  values: number[];
  times: string[];
  label: string;
  unit: string;
  limit?: number;
  max?: number;
}>();

const W = 260;
const H = 56;
const PAD = 4;
const hover = ref<number | null>(null);

const yMax = computed(() => props.max ?? Math.max(props.limit ?? 0, ...props.values, 1) * 1.05);
const x = (i: number) => PAD + (i / Math.max(1, props.values.length - 1)) * (W - 2 * PAD);
const y = (v: number) => H - PAD - (v / yMax.value) * (H - 2 * PAD);
const path = computed(() => props.values.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(''));

function onMove(e: MouseEvent) {
  const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
  const rel = ((e.clientX - rect.left) / rect.width) * W;
  const i = Math.round(((rel - PAD) / (W - 2 * PAD)) * (props.values.length - 1));
  hover.value = props.values.length ? Math.min(props.values.length - 1, Math.max(0, i)) : null;
}
</script>

<template>
  <figure class="spark">
    <figcaption>
      <span>{{ label }}</span>
      <span v-if="hover !== null" class="num readout">
        {{ values[hover]!.toFixed(0) }} {{ unit }} ·
        {{ new Date(times[hover]!).toLocaleTimeString() }}
      </span>
    </figcaption>
    <div class="plot">
    <span v-if="limit" class="limit-label num" :style="{ top: `${(y(limit) / H) * 100}%` }">trip {{ limit }}</span>
    <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" @mousemove="onMove" @mouseleave="hover = null">
      <line v-if="limit" :x1="0" :x2="W" :y1="y(limit)" :y2="y(limit)" class="limit" />
      <path :d="path" class="line" />
      <template v-if="hover !== null">
        <line :x1="x(hover)" :x2="x(hover)" y1="0" :y2="H" class="cross" />
        <circle :cx="x(hover)" :cy="y(values[hover]!)" r="4" class="pt" />
      </template>
    </svg>
    </div>
  </figure>
</template>

<style scoped>
.spark { margin: 8px 0 0; }
figcaption { display: flex; justify-content: space-between; font-size: 11px; color: var(--text-secondary); min-height: 16px; }
.readout { color: var(--text-primary); }
svg { width: 100%; height: 56px; display: block; overflow: visible; }
.line { fill: none; stroke: var(--accent); stroke-width: 2; vector-effect: non-scaling-stroke; stroke-linejoin: round; }
.limit { stroke: var(--critical); stroke-dasharray: 4 3; stroke-width: 1; vector-effect: non-scaling-stroke; }
.plot { position: relative; }
.limit-label { position: absolute; right: 0; transform: translateY(-115%); font-size: 10px; color: var(--text-secondary); }
.cross { stroke: var(--text-muted); stroke-width: 1; vector-effect: non-scaling-stroke; }
.pt { fill: var(--accent); stroke: var(--surface-1); stroke-width: 2; }
</style>

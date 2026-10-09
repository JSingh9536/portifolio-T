<script setup lang="ts">
import { MAX_FLOW_LPM, PRESSURE_LIMIT_KPA, type CommandType, type RobotLink, type Sample } from '~/utils/types';

const props = defineProps<{ sample: Sample; history: Sample[]; link?: RobotLink; selected: boolean }>();
const emit = defineEmits<{ command: [type: CommandType, params?: { flow_lpm: number }]; select: [] }>();

const flow = ref(140);
const fault = computed(() => props.sample.state === 'fault');
const stale = computed(() => Date.now() - Date.parse(props.sample.ts) > 15_000);
const controlAge = computed(() =>
  props.link?.last_poll_at ? Math.round((Date.now() - Date.parse(props.link.last_poll_at)) / 1000) : null,
);
const STATE_LABEL: Record<Sample['state'], string> = {
  idle: '◦ Idle', drilling: '↓ Drilling', injecting: '● Injecting', fault: '⚠ Fault',
};
</script>

<template>
  <article class="panel robot" :class="{ selected, fault }" @click="emit('select')">
    <header>
      <strong class="num">{{ sample.robot_id }}</strong>
      <span class="state" :class="sample.state">{{ STATE_LABEL[sample.state] }}</span>
      <span v-if="stale" class="stale" title="No telemetry for >15 s">stale</span>
    </header>

    <dl>
      <div><dt>Pressure</dt><dd class="num">{{ sample.injection_pressure_kpa.toFixed(0) }}<small> kPa</small></dd></div>
      <div><dt>Flow</dt><dd class="num">{{ sample.flow_rate_lpm.toFixed(0) }}<small> L/min</small></dd></div>
      <div><dt>Depth</dt><dd class="num">{{ sample.depth_m.toFixed(1) }}<small> m</small></dd></div>
      <div><dt>Uplift</dt><dd class="num">{{ sample.surface_uplift_mm.toFixed(2) }}<small> mm</small></dd></div>
      <div><dt>Battery</dt><dd class="num">{{ sample.battery_pct.toFixed(0) }}<small>%</small></dd></div>
      <div>
        <dt>Control link</dt>
        <dd class="num">{{ controlAge === null ? '—' : `${controlAge}s ago` }}<small v-if="link?.pending"> · {{ link.pending }} pending</small></dd>
      </div>
    </dl>

    <Sparkline
      label="Injection pressure"
      unit="kPa"
      :values="history.map((s) => s.injection_pressure_kpa)"
      :times="history.map((s) => s.ts)"
      :limit="PRESSURE_LIMIT_KPA"
    />

    <div class="controls" @click.stop>
      <button @click="emit('command', 'pause')">Pause</button>
      <button :disabled="fault" @click="emit('command', 'resume')">Resume</button>
      <button :disabled="!fault" @click="emit('command', 'clear_fault')">Clear fault</button>
      <span class="setflow">
        <input v-model.number="flow" type="number" min="1" :max="MAX_FLOW_LPM" :aria-label="`Flow setpoint for ${sample.robot_id}`" />
        <button @click="emit('command', 'set_flow', { flow_lpm: flow })">Set L/min</button>
      </span>
      <button class="danger" @click="emit('command', 'estop')">E-STOP</button>
    </div>
  </article>
</template>

<style scoped>
.robot { cursor: pointer; transition: border-color 0.15s; }
.robot.selected { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent); }
.robot.fault { border-color: var(--critical); }
header { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
header strong { font-size: 15px; }
.state { font-size: 12px; color: var(--text-secondary); }
.state.fault { color: var(--critical); font-weight: 600; }
.stale { margin-left: auto; font-size: 11px; color: var(--text-muted); border: 1px solid var(--border); border-radius: 4px; padding: 0 5px; }
dl { display: grid; grid-template-columns: repeat(3, 1fr); gap: 6px 12px; margin: 0; }
dt { font-size: 11px; color: var(--text-secondary); }
dd { margin: 0; font-size: 15px; }
dd small { font-size: 11px; color: var(--text-muted); }
.controls { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; align-items: center; }
.setflow { display: inline-flex; gap: 4px; }
.danger { margin-left: auto; }
@media (max-width: 480px) { dl { grid-template-columns: repeat(2, 1fr); } }
</style>

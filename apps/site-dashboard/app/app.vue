<script setup lang="ts">
import { summarize } from '~/utils/fleet';
import type { CommandType } from '~/utils/types';

const { fleet, version, connected, error, siteId } = useTelemetry();
const control = useControl();
const selected = ref<string | null>(null);

const robots = computed(() => (version.value, Object.values(fleet.latest).sort((a, b) => a.robot_id.localeCompare(b.robot_id))));
const summary = computed(() => (version.value, summarize(fleet)));

function send(robotId: string, type: CommandType, params?: { flow_lpm: number }) {
  if (type === 'estop' || window.confirm(`Send ${type} to ${robotId}?`)) control.issue(robotId, type, params);
}
</script>

<template>
  <div class="app">
    <header class="top">
      <div>
        <h1>Site console <span class="site num">{{ siteId }}</span></h1>
        <p class="sub">Subsurface injection fleet — live telemetry &amp; command</p>
      </div>
      <div class="links">
        <span :class="['link', connected ? 'ok' : 'down']">{{ connected ? '● Telemetry live' : '○ Telemetry offline' }}</span>
        <span :class="['link', control.reachable.value ? 'ok' : 'down']">{{ control.reachable.value ? '● Control online' : '○ Control offline' }}</span>
      </div>
    </header>

    <p v-if="error" class="banner">{{ error }}</p>
    <p v-if="control.lastError.value" class="banner">Command refused — {{ control.lastError.value }}</p>

    <section class="kpis">
      <div class="panel kpi"><span>Robots</span><strong class="num">{{ summary.robots }}</strong></div>
      <div class="panel kpi"><span>Injecting</span><strong class="num">{{ summary.injecting }}</strong></div>
      <div class="panel kpi" :class="{ alert: summary.faulted }"><span>Faulted</span><strong class="num">{{ summary.faulted }}</strong></div>
      <div class="panel kpi"><span>Fleet flow</span><strong class="num">{{ summary.totalFlowLpm.toFixed(0) }}<small> L/min</small></strong></div>
      <div class="panel kpi"><span>Peak uplift</span><strong class="num">{{ summary.maxUpliftMm.toFixed(2) }}<small> mm</small></strong></div>
    </section>

    <main class="grid">
      <SiteMap :fleet="fleet" :version="version" :selected="selected" @select="selected = $event" />
      <div class="robots">
        <RobotCard
          v-for="r in robots"
          :key="r.robot_id"
          :sample="r"
          :history="(version, fleet.history[r.robot_id] ?? [])"
          :link="control.links.value[r.robot_id]"
          :selected="selected === r.robot_id"
          @select="selected = r.robot_id"
          @command="(type, params) => send(r.robot_id, type, params)"
        />
      </div>
    </main>

    <CommandLog :commands="control.commands.value" />
  </div>
</template>

<style scoped>
.app { max-width: 1320px; margin: 0 auto; padding: 20px 16px 40px; display: flex; flex-direction: column; gap: 14px; }
.top { display: flex; justify-content: space-between; align-items: flex-end; gap: 12px; flex-wrap: wrap; }
h1 { margin: 0; font-size: 20px; }
.site { font-size: 14px; font-weight: 500; color: var(--text-secondary); margin-left: 6px; }
.sub { margin: 2px 0 0; color: var(--text-secondary); }
.links { display: flex; gap: 12px; font-size: 12px; }
.link.ok { color: var(--text-primary); }
.link.down { color: var(--critical); }
.banner { margin: 0; padding: 8px 12px; border-radius: 8px; border: 1px solid var(--critical); color: var(--text-primary); background: var(--surface-1); }
.kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 10px; }
.kpi { display: flex; flex-direction: column; gap: 2px; padding: 10px 14px; }
.kpi span { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.05em; }
.kpi strong { font-size: 24px; font-weight: 600; }
.kpi small { font-size: 12px; color: var(--text-muted); font-weight: 400; }
.kpi.alert { border-color: var(--critical); }
.kpi.alert strong::after { content: ' ⚠'; color: var(--critical); font-size: 18px; }
.grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 14px; align-items: start; }
.robots { display: grid; gap: 10px; }
@media (max-width: 860px) { .grid { grid-template-columns: 1fr; } }
</style>

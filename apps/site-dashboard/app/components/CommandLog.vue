<script setup lang="ts">
import type { RobotCommand } from '~/utils/types';

defineProps<{ commands: RobotCommand[] }>();
const ICON: Record<RobotCommand['status'], string> = {
  queued: '◦', sent: '→', completed: '✓', rejected: '✕', cancelled: '–', expired: '⌛',
};
</script>

<template>
  <section class="panel">
    <h2>Command log</h2>
    <p v-if="commands.length === 0" class="empty">No commands issued yet.</p>
    <div v-else class="scroll">
    <table>
      <thead><tr><th>Time</th><th>Robot</th><th>Command</th><th>Status</th><th>By</th></tr></thead>
      <tbody>
        <tr v-for="c in commands" :key="c.id">
          <td class="num">{{ new Date(c.created_at).toLocaleTimeString() }}</td>
          <td class="num">{{ c.robot_id }}</td>
          <td>{{ c.type }}{{ c.params?.flow_lpm ? ` ${c.params.flow_lpm} L/min` : '' }}</td>
          <td :class="['status', c.status]" :title="c.reason">{{ ICON[c.status] }} {{ c.status }}</td>
          <td>{{ c.issued_by }}</td>
        </tr>
      </tbody>
    </table>
    </div>
  </section>
</template>

<style scoped>
.scroll { overflow-x: auto; }
table { min-width: 420px; width: 100%; border-collapse: collapse; font-size: 12px; }
th { text-align: left; font-weight: 500; color: var(--text-secondary); border-bottom: 1px solid var(--border); padding: 4px 6px; }
td { padding: 4px 6px; border-bottom: 1px solid var(--surface-2); }
.status.rejected, .status.expired { color: var(--critical); }
.status.completed { color: var(--text-primary); }
.status.queued, .status.sent, .status.cancelled { color: var(--text-secondary); }
.empty { color: var(--text-muted); margin: 0; }
</style>

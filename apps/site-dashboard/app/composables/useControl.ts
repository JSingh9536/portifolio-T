import type { CommandType, RobotCommand, RobotLink } from '~/utils/types';

/**
 * Operator side of mission-control: issue commands, watch their lifecycle.
 * Calls go through the dashboard's own /api/control proxy, which holds the key.
 */
export function useControl(pollMs = 2000) {
  const base = '/api/control';
  const commands = ref<RobotCommand[]>([]);
  const links = ref<Record<string, RobotLink>>({});
  const reachable = ref(true);
  const lastError = ref<string | null>(null);

  async function refresh() {
    try {
      const [cmds, ls] = await Promise.all([
        $fetch<RobotCommand[]>(`${base}/commands?limit=40`),
        $fetch<RobotLink[]>(`${base}/robots`),
      ]);
      commands.value = cmds;
      links.value = Object.fromEntries(ls.map((l) => [l.robot_id, l]));
      reachable.value = true;
    } catch {
      reachable.value = false;
    }
  }

  async function issue(robotId: string, type: CommandType, params?: { flow_lpm: number }) {
    lastError.value = null;
    try {
      await $fetch(`${base}/robots/${encodeURIComponent(robotId)}/commands`, {
        method: 'POST',
        body: params ? { type, params } : { type },
      });
    } catch (e: unknown) {
      const msg = (e as { data?: { message?: string | string[] } }).data?.message;
      lastError.value = `${robotId} ${type}: ${Array.isArray(msg) ? msg.join('; ') : (msg ?? 'request failed')}`;
    }
    await refresh();
  }

  let timer: ReturnType<typeof setInterval> | undefined;
  onMounted(() => {
    refresh();
    timer = setInterval(refresh, pollMs);
  });
  onBeforeUnmount(() => clearInterval(timer));

  return { commands, links, reachable, lastError, issue, refresh };
}

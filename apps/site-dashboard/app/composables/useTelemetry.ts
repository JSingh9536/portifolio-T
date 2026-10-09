import { applySample, emptyFleet, type FleetState } from '~/utils/fleet';
import type { Sample } from '~/utils/types';

/**
 * Live fleet state from telemetry-ingest: seed from /latest, then follow the
 * SSE stream. EventSource reconnects on its own; we only track link status.
 *
 * Fleet state is a plain (non-reactive) object mutated in place; `version`
 * ticks at most once per animation frame so a 50-robot site doesn't trigger
 * 50 re-renders per upload.
 */
export function useTelemetry() {
  const { ingestUrl, siteId } = useRuntimeConfig().public;
  const fleet: FleetState = emptyFleet();
  const version = ref(0);
  const connected = ref(false);
  const lastMessageAt = ref<number | null>(null);
  const error = ref<string | null>(null);

  let es: EventSource | null = null;
  let frame = 0;
  const bump = () => {
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      version.value++;
    });
  };

  async function seed() {
    try {
      const rows = await $fetch<Sample[]>(`${ingestUrl}/v1/sites/${encodeURIComponent(siteId)}/latest`);
      rows.forEach((s) => applySample(fleet, s));
      bump();
      error.value = null;
    } catch (e) {
      error.value = `telemetry-ingest unreachable at ${ingestUrl}`;
    }
  }

  function connect() {
    es = new EventSource(`${ingestUrl}/v1/stream?site=${encodeURIComponent(siteId)}`);
    es.onopen = () => {
      connected.value = true;
      error.value = null;
    };
    es.onerror = () => {
      connected.value = false;
    };
    es.addEventListener('sample', (ev) => {
      applySample(fleet, JSON.parse((ev as MessageEvent).data) as Sample);
      lastMessageAt.value = Date.now();
      bump();
    });
  }

  onMounted(async () => {
    await seed();
    connect();
  });
  onBeforeUnmount(() => {
    es?.close();
    cancelAnimationFrame(frame);
  });

  return { fleet, version, connected, lastMessageAt, error, siteId };
}

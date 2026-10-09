/** Tracks prefers-color-scheme so canvas drawing can pick its ramp. */
export function useDarkMode() {
  const dark = ref(false);
  onMounted(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    dark.value = mq.matches;
    mq.addEventListener('change', (e) => (dark.value = e.matches));
  });
  return dark;
}

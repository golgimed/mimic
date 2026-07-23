const DEFAULT_DELAY_MS = Number(process.env.DEFAULT_DELAY_MS ?? 0);

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Simulates processing latency. Pass an explicit ms to override
 * per-call (e.g. from fault_config); otherwise falls back to
 * DEFAULT_DELAY_MS so latency is controllable without code changes.
 */
export async function simulateLatency(ms?: number): Promise<void> {
  const delay = ms ?? DEFAULT_DELAY_MS;
  if (delay > 0) {
    await sleep(delay);
  }
}

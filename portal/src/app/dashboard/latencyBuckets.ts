import type { LatencyBucket } from '@/components/LatencyChart';

/** The slice of AccessLog this chart needs, as /dashboard/logs returns it */
export interface LatencyLog {
  time: string;
  statusCode: number;
  durationMs: number;
  upstreamStatus?: number;
  upstreamDurationMs?: number;
  cacheStatus?: string;
}

/** Did this request actually reach the origin?
 *
 *  A cache HIT copies the cached status into upstreamStatus but records no duration,
 *  so counting it would read as the origin answering in 0 ms every time the cache is
 *  warm, dragging the line down on its own. */
export function reachedOrigin(log: LatencyLog): boolean {
  return log.cacheStatus !== 'HIT' && (log.upstreamStatus ?? 0) > 0;
}

/** Bucket size for a given window. Mirrors pickInterval in the Go service so this
 *  chart slices time the same way as "Requests over time" above it. */
export function latencyInterval(fromMs: number, toMs: number): string {
  const hours = (toMs - fromMs) / (60 * 60 * 1000);
  if (hours <= 6) return 'minute';
  if (hours <= 7 * 24) return 'hour';
  return 'day';
}

/** One bucket's length per interval */
export const STEP_MS: Record<string, number> = {
  minute: 60 * 1000,
  hour: 60 * 60 * 1000,
  day: 24 * 60 * 60 * 1000,
};

/** Floor to the bucket head in local time, so axis labels match the reader's clock */
export function truncateTo(date: Date, interval: string): number {
  const t = new Date(date);
  if (interval === 'day') t.setHours(0, 0, 0, 0);
  else if (interval === 'hour') t.setMinutes(0, 0, 0);
  else t.setSeconds(0, 0);
  return t.getTime();
}

interface Tally {
  count: number;
  sampleCount: number;
  gatewaySum: number;
  originSum: number;
}

export function bucketLatency(logs: LatencyLog[], interval: string): LatencyBucket[] {
  const tallies = new Map<number, Tally>();

  for (const log of logs) {
    const key = truncateTo(new Date(log.time), interval);
    let tally = tallies.get(key);
    if (!tally) {
      tally = { count: 0, sampleCount: 0, gatewaySum: 0, originSum: 0 };
      tallies.set(key, tally);
    }
    tally.count++;
    // Both averages come off the same set of requests. Counting cache hits on the
    // gateway side but not the origin side would compare two different populations.
    if (reachedOrigin(log)) {
      tally.sampleCount++;
      tally.gatewaySum += log.durationMs;
      tally.originSum += log.upstreamDurationMs ?? 0;
    }
  }

  return [...tallies.entries()]
    .sort(([a], [b]) => a - b)
    .map(([key, tally]) => ({
      time: new Date(key).toISOString(),
      count: tally.count,
      sampleCount: tally.sampleCount,
      avgGatewayMs: tally.sampleCount > 0 ? tally.gatewaySum / tally.sampleCount : 0,
      avgOriginMs: tally.sampleCount > 0 ? tally.originSum / tally.sampleCount : 0,
    }));
}

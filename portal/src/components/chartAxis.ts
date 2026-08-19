/** Plot box, in the same units as the rendered pixel width. Both charts draw 1:1
 *  against their measured width so axis text keeps its aspect. */
export const VIEW_H = 280;
export const PLOT_L = 48;
export const PAD_R = 16;
export const PLOT_T = 16;
export const PLOT_B = 236;
export const AXIS_Y = 258; // x-axis label band
export const FALLBACK_W = 720; // until ResizeObserver reports
export const STEPS = 4; // 4 gaps = 5 ticks on y
export const X_DIVISIONS = 5;

export const round = (n: number) => Math.round(n * 100) / 100;

/** Round the y scale to readable numbers — a peak of 222 gives 0/70/140/210/280.
 *  7 is in the multiplier set because quartering a peak often lands on 70/140/210. */
export function niceTicks(max: number, steps = STEPS): number[] {
  const target = Math.max(max, 1) / steps;
  const magnitude = 10 ** Math.floor(Math.log10(target));
  const step = ([1, 2, 2.5, 5, 7, 10].find((m) => m * magnitude >= target) ?? 10) * magnitude;
  return Array.from({ length: steps + 1 }, (_, i) => i * step);
}

/** Only as many decimals as the step needs, or a 0.25 step prints as 0.3 all the way up */
export function formatTickValue(value: number, step: number): string {
  const text = String(step);
  const dot = text.indexOf('.');
  return value.toFixed(dot === -1 ? 0 : Math.min(text.length - dot - 1, 3));
}

/** How x labels read follows the selected range: short ranges care about time of day,
 *  multi-day ranges on hourly buckets need both, long ones about which day or week */
export type AxisLabelFormat = 'time' | 'dateTime' | 'date' | 'dateRange';

const shortMonth = (d: Date) => d.toLocaleDateString([], { month: 'short' });

/** Assembled by hand: toLocaleDateString options pick which parts show, not their
 *  order — under en-US that yields "Aug 17, 2026" rather than "17 Aug 2026" */
const fullDate = (d: Date) => `${d.getDate()} ${shortMonth(d)} ${d.getFullYear()}`;

/** Drop the repeated parts: "1-7 Aug 2026", not "1 Aug 2026 - 7 Aug 2026" */
export function formatSpan(start: Date, end: Date): string {
  // one day wide is not a span — "18-18 Aug" would claim two days
  if (start.toDateString() === end.toDateString()) return fullDate(start);
  if (start.getFullYear() !== end.getFullYear()) {
    return `${start.getDate()} ${shortMonth(start)} ${start.getFullYear()} - ${end.getDate()} ${shortMonth(end)} ${end.getFullYear()}`;
  }
  if (start.getMonth() !== end.getMonth()) {
    return `${start.getDate()} ${shortMonth(start)} - ${end.getDate()} ${shortMonth(end)} ${end.getFullYear()}`;
  }
  return `${start.getDate()}-${end.getDate()} ${shortMonth(end)} ${end.getFullYear()}`;
}

/** Tooltip heading, "14 Aug 12:00". No year — the widest range is 30 days */
export function formatStamp(date: Date): string {
  const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
  return `${date.getDate()} ${shortMonth(date)} ${time}`;
}

/** Point labels only — spans go through formatSpan, which needs both ends */
export function formatTick(date: Date, format: AxisLabelFormat): string {
  if (format === 'time') {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
  }
  return fullDate(date);
}

/** Which bucket indices carry an x label. Five divisions, read two ways. Point labels:
 *  marks are label positions, spread end to end. Span labels: marks are each slot's left
 *  edge, with the label centred inside. Sharing one set of positions would put the last
 *  span label on the final bucket, so its head and tail were the same bucket. */
export function axisMarks(count: number, isSpan: boolean): number[] {
  const divisions = Math.min(X_DIVISIONS, count);
  return Array.from(
    new Set(
      Array.from({ length: divisions }, (_, k) =>
        isSpan
          ? Math.floor((k * count) / divisions)
          : divisions === 1
            ? 0
            : Math.round((k * (count - 1)) / (divisions - 1)),
      ),
    ),
  );
}

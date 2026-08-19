/* Hallmark · component: traffic-chart · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * One series of counts. Bars sit on a real time axis, so a quiet stretch shows as a gap.
 */
'use client';

import React from 'react';
import {
  VIEW_H,
  PLOT_L,
  PAD_R,
  PLOT_T,
  PLOT_B,
  AXIS_Y,
  FALLBACK_W,
  STEPS,
  round,
  niceTicks,
  formatTickValue,
  formatSpan,
  formatStamp,
  formatTick,
  axisMarks,
  type AxisLabelFormat,
} from './chartAxis';

export interface TrafficBucket {
  time: string;
  count: number;
}

const MIN_BAR_W = 1;
const BAR_GAP = 2; // surface showing between neighbours
const BAR_RADIUS = 4;

/** Marks are picked by bucket index, but bars sit on a time axis — an uneven spread can
 *  put two of them side by side, so drop any that would crowd the one kept before it. */
function spaceOut(marks: number[], centerAt: (i: number) => number, minGap: number): number[] {
  const kept: number[] = [];
  for (const idx of marks) {
    const prev = kept[kept.length - 1];
    if (prev === undefined || centerAt(idx) - centerAt(prev) >= minGap) kept.push(idx);
  }
  return kept;
}

export function TrafficChart({
  buckets,
  interval,
  stepMs,
  labelFormat,
}: {
  buckets: TrafficBucket[];
  interval: string;
  /** one bucket's width in ms, so a bar covers the time it actually represents */
  stepMs: number;
  labelFormat: AxisLabelFormat;
}) {
  const [hovered, setHovered] = React.useState<number | null>(null);

  const wrapRef = React.useRef<HTMLDivElement>(null);
  const [width, setWidth] = React.useState(FALLBACK_W);
  React.useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  const plotR = Math.max(width - PAD_R, PLOT_L + 1);

  const times = buckets.map((b) => new Date(b.time).getTime());
  const t0 = times[0];
  // the last bar occupies a whole step, so the axis runs to the end of it
  const t1 = times[times.length - 1] + stepMs;
  const span = t1 - t0;
  const plotW = plotR - PLOT_L;

  // x follows real time, not index — empty buckets are absent from the response, and an
  // index scale would close those gaps and misdate every label after one
  const xAt = (ms: number) => (span > 0 ? PLOT_L + ((ms - t0) / span) * plotW : PLOT_L);
  const barW = span > 0 ? Math.max((stepMs / span) * plotW - BAR_GAP, MIN_BAR_W) : plotW;
  // the anchor for everything that lines up with a bucket: its bar's middle
  const centerAt = (i: number) => xAt(times[i]) + BAR_GAP / 2 + barW / 2;

  const total = buckets.reduce((sum, b) => sum + b.count, 0);
  const peak = buckets.reduce((m, b) => Math.max(m, b.count), 0);
  const ticks = niceTicks(peak);
  const yMax = ticks[STEPS];
  const yAt = (value: number) => PLOT_B - (value / yMax) * (PLOT_B - PLOT_T);

  const isSpan = labelFormat === 'dateRange';
  // roughly half a rendered label at 11px, so edge labels can be kept inside the plot
  const labelHalf = isSpan ? 44 : labelFormat === 'time' ? 20 : 36;
  const marks = spaceOut(axisMarks(buckets.length, isSpan), centerAt, labelHalf * 2 + 8);

  const hoveredBucket = hovered !== null ? buckets[hovered] : undefined;
  const hoverTopY = hoveredBucket ? yAt(hoveredBucket.count) : 0;
  const hoverAbove = hoverTopY > 96;

  return (
    <div>
      {/* y unit lives up here so the ticks stay bare numbers */}
      <p className="mb-1 text-xs text-muted">requests</p>
      <div ref={wrapRef} className="relative" onMouseLeave={() => setHovered(null)}>
        <svg
          className="w-full"
          viewBox={`0 0 ${width} ${VIEW_H}`}
          role="img"
          aria-label={`Request volume across ${buckets.length} buckets — ${total.toLocaleString()} requests in total, peaking at ${peak.toLocaleString()} per ${interval}`}
        >
          {ticks.map((t, i) => (
            <g key={t}>
              <line
                x1={PLOT_L}
                y1={yAt(t)}
                x2={plotR}
                y2={yAt(t)}
                stroke="var(--color-rule)"
                strokeWidth="1"
                strokeDasharray="4 4"
              />
              <text
                className="tabular"
                x={PLOT_L - 8}
                y={yAt(t) + 4}
                textAnchor="end"
                fontSize="11"
                fill="var(--color-muted)"
              >
                {formatTickValue(t, ticks[1] || 1)}
              </text>
              {/* verticals drawn once, hung off the first tick so they do not repeat per tick */}
              {i === 0 &&
                marks.map((idx) => (
                  <line
                    key={idx}
                    x1={centerAt(idx)}
                    y1={PLOT_T}
                    x2={centerAt(idx)}
                    y2={PLOT_B}
                    stroke="var(--color-rule)"
                    strokeWidth="1"
                    strokeDasharray="4 4"
                  />
                ))}
            </g>
          ))}

          {buckets.map((b, i) => {
            const h = Math.max(PLOT_B - yAt(b.count), 0);
            return (
              <rect
                key={b.time}
                x={round(centerAt(i) - barW / 2)}
                y={round(PLOT_B - h)}
                width={round(barW)}
                height={round(h)}
                rx={Math.min(BAR_RADIUS, barW / 2)}
                fill="var(--color-accent)"
                opacity={hovered === null || hovered === i ? 1 : 0.55}
              />
            );
          })}

          {marks.map((i, k) => {
            // a slot runs from its own left edge to just before the next one; the last reaches the end
            const spanEnd = (marks[k + 1] ?? buckets.length) - 1;
            const x = isSpan ? (centerAt(i) + centerAt(spanEnd)) / 2 : centerAt(i);
            return (
              <text
                className="tabular"
                key={buckets[i].time}
                // always centred on the bar, nudged in only when it would spill past an edge
                x={Math.min(Math.max(x, PLOT_L + labelHalf), plotR - labelHalf)}
                y={AXIS_Y}
                textAnchor="middle"
                fontSize="11"
                fill="var(--color-muted)"
              >
                {isSpan
                  ? formatSpan(new Date(buckets[i].time), new Date(buckets[spanEnd].time))
                  : formatTick(new Date(buckets[i].time), labelFormat)}
              </text>
            );
          })}

          {/* hit strips reach halfway to each neighbour; must render last to receive events */}
          {buckets.map((b, i) => {
            const left = i === 0 ? PLOT_L : (centerAt(i - 1) + centerAt(i)) / 2;
            const right = i === buckets.length - 1 ? plotR : (centerAt(i) + centerAt(i + 1)) / 2;
            return (
              <rect
                key={b.time}
                x={round(left)}
                y={PLOT_T}
                width={Math.max(round(right - left), 1)}
                height={PLOT_B - PLOT_T}
                fill="transparent"
                onMouseEnter={() => setHovered(i)}
              />
            );
          })}
        </svg>

        {hoveredBucket && (
          <div
            className="pointer-events-none absolute z-10 rounded-surface border border-rule bg-paper px-3 py-2 text-xs whitespace-nowrap"
            style={{
              // keep the box inside the plot by reserving half its width at each edge
              left: Math.min(Math.max(centerAt(hovered as number), 95), Math.max(width - 95, 95)),
              top: hoverAbove ? hoverTopY - 12 : hoverTopY + 12,
              transform: hoverAbove ? 'translate(-50%, -100%)' : 'translate(-50%, 0)',
              boxShadow: 'var(--shadow-pop)',
            }}
          >
            <p className="text-muted">{formatStamp(new Date(hoveredBucket.time))}</p>
            <p className="tabular mt-1 font-medium text-ink">
              {hoveredBucket.count.toLocaleString()} request
              {hoveredBucket.count === 1 ? '' : 's'}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

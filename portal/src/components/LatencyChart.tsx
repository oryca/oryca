/* Hallmark · component: latency-chart · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * Two series of equal weight — one unbroken line each, dropping to zero wherever no
 * request reached the origin
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

/** kept as the old name so the dashboard's RANGE_TICK_FORMAT map still reads right */
export type LatencyLabelFormat = AxisLabelFormat;

export interface LatencyBucket {
  time: string;
  /** every request in the bucket, including those that never left the gateway */
  count: number;
  /** 0 = nothing reached the origin, so both averages below are meaningless */
  sampleCount: number;
  /** durationMs — the whole round trip through the gateway */
  avgGatewayMs: number;
  /** upstreamDurationMs — the origin's share of it */
  avgOriginMs: number;
}

type SeriesKey = 'gateway' | 'origin';

const SERIES: Array<{ key: SeriesKey; label: string; color: string }> = [
  { key: 'gateway', label: 'Gateway', color: 'var(--color-series-1)' },
  { key: 'origin', label: 'Origin', color: 'var(--color-series-2)' },
];

function valueOf(b: LatencyBucket, key: SeriesKey): number {
  return key === 'origin' ? b.avgOriginMs : b.avgGatewayMs;
}

interface Pt {
  x: number;
  y: number;
}

const clampY = (y: number) => Math.min(Math.max(y, PLOT_T), PLOT_B);

/** Catmull-Rom as cubic beziers, control points clamped so the curve cannot bow past the axis */
function smoothPath(pts: Pt[]): string {
  if (pts.length === 0) return '';
  let d = `M ${round(pts[0].x)} ${round(pts[0].y)}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2] ?? p2;
    const c1x = p1.x + (p2.x - p0.x) / 6;
    const c1y = clampY(p1.y + (p2.y - p0.y) / 6);
    const c2x = p2.x - (p3.x - p1.x) / 6;
    const c2y = clampY(p2.y - (p3.y - p1.y) / 6);
    d += ` C ${round(c1x)} ${round(c1y)} ${round(c2x)} ${round(c2y)} ${round(p2.x)} ${round(p2.y)}`;
  }
  return d;
}

function areaPath(pts: Pt[]): string {
  if (pts.length < 2) return '';
  const last = pts[pts.length - 1];
  return `${smoothPath(pts)} L ${round(last.x)} ${PLOT_B} L ${round(pts[0].x)} ${PLOT_B} Z`;
}

/** Sits in the card head. Colour never travels without a word beside it. */
export function LatencyLegend() {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
      {SERIES.map((s) => (
        <span key={s.key} className="flex items-center gap-1.5 text-xs text-ink-3">
          <span
            aria-hidden="true"
            className="h-2.5 w-2.5 shrink-0 rounded-full"
            style={{ background: s.color }}
          />
          {s.label}
        </span>
      ))}
    </div>
  );
}

export function LatencyChart({
  buckets: allBuckets,
  labelFormat,
  stepMs,
}: {
  buckets: LatencyBucket[];
  labelFormat: LatencyLabelFormat;
  /** one bucket's width in ms, used to tell an adjacent bucket from a quiet gap */
  stepMs: number;
}) {
  const gradientId = React.useId();
  const [hovered, setHovered] = React.useState<number | null>(null);

  // Leading and trailing buckets with no origin call must not hold x-axis space, or the
  // chart opens and closes with a flat run along zero that says nothing.
  let firstSampled = 0;
  let lastSampled = allBuckets.length - 1;
  while (firstSampled <= lastSampled && allBuckets[firstSampled].sampleCount <= 0) firstSampled++;
  while (lastSampled >= firstSampled && allBuckets[lastSampled].sampleCount <= 0) lastSampled--;
  const buckets =
    firstSampled > lastSampled ? allBuckets : allBuckets.slice(firstSampled, lastSampled + 1);

  // Draw to whatever width the card gives us
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
  const t1 = times[times.length - 1];
  const span = t1 - t0;

  // x follows real time, not index — quiet periods produce no bucket at all, and an
  // index scale would silently squeeze them out of the timeline
  const xOf = (t: number) =>
    span > 0 ? PLOT_L + ((t - t0) / span) * (plotR - PLOT_L) : (PLOT_L + plotR) / 2;
  const xAt = (i: number) => xOf(times[i]);

  const sampled = buckets.filter((b) => b.sampleCount > 0);
  const peak = sampled.reduce((m, b) => Math.max(m, b.avgOriginMs, b.avgGatewayMs), 0);
  const ticks = niceTicks(peak);
  const yMax = ticks[STEPS];
  const yAt = (value: number) => PLOT_B - (value / yMax) * (PLOT_B - PLOT_T);

  // One continuous run of points per series. A bucket with no origin call already carries
  // zero for both averages, so it lands on the axis on its own.
  //
  // A quiet period produces no bucket at all, though, and left alone the line would sail
  // straight across it at whatever height its neighbours sit. Plant a zero at one step
  // inside each end of the gap so the line dips through it the same way.
  const pointsFor = (key: SeriesKey): Pt[] => {
    const pts: Pt[] = [];
    buckets.forEach((b, i) => {
      const prev = times[i - 1];
      if (prev !== undefined && times[i] - prev > stepMs) {
        pts.push({ x: xOf(prev + stepMs), y: yAt(0) });
        if (times[i] - stepMs > prev + stepMs) pts.push({ x: xOf(times[i] - stepMs), y: yAt(0) });
      }
      pts.push({ x: xAt(i), y: yAt(valueOf(b, key)) });
    });
    return pts;
  };

  const totalSamples = sampled.reduce((sum, b) => sum + b.sampleCount, 0);
  const weighted = (pick: (b: LatencyBucket) => number) =>
    totalSamples > 0
      ? sampled.reduce((sum, b) => sum + pick(b) * b.sampleCount, 0) / totalSamples
      : 0;
  const meanGateway = weighted((b) => b.avgGatewayMs);
  const meanOrigin = weighted((b) => b.avgOriginMs);

  // indexed, not held directly — a refetch can shrink the array under a stale index
  const hoveredBucket = hovered !== null ? buckets[hovered] : undefined;
  const hoverTopY = hoveredBucket
    ? Math.min(yAt(hoveredBucket.avgGatewayMs), yAt(hoveredBucket.avgOriginMs))
    : 0;
  // floats above the point, flipping below when the point sits too high for it
  const hoverAbove = hoverTopY > 96;

  const isSpan = labelFormat === 'dateRange';
  const marks = axisMarks(buckets.length, isSpan);

  return (
    <div>
      {/* y unit lives up here so the ticks stay bare numbers */}
      <p className="mb-1 text-xs text-muted">milliseconds</p>
      <div ref={wrapRef} className="relative" onMouseLeave={() => setHovered(null)}>
        <svg
          className="w-full"
          viewBox={`0 0 ${width} ${VIEW_H}`}
          role="img"
          aria-label={`Average gateway latency against average origin latency across ${buckets.length} buckets — a request takes ${meanGateway.toFixed(0)} milliseconds through the gateway on average, of which the origin accounts for ${meanOrigin.toFixed(0)} milliseconds.`}
        >
          <defs>
            {SERIES.map((s) => (
              <linearGradient key={s.key} id={`${gradientId}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={s.color} stopOpacity="0.16" />
                <stop offset="100%" stopColor={s.color} stopOpacity="0" />
              </linearGradient>
            ))}
          </defs>

          {ticks.map((t, i) => {
            const tickLabel = formatTickValue(t, ticks[1] || 1);
            return (
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
                  {tickLabel}
                </text>
                {/* verticals drawn once, hung off the first tick so they do not repeat per tick */}
                {i === 0 &&
                  marks.map((idx) => (
                    <line
                      key={idx}
                      x1={xAt(idx)}
                      y1={PLOT_T}
                      x2={xAt(idx)}
                      y2={PLOT_B}
                      stroke="var(--color-rule)"
                      strokeWidth="1"
                      strokeDasharray="4 4"
                    />
                  ))}
              </g>
            );
          })}

          {SERIES.map((s) => {
            const pts = pointsFor(s.key);
            if (pts.length === 1) {
              return (
                <circle
                  key={s.key}
                  cx={round(pts[0].x)}
                  cy={round(pts[0].y)}
                  r="2.5"
                  fill={s.color}
                />
              );
            }
            return (
              <g key={s.key}>
                <path d={areaPath(pts)} fill={`url(#${gradientId}-${s.key})`} />
                <path
                  d={smoothPath(pts)}
                  fill="none"
                  stroke={s.color}
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </g>
            );
          })}

          {marks.map((i, k) => {
            // a slot runs from its own left edge to just before the next one; the last reaches the end
            const spanEnd = (marks[k + 1] ?? buckets.length) - 1;
            const x = isSpan ? (xAt(i) + xAt(spanEnd)) / 2 : xAt(i);
            // first and last point labels anchor inward, or half of each spills outside the plot
            const anchor = isSpan
              ? 'middle'
              : k === 0
                ? 'start'
                : k === marks.length - 1
                  ? 'end'
                  : 'middle';
            return (
              <text
                className="tabular"
                key={buckets[i].time}
                // a centred label can overflow when the edge slot is narrow, so clamp it inside
                x={isSpan ? Math.min(Math.max(x, PLOT_L + 44), plotR - 44) : x}
                y={AXIS_Y}
                textAnchor={anchor}
                fontSize="11"
                fill="var(--color-muted)"
              >
                {isSpan
                  ? formatSpan(new Date(buckets[i].time), new Date(buckets[spanEnd].time))
                  : formatTick(new Date(buckets[i].time), labelFormat)}
              </text>
            );
          })}

          {/* shows which point on each line the tooltip figures come from */}
          {hoveredBucket && (
            <g>
              {SERIES.map((s) => (
                <circle
                  key={s.key}
                  cx={xAt(hovered as number)}
                  cy={yAt(valueOf(hoveredBucket, s.key))}
                  r="3.5"
                  fill="var(--color-paper)"
                  stroke={s.color}
                  strokeWidth="2"
                />
              ))}
            </g>
          )}

          {/* hit strips reach halfway to each neighbour; must render last to receive events */}
          {buckets.map((b, i) => {
            const left = i === 0 ? PLOT_L : (xAt(i - 1) + xAt(i)) / 2;
            const right = i === buckets.length - 1 ? plotR : (xAt(i) + xAt(i + 1)) / 2;
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
              // keep the ~190px box inside the plot by reserving half its width at each edge
              left: Math.min(Math.max(xAt(hovered as number), 95), Math.max(width - 95, 95)),
              top: hoverAbove ? hoverTopY - 12 : hoverTopY + 12,
              transform: hoverAbove ? 'translate(-50%, -100%)' : 'translate(-50%, 0)',
              boxShadow: 'var(--shadow-pop)',
            }}
          >
            <p className="text-muted">{formatStamp(new Date(hoveredBucket.time))}</p>
            {hoveredBucket.sampleCount > 0 ? (
              <>
                <p className="tabular mt-1 font-medium" style={{ color: 'var(--color-series-1)' }}>
                  Avg Gateway Latency: {hoveredBucket.avgGatewayMs.toFixed(0)} ms
                </p>
                <p className="tabular font-medium" style={{ color: 'var(--color-series-2)' }}>
                  Avg Origin Latency: {hoveredBucket.avgOriginMs.toFixed(0)} ms
                </p>
              </>
            ) : (
              <p className="mt-1 text-ink-3">No request reached the origin</p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

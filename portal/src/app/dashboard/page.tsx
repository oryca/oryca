/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import { LatencyChart, LatencyLegend, type LatencyLabelFormat } from '@/components/LatencyChart';
import { TrafficChart, type TrafficBucket } from '@/components/TrafficChart';
import {
  bucketLatency,
  latencyInterval,
  truncateTo,
  STEP_MS,
  type LatencyLog,
} from './latencyBuckets';
import {
  Button,
  PageHeader,
  SectionCard,
  EmptyState,
  SkeletonLine,
  Loading,
  useToast,
} from '@/components/ui';
import { ArrowRight, ChevronDown, RefreshCw, Server, Activity } from 'lucide-react';

interface DashboardBucket {
  time: string;
  count: number;
  avgTimeMs: number;
  status2xx: number;
  status4xx: number;
  status5xx: number;
}

interface DashboardStats {
  interval: string;
  buckets: DashboardBucket[];
}

interface DashboardServiceUsage {
  serviceId: string;
  name?: string;
  count: number;
}

interface DashboardSummary {
  total: number;
  avgTimeMs: number;
  status2xx: number;
  status4xx: number;
  status5xx: number;
  topServices: DashboardServiceUsage[];
}

interface AccessLog {
  id: string;
  time: string;
  traceId?: string;
  userId?: string;
  apiKeyId?: string;
  serviceId?: string;
  method?: string;
  path?: string;
  host?: string;
  ip?: string;
  statusCode: number;
  size?: number;
  durationMs: number;
  packageId?: string;
  cacheStatus?: string;
  upstreamStatus?: number;
  upstreamDurationMs?: number;
  authMs?: number;
  rateLimitMs?: number;
  cacheCheckMs?: number;
  singleflightMs?: number;
  bodyReadMs?: number;
  postUpstreamMs?: number;
}

interface AccessLogPage {
  data: AccessLog[];
  nextCursor?: string;
}

interface ServiceOption {
  id: string;
  name: string;
  basePath: string;
}

interface UserOption {
  id: string;
  email: string;
  role: string;
}

/** /dashboard/logs caps at 100 per page, so the chart walks the cursor. Five pages =
 *  the 500 newest requests in range; a busier window shows only its tail. */
const LATENCY_PAGE_SIZE = 100;
const LATENCY_MAX_PAGES = 5;

type Range = '1h' | '24h' | '7d' | '30d';
const RANGES: Range[] = ['1h', '24h', '7d', '30d'];

const RANGE_TICK_FORMAT: Record<Range, LatencyLabelFormat> = {
  '1h': 'time',
  '24h': 'time',
  '7d': 'date',
  '30d': 'dateRange',
};

/** The service buckets anything up to 7 days by hour, so a week arrives as 168 bars while
 *  its axis is labelled by date. Fold those into days for the chart instead of changing
 *  the interval the endpoint picks, which the latency chart and its callers still follow. */
function rollUpToDays(buckets: TrafficBucket[]): TrafficBucket[] {
  const days = new Map<number, number>();
  for (const bucket of buckets) {
    const key = truncateTo(new Date(bucket.time), 'day');
    days.set(key, (days.get(key) ?? 0) + bucket.count);
  }
  return [...days.entries()]
    .sort(([a], [b]) => a - b)
    .map(([key, count]) => ({ time: new Date(key).toISOString(), count }));
}

const RANGE_LABEL: Record<Range, string> = {
  '1h': 'Last hour',
  '24h': 'Last 24h',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
};

function rangeWindow(range: Range) {
  const to = new Date();
  const from = new Date(to);
  if (range === '1h') from.setHours(to.getHours() - 1);
  else if (range === '24h') from.setHours(to.getHours() - 24);
  else if (range === '7d') from.setDate(to.getDate() - 7);
  else from.setDate(to.getDate() - 30);
  return { from: from.toISOString(), to: to.toISOString() };
}

function statusTone(code: number) {
  if (code >= 500) return 'border-danger-edge bg-danger-wash text-danger';
  if (code >= 400) return 'border-warn-edge bg-warn-wash text-warn';
  return 'border-ok-edge bg-ok-wash text-ok';
}

function cacheTone(status?: string) {
  if (status === 'HIT') return 'border-ok-edge bg-ok-wash text-ok';
  if (status === 'MISS') return 'border-rule bg-paper-2 text-muted';
  return 'border-rule bg-paper-2 text-ink-3';
}

/** เวลาที่ gateway เพิ่มเข้ามาเอง คือเวลารวมลบเวลาที่ upstream ใช้
 *  คืน null เมื่อไม่ได้ยิง upstream เช่น cache HIT หรือถูกปฏิเสธที่ auth */
function gatewayOverhead(log: AccessLog): number | null {
  if (!log.upstreamDurationMs && log.upstreamStatus === undefined) return null;
  const overhead = log.durationMs - (log.upstreamDurationMs ?? 0);
  return overhead < 0 ? 0 : overhead;
}

/** เวลาที่ทำให้ตอบช้าอยู่ขั้นไหนของ gateway ใช้ตอน hover ดูรายละเอียด */
function timingBreakdown(log: AccessLog): string {
  const steps: Array<[string, number | undefined]> = [
    ['auth', log.authMs],
    ['rate limit', log.rateLimitMs],
    ['cache', log.cacheCheckMs],
    ['coalesce', log.singleflightMs],
    ['read body', log.bodyReadMs],
    ['transform', log.postUpstreamMs],
  ];
  const shown = steps.filter(([, v]) => v !== undefined);
  if (shown.length === 0) return 'No timing recorded for this request';
  return shown.map(([label, v]) => `${label} ${v ?? 0} ms`).join(' · ');
}

function StatTile({
  label,
  value,
  note,
  tone = 'ink',
}: {
  label: string;
  value: string;
  note: string;
  tone?: 'ink' | 'ok' | 'warn' | 'danger';
}) {
  const valueColor = {
    ink: 'text-ink',
    ok: 'text-ok',
    warn: 'text-warn',
    danger: 'text-danger',
  }[tone];

  return (
    <div className="ui-card p-4">
      <p className="text-xs text-muted">{label}</p>
      <p className={`tabular mt-1 font-title text-xl font-semibold ${valueColor}`}>{value}</p>
      <p className="mt-1 text-xs text-muted">{note}</p>
    </div>
  );
}

export default function DashboardPage() {
  const { user } = useAuth();
  const { error: toastError } = useToast();
  const isAdmin = user?.role === 'admin' || user?.role === 'root';

  const [range, setRange] = useState<Range>('24h');
  const [selectedServiceId, setSelectedServiceId] = useState('');
  const [selectedUserId, setSelectedUserId] = useState('');
  const [isRefreshing, setIsRefreshing] = useState(false);

  // หน้าถัดๆ ไปของ log เก็บแยกจาก query ของหน้าแรก
  const [extraLogs, setExtraLogs] = useState<{ rows: AccessLog[]; cursor?: string } | null>(null);
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // ตรึงช่วงเวลาไว้กับ range ไม่ให้ขยับทุก render
  const { from: fromTime, to: toTime } = useMemo(() => rangeWindow(range), [range]);

  const filterKey = `${range}|${selectedServiceId}|${selectedUserId}`;
  const [syncedFilter, setSyncedFilter] = useState(filterKey);
  if (filterKey !== syncedFilter) {
    // เปลี่ยนตัวกรองแล้วต้องทิ้งหน้าที่โหลดเพิ่มไว้ ไม่งั้นข้อมูลคนละชุดจะปนกัน
    setSyncedFilter(filterKey);
    setExtraLogs(null);
  }

  const scopedParams = useMemo(() => {
    const params: Record<string, string> = { from: fromTime, to: toTime };
    if (selectedServiceId) params.serviceId = selectedServiceId;
    if (selectedUserId && isAdmin) params.userId = selectedUserId;
    return params;
  }, [fromTime, toTime, selectedServiceId, selectedUserId, isAdmin]);

  const { data: services } = useQuery<ServiceOption[]>({
    queryKey: ['services-list'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  const { data: users } = useQuery<UserOption[]>({
    queryKey: ['users-list'],
    queryFn: async () => {
      const res = await api.get('/users');
      return res.data.items || [];
    },
    enabled: isAdmin,
  });

  const {
    data: summary,
    isLoading: isLoadingSummary,
    refetch: refetchSummary,
  } = useQuery<DashboardSummary>({
    queryKey: ['summary', filterKey],
    queryFn: async () => (await api.get('/dashboard/summary', { params: scopedParams })).data,
  });

  const {
    data: stats,
    isLoading: isLoadingStats,
    refetch: refetchStats,
  } = useQuery<DashboardStats>({
    queryKey: ['stats', filterKey],
    queryFn: async () => (await api.get('/dashboard/stats', { params: scopedParams })).data,
  });

  const {
    data: firstLogPage,
    isLoading: isLoadingLogs,
    refetch: refetchLogs,
  } = useQuery<AccessLogPage>({
    queryKey: ['logs', filterKey],
    queryFn: async () =>
      (await api.get<AccessLogPage>('/dashboard/logs', { params: { ...scopedParams, limit: 15 } }))
        .data,
  });

  // The chart needs upstreamDurationMs, which only exists on the per-request log —
  // /dashboard/stats gives just avgTimeMs, so sweep the logs and aggregate here.
  // `truncated` says the sweep hit its page cap with more rows still behind the cursor.
  const {
    data: latencySweep,
    isLoading: isLoadingLatency,
    isError: isLatencyError,
    refetch: refetchLatency,
  } = useQuery<{ rows: LatencyLog[]; truncated: boolean }>({
    queryKey: ['latency-logs', filterKey],
    queryFn: async () => {
      const rows: LatencyLog[] = [];
      let cursor: string | undefined;
      for (let page = 0; page < LATENCY_MAX_PAGES; page++) {
        const res = await api.get<AccessLogPage>('/dashboard/logs', {
          params: { ...scopedParams, limit: LATENCY_PAGE_SIZE, ...(cursor ? { cursor } : {}) },
        });
        rows.push(...(res.data.data ?? []));
        cursor = res.data.nextCursor;
        if (!cursor) break;
      }
      return { rows, truncated: !!cursor };
    },
    // the card is hidden under "all services", so skip five pages of wasted fetching
    enabled: !!selectedServiceId,
  });

  const logs = [...(firstLogPage?.data ?? []), ...(extraLogs?.rows ?? [])];
  const nextCursor = extraLogs ? extraLogs.cursor : firstLogPage?.nextCursor;

  async function handleRefresh() {
    setIsRefreshing(true);
    setExtraLogs(null);
    try {
      await Promise.all([refetchSummary(), refetchStats(), refetchLogs(), refetchLatency()]);
    } finally {
      setIsRefreshing(false);
    }
  }

  async function handleLoadMore() {
    if (!nextCursor || isLoadingMore) return;
    setIsLoadingMore(true);
    try {
      const res = await api.get<AccessLogPage>('/dashboard/logs', {
        params: { ...scopedParams, limit: 15, cursor: nextCursor },
      });
      setExtraLogs((prev) => ({
        rows: [...(prev?.rows ?? []), ...(res.data.data ?? [])],
        cursor: res.data.nextCursor,
      }));
    } catch {
      toastError('Could not load more logs', handleLoadMore);
    } finally {
      setIsLoadingMore(false);
    }
  }

  const buckets = stats?.buckets ?? [];
  // 7d is the one range where what the endpoint returns is finer than the axis reads
  const rollUp = range === '7d' && (stats?.interval ?? 'hour') === 'hour';
  const trafficInterval = rollUp ? 'day' : (stats?.interval ?? 'hour');
  const trafficBuckets = useMemo<TrafficBucket[]>(
    () => (rollUp ? rollUpToDays(buckets) : buckets),
    [buckets, rollUp],
  );

  // bucket size follows the width of the selected window
  const latencyBucketInterval = latencyInterval(Date.parse(fromTime), Date.parse(toTime));
  const latencyLogs = latencySweep?.rows;
  const latencyBuckets = useMemo(
    () => bucketLatency(latencyLogs ?? [], latencyBucketInterval),
    [latencyLogs, latencyBucketInterval],
  );
  // every bucket cached = nothing to compare against, which is not the same as no traffic
  const hasUpstreamSamples = latencyBuckets.some((b) => b.sampleCount > 0);
  // the sweep walks newest first, so a truncated result covers only the end of the window
  const latencyCoverage = latencySweep?.truncated
    ? `Showing the ${latencyLogs?.length.toLocaleString()} most recent requests in this window - older ones are not included.`
    : null;

  return (
    <NavigationShell>
      <PageHeader
        title="Gateway Dashboard"
        description={
          user?.role === 'user'
            ? 'Usage and request logs for your own API keys'
            : 'Request volume, latency and response codes across the whole gateway'
        }
        actions={
          <>
            <div
              role="group"
              aria-label="Time range"
              className="flex h-10 overflow-hidden rounded-control border border-rule bg-paper"
            >
              {RANGES.map((r) => (
                <button
                  key={r}
                  type="button"
                  aria-pressed={range === r}
                  onClick={() => setRange(r)}
                  className={`px-3 text-xs font-medium whitespace-nowrap transition-colors duration-200 ${
                    range === r
                      ? 'bg-accent-wash text-accent'
                      : 'text-ink-3 hover:bg-paper-2 hover:text-ink'
                  }`}
                >
                  {RANGE_LABEL[r]}
                </button>
              ))}
            </div>

            <Button
              onClick={handleRefresh}
              loading={isRefreshing}
              aria-label="Refresh"
              icon={<RefreshCw className="h-4 w-4" />}
            />
          </>
        }
      />

      <SectionCard className="mb-6" title="Filters">
        <div className="flex flex-wrap gap-4">
          <label className="min-w-0 flex-1 basis-56">
            <span className="ui-field__label">Service</span>
            <div className="relative">
              <select
                className="ui-input appearance-none"
                value={selectedServiceId}
                onChange={(e) => setSelectedServiceId(e.target.value)}
              >
                <option value="">All services</option>
                {services?.map((svc) => (
                  <option key={svc.id} value={svc.id}>
                    {svc.name} ({svc.basePath})
                  </option>
                ))}
              </select>
              <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
            </div>
          </label>

          {isAdmin && (
            <label className="min-w-0 flex-1 basis-56">
              <span className="ui-field__label">Consumer</span>
              <div className="relative">
                <select
                  className="ui-input appearance-none"
                  value={selectedUserId}
                  onChange={(e) => setSelectedUserId(e.target.value)}
                >
                  <option value="">Everyone</option>
                  {users?.map((u) => (
                    <option key={u.id} value={u.id}>
                      {u.email} ({u.role})
                    </option>
                  ))}
                </select>
                <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
              </div>
            </label>
          )}
        </div>
      </SectionCard>

      {/* ---- ตัวเลขสรุป ---- */}
      {isLoadingSummary && (
        <Loading label="Loading summary">
          <div className="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-5">
            {[0, 1, 2, 3, 4].map((i) => (
              <div key={i} className="ui-card p-4">
                <SkeletonLine width="60%" />
                <SkeletonLine width="45%" className="mt-2 h-5" />
                <SkeletonLine width="80%" className="mt-2" />
              </div>
            ))}
          </div>
        </Loading>
      )}

      {!isLoadingSummary && !summary && (
        <div className="ui-card mb-6">
          <EmptyState
            icon={<Activity className="h-5 w-5" />}
            title="Could not load the summary"
            description="Try again. If it keeps failing, the control plane may not be responding."
            action={
              <Button variant="secondary" onClick={handleRefresh} loading={isRefreshing}>
                Refresh
              </Button>
            }
          />
        </div>
      )}

      {!isLoadingSummary && summary && (
        <div className="ui-fade mb-6 grid grid-cols-2 gap-4 lg:grid-cols-5">
          <StatTile
            label="Requests"
            value={summary.total.toLocaleString()}
            note="in the selected range"
          />
          <StatTile
            label="Average latency"
            value={`${summary.avgTimeMs.toFixed(1)} ms`}
            note="round trip per request"
          />
          <StatTile
            label="Success rate"
            value={`${summary.total > 0 ? ((summary.status2xx / summary.total) * 100).toFixed(1) : '100.0'}%`}
            note={`${summary.status2xx.toLocaleString()} responses in 2xx`}
            tone="ok"
          />
          <StatTile
            label="Client errors"
            value={summary.status4xx.toLocaleString()}
            note="responses in 4xx"
            tone="warn"
          />
          <StatTile
            label="Server errors"
            value={summary.status5xx.toLocaleString()}
            note="responses in 5xx"
            tone="danger"
          />
        </div>
      )}

      {/* ---- กราฟ + service ที่ถูกเรียกมากสุด ---- */}
      <div className="mb-6 grid grid-cols-1 gap-6 lg:grid-cols-3">
        <SectionCard
          className="lg:col-span-2"
          title="Requests over time"
        >
          {isLoadingStats && (
            <Loading label="Loading chart">
              <div className="ui-skeleton h-56 w-full" />
            </Loading>
          )}
          {!isLoadingStats && buckets.length === 0 && (
            <EmptyState
              title="Nothing in this range"
              description="Widen the time range, or clear the service filter."
            />
          )}
          {!isLoadingStats && trafficBuckets.length > 0 && (
            <TrafficChart
              buckets={trafficBuckets}
              interval={trafficInterval}
              stepMs={STEP_MS[trafficInterval] ?? STEP_MS.hour}
              labelFormat={RANGE_TICK_FORMAT[range]}
            />
          )}
        </SectionCard>

        <SectionCard title="Busiest services">
          {isLoadingSummary && (
            <Loading label="Loading service ranking">
              <div className="space-y-3">
                {[0, 1, 2].map((i) => (
                  <SkeletonLine key={i} className="h-8" />
                ))}
              </div>
            </Loading>
          )}
          {!isLoadingSummary && (!summary?.topServices || summary.topServices.length === 0) && (
            <EmptyState
              icon={<Server className="h-5 w-5" />}
              title="No traffic yet"
              description="Once requests flow through the gateway, the ranking shows up here."
            />
          )}
          {!isLoadingSummary && summary?.topServices && summary.topServices.length > 0 && (
            <ul className="ui-fade space-y-2">
              {summary.topServices.map((svc) => (
                <li
                  key={svc.serviceId}
                  className="flex items-center justify-between gap-2 rounded-control border border-rule bg-paper-2 p-2"
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <Server className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                    <div className="min-w-0">
                      <p className="truncate text-sm text-ink">{svc.name || 'Unnamed service'}</p>
                      <p className="truncate font-mono text-xs text-muted">
                        {svc.serviceId.substring(0, 8)}
                      </p>
                    </div>
                  </div>
                  <span className="tabular shrink-0 rounded-chip border border-accent-edge bg-accent-wash px-2 py-0.5 text-xs font-medium whitespace-nowrap text-accent">
                    {svc.count.toLocaleString()}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </SectionCard>
      </div>

      {/* ---- Where the round trip goes: the gateway or the origin ----
        * Only for a single service. Every service has its own origin at its own speed,
        * so averaging them together yields a middling number that belongs to no one. */}
      {selectedServiceId && (
        <SectionCard
          className="mb-6"
          title="Average Latency"
          actions={<LatencyLegend />}
        >
          {isLoadingLatency && (
            <Loading label="Loading latency chart">
              <div className="ui-skeleton h-[280px] w-full" />
            </Loading>
          )}
          {!isLoadingLatency && isLatencyError && (
            <EmptyState
              icon={<Activity className="h-5 w-5" />}
              title="Could not load latency"
              description="The request log did not come back, so there is nothing to chart yet."
              action={
                <button
                  onClick={() => refetchLatency()}
                  className="rounded-control border border-rule px-3 py-1.5 text-xs font-medium text-ink-2 transition hover:bg-paper-2"
                >
                  Try again
                </button>
              }
            />
          )}
          {!isLoadingLatency && !isLatencyError && latencyBuckets.length === 0 && (
            <EmptyState
              title="Nothing in this range"
              description="This service saw no traffic in the selected window. Widen the time range."
            />
          )}
          {!isLoadingLatency && !isLatencyError && latencyBuckets.length > 0 && !hasUpstreamSamples && (
            <EmptyState
              icon={<Activity className="h-5 w-5" />}
              title="No origin latency in this range"
            />
          )}
          {!isLoadingLatency && !isLatencyError && latencyBuckets.length > 0 && hasUpstreamSamples && (
            <>
              <LatencyChart
                buckets={latencyBuckets}
                labelFormat={RANGE_TICK_FORMAT[range]}
                stepMs={STEP_MS[latencyBucketInterval]}
              />
              {latencyCoverage && <p className="mt-2 text-xs text-muted">{latencyCoverage}</p>}
            </>
          )}
        </SectionCard>
      )}

      {/* ---- log ดิบ ---- */}
      <SectionCard title="Recent requests" flush>
        {isLoadingLogs && logs.length === 0 && (
          <Loading label="Loading recent requests">
            <div className="space-y-2 p-4">
              {[0, 1, 2, 3, 4].map((i) => (
                <SkeletonLine key={i} className="h-6" />
              ))}
            </div>
          </Loading>
        )}

        {!isLoadingLogs && logs.length === 0 && (
          <EmptyState
            title="No requests match these filters"
            description="Widen the time range, or clear the service and consumer filters."
          />
        )}

        {logs.length > 0 && (
          <>
            <div className="ui-scroll-x">
              <table className="w-full border-collapse text-left text-sm">
                <thead>
                  <tr className="border-b border-rule text-xs text-muted">
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Status</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Method</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Path</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Total</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Upstream</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Gateway</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Cache</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">IP</th>
                    <th className="px-3 py-2 font-medium whitespace-nowrap">Time</th>
                  </tr>
                </thead>
                <tbody className="tabular divide-y divide-rule font-mono text-xs">
                  {logs.map((log) => (
                    <tr key={log.id}>
                      <td className="px-3 py-2.5">
                        <span
                          className={`rounded-chip border px-2 py-0.5 font-medium ${statusTone(log.statusCode)}`}
                        >
                          {log.statusCode}
                        </span>
                      </td>
                      <td className="px-3 py-2.5 font-medium whitespace-nowrap text-ink">
                        {log.method || 'GET'}
                      </td>
                      <td className="max-w-xs truncate px-3 py-2.5 text-ink-3" title={log.path}>
                        {log.path || '/'}
                      </td>
                      <td className="px-3 py-2.5 font-medium whitespace-nowrap text-ink">
                        {log.durationMs} ms
                      </td>
                      <td className="px-3 py-2.5 whitespace-nowrap text-muted">
                        {log.upstreamDurationMs !== undefined ? `${log.upstreamDurationMs} ms` : '-'}
                      </td>
                      <td
                        className="px-3 py-2.5 whitespace-nowrap text-muted"
                        title={timingBreakdown(log)}
                      >
                        {gatewayOverhead(log) !== null ? `${gatewayOverhead(log)} ms` : '-'}
                      </td>
                      <td className="px-3 py-2.5 whitespace-nowrap">
                        {log.cacheStatus ? (
                          <span
                            className={`rounded-chip border px-2 py-0.5 text-[11px] font-medium ${cacheTone(log.cacheStatus)}`}
                          >
                            {log.cacheStatus}
                          </span>
                        ) : (
                          <span className="text-muted">-</span>
                        )}
                      </td>
                      <td className="px-3 py-2.5 whitespace-nowrap text-muted">{log.ip || '-'}</td>
                      <td className="px-3 py-2.5 whitespace-nowrap text-muted">
                        {new Date(log.time).toLocaleTimeString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {nextCursor && (
              <div className="flex justify-center border-t border-rule p-4">
                <Button
                  onClick={handleLoadMore}
                  loading={isLoadingMore}
                  icon={<ArrowRight className="h-4 w-4" />}
                >
                  Load more
                </Button>
              </div>
            )}
          </>
        )}
      </SectionCard>
    </NavigationShell>
  );
}

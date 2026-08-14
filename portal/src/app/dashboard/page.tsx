'use client';

import React, { useState, useEffect } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import {
  Activity,
  ArrowRight,
  ChevronDown,
  Clock,
  Filter,
  RefreshCw,
  Server,
  User,
  AlertTriangle,
  FileText,
  Search,
  Database
} from 'lucide-react';

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
}

interface AccessLogPage {
  data: AccessLog[];
  nextCursor?: string;
}

export default function DashboardPage() {
  const { user } = useAuth();
  
  // Filters
  const [range, setRange] = useState<'1h' | '24h' | '7d' | '30d'>('24h');
  const [selectedServiceId, setSelectedServiceId] = useState<string>('');
  const [selectedUserId, setSelectedUserId] = useState<string>('');
  const [isRefreshing, setIsRefreshing] = useState(false);

  // Raw logs pagination state
  const [logs, setLogs] = useState<AccessLog[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>(undefined);
  const [isLoadingMoreLogs, setIsLoadingMoreLogs] = useState(false);

  // Time calculate helpers
  const getRangeTimes = () => {
    const to = new Date();
    const from = new Date();
    if (range === '1h') {
      from.setHours(to.getHours() - 1);
    } else if (range === '24h') {
      from.setHours(to.getHours() - 24);
    } else if (range === '7d') {
      from.setDate(to.getDate() - 7);
    } else if (range === '30d') {
      from.setDate(to.getDate() - 30);
    }
    return {
      from: from.toISOString(),
      to: to.toISOString(),
    };
  };

  const { from: fromTime, to: toTime } = getRangeTimes();

  // Fetch Services list for dropdown
  const { data: services } = useQuery<any[]>({
    queryKey: ['services-list'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  // Fetch Users list (only if admin/root)
  const { data: users } = useQuery<any[]>({
    queryKey: ['users-list'],
    queryFn: async () => {
      const res = await api.get('/users');
      return res.data.items || [];
    },
    enabled: user?.role === 'admin' || user?.role === 'root',
  });

  // Fetch Dashboard Summary
  const { data: summary, isLoading: isLoadingSummary, refetch: refetchSummary } = useQuery<DashboardSummary>({
    queryKey: ['summary', range, selectedServiceId, selectedUserId],
    queryFn: async () => {
      const params: any = { from: fromTime, to: toTime };
      if (selectedServiceId) params.serviceId = selectedServiceId;
      if (selectedUserId && (user?.role === 'admin' || user?.role === 'root')) {
        params.userId = selectedUserId;
      }
      const res = await api.get('/dashboard/summary', { params });
      return res.data;
    },
  });

  // Fetch Dashboard Stats for Chart
  const { data: stats, isLoading: isLoadingStats, refetch: refetchStats } = useQuery<DashboardStats>({
    queryKey: ['stats', range, selectedServiceId, selectedUserId],
    queryFn: async () => {
      const params: any = { from: fromTime, to: toTime };
      if (selectedServiceId) params.serviceId = selectedServiceId;
      if (selectedUserId && (user?.role === 'admin' || user?.role === 'root')) {
        params.userId = selectedUserId;
      }
      const res = await api.get('/dashboard/stats', { params });
      return res.data;
    },
  });

  // Fetch Initial Raw Logs
  const fetchLogs = async (cursor?: string) => {
    const params: any = { from: fromTime, to: toTime, limit: 15 };
    if (selectedServiceId) params.serviceId = selectedServiceId;
    if (selectedUserId && (user?.role === 'admin' || user?.role === 'root')) {
      params.userId = selectedUserId;
    }
    if (cursor) params.cursor = cursor;
    
    const res = await api.get<AccessLogPage>('/dashboard/logs', { params });
    return res.data;
  };

  const { data: initialLogsData, isLoading: isLoadingLogs, refetch: refetchLogsInitial } = useQuery<AccessLogPage>({
    queryKey: ['logs-initial', range, selectedServiceId, selectedUserId],
    queryFn: () => fetchLogs(),
  });

  useEffect(() => {
    if (initialLogsData) {
      setLogs(initialLogsData.data || []);
      setNextCursor(initialLogsData.nextCursor);
    }
  }, [initialLogsData]);

  // Refetch helper
  const handleRefresh = async () => {
    setIsRefreshing(true);
    await Promise.all([
      refetchSummary(),
      refetchStats(),
      refetchLogsInitial(),
    ]);
    setIsRefreshing(false);
  };

  // Load More Logs
  const handleLoadMore = async () => {
    if (!nextCursor || isLoadingMoreLogs) return;
    setIsLoadingMoreLogs(true);
    try {
      const res = await fetchLogs(nextCursor);
      setLogs((prev) => [...prev, ...(res.data || [])]);
      setNextCursor(res.nextCursor);
    } catch (err) {
      console.error(err);
    } finally {
      setIsLoadingMoreLogs(false);
    }
  };

  // Trigger load logs on query changes
  useEffect(() => {
    fetchLogs().then((data) => {
      setLogs(data.data || []);
      setNextCursor(data.nextCursor);
    }).catch(console.error);
  }, [range, selectedServiceId, selectedUserId]);

  // Chart Draw Variables
  const buckets = stats?.buckets || [];
  const maxCount = Math.max(...buckets.map((b) => b.count), 1);
  const intervalLabel = stats?.interval || 'hour';

  return (
    <NavigationShell>
      <div className="space-y-6">
        {/* Top Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <h1 className="font-title text-2xl font-bold tracking-tight text-ink">
              Gateway Dashboard
            </h1>
            <p className="text-sm text-muted">
              {user?.role === 'user'
                ? 'Usage logs and traffic statistics for your API keys'
                : 'Global gateway requests volume, latency, and status diagnostics'}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {/* Range Select */}
            <div className="flex border border-rule rounded-control overflow-hidden bg-paper">
              {(['1h', '24h', '7d', '30d'] as const).map((r) => (
                <button
                  key={r}
                  onClick={() => setRange(r)}
                  className={`px-3 py-1.5 text-xs font-semibold uppercase transition ${
                    range === r ? 'bg-accent-wash border-r border-accent-edge text-accent' : 'text-muted hover:text-ink hover:bg-paper-2'
                  }`}
                >
                  {r}
                </button>
              ))}
            </div>

            {/* Refresh Button */}
            <button
              onClick={handleRefresh}
              disabled={isRefreshing}
              className="p-2 bg-paper border border-rule hover:border-faint rounded-control text-muted hover:text-ink transition duration-short flex items-center justify-center disabled:opacity-50"
              title="Refresh data"
            >
              <RefreshCw className={`w-4 h-4 ${isRefreshing ? 'animate-spin' : ''}`} />
            </button>
          </div>
        </div>

        {/* Diagnostic Filters Dropdowns */}
        <div className="bg-paper p-4 border border-rule rounded-surface flex flex-wrap gap-4 items-center shadow-sm">
          <div className="flex items-center gap-2 text-xs font-bold text-muted uppercase tracking-wider">
            <Filter className="w-3.5 h-3.5" /> Filter:
          </div>

          {/* Service filter */}
          <div className="flex-1 min-w-[200px] max-w-xs relative">
            <select
              value={selectedServiceId}
              onChange={(e) => setSelectedServiceId(e.target.value)}
              className="w-full bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none appearance-none focus:border-focus"
            >
              <option value="">All Services</option>
              {services?.map((svc) => (
                <option key={svc.id} value={svc.id}>
                  {svc.name} ({svc.basePath})
                </option>
              ))}
            </select>
            <ChevronDown className="w-3.5 h-3.5 text-muted absolute right-3 top-2.5 pointer-events-none" />
          </div>

          {/* User filter (Admin only) */}
          {(user?.role === 'admin' || user?.role === 'root') && (
            <div className="flex-1 min-w-[200px] max-w-xs relative">
              <select
                value={selectedUserId}
                onChange={(e) => setSelectedUserId(e.target.value)}
                className="w-full bg-paper-2 border border-rule rounded-control px-3 py-1.5 text-xs text-ink outline-none appearance-none focus:border-focus"
              >
                <option value="">All Consumers</option>
                {users?.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.email} ({u.role})
                  </option>
                ))}
              </select>
              <ChevronDown className="w-3.5 h-3.5 text-muted absolute right-3 top-2.5 pointer-events-none" />
            </div>
          )}
        </div>

        {/* Summary KPI Cards */}
        {isLoadingSummary ? (
          <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
            {[...Array(5)].map((_, i) => (
              <div key={i} className="bg-paper border border-rule rounded-surface p-4 h-24 animate-pulse"></div>
            ))}
          </div>
        ) : !summary ? (
          <div className="bg-paper border border-rule rounded-surface p-8 text-center text-muted">
            Failed to load request summaries.
          </div>
        ) : (
          <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
            <div className="bg-paper border border-rule rounded-surface p-4 relative overflow-hidden shadow-sm">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted">Total Requests</span>
              <p className="font-title text-2xl font-bold tracking-tight text-ink mt-1">
                {summary.total.toLocaleString()}
              </p>
              <span className="text-[10px] text-muted block mt-1">requests matched</span>
              <div className="absolute bottom-0 left-0 right-0 h-1 bg-accent"></div>
            </div>

            <div className="bg-paper border border-rule rounded-surface p-4 relative overflow-hidden shadow-sm">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted">Avg Latency</span>
              <p className="font-title text-2xl font-bold tracking-tight text-ink mt-1">
                {summary.avgTimeMs.toFixed(1)} ms
              </p>
              <span className="text-[10px] text-muted block mt-1">average roundtrip speed</span>
              <div className="absolute bottom-0 left-0 right-0 h-1 bg-ok"></div>
            </div>

            <div className="bg-paper border border-rule rounded-surface p-4 relative overflow-hidden shadow-sm">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted">Success Rate</span>
              <p className="font-title text-2xl font-bold tracking-tight text-ok mt-1">
                {summary.total > 0 ? ((summary.status2xx / summary.total) * 100).toFixed(1) : '100.0'}%
              </p>
              <span className="text-[10px] text-muted block mt-1">{summary.status2xx} responses (2xx)</span>
              <div className="absolute bottom-0 left-0 right-0 h-1 bg-ok"></div>
            </div>

            <div className="bg-paper border border-rule rounded-surface p-4 relative overflow-hidden shadow-sm">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted">Client Errors</span>
              <p className="font-title text-2xl font-bold tracking-tight text-warn mt-1">
                {summary.status4xx.toLocaleString()}
              </p>
              <span className="text-[10px] text-muted block mt-1">responses (4xx)</span>
              <div className="absolute bottom-0 left-0 right-0 h-1 bg-warn"></div>
            </div>

            <div className="bg-paper border border-rule rounded-surface p-4 relative overflow-hidden shadow-sm">
              <span className="text-[10px] uppercase font-bold tracking-wider text-muted">Server Errors</span>
              <p className="font-title text-2xl font-bold tracking-tight text-danger mt-1">
                {summary.status5xx.toLocaleString()}
              </p>
              <span className="text-[10px] text-muted block mt-1">responses (5xx)</span>
              <div className="absolute bottom-0 left-0 right-0 h-1 bg-danger"></div>
            </div>
          </div>
        )}

        {/* Charts & Top Services */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* SVG Custom Chart */}
          <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm lg:col-span-2">
            <div className="mb-4">
              <h2 className="font-title text-sm font-semibold text-ink">
                Traffic Over Time
              </h2>
              <p className="text-[11px] text-muted mt-0.5">
                Aggregated in {intervalLabel} intervals
              </p>
            </div>

            {isLoadingStats ? (
              <div className="h-64 bg-paper-2 rounded-control animate-pulse flex items-center justify-center text-xs text-muted">
                Generating visual series...
              </div>
            ) : buckets.length === 0 ? (
              <div className="h-64 flex items-center justify-center text-xs text-muted border border-dashed border-rule rounded-control">
                No telemetry data for this range.
              </div>
            ) : (
              <div className="relative">
                {/* SVG Visual Representation */}
                <div className="h-56 w-full flex items-end">
                  <svg className="w-full h-full" viewBox="0 0 100 100" preserveAspectRatio="none">
                    {/* Grid Lines */}
                    <line x1="0" y1="25" x2="100" y2="25" stroke="var(--color-rule)" strokeWidth="0.25" strokeDasharray="2" />
                    <line x1="0" y1="50" x2="100" y2="50" stroke="var(--color-rule)" strokeWidth="0.25" strokeDasharray="2" />
                    <line x1="0" y1="75" x2="100" y2="75" stroke="var(--color-rule)" strokeWidth="0.25" strokeDasharray="2" />
                    
                    {/* Render Bar Chart columns for high visual accuracy */}
                    {buckets.map((b, idx) => {
                      const width = 100 / buckets.length;
                      const x = idx * width;
                      const height = (b.count / maxCount) * 80; // Scale to 80% max height
                      const y = 100 - height;
                      return (
                        <g key={idx} className="group cursor-pointer">
                          <rect
                            x={x + 0.5}
                            y={y}
                            width={width - 1}
                            height={height}
                            fill="var(--color-accent)"
                            opacity="0.8"
                            className="hover:opacity-100 transition"
                          />
                        </g>
                      );
                    })}
                  </svg>
                </div>
                
                {/* Chart labels */}
                <div className="flex justify-between mt-2 pt-2 border-t border-rule text-[10px] text-muted">
                  <span>{new Date(buckets[0].time).toLocaleTimeString()}</span>
                  <span>{new Date(buckets[Math.floor(buckets.length / 2)].time).toLocaleDateString()}</span>
                  <span>{new Date(buckets[buckets.length - 1].time).toLocaleTimeString()}</span>
                </div>
              </div>
            )}
          </div>

          {/* Busiest Catalog Services list */}
          <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm">
            <h2 className="font-title text-sm font-semibold text-ink mb-4 border-b border-rule pb-2">
              Busiest Services
            </h2>

            {isLoadingSummary ? (
              <div className="space-y-3">
                {[...Array(3)].map((_, i) => (
                  <div key={i} className="h-10 bg-paper-2 rounded-control animate-pulse"></div>
                ))}
              </div>
            ) : !summary?.topServices || summary.topServices.length === 0 ? (
              <p className="text-xs text-muted text-center py-8">No service calls recorded.</p>
            ) : (
              <div className="space-y-3">
                {summary.topServices.map((svc) => (
                  <div key={svc.serviceId} className="flex items-center justify-between p-2 rounded-control bg-paper-2 border border-rule hover:border-faint transition">
                    <div className="flex items-center gap-2">
                      <Server className="w-4 h-4 text-accent" />
                      <div>
                        <p className="text-xs font-semibold text-ink max-w-[140px] truncate">{svc.name || 'API Service'}</p>
                        <p className="text-[10px] text-muted truncate">ID: {svc.serviceId.substring(0, 8)}</p>
                      </div>
                    </div>
                    <span className="text-xs font-bold text-accent px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge">
                      {svc.count.toLocaleString()} reqs
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Live Logs Table */}
        <div className="bg-paper border border-rule rounded-surface p-6 shadow-sm">
          <h2 className="font-title text-sm font-semibold text-ink mb-4 border-b border-rule pb-2">
            Request Streams
          </h2>

          {isLoadingLogs && logs.length === 0 ? (
            <div className="space-y-4">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="h-10 bg-paper-2 rounded-control animate-pulse"></div>
              ))}
            </div>
          ) : logs.length === 0 ? (
            <div className="text-center py-8 text-xs text-muted">
              No matching request entries found.
            </div>
          ) : (
            <div className="space-y-4">
              <div className="overflow-x-auto">
                <table className="w-full text-left border-collapse text-xs">
                  <thead>
                    <tr className="border-b border-rule text-muted text-[10px] uppercase font-bold tracking-wider">
                      <th className="py-2 px-3">Status</th>
                      <th className="py-2 px-3">Method</th>
                      <th className="py-2 px-3">Path</th>
                      <th className="py-2 px-3">Duration</th>
                      <th className="py-2 px-3">IP Address</th>
                      <th className="py-2 px-3">Timestamp</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-rule font-mono">
                    {logs.map((log) => {
                      let statusColor = 'text-ok bg-ok-wash border-ok-edge';
                      if (log.statusCode >= 400 && log.statusCode < 500) {
                        statusColor = 'text-warn bg-warn-wash border-warn-edge';
                      } else if (log.statusCode >= 500) {
                        statusColor = 'text-danger bg-danger-wash border-danger-edge';
                      }
                      
                      return (
                        <tr key={log.id} className="hover:bg-paper-2 transition-colors">
                          <td className="py-2.5 px-3">
                            <span className={`px-2 py-0.5 rounded-chip border text-[10px] font-bold ${statusColor}`}>
                              {log.statusCode}
                            </span>
                          </td>
                          <td className="py-2.5 px-3 font-semibold text-ink">{log.method || 'GET'}</td>
                          <td className="py-2.5 px-3 text-ink-3 max-w-xs truncate" title={log.path}>
                            {log.path || '/'}
                          </td>
                          <td className="py-2.5 px-3 text-muted">{log.durationMs} ms</td>
                          <td className="py-2.5 px-3 text-muted">{log.ip || '127.0.0.1'}</td>
                          <td className="py-2.5 px-3 text-muted">
                            {new Date(log.time).toLocaleTimeString()}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>

              {/* Load More Button */}
              {nextCursor && (
                <div className="flex justify-center pt-2">
                  <button
                    onClick={handleLoadMore}
                    disabled={isLoadingMoreLogs}
                    className="px-4 py-2 border border-rule hover:border-faint rounded-control text-xs font-semibold text-ink-2 hover:bg-paper-2 transition flex items-center gap-2 disabled:opacity-50"
                  >
                    {isLoadingMoreLogs ? 'Loading entries...' : 'Load More Logs'}
                    {!isLoadingMoreLogs && <ArrowRight className="w-3 h-3" />}
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </NavigationShell>
  );
}

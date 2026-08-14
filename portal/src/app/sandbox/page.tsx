'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import axios from 'axios';
import { api, GATEWAY_URL } from '@/lib/api';
import NavigationShell from '@/components/NavigationShell';
import { Send, Copy, Check, Plus, Trash2 } from 'lucide-react';

interface ResourcePath {
  path: string;
  methods: string[];
}

interface GatewayService {
  id: string;
  name: string;
  basePath: string;
  type: string;
  resourcePaths?: ResourcePath[];
}

interface ApiKey {
  id: string;
  name: string;
  apiKey?: string;
  key?: string;
}

interface HeaderRow {
  name: string;
  value: string;
}

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];
const RATE_LIMIT_HEADERS = ['ratelimit-limit', 'ratelimit-remaining', 'ratelimit-reset', 'retry-after'];

export default function SandboxPage() {
  const [serviceId, setServiceId] = useState('');
  const [method, setMethod] = useState('GET');
  const [path, setPath] = useState('/');
  const [query, setQuery] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [headers, setHeaders] = useState<HeaderRow[]>([]);
  const [body, setBody] = useState('');

  const [status, setStatus] = useState<number | null>(null);
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);
  const [responseBody, setResponseBody] = useState('');
  const [responseHeaders, setResponseHeaders] = useState<Record<string, string>>({});
  const [isSending, setIsSending] = useState(false);
  const [copied, setCopied] = useState(false);

  const { data: services } = useQuery<GatewayService[]>({
    queryKey: ['catalog-services'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  const { data: apiKeys } = useQuery<ApiKey[]>({
    queryKey: ['api-keys'],
    queryFn: async () => {
      const res = await api.get('/api-keys');
      return res.data.items || [];
    },
  });

  const service = useMemo(
    () => (services || []).find((s) => s.id === serviceId),
    [services, serviceId]
  );

  // Land on the first service and its first path, so the page is usable immediately
  useEffect(() => {
    if (!serviceId && services && services.length > 0) {
      setServiceId(services[0].id);
    }
  }, [services, serviceId]);

  useEffect(() => {
    const first = service?.resourcePaths?.[0];
    if (first) {
      setPath(first.path);
      setMethod(first.methods?.[0] || 'GET');
    }
  }, [service]);

  const url = useMemo(() => {
    if (!service) return '';
    const base = service.basePath.replace(/^\//, '').replace(/\/$/, '');
    const rest = path.replace(/^\//, '');
    return `${GATEWAY_URL}/resources/${base}${rest ? `/${rest}` : ''}${query ? `?${query.replace(/^\?/, '')}` : ''}`;
  }, [service, path, query]);

  const curl = useMemo(() => {
    if (!url) return '';
    const lines = [`curl -X ${method} "${url}"`, `  -H "X-API-Key: ${apiKey || '<your-key>'}"`];
    headers.filter((h) => h.name).forEach((h) => lines.push(`  -H "${h.name}: ${h.value}"`));
    if (body && method !== 'GET') lines.push(`  -d '${body}'`);
    return lines.join(' \\\n');
  }, [url, method, apiKey, headers, body]);

  const send = async () => {
    if (!url) return;
    setIsSending(true);
    setStatus(null);
    setResponseBody('');
    setResponseHeaders({});
    const startedAt = performance.now();

    const sent: Record<string, string> = {};
    if (apiKey) sent['X-API-Key'] = apiKey;
    headers.filter((h) => h.name).forEach((h) => (sent[h.name] = h.value));

    try {
      const res = await axios({
        method,
        url,
        headers: sent,
        data: body && method !== 'GET' ? body : undefined,
        timeout: 15000,
        transformResponse: [(d) => d],
      });
      setStatus(res.status);
      setResponseBody(pretty(res.data));
      setResponseHeaders(res.headers as any);
    } catch (err: any) {
      if (err.response) {
        setStatus(err.response.status);
        setResponseBody(pretty(err.response.data));
        setResponseHeaders(err.response.headers as any);
      } else {
        setStatus(null);
        setResponseBody(err.message || 'The request never got a reply.');
      }
    } finally {
      setElapsedMs(Math.round(performance.now() - startedAt));
      setIsSending(false);
    }
  };

  const rateLimits = Object.entries(responseHeaders).filter(([k]) =>
    RATE_LIMIT_HEADERS.includes(k.toLowerCase())
  );

  return (
    <NavigationShell>
      <div className="space-y-4">
        <div>
          <h1 className="text-2xl font-bold text-ink">Sandbox</h1>
          <p className="text-sm text-muted">
            Call a published service the way your users will, through the gateway and with a key.
          </p>
        </div>

        {services && services.length === 0 ? (
          <div className="p-6 border border-rule rounded-control bg-paper-2 text-sm text-muted">
            No services published yet. An administrator adds them under Manage Services.
          </div>
        ) : (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {/* Request */}
            <div className="space-y-3 p-4 border border-rule rounded-control bg-paper-2">
              <div className="grid grid-cols-2 gap-2">
                <label className="block">
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted">Service</span>
                  <select
                    value={serviceId}
                    onChange={(e) => setServiceId(e.target.value)}
                    className="mt-1 w-full bg-paper border border-rule rounded-control px-2.5 py-1.5 text-xs text-ink outline-none focus:border-focus"
                  >
                    {(services || []).map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name} ({s.basePath})
                      </option>
                    ))}
                  </select>
                </label>

                <label className="block">
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted">API key</span>
                  <select
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    className="mt-1 w-full bg-paper border border-rule rounded-control px-2.5 py-1.5 text-xs text-ink outline-none focus:border-focus"
                  >
                    <option value="">No key (expect 401)</option>
                    {(apiKeys || []).map((k) => (
                      <option key={k.id} value={k.apiKey || k.key || ''}>
                        {k.name}
                      </option>
                    ))}
                  </select>
                </label>
              </div>

              <div className="flex gap-2">
                <select
                  value={method}
                  onChange={(e) => setMethod(e.target.value)}
                  className="bg-paper border border-rule rounded-control px-2.5 py-1.5 text-xs font-mono text-ink outline-none"
                >
                  {METHODS.map((m) => (
                    <option key={m}>{m}</option>
                  ))}
                </select>
                <input
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  placeholder="/collections"
                  list="known-paths"
                  className="flex-1 min-w-0 bg-paper border border-rule rounded-control px-2.5 py-1.5 text-xs font-mono text-ink outline-none focus:border-focus"
                />
                <datalist id="known-paths">
                  {(service?.resourcePaths || []).map((rp) => (
                    <option key={rp.path} value={rp.path} />
                  ))}
                </datalist>
              </div>

              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="limit=10&bbox=..."
                className="w-full bg-paper border border-rule rounded-control px-2.5 py-1.5 text-xs font-mono text-ink outline-none focus:border-focus"
              />

              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted">Headers</span>
                  <button
                    type="button"
                    onClick={() => setHeaders((h) => [...h, { name: '', value: '' }])}
                    className="text-[11px] font-semibold text-accent flex items-center gap-1"
                  >
                    <Plus className="w-3 h-3" /> Add
                  </button>
                </div>
                {headers.map((h, i) => (
                  <div key={i} className="flex gap-2">
                    <input
                      value={h.name}
                      onChange={(e) =>
                        setHeaders((prev) => prev.map((x, j) => (i === j ? { ...x, name: e.target.value } : x)))
                      }
                      placeholder="Accept"
                      className="flex-1 min-w-0 bg-paper border border-rule rounded-control px-2.5 py-1 text-xs font-mono text-ink outline-none"
                    />
                    <input
                      value={h.value}
                      onChange={(e) =>
                        setHeaders((prev) => prev.map((x, j) => (i === j ? { ...x, value: e.target.value } : x)))
                      }
                      placeholder="application/geo+json"
                      className="flex-[2] min-w-0 bg-paper border border-rule rounded-control px-2.5 py-1 text-xs font-mono text-ink outline-none"
                    />
                    <button
                      type="button"
                      onClick={() => setHeaders((prev) => prev.filter((_, j) => j !== i))}
                      className="p-1 text-muted hover:text-danger"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                ))}
              </div>

              {method !== 'GET' && (
                <label className="block">
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted">Body</span>
                  <textarea
                    value={body}
                    onChange={(e) => setBody(e.target.value)}
                    rows={4}
                    placeholder='{"name": "example"}'
                    className="mt-1 w-full bg-paper border border-rule rounded-control px-2.5 py-1.5 text-xs font-mono text-ink outline-none focus:border-focus"
                  />
                </label>
              )}

              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={send}
                  disabled={isSending || !service}
                  className="px-4 py-1.5 rounded-control bg-accent text-white text-xs font-semibold flex items-center gap-1.5 disabled:opacity-50"
                >
                  <Send className="w-3.5 h-3.5" />
                  {isSending ? 'Sending...' : 'Send'}
                </button>
                <code className="flex-1 min-w-0 truncate text-[11px] text-muted font-mono">{url}</code>
              </div>
            </div>

            {/* Response */}
            <div className="space-y-3 p-4 border border-rule rounded-control bg-paper-2">
              <div className="flex items-center gap-3">
                <span className="text-[11px] font-bold uppercase tracking-wider text-muted">Response</span>
                {status !== null && (
                  <span
                    className={`px-2 py-0.5 rounded-control text-xs font-bold font-mono ${
                      status < 300
                        ? 'bg-success-wash text-success'
                        : status < 500
                          ? 'bg-warning-wash text-warning'
                          : 'bg-danger-wash text-danger'
                    }`}
                  >
                    {status}
                  </span>
                )}
                {elapsedMs !== null && <span className="text-[11px] text-muted">{elapsedMs} ms</span>}
              </div>

              {rateLimits.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {rateLimits.map(([k, v]) => (
                    <span
                      key={k}
                      className="px-2 py-0.5 rounded-control bg-accent-wash text-[11px] font-mono text-ink-2"
                    >
                      {k}: <strong>{v}</strong>
                    </span>
                  ))}
                </div>
              )}

              <pre className="max-h-72 overflow-auto bg-paper border border-rule rounded-control p-2.5 text-[11px] font-mono text-ink whitespace-pre-wrap break-words">
                {responseBody || 'Nothing sent yet.'}
              </pre>

              {Object.keys(responseHeaders).length > 0 && (
                <details>
                  <summary className="text-[11px] font-bold uppercase tracking-wider text-muted cursor-pointer">
                    All headers
                  </summary>
                  <div className="mt-1.5 space-y-0.5">
                    {Object.entries(responseHeaders).map(([k, v]) => (
                      <div key={k} className="text-[11px] font-mono text-muted break-all">
                        <span className="text-ink-2">{k}</span>: {String(v)}
                      </div>
                    ))}
                  </div>
                </details>
              )}

              <div>
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[11px] font-bold uppercase tracking-wider text-muted">
                    The same call, as curl
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      navigator.clipboard.writeText(curl);
                      setCopied(true);
                      setTimeout(() => setCopied(false), 2000);
                    }}
                    className="text-[11px] font-semibold text-accent flex items-center gap-1"
                  >
                    {copied ? <Check className="w-3 h-3" /> : <Copy className="w-3 h-3" />}
                    {copied ? 'Copied' : 'Copy'}
                  </button>
                </div>
                <pre className="bg-paper border border-rule rounded-control p-2.5 text-[11px] font-mono text-muted whitespace-pre-wrap break-all">
                  {curl}
                </pre>
              </div>
            </div>
          </div>
        )}
      </div>
    </NavigationShell>
  );
}

// pretty prints JSON when it is JSON, and leaves anything else alone
function pretty(raw: any): string {
  if (typeof raw !== 'string') return JSON.stringify(raw, null, 2);
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

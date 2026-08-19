/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import axios from 'axios';
import { api, GATEWAY_URL } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import {
  Button,
  PageHeader,
  SectionCard,
  EmptyState,
  SearchableSelect,
  GeoMapPreview,
} from '@/components/ui';
import {
  Send,
  Copy,
  Check,
  Plus,
  Trash2,
  Radio,
  MapPin,
  Code,
  Link2,
  Compass,
  ArrowRight,
  ChevronDown,
  Eye,
  FileText,
} from 'lucide-react';

interface HateoasLink {
  rel: string;
  href: string;
  type?: string;
  title?: string;
}

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
  id: string;
  name: string;
  value: string;
}

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];
const RATE_LIMIT_HEADERS = [
  'ratelimit-limit',
  'ratelimit-remaining',
  'ratelimit-reset',
  'retry-after',
];


/** จัดรูป JSON ให้อ่านง่าย ถ้าไม่ใช่ JSON ก็ปล่อยตามเดิม */
function pretty(raw: unknown): string {
  if (typeof raw !== 'string') return JSON.stringify(raw, null, 2);
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

function toHeaderRecord(raw: unknown): Record<string, string> {
  if (!raw || typeof raw !== 'object') return {};
  return Object.fromEntries(
    Object.entries(raw as Record<string, unknown>).map(([k, v]) => {
      if (Array.isArray(v)) return [k, v.join(', ')];
      if (v === null || v === undefined) return [k, ''];
      if (typeof v === 'object') return [k, JSON.stringify(v)];
      return [k, String(v)];
    }),
  );
}

/** สถานะตอบกลับสื่อด้วยสี + ตัวเลข ไม่ใช่สีอย่างเดียว */
function statusTone(status: number) {
  if (status < 300) return 'border-ok-edge bg-ok-wash text-ok';
  if (status < 500) return 'border-warn-edge bg-warn-wash text-warn';
  return 'border-danger-edge bg-danger-wash text-danger';
}

export default function SandboxPage() {
  const { user } = useAuth();
  const [serviceId, setServiceId] = useState('');
  const [method, setMethod] = useState('GET');
  const [path, setPath] = useState('/');
  const [query, setQuery] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [headers, setHeaders] = useState<HeaderRow[]>([]);
  const [body, setBody] = useState('');

  const [status, setStatus] = useState<number | null>(null);
  const [transportError, setTransportError] = useState<string | null>(null);
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);
  const [responseBody, setResponseBody] = useState('');
  const [responseHeaders, setResponseHeaders] = useState<Record<string, string>>({});
  const [isSending, setIsSending] = useState(false);
  const [copied, setCopied] = useState(false);
  const [urlCopied, setUrlCopied] = useState(false);
  const [responseView, setResponseView] = useState<'preview' | 'raw' | 'links' | 'headers'>('preview');

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
    [services, serviceId],
  );

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect
  if (!serviceId && services && services.length > 0) {
    setServiceId(services[0].id);
  }

  // Auto-select first API key if available
  const [hasClearedKey, setHasClearedKey] = useState(false);
  if (!apiKey && !hasClearedKey && apiKeys && apiKeys.length > 0) {
    const firstKey = apiKeys[0].apiKey || apiKeys[0].key || '';
    if (firstKey) {
      setApiKey(firstKey);
    }
  }

  // เปลี่ยน service แล้วเด้งไป path แรกของมัน
  const [syncedServiceId, setSyncedServiceId] = useState<string | null>(null);
  if (service && service.id !== syncedServiceId) {
    setSyncedServiceId(service.id);
    const first = service.resourcePaths?.[0];
    if (first) {
      setPath(first.path);
      setMethod(first.methods?.[0] || 'GET');
    }
  }

  const [pathParamValues, setPathParamValues] = useState<Record<string, string>>({});

  const pathParams = useMemo(() => {
    const matches = path.match(/\{([^}]+)\}/g);
    if (!matches) return [];
    return Array.from(new Set(matches.map((m) => m.slice(1, -1))));
  }, [path]);

  const isTileEndpoint = useMemo(() => {
    return (
      (path.includes('{z}') && path.includes('{x}') && path.includes('{y}')) ||
      service?.type === 'OGC_API_Tiles' ||
      service?.type === 'tiles'
    );
  }, [path, service]);

  const resolvedPath = useMemo(() => {
    let p = path;
    const tileDefaults: Record<string, string> = {
      z: '6',
      x: '50',
      y: '28',
    };
    for (const [k, v] of Object.entries(pathParamValues)) {
      if (v) {
        p = p.replaceAll(`{${k}}`, v);
      }
    }
    // Auto-fill tile coordinates defaults if left empty so Send always works
    if (isTileEndpoint) {
      for (const [k, defVal] of Object.entries(tileDefaults)) {
        p = p.replaceAll(`{${k}}`, pathParamValues[k] || defVal);
      }
    }
    // Clean up trailing colons or malformed endings
    p = p.replace(/\/:+$/, '').replace(/:+$/, '');
    return p;
  }, [path, pathParamValues, isTileEndpoint]);

  const isGeoJson = useMemo(() => {
    if (!responseBody) return false;
    try {
      const parsed = JSON.parse(responseBody);
      return (
        parsed?.type === 'FeatureCollection' ||
        parsed?.type === 'Feature' ||
        Array.isArray(parsed?.features) ||
        (Array.isArray(parsed) && parsed[0]?.geometry)
      );
    } catch {
      return false;
    }
  }, [responseBody]);

  const tileTemplateUrl = useMemo(() => {
    if (!isTileEndpoint || !service) return undefined;
    const base = service.basePath.replace(/^\//, '').replace(/\/$/, '');
    let rest = path.replace(/^\//, '');
    if (!rest || rest === '/') {
      rest = '{z}/{x}/{y}.png';
    }
    const full = `${GATEWAY_URL}/resources/${base}/${rest}`;
    return apiKey ? `${full}?api_key=${apiKey}` : full;
  }, [isTileEndpoint, service, path, apiKey]);

  const isMapDisplayable = isGeoJson || isTileEndpoint;

  const contentTypeHeader = useMemo(() => {
    for (const [k, v] of Object.entries(responseHeaders)) {
      if (k.toLowerCase() === 'content-type') return v.toLowerCase();
    }
    return '';
  }, [responseHeaders]);

  const isHtml = useMemo(() => {
    if (!responseBody) return false;
    const trimmed = responseBody.trim();
    return (
      contentTypeHeader.includes('text/html') ||
      trimmed.startsWith('<!DOCTYPE html>') ||
      trimmed.startsWith('<html') ||
      (trimmed.startsWith('<') && trimmed.includes('</html>'))
    );
  }, [responseBody, contentTypeHeader]);

  const isImage = useMemo(() => {
    return contentTypeHeader.startsWith('image/');
  }, [contentTypeHeader]);

  const isPreviewable = isMapDisplayable || isHtml || isImage;

  // มุมมองที่แสดงจริง: ถ้าตอบกลับไม่มี preview ให้ตกไป raw พร้อมไฮไลต์ปุ่มตั้งแต่ครั้งแรก
  const effectiveView =
    responseView === 'preview' && !isPreviewable ? 'raw' : responseView;

  const responseSizeKb = useMemo(() => {
    if (!responseBody) return null;
    return (new Blob([responseBody]).size / 1024).toFixed(2);
  }, [responseBody]);

  const responseLinks = useMemo<HateoasLink[]>(() => {
    if (!responseBody) return [];
    try {
      const parsed = JSON.parse(responseBody);
      const links: HateoasLink[] = [];
      if (Array.isArray(parsed?.links)) {
        links.push(...parsed.links);
      }
      if (Array.isArray(parsed?.collections)) {
        parsed.collections.forEach((c: any) => {
          if (c.id) {
            links.push({
              rel: 'collection',
              href: `/collections/${c.id}`,
              title: c.title || c.id,
            });
            links.push({
              rel: 'items',
              href: `/collections/${c.id}/items`,
              title: `${c.title || c.id} Items`,
            });
          }
        });
      }
      return links;
    } catch {
      return [];
    }
  }, [responseBody]);

  function handleFollowLink(rawHref: string) {
    if (!rawHref || !service) return;

    let cleanPath = rawHref;
    let cleanQuery = '';

    try {
      const dummyOrigin = 'http://localhost';
      const parsedUrl = new URL(
        rawHref.startsWith('http')
          ? rawHref
          : `${dummyOrigin}${rawHref.startsWith('/') ? '' : '/'}${rawHref}`,
      );
      const pathname = parsedUrl.pathname;
      cleanQuery = parsedUrl.search ? parsedUrl.search.replace(/^\?/, '') : '';

      const serviceBase = service.basePath.replace(/^\//, '').replace(/\/$/, '');
      const baseSegment = `/resources/${serviceBase}`;

      if (pathname.includes(baseSegment)) {
        cleanPath = pathname.split(baseSegment)[1] || '/';
      } else if (serviceBase && pathname.includes(`/${serviceBase}`)) {
        cleanPath = pathname.split(`/${serviceBase}`)[1] || '/';
      } else if (pathname.includes('stac-landuse')) {
        cleanPath = pathname.split('stac-landuse')[1] || '/';
      } else {
        // Strip upstream path prefixes (e.g. /core/api/features/1.1 or /core/api/stac/1.0)
        const knownRoots = [
          '/collections',
          '/conformance',
          '/items',
          '/children',
          '/catalogs',
          '/search',
          '/queryables',
          '/api-docs',
          '/api',
        ];
        let matchedRoot = false;
        for (const root of knownRoots) {
          if (pathname.includes(root)) {
            cleanPath = root + pathname.split(root)[1];
            matchedRoot = true;
            break;
          }
        }
        if (!matchedRoot) {
          cleanPath = pathname;
        }
      }

      if (!cleanPath.startsWith('/')) {
        cleanPath = '/' + cleanPath;
      }
    } catch {
      if (rawHref.includes('?')) {
        const parts = rawHref.split('?');
        cleanPath = parts[0];
        cleanQuery = parts[1] || '';
      }
    }

    // Clean up query string if API Key is already sent via headers
    if (cleanQuery) {
      const qParams = new URLSearchParams(cleanQuery);
      if (apiKey) {
        qParams.delete('api_key');
        qParams.delete('token');
      }
      cleanQuery = qParams.toString();
    }

    setPath(cleanPath || '/');
    setQuery(cleanQuery);
    setMethod('GET');
    setPathParamValues({});
  }

  const url = useMemo(() => {
    if (!service) return '';
    const base = service.basePath.replace(/^\//, '').replace(/\/$/, '');
    const rest = resolvedPath.replace(/^\//, '');
    return `${GATEWAY_URL}/resources/${base}${rest ? `/${rest}` : ''}${
      query ? `?${query.replace(/^\?/, '')}` : ''
    }`;
  }, [service, resolvedPath, query]);

  const curl = useMemo(() => {
    if (!url) return '';
    const lines = [`curl -X ${method} "${url}"`, `  -H "X-API-Key: ${apiKey || '<your-key>'}"`];
    headers.filter((h) => h.name).forEach((h) => lines.push(`  -H "${h.name}: ${h.value}"`));
    if (body && method !== 'GET') lines.push(`  -d '${body}'`);
    return lines.join(' \\\n');
  }, [url, method, apiKey, headers, body]);

  async function send() {
    if (!url) return;
    setIsSending(true);
    setStatus(null);
    setTransportError(null);
    setResponseBody('');
    setResponseHeaders({});
    const startedAt = performance.now();

    const sent: Record<string, string> = {};
    if (apiKey) sent['X-API-Key'] = apiKey;
    headers.filter((h) => h.name).forEach((h) => (sent[h.name] = h.value));

    try {
      const res = await fetch(url, {
        method,
        headers: sent,
        body: body && method !== 'GET' ? body : undefined,
        signal: AbortSignal.timeout(15000),  
        cache: 'no-store',                    
      });
      setStatus(res.status);
      setResponseBody(pretty(await res.text()));                        
      setResponseHeaders(Object.fromEntries(res.headers.entries()));
    } catch (err) {
        setStatus(null);
        setTransportError(
          err instanceof Error ? err.message : 'The request never got a reply.',
        );
    } finally {
      setElapsedMs(Math.round(performance.now() - startedAt));
      setIsSending(false);
    }
  }

  const rateLimits = Object.entries(responseHeaders).filter(([k]) =>
    RATE_LIMIT_HEADERS.includes(k.toLowerCase()),
  );

  if (services?.length === 0) {
    return (
      <NavigationShell>
        <PageHeader
          title="Sandbox"
          description="Call a published service the way your users will - through the gateway, with a key"
        />
        <div className="ui-card">
          <EmptyState
            icon={<Radio className="h-5 w-5" />}
            title="No services published yet"
            description="An administrator adds them under Manage Services. Once one exists, you can call it from here."
          />
        </div>
      </NavigationShell>
    );
  }

  return (
    <NavigationShell>
      <PageHeader
        title="Sandbox"
        description="Call a published service the way your users will - through the gateway, with a key"
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* ---- คำขอ ---- */}
        <SectionCard title="Request">
          <div className="space-y-4">
            <div>
              <span className="ui-field__label block mb-1">Preview URL</span>
              <div className="flex items-center gap-2 rounded-control border border-rule bg-paper-2 px-3 py-1.5">
                <p className="ui-scroll-x min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs text-muted">
                  {url || '-'}
                </p>
                <button
                  type="button"
                  aria-label="Copy request URL"
                  onClick={async () => {
                    if (!url) return;
                    await navigator.clipboard.writeText(url);
                    setUrlCopied(true);
                    setTimeout(() => setUrlCopied(false), 2500);
                  }}
                  className="shrink-0 rounded-control p-1 text-muted transition-colors hover:bg-paper-3 hover:text-ink"
                >
                  {urlCopied ? (
                    <Check className="h-3.5 w-3.5 text-ok" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                </button>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <SearchableSelect
                label="Service"
                value={serviceId}
                onChange={setServiceId}
                options={
                  (services || []).map((s) => ({
                    value: s.id,
                    label: s.name,
                    subtext: s.basePath,
                  }))
                }
                placeholder="Choose service..."
                searchPlaceholder="Search services by name or path..."
              />

              <label className="block">
                <span className="ui-field__label">API key</span>
                <div className="relative">
                  <select
                    className="ui-input appearance-none pr-8"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                  >
                    <option value="">No key (expect 401)</option>
                    {(apiKeys || []).map((k) => (
                      <option key={k.id} value={k.apiKey || k.key || ''}>
                        {k.name}
                      </option>
                    ))}
                  </select>
                  <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
                </div>
              </label>
            </div>

            <div>
              <label className="ui-field__label block mb-1">Method and path</label>
              <div className="flex gap-2">
                <div className="relative shrink-0">
                  <select
                    className="ui-input w-auto appearance-none pr-8 font-mono"
                    aria-label="HTTP method"
                    value={method}
                    onChange={(e) => setMethod(e.target.value)}
                  >
                    {METHODS.map((m) => (
                      <option key={m}>{m}</option>
                    ))}
                  </select>
                  <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted" />
                </div>
                <input
                  className="ui-input min-w-0 flex-1 font-mono"
                  aria-label="Path"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  placeholder="/collections/{collectionId}/items"
                />
              </div>

              {/* Clean Predefined Endpoints shortcuts */}
              {service?.resourcePaths && service.resourcePaths.length > 1 && (
                <div className="mt-2 flex items-center gap-1.5 flex-wrap">
                  <span className="text-[11px] text-muted font-medium">Mapped endpoints:</span>
                  {service.resourcePaths.map((rp) => (
                    <button
                      key={rp.path}
                      type="button"
                      onClick={() => {
                        setPath(rp.path);
                        setPathParamValues({});
                        setMethod(rp.methods?.[0] || 'GET');
                      }}
                      className={`px-2 py-0.5 rounded-chip font-mono text-[10px] transition-colors ${
                        path === rp.path
                          ? 'bg-accent-wash text-accent font-semibold border border-accent-edge'
                          : 'bg-paper-2 text-muted hover:text-ink border border-rule hover:bg-paper-3'
                      }`}
                    >
                      {rp.path}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Path Parameters */}
            {pathParams.length > 0 && (
              <div>
                <span className="ui-field__label block mb-1">Path Parameters</span>
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {pathParams.map((param) => (
                    <label key={param} className="block">
                      <span className="mb-1 block font-mono text-[11px] font-semibold text-ink">
                        {`{${param}}`}
                      </span>
                      <input
                        type="text"
                        className="ui-input font-mono"
                        placeholder={
                          param === 'z'
                            ? 'e.g. 6'
                            : param === 'x'
                            ? 'e.g. 50'
                            : param === 'y'
                            ? 'e.g. 28'
                            : `Enter ${param}...`
                        }
                        value={pathParamValues[param] || ''}
                        onChange={(e) => {
                          const val = e.target.value;
                          setPathParamValues((prev) => ({ ...prev, [param]: val }));
                        }}
                      />
                    </label>
                  ))}
                </div>
              </div>
            )}

            <label className="block">
              <span className="ui-field__label">Query string</span>
              <input
                className="ui-input font-mono"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="limit=10&bbox=..."
              />
            </label>

            <div>
              <div className="flex items-center justify-between">
                <span className="ui-field__label mb-0">Headers</span>
                <Button
                  size="sm"
                  variant="ghost"
                  icon={<Plus className="h-3.5 w-3.5" />}
                  onClick={() =>
                    setHeaders((h) => [
                      ...h,
                      { id: crypto.randomUUID(), name: '', value: '' },
                    ])
                  }
                >
                  Add
                </Button>
              </div>

              {headers.length === 0 && (
                <p className="mt-1 text-xs text-muted">
                  X-API-Key is added for you from the key selected above.
                </p>
              )}

              <div className="mt-2 space-y-2">
                {headers.map((h) => (
                  <div key={h.id} className="flex gap-2">
                    <input
                      className="ui-input min-w-0 flex-1 font-mono"
                      aria-label="Header name"
                      value={h.name}
                      onChange={(e) =>
                        setHeaders((prev) =>
                          prev.map((x) => (x.id === h.id ? { ...x, name: e.target.value } : x)),
                        )
                      }
                      placeholder="Accept"
                    />
                    <input
                      className="ui-input min-w-0 flex-[2] font-mono"
                      aria-label="Header value"
                      value={h.value}
                      onChange={(e) =>
                        setHeaders((prev) =>
                          prev.map((x) => (x.id === h.id ? { ...x, value: e.target.value } : x)),
                        )
                      }
                      placeholder="application/geo+json"
                    />
                    <Button
                      variant="ghost-danger"
                      aria-label="Remove this header"
                      icon={<Trash2 className="h-4 w-4" />}
                      onClick={() => setHeaders((prev) => prev.filter((x) => x.id !== h.id))}
                    />
                  </div>
                ))}
              </div>
            </div>

            {method !== 'GET' && (
              <label className="block">
                <span className="ui-field__label">Body</span>
                <textarea
                  className="ui-input font-mono"
                  rows={4}
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                  placeholder='{"name": "example"}'
                />
              </label>
            )}

            <div className="border-t border-rule pt-4">
              <Button
                variant="primary"
                onClick={send}
                loading={isSending}
                disabled={!service}
                icon={<Send className="h-4 w-4" />}
              >
                Send
              </Button>
            </div>
          </div>
        </SectionCard>

        {/* ---- คำตอบ ---- */}
        <SectionCard title="Response">
          <div className="space-y-4">
            {/* สรุปคำตอบ: แถวเดียวรวมสถานะ+เวลา+ขนาด (ซ่อนตอนยังไม่เรียก) แล้วตามด้วยปุ่มเลือกมุมมอง */}
            <div className="space-y-1.5 border-b border-rule pb-3" aria-live="polite">
              {(status !== null || elapsedMs !== null || responseSizeKb) && (
                <div className="flex min-h-6 flex-wrap items-center gap-2">
                  {status !== null && (
                    <span
                      className={`tabular rounded-chip border px-2 py-0.5 font-mono text-xs font-semibold ${statusTone(status)}`}
                    >
                      {status}
                    </span>
                  )}
                  <span className="tabular flex items-center gap-2 font-mono text-xs text-muted">
                    {elapsedMs !== null && <span>{elapsedMs} ms</span>}
                    {responseSizeKb && <span>{responseSizeKb} KB</span>}
                  </span>
                </div>
              )}
              <div className="flex w-fit rounded-control border border-rule bg-paper-2 p-0.5 text-xs">
                {isPreviewable && (
                  <button
                    type="button"
                    onClick={() => setResponseView('preview')}
                    className={`flex items-center gap-1 rounded-chip px-2.5 py-1 text-2xs font-medium transition-colors ${
                      effectiveView === 'preview'
                        ? 'bg-accent text-accent-ink shadow-xs font-semibold'
                        : 'text-muted hover:text-ink'
                    }`}
                  >
                    {isMapDisplayable ? (
                      <>
                        <MapPin className="h-3 w-3" /> Map
                      </>
                    ) : isHtml ? (
                      <>
                        <Eye className="h-3 w-3" /> HTML View
                      </>
                    ) : isImage ? (
                      <>
                        <Eye className="h-3 w-3" /> Image
                      </>
                    ) : (
                      <>
                        <Eye className="h-3 w-3" /> Preview
                      </>
                    )}
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => setResponseView('raw')}
                  className={`flex items-center gap-1 rounded-chip px-2.5 py-1 text-2xs font-medium transition-colors ${
                    effectiveView === 'raw'
                      ? 'bg-accent text-accent-ink shadow-xs font-semibold'
                      : 'text-muted hover:text-ink'
                  }`}
                >
                  <Code className="h-3 w-3" /> Raw JSON
                </button>
                {responseLinks.length > 0 && (
                  <button
                    type="button"
                    onClick={() => setResponseView('links')}
                    className={`flex items-center gap-1 rounded-chip px-2.5 py-1 text-2xs font-medium transition-colors ${
                      effectiveView === 'links'
                        ? 'bg-accent text-accent-ink shadow-xs font-semibold'
                        : 'text-muted hover:text-ink'
                    }`}
                  >
                    <Link2 className="h-3 w-3" /> Links ({responseLinks.length})
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => setResponseView('headers')}
                  className={`flex items-center gap-1 rounded-chip px-2.5 py-1 text-2xs font-medium transition-colors ${
                    effectiveView === 'headers'
                      ? 'bg-accent text-accent-ink shadow-xs font-semibold'
                      : 'text-muted hover:text-ink'
                  }`}
                >
                  <FileText className="h-3 w-3" /> Headers & cURL
                </button>
              </div>
            </div>

            {transportError && (
              <p className="rounded-control border border-danger-edge bg-danger-wash p-3 text-sm text-danger">
                {transportError}
              </p>
            )}

            {rateLimits.length > 0 && (
              <div className="flex flex-wrap gap-2">
                {rateLimits.map(([k, v]) => (
                  <span
                    key={k}
                    className="rounded-chip border border-accent-edge bg-accent-wash px-2 py-0.5 font-mono text-xs text-ink-2"
                  >
                    {k}: <strong>{v}</strong>
                  </span>
                ))}
              </div>
            )}

            {/* TAB 1: PREVIEW (Map / HTML / Image) */}
            {effectiveView === 'preview' && isMapDisplayable ? (
              <GeoMapPreview
                data={responseBody}
                tileUrl={tileTemplateUrl}
                onNavigateLink={handleFollowLink}
              />
            ) : effectiveView === 'preview' && isHtml ? (
              <div className="rounded-control border border-rule overflow-hidden bg-white shadow-xs">
                <div className="bg-paper-2 px-3 py-1.5 border-b border-rule flex items-center justify-between text-xs text-muted">
                  <span className="flex items-center gap-1.5">
                    <FileText className="h-3.5 w-3.5 text-accent" />
                    HTML Document Preview
                  </span>
                  <span className="text-[10px] text-ink-3">Sandboxed Browser Frame</span>
                </div>
                <iframe
                  srcDoc={responseBody}
                  title="HTML Response Preview"
                  className="w-full h-[460px] border-0 bg-white"
                  sandbox="allow-same-origin allow-scripts"
                />
              </div>
            ) : effectiveView === 'preview' && isImage ? (
              <div className="rounded-control border border-rule overflow-hidden bg-paper-2 p-6 flex flex-col items-center justify-center min-h-[380px]">
                <img
                  src={url}
                  alt="HTTP Response"
                  className="max-h-96 max-w-full rounded-control border border-rule object-contain shadow-md"
                />
              </div>
            ) : effectiveView === 'links' && responseLinks.length > 0 ? (
              /* TAB 2: RESOURCE & NAVIGATION LINKS */
              <div className="rounded-control border border-rule bg-paper p-3.5 space-y-3">
                <div className="flex items-center justify-between border-b border-rule pb-2">
                  <span className="text-xs font-semibold text-ink flex items-center gap-1.5">
                    <Link2 className="h-4 w-4 text-accent" />
                    {service?.type === 'OGC_API_STAC'
                      ? `STAC Catalog Navigation (${responseLinks.length})`
                      : service?.type === 'OGC_API_Features'
                      ? `Features & Resource Links (${responseLinks.length})`
                      : service?.type === 'OGC_API_Tiles'
                      ? `Tile & Metadata Links (${responseLinks.length})`
                      : `API Navigation Links (${responseLinks.length})`}
                  </span>
                  <span className="text-[11px] text-muted">Click any link to load into request</span>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-[420px] overflow-y-auto pr-1">
                  {responseLinks.map((lnk, idx) => (
                    <button
                      key={`${lnk.href}-${idx}`}
                      type="button"
                      onClick={() => handleFollowLink(lnk.href)}
                      className="flex min-w-0 items-center justify-between gap-2 p-2.5 rounded-control bg-paper-2 hover:bg-accent-wash hover:border-accent-edge border border-rule text-xs text-ink transition-colors cursor-pointer text-left group"
                      title={lnk.href}
                    >
                      <div className="min-w-0 flex-1">
                        <span className="font-mono text-[9px] uppercase px-1.5 py-0.5 rounded-chip bg-ink/10 text-ink group-hover:bg-accent group-hover:text-accent-ink font-semibold inline-block mb-1">
                          {lnk.rel}
                        </span>
                        <p className="truncate font-mono text-xs text-ink font-medium group-hover:text-accent">
                          {lnk.title || lnk.href}
                        </p>
                        <p className="truncate font-mono text-[10px] text-muted">
                          {lnk.href}
                        </p>
                      </div>
                      <ArrowRight className="h-4 w-4 text-muted group-hover:text-accent shrink-0" />
                    </button>
                  ))}
                </div>
              </div>
            ) : effectiveView === 'headers' ? (
              /* TAB 3: HEADERS & CURL */
              <div className="space-y-4">
                <div>
                  <div className="mb-2 flex items-center justify-between">
                    <span className="ui-field__label mb-0">The same call, as curl</span>
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={
                        copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />
                      }
                      onClick={async () => {
                        await navigator.clipboard.writeText(curl);
                        setCopied(true);
                        setTimeout(() => setCopied(false), 2500);
                      }}
                    >
                      {copied ? 'Copied' : 'Copy cURL'}
                    </Button>
                  </div>
                  <pre className="rounded-control border border-rule bg-paper-2 p-3 font-mono text-xs break-all whitespace-pre-wrap text-ink-3 max-h-48 overflow-y-auto">
                    {curl}
                  </pre>
                </div>

                {Object.keys(responseHeaders).length > 0 && (
                  <div>
                    <span className="ui-field__label block mb-2">
                      Response Headers ({Object.keys(responseHeaders).length})
                    </span>
                    <div className="rounded-control border border-rule bg-paper-2 p-3 font-mono text-xs space-y-1.5 max-h-60 overflow-y-auto">
                      {Object.entries(responseHeaders).map(([k, v]) => (
                        <p key={k} className="text-xs break-all text-muted flex justify-between gap-2 border-b border-rule/50 pb-1">
                          <strong className="text-ink font-mono">{k}:</strong>
                          <span className="font-mono text-ink-2 text-right">{v}</span>
                        </p>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              /* TAB 4: RAW JSON / BODY */
              <div className="relative">
                <pre className="max-h-[460px] overflow-auto rounded-control border border-rule bg-paper-2 p-3 font-mono text-xs break-words whitespace-pre-wrap text-ink">
                  {responseBody || 'Nothing sent yet.'}
                </pre>
              </div>
            )}
          </div>
        </SectionCard>
      </div>
    </NavigationShell>
  );
}

/* Hallmark · genre: modern-minimal · macrostructure: Workbench
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useMemo, useState } from 'react';
import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from '@tanstack/react-query';
import axios from 'axios';
import { api, GATEWAY_URL } from '@/lib/api';
import { useAuth } from '@/app/providers';
import NavigationShell from '@/components/NavigationShell';
import {
  Button,
  Field,
  PageHeader,
  SectionCard,
  EmptyState,
  SkeletonLine,
  Loading,
  useToast,
  useConfirm,
  GeoMapPreview,
} from '@/components/ui';
import {
  Key,
  Server,
  Play,
  Copy,
  Check,
  Plus,
  Trash2,
  Calendar,
  ChevronRight,
  BookOpen,
  FileText,
  MapPin,
  Code,
  Search,
  X,
} from 'lucide-react';

interface ResourcePath {
  path: string;
  methods: string[];
  sourceAlias: string;
}

interface GatewayService {
  id: string;
  name: string;
  description?: string;
  type: string;
  basePath: string;
  enabled?: boolean;
  isPublic?: boolean;
  resourcePaths?: ResourcePath[];
}

interface ApiKey {
  id: string;
  name: string;
  apiKey?: string;
  expiredAt?: string;
  createdAt?: string;
  enabled?: boolean;
}

interface ApiDoc {
  id: string;
  info: { title: string; version: string; description?: string };
}

interface OpenApiOperation {
  summary?: string;
  description?: string;
}

interface OpenApiSpec {
  info?: { title?: string; version?: string; description?: string };
  paths?: Record<string, Record<string, OpenApiOperation>>;
}

type MainTab = 'catalog' | 'keys';
type DetailTab = 'info' | 'spec' | 'try' | 'docs';

const DETAIL_TABS: { id: DetailTab; label: string }[] = [
  { id: 'info', label: 'Overview' },
  { id: 'spec', label: 'OpenAPI' },
  { id: 'try', label: 'Try it' },
  { id: 'docs', label: 'Guides' },
];

function messageOf(err: unknown, fallback: string) {
  return err instanceof Error && err.message ? err.message : fallback;
}

function toHeaderRecord(raw: unknown): Record<string, string> {
  if (!raw || typeof raw !== 'object') return {};
  return Object.fromEntries(
    Object.entries(raw as Record<string, unknown>).map(([k, v]) => [
      k,
      Array.isArray(v) ? v.join(', ') : String(v ?? ''),
    ]),
  );
}

function MethodChip({ method }: { method: string }) {
  return (
    <span className="rounded-chip bg-accent px-1.5 py-0.5 font-mono text-xs font-medium uppercase text-accent-ink">
      {method}
    </span>
  );
}

/** ป้ายเปลี่ยนเป็น "คัดลอกแล้ว" คือ feedback ในตัว ไม่ต้อง toast ซ้ำ */
function CopyButton({
  value,
  label = 'Copy',
  variant = 'ghost',
  size = 'sm',
}: {
  value: string;
  label?: string;
  variant?: 'ghost' | 'secondary' | 'primary';
  size?: 'sm' | 'md';
}) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      size={size}
      variant={variant}
      icon={copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
      onClick={async () => {
        await navigator.clipboard.writeText(value);
        setCopied(true);
        setTimeout(() => setCopied(false), 2500);
      }}
    >
      {copied ? 'Copied' : label}
    </Button>
  );
}

export default function ServicesAndKeysPage() {
  const queryClient = useQueryClient();
  const { toast, error: toastError } = useToast();
  const confirm = useConfirm();

  const [mainTab, setMainTab] = useState<MainTab>('catalog');
  const [selectedService, setSelectedService] = useState<GatewayService | null>(null);
  const [detailTab, setDetailTab] = useState<DetailTab>('info');

  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyExpiry, setNewKeyExpiry] = useState('');
  const [createdKey, setCreatedKey] = useState<string | null>(null);

  const [selectedApiKey, setSelectedApiKey] = useState('');
  const [selectedPath, setSelectedPath] = useState('');
  const [selectedMethod, setSelectedMethod] = useState('GET');
  const [customParams, setCustomParams] = useState('');
  const { user } = useAuth();
  const [responseStatus, setResponseStatus] = useState<number | null>(null);
  const [responseHeaders, setResponseHeaders] = useState<Record<string, string>>({});
  const [responseBody, setResponseBody] = useState('');
  const [responseElapsedMs, setResponseElapsedMs] = useState<number | null>(null);
  const [tryItResponseView, setTryItResponseView] = useState<'preview' | 'raw'>('preview');
  const [isSending, setIsSending] = useState(false);

  const { data: services, isLoading: isLoadingServices } = useQuery<GatewayService[]>({
    queryKey: ['catalog-services'],
    queryFn: async () => {
      const res = await api.get('/services');
      return res.data.items || [];
    },
  });

  const { data: apiKeys, isLoading: isLoadingKeys } = useQuery<ApiKey[]>({
    queryKey: ['api-keys'],
    queryFn: async () => {
      const res = await api.get('/api-keys');
      return res.data.items || [];
    },
  });

  const {
    data: openApiSpec,
    isLoading: isLoadingSpec,
    error: specError,
  } = useQuery<OpenApiSpec | null>({
    queryKey: ['service-openapi', selectedService?.id],
    queryFn: async () => {
      if (!selectedService) return null;
      const res = await api.get(`/services/${selectedService.id}/spec/openapi`);
      return res.data;
    },
    enabled: !!selectedService && detailTab === 'spec',
    retry: false,
  });

  const { data: apiDocs, isLoading: isLoadingDocs } = useQuery<ApiDoc[]>({
    queryKey: ['api-docs'],
    queryFn: async () => {
      const res = await api.get('/api-docs');
      return res.data.items || [];
    },
    enabled: !!selectedService && detailTab === 'docs',
  });

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect (กัน cascading render)
  const [syncedServiceId, setSyncedServiceId] = useState<string | null>(null);
  if (selectedService && selectedService.id !== syncedServiceId) {
    setSyncedServiceId(selectedService.id);
    const first = selectedService.resourcePaths?.[0];
    setSelectedPath(first?.path ?? '');
    setSelectedMethod(first?.methods?.[0] ?? 'GET');
    setResponseStatus(null);
    setResponseHeaders({});
    setResponseBody('');
  }

  // เลือกคีย์แรกให้ครั้งเดียว ไม่ผูกกับค่าว่างของ selectedApiKey
  // ถ้าเช็คด้วย !selectedApiKey แล้วคีย์แรกไม่มีค่า apiKey เงื่อนไขจะเป็นจริงทุก render
  const [pickedFirstKey, setPickedFirstKey] = useState(false);
  if (!pickedFirstKey && apiKeys && apiKeys.length > 0) {
    setPickedFirstKey(true);
    setSelectedApiKey(apiKeys[0].apiKey || '');
  }

  const createKey: UseMutationResult<{ apiKey: string }, Error, void> = useMutation({
    mutationFn: async () => {
      const res = await api.post('/api-keys', {
        name: newKeyName,
        expiredAt: newKeyExpiry ? new Date(newKeyExpiry).toISOString() : undefined,
      });
      return res.data;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
      setCreatedKey(data.apiKey);
      setNewKeyName('');
      setNewKeyExpiry('');
    },
    onError: (err) => toastError(messageOf(err, 'Could not issue the key')),
  });

  const regenerateKey: UseMutationResult<{ apiKey: string }, Error, string> = useMutation({
    mutationFn: async (id: string) => {
      const res = await api.post(`/api-keys/${id}/regenerate`);
      return res.data;
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
      setCreatedKey(data.apiKey);
    },
    onError: (err) => toastError(messageOf(err, 'Could not rotate the key')),
  });

  const deleteKey: UseMutationResult<void, Error, string> = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/api-keys/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['api-keys'] });
      toast({ tone: 'ok', message: 'Key deleted' });
    },
    onError: (err, id) =>
      toastError(messageOf(err, 'Could not delete the key'), () => deleteKey.mutate(id)),
  });

  const [serviceSearchTerm, setServiceSearchTerm] = useState('');

  const filteredCatalogServices = useMemo(() => {
    if (!services) return [];
    const q = serviceSearchTerm.toLowerCase().trim();
    if (!q) return services;
    return services.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.basePath.toLowerCase().includes(q) ||
        (s.description && s.description.toLowerCase().includes(q)) ||
        s.type.toLowerCase().includes(q),
    );
  }, [services, serviceSearchTerm]);

  const [tryItPathParamValues, setTryItPathParamValues] = useState<Record<string, string>>({});

  const tryItPathParams = useMemo(() => {
    const matches = selectedPath.match(/\{([^}]+)\}/g);
    if (!matches) return [];
    return Array.from(new Set(matches.map((m) => m.slice(1, -1))));
  }, [selectedPath]);

  const resolvedTryPath = useMemo(() => {
    let p = selectedPath;
    for (const [k, v] of Object.entries(tryItPathParamValues)) {
      if (v) {
        p = p.replaceAll(`{${k}}`, v);
      }
    }
    return p;
  }, [selectedPath, tryItPathParamValues]);

  const tryUrl = useMemo(() => {
    if (!selectedService) return '';
    const base = selectedService.basePath.replace(/^\//, '');
    const rest = resolvedTryPath.replace(/^\//, '');
    return `${GATEWAY_URL}/resources/${base}/${rest}${customParams ? `?${customParams}` : ''}`;
  }, [selectedService, resolvedTryPath, customParams]);

  const curl = tryUrl
    ? `curl -X ${selectedMethod} "${tryUrl}" \\\n  -H "X-API-Key: ${selectedApiKey || '<your_key>'}"`
    : '';

  const tryItIsGeoJson = useMemo(() => {
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

  const tryItSizeKb = useMemo(() => {
    if (!responseBody) return null;
    return (new Blob([responseBody]).size / 1024).toFixed(2);
  }, [responseBody]);

  async function handleTryIt() {
    if (!selectedService || !tryUrl) return;
    setIsSending(true);
    setResponseStatus(null);
    setResponseBody('');
    setResponseHeaders({});
    setResponseElapsedMs(null);

    const start = performance.now();
    try {
      const res = await axios({
        method: selectedMethod,
        url: tryUrl,
        headers: selectedApiKey ? { 'X-API-Key': selectedApiKey } : {},
        timeout: 10000,
      });
      setResponseElapsedMs(Math.round(performance.now() - start));
      setResponseStatus(res.status);
      setResponseBody(JSON.stringify(res.data, null, 2));
      setResponseHeaders(toHeaderRecord(res.headers));
    } catch (err) {
      setResponseElapsedMs(Math.round(performance.now() - start));
      if (axios.isAxiosError(err) && err.response) {
        setResponseStatus(err.response.status);
        setResponseBody(JSON.stringify(err.response.data, null, 2));
        setResponseHeaders(toHeaderRecord(err.response.headers));
      } else {
        setResponseStatus(null);
        setResponseBody(messageOf(err, 'The request never got a reply.'));
      }
    } finally {
      setIsSending(false);
    }
  }

  async function askRegenerate(key: ApiKey) {
    const ok = await confirm({
      title: `Rotate the key "${key.name}"`,
      consequences: [
        'The old value stops working immediately',
        'Anything still using it will fail until you swap the value in',
        'The new value is shown once and never again',
      ],
      confirmLabel: 'Rotate key',
      danger: true,
    });
    if (ok) regenerateKey.mutate(key.id);
  }

  async function askDelete(key: ApiKey) {
    const ok = await confirm({
      title: 'Delete this API key for good',
      consequences: [
        'This cannot be undone',
        'Anything using this key stops working immediately',
      ],
      typeToConfirm: key.name,
      confirmLabel: 'Delete key',
      danger: true,
    });
    if (ok) deleteKey.mutate(key.id);
  }

  return (
    <NavigationShell>
      <PageHeader
        title="Services & Keys"
        description="Browse the published APIs, try them out, and manage the keys you call them with"
      />

      <div role="tablist" aria-label="View" className="mb-6 flex gap-6 border-b border-rule">
        {(
          [
            { id: 'catalog' as const, label: 'Published APIs', icon: Server },
            { id: 'keys' as const, label: 'My API keys', icon: Key },
          ]
        ).map((t) => {
          const Icon = t.icon;
          const active = mainTab === t.id;
          return (
            <button
              key={t.id}
              role="tab"
              type="button"
              aria-selected={active}
              onClick={() => setMainTab(t.id)}
              className={`-mb-px flex items-center gap-2 border-b-2 pb-3 text-sm whitespace-nowrap transition-colors duration-200 ${
                active
                  ? 'border-accent font-medium text-ink'
                  : 'border-transparent text-muted hover:text-ink'
              }`}
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
              {t.label}
            </button>
          );
        })}
      </div>

      {/* ================= API ที่เผยแพร่ ================= */}
      {mainTab === 'catalog' && (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div className="lg:col-span-1">
            <SectionCard title="Services" flush>
              {services && services.length > 0 && (
                <div className="border-b border-rule p-2.5">
                  <div className="relative">
                    <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted" />
                    <input
                      type="text"
                      value={serviceSearchTerm}
                      onChange={(e) => setServiceSearchTerm(e.target.value)}
                      placeholder="Search services..."
                      className="w-full bg-paper border border-rule rounded-control pl-8 pr-7 py-1 text-xs text-ink placeholder:text-muted outline-none focus:border-focus"
                    />
                    {serviceSearchTerm && (
                      <button
                        type="button"
                        onClick={() => setServiceSearchTerm('')}
                        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-ink"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    )}
                  </div>
                </div>
              )}

              {isLoadingServices && (
                <Loading label="Loading services">
                  <div className="space-y-3 p-4">
                    {[0, 1, 2].map((i) => (
                      <div key={i}>
                        <SkeletonLine width="55%" className="mb-2 h-4" />
                        <SkeletonLine width="35%" />
                      </div>
                    ))}
                  </div>
                </Loading>
              )}

              {!isLoadingServices && (!services || services.length === 0) && (
                <EmptyState
                  icon={<Server className="h-5 w-5" />}
                  title="No services published yet"
                  description="An administrator adds them under Manage Services."
                />
              )}

              {!isLoadingServices && services && services.length > 0 && filteredCatalogServices.length === 0 && (
                <div className="p-6 text-center text-xs text-muted">
                  No services match &quot;{serviceSearchTerm}&quot;
                </div>
              )}

              {!isLoadingServices && filteredCatalogServices.length > 0 && (
                <ul className="divide-y divide-rule max-h-[600px] overflow-y-auto">
                  {filteredCatalogServices.map((svc) => {
                    const active = selectedService?.id === svc.id;
                    return (
                      <li key={svc.id}>
                        <button
                          type="button"
                          onClick={() => setSelectedService(svc)}
                          aria-current={active ? 'true' : undefined}
                          className={`flex w-full items-center justify-between gap-2 p-4 text-left transition-colors duration-200 ${
                            active ? 'bg-accent-wash' : 'hover:bg-paper-2'
                          }`}
                        >
                          <span className="flex min-w-0 items-center gap-3">
                            <span
                              aria-hidden="true"
                              className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-control ${
                                active ? 'bg-accent text-accent-ink' : 'bg-paper-3 text-muted'
                              }`}
                            >
                              <Server className="h-4 w-4" />
                            </span>
                            <span className="min-w-0">
                              <span
                                className={`block truncate text-sm ${active ? 'font-medium text-accent' : 'text-ink'}`}
                              >
                                {svc.name}
                              </span>
                              <span className="block truncate font-mono text-xs text-muted">
                                {svc.basePath}
                              </span>
                            </span>
                          </span>
                          <ChevronRight
                            className="h-4 w-4 shrink-0 text-muted"
                            aria-hidden="true"
                          />
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </SectionCard>
          </div>

          <div className="lg:col-span-2">
            {!selectedService && (
              <div className="ui-card">
                <EmptyState
                  icon={<Server className="h-5 w-5" />}
                  title="Pick a service from the list"
                  description="You will get its routes, its OpenAPI spec, and a place to call it for real."
                />
              </div>
            )}

            {selectedService && (
              <SectionCard
                title={selectedService.name}
                description={selectedService.description || 'No description yet'}
                actions={
                  <span className="rounded-chip border border-accent-edge bg-accent-wash px-2 py-0.5 text-xs font-medium text-accent">
                    {selectedService.type}
                  </span>
                }
                flush
              >
                <div
                  role="tablist"
                  aria-label="Service details"
                  className="ui-scroll-x flex gap-5 border-b border-rule px-4"
                >
                  {DETAIL_TABS.map((t) => {
                    const active = detailTab === t.id;
                    return (
                      <button
                        key={t.id}
                        role="tab"
                        type="button"
                        aria-selected={active}
                        onClick={() => setDetailTab(t.id)}
                        className={`-mb-px border-b-2 py-3 text-sm whitespace-nowrap transition-colors duration-200 ${
                          active
                            ? 'border-accent font-medium text-accent'
                            : 'border-transparent text-muted hover:text-ink'
                        }`}
                      >
                        {t.label}
                      </button>
                    );
                  })}
                </div>

                <div className="ui-fade p-4" key={detailTab}>
                  {/* ---- ข้อมูล ---- */}
                  {detailTab === 'info' && (
                    <div className="space-y-4">
                      <div>
                        <h3 className="mb-2 text-sm font-medium text-ink">Routes</h3>
                        {!selectedService.resourcePaths ||
                        selectedService.resourcePaths.length === 0 ? (
                          <p className="text-sm text-muted">No routes mapped yet.</p>
                        ) : (
                          <ul className="divide-y divide-rule rounded-control border border-rule">
                            {selectedService.resourcePaths.map((rp) => (
                              <li
                                key={`${rp.path}-${rp.sourceAlias}`}
                                className="flex flex-wrap items-center justify-between gap-2 p-3"
                              >
                                <span className="flex min-w-0 flex-wrap items-center gap-2">
                                  {rp.methods.map((m) => (
                                    <MethodChip key={m} method={m} />
                                  ))}
                                  <span className="font-mono text-sm text-ink">{rp.path}</span>
                                </span>
                                <span className="font-mono text-xs text-muted">
                                  {rp.sourceAlias}
                                </span>
                              </li>
                            ))}
                          </ul>
                        )}
                      </div>

                      <div className="rounded-control border border-rule bg-paper-2 p-4">
                        <p className="text-sm font-medium text-ink">How to call it</p>
                        <pre className="ui-scroll-x mt-2 rounded-control bg-graphite p-3 font-mono text-xs text-graphite-ink">
                          {`GET ${GATEWAY_URL}/resources/${selectedService.basePath.replace(/^\//, '')}/<path>`}
                        </pre>
                        <p className="mt-2 text-sm text-ink-3">
                          Send an active key in the{' '}
                          <code className="rounded-chip bg-paper-3 px-1 py-0.5 font-mono text-xs">
                            X-API-Key
                          </code> header.
                        </p>
                      </div>
                    </div>
                  )}

                  {/* ---- OpenAPI ---- */}
                  {detailTab === 'spec' && (
                    <>
                      {isLoadingSpec && (
                        <Loading label="Loading spec">
                          <div className="space-y-2">
                            {[0, 1, 2].map((i) => (
                              <SkeletonLine key={i} className="h-8" />
                            ))}
                          </div>
                        </Loading>
                      )}

                      {!isLoadingSpec && (specError || !openApiSpec) && (
                        <EmptyState
                          icon={<FileText className="h-5 w-5" />}
                          title="No OpenAPI spec yet"
                          description="Nobody has published one for this service. Administrators add it under Manage Services."
                        />
                      )}

                      {!isLoadingSpec && openApiSpec && (
                        <div className="space-y-4">
                          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-rule pb-2">
                            <span className="text-sm font-medium text-ink">
                              {openApiSpec.info?.title || 'OpenAPI Specification'}
                            </span>
                            <span className="font-mono text-xs text-muted">
                              v{openApiSpec.info?.version || '1.0.0'}
                            </span>
                          </div>

                          <div className="max-h-96 space-y-2 overflow-y-auto pr-1">
                            {Object.entries(openApiSpec.paths || {}).map(([path, methods]) => (
                              <div
                                key={path}
                                className="rounded-control border border-rule bg-paper-2 p-3"
                              >
                                {Object.entries(methods).map(([method, details]) => (
                                  <div key={method} className="space-y-1">
                                    <div className="flex flex-wrap items-center gap-2">
                                      <MethodChip method={method} />
                                      <span className="font-mono text-sm text-ink">{path}</span>
                                      {details.summary && (
                                        <span className="text-xs text-muted">
                                          — {details.summary}
                                        </span>
                                      )}
                                    </div>
                                    {details.description && (
                                      <p className="text-xs text-muted">{details.description}</p>
                                    )}
                                  </div>
                                ))}
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </>
                  )}

                  {/* ---- ลองเรียก ---- */}
                  {detailTab === 'try' && (
                    <div className="space-y-4">
                      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                        <label className="block">
                          <span className="ui-field__label">Route to call</span>
                          <select
                            className="ui-input font-mono"
                            value={selectedPath}
                            onChange={(e) => {
                              setSelectedPath(e.target.value);
                              const match = selectedService.resourcePaths?.find(
                                (r) => r.path === e.target.value,
                              );
                              if (match?.methods[0]) setSelectedMethod(match.methods[0]);
                            }}
                          >
                            {selectedService.resourcePaths?.map((rp) => (
                              <option key={rp.path} value={rp.path}>
                                {rp.path}
                              </option>
                            ))}
                          </select>
                        </label>

                        <label className="block">
                          <span className="ui-field__label">API key</span>
                          <select
                            className="ui-input"
                            value={selectedApiKey}
                            onChange={(e) => setSelectedApiKey(e.target.value)}
                          >
                            {apiKeys && apiKeys.length > 0 ? (
                              apiKeys.map((k) => (
                                <option key={k.id} value={k.apiKey}>
                                  {k.name}
                                </option>
                              ))
                            ) : (
                              <option value="">No keys yet — issue one on the next tab</option>
                            )}
                          </select>
                        </label>
                      </div>

                      {/* Path Parameters Helper */}
                      {tryItPathParams.length > 0 && (
                        <div className="rounded-control border border-accent-edge bg-accent-wash/30 p-3 space-y-2">
                          <div className="flex items-center justify-between text-xs font-semibold text-accent">
                            <span>Path Parameters</span>
                            <span className="text-[10px] font-normal text-muted">
                              Type ID below to automatically replace in path
                            </span>
                          </div>
                          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                            {tryItPathParams.map((param) => (
                              <div key={param}>
                                <label className="text-[11px] font-mono font-semibold text-ink block mb-1">
                                  {`{${param}}`}
                                </label>
                                <input
                                  type="text"
                                  placeholder={`Enter ${param}...`}
                                  value={tryItPathParamValues[param] || ''}
                                  onChange={(e) => {
                                    const val = e.target.value;
                                    setTryItPathParamValues((prev) => ({ ...prev, [param]: val }));
                                  }}
                                  className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs font-mono text-ink outline-none focus:border-focus"
                                />
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      <label className="block">
                        <span className="ui-field__label">Query string</span>
                        <input
                          className="ui-input font-mono"
                          value={customParams}
                          onChange={(e) => setCustomParams(e.target.value)}
                          placeholder="f=json&limit=10"
                        />
                      </label>

                      <div>
                        <div className="mb-1 flex items-center justify-between">
                          <span className="ui-field__label mb-0">The same call, as curl</span>
                          <CopyButton value={curl} />
                        </div>
                        <pre className="ui-scroll-x rounded-control bg-graphite p-3 font-mono text-xs text-graphite-ink">
                          {curl}
                        </pre>
                      </div>

                      <div className="flex justify-end border-t border-rule pt-4">
                        <Button
                          variant="primary"
                          onClick={handleTryIt}
                          loading={isSending}
                          disabled={!selectedPath}
                          icon={<Play className="h-4 w-4" />}
                        >
                          Send request
                        </Button>
                      </div>

                      {(responseStatus !== null || responseBody) && (
                        <div className="divide-y divide-rule overflow-hidden rounded-control border border-rule">
                          <div
                            className="flex flex-wrap items-center justify-between gap-3 bg-paper-2 p-3"
                            aria-live="polite"
                          >
                            <div className="flex flex-wrap items-center gap-2 text-sm">
                              <span>Status</span>
                              {responseStatus !== null && (
                                <span
                                  className={`tabular rounded-chip border px-2 py-0.5 font-mono text-xs font-medium ${
                                    responseStatus < 400
                                      ? 'border-ok-edge bg-ok-wash text-ok'
                                      : 'border-danger-edge bg-danger-wash text-danger'
                                  }`}
                                >
                                  {responseStatus}
                                </span>
                              )}
                              {responseElapsedMs !== null && (
                                <span className="tabular font-mono text-xs text-muted">
                                  {responseElapsedMs} ms
                                </span>
                              )}
                              {tryItSizeKb && (
                                <span className="tabular font-mono text-xs text-muted">
                                  {tryItSizeKb} KB
                                </span>
                              )}
                            </div>

                            <div className="flex items-center gap-3">
                              {tryItIsGeoJson && (
                                <div className="flex rounded-control border border-rule bg-paper p-0.5 text-xs">
                                  <button
                                    type="button"
                                    onClick={() => setTryItResponseView('preview')}
                                    className={`flex items-center gap-1 rounded-chip px-2 py-0.5 text-2xs font-medium transition-colors ${
                                      tryItResponseView === 'preview'
                                        ? 'bg-accent text-accent-ink shadow-xs'
                                        : 'text-muted hover:text-ink'
                                    }`}
                                  >
                                    <MapPin className="h-3 w-3" /> Map
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => setTryItResponseView('raw')}
                                    className={`flex items-center gap-1 rounded-chip px-2 py-0.5 text-2xs font-medium transition-colors ${
                                      tryItResponseView === 'raw'
                                        ? 'bg-accent text-accent-ink shadow-xs'
                                        : 'text-muted hover:text-ink'
                                    }`}
                                  >
                                    <Code className="h-3 w-3" /> Raw
                                  </button>
                                </div>
                              )}

                              <span className="tabular flex flex-wrap gap-x-3 font-mono text-xs text-muted">
                                {responseHeaders['ratelimit-remaining'] && (
                                  <span>remaining {responseHeaders['ratelimit-remaining']}</span>
                                )}
                                {responseHeaders['ratelimit-limit'] && (
                                  <span>limit {responseHeaders['ratelimit-limit']}</span>
                                )}
                              </span>
                            </div>
                          </div>

                          {tryItIsGeoJson && tryItResponseView === 'preview' ? (
                            <div className="p-2 bg-paper">
                              <GeoMapPreview data={responseBody} />
                            </div>
                          ) : (
                            <pre className="max-h-72 overflow-auto bg-graphite p-4 font-mono text-xs break-words whitespace-pre-wrap text-graphite-ink">
                              {responseBody || 'No response body.'}
                            </pre>
                          )}
                        </div>
                      )}
                    </div>
                  )}

                  {/* ---- คู่มือ ---- */}
                  {detailTab === 'docs' && (
                    <>
                      {isLoadingDocs && (
                        <Loading label="Loading guides">
                          <div className="space-y-2">
                            {[0, 1].map((i) => (
                              <SkeletonLine key={i} className="h-8" />
                            ))}
                          </div>
                        </Loading>
                      )}

                      {!isLoadingDocs && (!apiDocs || apiDocs.length === 0) && (
                        <EmptyState
                          icon={<BookOpen className="h-5 w-5" />}
                          title="No guides published"
                          description="When an administrator publishes guides for this API, they show up here."
                        />
                      )}

                      {!isLoadingDocs && apiDocs && apiDocs.length > 0 && (
                        <ul className="space-y-2">
                          {apiDocs.map((doc) => (
                            <li
                              key={doc.id}
                              className="rounded-control border border-rule bg-paper-2 p-3"
                            >
                              <p className="text-sm font-medium text-ink">
                                {doc.info.title}{' '}
                                <span className="font-mono text-xs font-normal text-muted">
                                  v{doc.info.version}
                                </span>
                              </p>
                              {doc.info.description && (
                                <p className="mt-1 text-sm text-ink-3">{doc.info.description}</p>
                              )}
                            </li>
                          ))}
                        </ul>
                      )}
                    </>
                  )}
                </div>
              </SectionCard>
            )}
          </div>
        </div>
      )}

      {/* ================= API key ================= */}
      {mainTab === 'keys' && (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div className="lg:col-span-1">
            <SectionCard title="Issue a key">
              <form
                className="space-y-4"
                onSubmit={(e) => {
                  e.preventDefault();
                  createKey.mutate();
                }}
              >
                <Field
                  label="Key name"
                  value={newKeyName}
                  onChange={(e) => setNewKeyName(e.target.value)}
                  placeholder="e.g. Web frontend"
                  helper="Name it after where it runs, so you revoke the right one later"
                  required
                />
                <Field
                  label="Expires on"
                  type="date"
                  value={newKeyExpiry}
                  onChange={(e) => setNewKeyExpiry(e.target.value)}
                  helper="Leave empty for a key that never expires"
                />
                <Button type="submit" variant="primary" block loading={createKey.isPending}>
                  Issue key
                </Button>
              </form>
            </SectionCard>
          </div>

          <div className="space-y-6 lg:col-span-2">
            {createdKey && (
              <section className="ui-rise rounded-surface border border-ok-edge bg-ok-wash p-4">
                <h2 className="font-title text-base font-semibold text-ink">
                  Copy this key now
                </h2>
                <p className="mt-1 text-sm text-ink-2">
                  This value is shown once. Close or refresh the page and it is gone for good.
                </p>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <pre className="ui-scroll-x min-w-0 flex-1 rounded-control border border-ok-edge bg-paper p-3 font-mono text-sm break-all text-ink select-all">
                    {createdKey}
                  </pre>
                  <CopyButton value={createdKey} variant="primary" size="md" />
                </div>

                <div className="mt-3.5 border-t border-ok-edge/40 pt-3">
                  <span className="block text-xs font-medium text-ink mb-1.5">
                    Test your new key with curl:
                  </span>
                  <div className="flex items-center justify-between gap-2 rounded-control border border-rule bg-paper p-2.5">
                    <code className="ui-scroll-x font-mono text-xs text-ink truncate">
                      curl -H &quot;X-API-Key: {createdKey}&quot; http://localhost:9002/gateway/api/resources/{services?.[0]?.basePath?.replace(/^\//, '') || 'my-service'}
                    </code>
                    <CopyButton
                      value={`curl -H "X-API-Key: ${createdKey}" http://localhost:9002/gateway/api/resources/${services?.[0]?.basePath?.replace(/^\//, '') || 'my-service'}`}
                      size="sm"
                    />
                  </div>
                </div>

                <div className="mt-3">
                  <Button size="sm" variant="ghost" onClick={() => setCreatedKey(null)}>
                    Saved it, close this
                  </Button>
                </div>
              </section>
            )}

            <SectionCard title="Active keys" flush>
              {isLoadingKeys && (
                <Loading label="Loading keys">
                  <div className="space-y-2 p-4">
                    {[0, 1, 2].map((i) => (
                      <SkeletonLine key={i} className="h-6" />
                    ))}
                  </div>
                </Loading>
              )}

              {!isLoadingKeys && (!apiKeys || apiKeys.length === 0) && (
                <EmptyState
                  icon={<Key className="h-5 w-5" />}
                  title="No keys yet"
                  description="Issue one on the left, then you can call the APIs through the gateway."
                  action={
                    <Button variant="secondary" icon={<Plus className="h-4 w-4" />} disabled>
                      Use the form on the left
                    </Button>
                  }
                />
              )}

              {!isLoadingKeys && apiKeys && apiKeys.length > 0 && (
                <div className="ui-scroll-x">
                  <table className="w-full border-collapse text-left text-sm">
                    <thead>
                      <tr className="border-b border-rule text-xs text-muted">
                        <th className="px-3 py-2 font-medium whitespace-nowrap">Name</th>
                        <th className="px-3 py-2 font-medium whitespace-nowrap">Expires</th>
                        <th className="px-3 py-2 font-medium whitespace-nowrap">Created</th>
                        <th className="px-3 py-2 text-right font-medium whitespace-nowrap">
                          Actions
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-rule">
                      {apiKeys.map((key) => (
                        <tr key={key.id}>
                          <td className="px-3 py-3 font-medium text-ink">{key.name}</td>
                          <td className="tabular px-3 py-3 whitespace-nowrap text-muted">
                            {key.expiredAt ? (
                              <span className="inline-flex items-center gap-1">
                                <Calendar className="h-3.5 w-3.5" aria-hidden="true" />
                                {new Date(key.expiredAt).toLocaleDateString()}
                              </span>
                            ) : (
                              'Never'
                            )}
                          </td>
                          <td className="tabular px-3 py-3 whitespace-nowrap text-muted">
                            {key.createdAt ? new Date(key.createdAt).toLocaleDateString() : '—'}
                          </td>
                          <td className="px-3 py-3">
                            <span className="flex justify-end gap-1">
                              <Button
                                size="sm"
                                onClick={() => askRegenerate(key)}
                                loading={
                                  regenerateKey.isPending && regenerateKey.variables === key.id
                                }
                              >
                                Rotate
                              </Button>
                              <Button
                                size="sm"
                                variant="ghost-danger"
                                onClick={() => askDelete(key)}
                                loading={deleteKey.isPending && deleteKey.variables === key.id}
                                aria-label={`Delete key ${key.name}`}
                                icon={<Trash2 className="h-4 w-4" />}
                              />
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </SectionCard>
          </div>
        </div>
      )}
    </NavigationShell>
  );
}

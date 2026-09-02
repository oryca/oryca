'use client';

import { useState, useMemo, useRef } from 'react';
import { useMutation, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { useInfiniteList } from '@/lib/useInfiniteList';
import { useDebounced } from '@/lib/useDebounced';
import NavigationShell from '@/components/NavigationShell';
import Link from 'next/link';
import {
  PageHeader,
  SkeletonLine,
  Select,
  useToast,
  useConfirm,
  InfiniteScrollSentinel,
} from '@/components/ui';
import {
  Server,
  Plus,
  Trash2,
  Edit,
  AlertTriangle,
  BookOpen,
  Sliders,
  Search,
  X,
  Filter,
} from 'lucide-react';

const SERVICE_TYPE_OPTIONS = [
  { value: 'General', label: 'General API' },
  { value: 'OGC_API_Features', label: 'OGC API Features' },
  { value: 'OGC_API_Styles', label: 'OGC API Styles' },
  { value: 'OGC_API_Tiles', label: 'OGC API Tiles' },
];

const METHOD_OPTIONS = ['GET', 'POST', 'PUT', 'DELETE'].map((m) => ({ value: m, label: m }));

const TARGET_OPTIONS = [
  { value: '__api__', label: 'Forward to a URL' },
  { value: '__static__', label: 'Answer with a fixed body' },
];

interface GatewaySource {
  id: string;
  alias: string;
  name: string;
  description?: string;
  type: string; // upstream | static
  protocol?: string; // http | https
  url?: string;
  headers?: Array<{ key: string; value: string }>;
  contentType?: string;
  body?: string;
}

/** The upstream a route carries with it, saved in the same call as the service */
interface InlineSource {
  type: 'api' | 'static';
  protocol?: string;
  url?: string;
  contentType?: string;
  body?: string;
  headers?: Array<{ key: string; value: string }>;
}

interface ResourcePath {
  path: string;
  methods: string[];
  sourceAlias: string;
  /** Present when this route defines its own upstream instead of reusing a saved one */
  source?: InlineSource;
}

interface GatewayService {
  id: string;
  name: string;
  description?: string;
  type: string; // General | OGC_API_Features | OGC_API_Styles | OGC_API_Tiles
  basePath: string;
  enabled?: boolean;
  isPublic?: boolean;
  resourcePaths?: Array<ResourcePath & { source?: GatewaySource }>;
  ogc?: { version?: string; parts?: string[] } | null;
}

/** A saved route becomes an editable row, with the upstream it resolved to opened
 *  up for editing in place — one route, one target. */
function toFormRow(rp: ResourcePath & { source?: GatewaySource }): ResourcePath {
  const base = { path: rp.path, methods: rp.methods, sourceAlias: rp.sourceAlias };
  if (!rp.source) return base;
  // the API hands back a source without its url or body when the caller is not
  // allowed to see them; editing that as if it were empty would erase it
  const isStatic = rp.source.type === 'static';
  if (isStatic ? !rp.source.body : !rp.source.url) return base;
  return {
    ...base,
    source: {
      type: rp.source.type === 'static' ? 'static' : 'api',
      protocol: rp.source.protocol,
      url: rp.source.url || '',
      contentType: rp.source.contentType || 'application/json',
      body: rp.source.body || '',
      // PUT replaces the whole source, so headers set elsewhere ride along untouched
      headers: rp.source.headers || [],
    },
  };
}

/** A row as the service payload wants it. An inline target rides along under
 *  `source`; an empty alias tells the control plane to mint one for it. */
function toApiRow(rp: ResourcePath): ResourcePath {
  const row = { path: rp.path, methods: rp.methods, sourceAlias: rp.sourceAlias };
  if (!rp.source) return row;
  if (rp.source.type === 'static') {
    return {
      ...row,
      source: {
        ...rp.source,
        protocol: 'https',
        url: '',
        contentType: (rp.source.contentType || '').trim() || 'application/json',
      },
    };
  }
  const url = (rp.source.url || '').trim();
  const withScheme = /^https?:\/\//i.test(url) ? url : `https://${url}`;
  return {
    ...row,
    source: {
      ...rp.source,
      url: withScheme,
      protocol: withScheme.toLowerCase().startsWith('http://') ? 'http' : 'https',
      body: '',
    },
  };
}

function targetSelectValue(rp: ResourcePath): string {
  if (!rp.source) return '';
  return rp.source.type === 'static' ? '__static__' : '__api__';
}

export default function AdminServicesPage() {
  const queryClient = useQueryClient();

  // Form toggles
  const [editingService, setEditingService] = useState<GatewayService | null>(null);
  const [isServiceFormOpen, setIsServiceFormOpen] = useState(false);

  return (
    <NavigationShell>
      <PageHeader
        title="Manage Services"
        description="Upstream servers, the routes you publish from them, and the rules that rewrite responses"
      />

      <div className="space-y-6">
        <ServicesManager
          isOpen={isServiceFormOpen}
          setIsOpen={setIsServiceFormOpen}
          editingService={editingService}
          setEditingService={setEditingService}
          queryClient={queryClient}
        />
      </div>
    </NavigationShell>
  );
}

// ================================= SERVICES SUB-COMPONENT =================================
function ServicesManager({
  isOpen,
  setIsOpen,
  editingService,
  setEditingService,
  queryClient
}: {
  isOpen: boolean;
  setIsOpen: (val: boolean) => void;
  editingService: GatewayService | null;
  setEditingService: (svc: GatewayService | null) => void;
  queryClient: QueryClient;
}) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [type, setType] = useState('General');
  const [basePath, setBasePath] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [isPublic, setIsPublic] = useState(false);
  const [resourcePaths, setResourcePaths] = useState<ResourcePath[]>([
    { path: '/', methods: ['GET'], sourceAlias: '' },
  ]);

  const [searchQuery, setSearchQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('ALL');
  const debouncedSearch = useDebounced(searchQuery.trim());
  const listScrollRef = useRef<HTMLDivElement>(null);

  // Search and type filter go to the server — the list is paged, so filtering here
  // would only ever look at the rows already scrolled into view.
  const {
    items: filteredServices,
    isLoading,
    total,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useInfiniteList<GatewayService>(['admin-services'], '/services', {
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
    ...(typeFilter !== 'ALL' ? { type: typeFilter } : {}),
  });

  // The closed set of types, not what the loaded page happens to contain.
  const serviceTypes = useMemo(() => SERVICE_TYPE_OPTIONS.map((o) => o.value), []);

  const [formError, setFormError] = useState<string | null>(null);
  const { toast } = useToast();
  const confirm = useConfirm();
  const [isApplyingOgcTemplate, setIsApplyingOgcTemplate] = useState(false);

  // ปรับ state ระหว่าง render ตามแนวทางของ React ไม่ใช่ใน effect (กัน cascading render)
  const serviceFormKey = editingService?.id ?? (isOpen ? 'new' : 'closed');
  const [syncedServiceKey, setSyncedServiceKey] = useState(serviceFormKey);
  if (serviceFormKey !== syncedServiceKey) {
    setSyncedServiceKey(serviceFormKey);
    if (editingService) {
      setName(editingService.name || '');
      setDescription(editingService.description || '');
      setType(editingService.type || 'General');
      setBasePath(editingService.basePath || '');
      setEnabled(editingService.enabled !== false);
      setIsPublic(editingService.isPublic === true);
      setResourcePaths(
        editingService.resourcePaths && editingService.resourcePaths.length > 0
          ? editingService.resourcePaths.map(toFormRow)
          : [{ path: '/', methods: ['GET'], sourceAlias: '' }]
      );
    } else {
      setName('');
      setDescription('');
      setType('General');
      setBasePath('');
      setEnabled(true);
      setIsPublic(false);
      setResourcePaths([{ path: '/', methods: ['GET'], sourceAlias: '' }]);
    }
  }

  // A path that cannot match any real standard path, sent as the current
  // resourcePaths so every core path of the type comes back as "missing" —
  // that missing list is the full template. Reuses the same backend data
  // /services/check-paths already serves, just asked for differently.
  const OGC_TEMPLATE_PROBE_PATH = '/__ogc-template-probe__';
  // check-paths requires a non-empty basePath to also check for conflicts;
  // there is no real basePath yet at the point a type is picked, and the
  // conflict result is discarded here, so a placeholder is enough.
  const OGC_TEMPLATE_PROBE_BASE_PATH = '/__ogc-template-probe__';

  async function fetchOgcCorePaths(svcType: string): Promise<string[]> {
    const res = await api.post('/services/check-paths', {
      type: svcType,
      basePath: OGC_TEMPLATE_PROBE_BASE_PATH,
      resourcePaths: [OGC_TEMPLATE_PROBE_PATH],
    });
    return res.data.missingOgcPaths || [];
  }

  /** Counts type changes so a slow template fetch cannot land after a newer one */
  const ogcTemplateRequest = useRef(0);

  /** Path work a retype would destroy: a saved service's routes, or rows already
   *  mapped to an upstream. A fresh template has neither, so it stays quiet. */
  function hasPathWork(): boolean {
    if (resourcePaths.length === 0) return false;
    return !!editingService || resourcePaths.some((rp) => rp.sourceAlias !== '' || !!rp.source);
  }

  /** Picking a standard replaces the path list outright with what it expects —
   *  there is no partial state to preserve once the type itself has changed. */
  async function applyServiceType(newType: string) {
    if (newType === type) return;
    if (hasPathWork()) {
      const wants = await confirm({
        title: 'Replace the routes below?',
        description: `Switching to ${newType === 'General' ? 'General API' : newType} rewrites the path list from scratch.`,
        consequences: [
          `The ${resourcePaths.length} route${resourcePaths.length === 1 ? '' : 's'} configured here are discarded`,
          'Their methods and upstream mappings go with them',
        ],
        confirmLabel: 'Replace routes',
        cancelLabel: 'Keep current type',
      });
      if (!wants) return;
    }

    const request = ++ogcTemplateRequest.current;
    setType(newType);
    if (newType === 'General') {
      setResourcePaths([{ path: '/', methods: ['GET'], sourceAlias: '' }]);
      return;
    }
    setIsApplyingOgcTemplate(true);
    try {
      const paths = await fetchOgcCorePaths(newType);
      if (request !== ogcTemplateRequest.current) return;
      // left unmapped on purpose — the standard only supplies the routes,
      // not which upstream answers each one, so that stays a manual choice
      setResourcePaths(
        (paths.length > 0 ? paths : ['/']).map((path) => ({
          path,
          methods: ['GET'],
          sourceAlias: '',
        })),
      );
    } catch {
      // could not reach the standard's path list — leave the rows as they were
    } finally {
      if (request === ogcTemplateRequest.current) setIsApplyingOgcTemplate(false);
    }
  }

  const addPathRow = (pathValue = '/') => {
    setResourcePaths((prev) => [...prev, { path: pathValue, methods: ['GET'], sourceAlias: '' }]);
  };

  /** Switches what a route answers with. The alias rides along untouched: an empty one
   *  has the control plane mint an upstream, an existing one is rewritten in place. */
  const setRowTarget = (index: number, value: string) => {
    setResourcePaths((prev) =>
      prev.map((rp, i) => {
        if (i !== index) return rp;
        if (value !== '__api__' && value !== '__static__') return { ...rp, source: undefined };
        const type = value === '__api__' ? 'api' : 'static';
        if (rp.source) return { ...rp, source: { ...rp.source, type } };
        return {
          ...rp,
          source: { type, protocol: 'https', url: '', contentType: 'application/json', body: '' },
        };
      }),
    );
  };

  const updateRowSource = (index: number, patch: Partial<InlineSource>) => {
    setResourcePaths((prev) =>
      prev.map((rp, i) => (i === index && rp.source ? { ...rp, source: { ...rp.source, ...patch } } : rp)),
    );
  };

  const removePathRow = (index: number) => {
    setResourcePaths((prev) => prev.filter((_, i) => i !== index));
  };

  const updatePathRow = (index: number, field: keyof ResourcePath, value: string | string[]) => {
    setResourcePaths((prev) =>
      prev.map((rp, i) => (i === index ? { ...rp, [field]: value } : rp))
    );
  };

  const saveMutation = useMutation({
    mutationFn: async () => {
      setFormError(null);
      resourcePaths.forEach((rp) => {
        if (rp.source) {
          if (rp.source.type === 'api' && !(rp.source.url || '').trim()) {
            throw new Error(`${rp.path} needs a URL to forward to.`);
          }
          if (rp.source.type === 'static' && !(rp.source.body || '').trim()) {
            throw new Error(`${rp.path} needs a body to answer with.`);
          }
        } else if (!rp.sourceAlias) {
          throw new Error(`${rp.path} has no target yet.`);
        }
      });
      if (!basePath.startsWith('/')) {
        throw new Error('Base Path must start with /');
      }

      const payload = {
        name,
        description,
        type,
        basePath,
        enabled,
        isPublic,
        resourcePaths: resourcePaths.map(toApiRow),
        // ฟอร์มนี้ไม่ได้แก้ ogc แต่ PUT เป็น replace ไม่ส่งคืนไปเท่ากับล้างทิ้ง
        ogc: editingService?.ogc ?? undefined,
      };

      let serviceId = editingService?.id;
      if (editingService) {
        await api.put(`/services/${editingService.id}`, payload);
      } else {
        const res = await api.post('/services', payload);
        serviceId = res.data?.id;
      }

      // An OGC service almost always needs its links rewritten, so offer it here
      // rather than leaving it to be discovered on another page.
      if (serviceId && type !== 'General') {
        try {
          const presets = await api.get(`/response-transforms/presets?type=${type}`);
          const preset = (presets.data.items || [])[0];
          const wantsPreset =
            preset &&
            (await confirm({
              title: 'Rewrite the links this service returns?',
              description: `${name} answers with links to itself, which would send clients past the gateway.`,
              consequences: [`Applies the "${preset.title || preset.name}" preset`, 'You can edit or remove it later under Transforms & Presets'],
              confirmLabel: 'Apply preset',
              cancelLabel: 'Not now',
            }));
          if (wantsPreset) {
            await api.post(`/response-transforms/presets/${preset.name}/apply`, { serviceId });
          }
        } catch (err) {
          console.error('Could not offer the link-rewrite preset:', err);
        }
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-services'] });
      // saving a service now creates, updates, and retires upstreams too
      queryClient.invalidateQueries({ queryKey: ['admin-sources'] });
      setIsOpen(false);
      setEditingService(null);
      toast({ tone: 'ok', message: 'Service saved' });
    },
    onError: (err: unknown) => {
      setFormError(err instanceof Error && err.message ? err.message : 'Failed to save service.');
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/services/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-services'] });
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row justify-between sm:items-center gap-3">
        <div>
          <h2 className="text-xs font-bold text-muted uppercase tracking-wider">Publish Routing Gateways</h2>
          <p className="text-xs text-muted mt-0.5">
            {total} published gateway service{total === 1 ? '' : 's'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative min-w-[220px]">
              <Search className="w-3.5 h-3.5 text-muted absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search services or paths..."
                className="w-full bg-paper border border-rule rounded-control pl-8 pr-7 py-1.5 text-xs text-ink placeholder:text-muted outline-none focus:border-focus"
              />
              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-ink"
                >
                  <X className="w-3 h-3" />
                </button>
              )}
          </div>
          <button
            onClick={() => {
              setEditingService(null);
              setIsOpen(true);
            }}
            className="px-3 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short flex items-center gap-1 shrink-0"
          >
            <Plus className="w-4 h-4" /> Publish Service
          </button>
        </div>
      </div>

      {/* Type Filter Pills */}
      {serviceTypes.length > 1 && (
        <div className="flex flex-wrap gap-1.5 items-center">
          <span className="text-[11px] text-muted flex items-center gap-1 mr-1">
            <Filter className="w-3 h-3" /> Type:
          </span>
          <button
            type="button"
            onClick={() => setTypeFilter('ALL')}
            className={`px-2.5 py-1 text-[11px] rounded-control transition-colors ${
              typeFilter === 'ALL'
                ? 'bg-accent text-accent-ink font-semibold'
                : 'bg-paper-2 border border-rule text-muted hover:text-ink'
            }`}
          >
            All
          </button>
          {serviceTypes.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTypeFilter(t)}
              className={`px-2.5 py-1 text-[11px] rounded-control transition-colors ${
                typeFilter === t
                  ? 'bg-accent text-accent-ink font-semibold'
                  : 'bg-paper-2 border border-rule text-muted hover:text-ink'
              }`}
            >
              {t}
            </button>
          ))}
        </div>
      )}

      {isOpen && (
        <div className="ui-modal-scrim">
          <div className="bg-paper border border-rule rounded-surface p-6 w-full max-w-2xl lg:max-w-4xl xl:max-w-5xl shadow-sm space-y-4 max-h-[95vh] overflow-y-auto">
            <div className="flex items-center justify-between">
              <h3 className="font-title text-base font-bold text-ink">
                {editingService ? 'Edit Gateway Service' : 'Publish New Service'}
              </h3>
              <button
                onClick={() => setIsOpen(false)}
                className="p-1.5 rounded-control text-muted transition hover:bg-paper-3 hover:text-ink cursor-pointer"
                aria-label="Close"
                title="Close"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {formError && (
              <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <div>{formError}</div>
              </div>
            )}

            <form
              onSubmit={(e) => {
                e.preventDefault();
                saveMutation.mutate();
              }}
              className="space-y-4 text-xs"
            >
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Service Name<span className="ui-field__req" aria-hidden="true">*</span>
                  </label>
                  <input
                    type="text"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. OGC Features API"
                    className="ui-input"
                  />
                </div>
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Base Path (Prefix route)<span className="ui-field__req" aria-hidden="true">*</span>
                  </label>
                  <input
                    type="text"
                    required
                    value={basePath}
                    onChange={(e) => setBasePath(e.target.value)}
                    placeholder="/my-service"
                    className="ui-input font-mono"
                  />
                </div>
              </div>

              <div>
                <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Description
                </label>
                <input
                  type="text"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Geospatial feature server description"
                  className="ui-input"
                />
              </div>

              <div className="grid grid-cols-3 gap-4 items-center">
                <div>
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Service Type<span className="ui-field__req" aria-hidden="true">*</span>
                  </label>
                  {/* value stays on the current type until applyServiceType
                      confirms and calls setType */}
                  <Select
                    required
                    value={type}
                    onChange={applyServiceType}
                    options={SERVICE_TYPE_OPTIONS}
                  />
                </div>

                <div className="flex items-center gap-2 pt-5">
                  <input
                    type="checkbox"
                    id="svc-enabled"
                    checked={enabled}
                    onChange={(e) => setEnabled(e.target.checked)}
                    className="w-4 h-4 accent-accent cursor-pointer"
                  />
                  <label htmlFor="svc-enabled" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                    Enable Routing
                  </label>
                </div>

                <div className="flex items-center gap-2 pt-5">
                  <input
                    type="checkbox"
                    id="svc-public"
                    checked={isPublic}
                    onChange={(e) => setIsPublic(e.target.checked)}
                    className="w-4 h-4 accent-accent cursor-pointer"
                  />
                  <label htmlFor="svc-public" className="text-xs font-semibold text-ink-2 select-none cursor-pointer">
                    Public access (no API key)
                  </label>
                </div>
              </div>

              {/* Picking a standard already filled the paths below in with its template */}
              {isApplyingOgcTemplate && (
                <p className="mt-2 text-xs text-muted">Loading the standard routes for this type…</p>
              )}

              {/* Resource paths editor mapping */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-semibold text-muted uppercase tracking-wider">
                    Resource Paths & Target Sources<span className="ui-field__req" aria-hidden="true">*</span>
                  </label>
                  <button
                    type="button"
                    onClick={() => addPathRow()}
                    className="text-xs text-accent font-semibold underline underline-offset-2 cursor-pointer"
                  >
                    + Add Path
                  </button>
                </div>

                <div className="space-y-2 max-h-[40vh] lg:max-h-[50vh] overflow-y-auto pr-1">
                  {resourcePaths.map((rp, index) => (
                    <div
                      key={index}
                      className="bg-paper-2 p-2 border border-rule rounded-control space-y-2"
                    >
                      <div className="flex gap-2 items-center">
                        <div className="flex-[2] min-w-0">
                          <input
                            type="text"
                            required
                            value={rp.path}
                            onChange={(e) => updatePathRow(index, 'path', e.target.value)}
                            placeholder="/collections/*"
                            className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                          />
                        </div>

                        <Select
                          size="sm"
                          className="flex-1"
                          aria-label="Method"
                          value={rp.methods[0] || 'GET'}
                          onChange={(val) => updatePathRow(index, 'methods', [val])}
                          options={METHOD_OPTIONS}
                        />

                        <Select
                          size="sm"
                          className="flex-[2]"
                          required
                          aria-label="Upstream mapping"
                          placeholder="Map upstream..."
                          value={targetSelectValue(rp)}
                          onChange={(val) => setRowTarget(index, val)}
                          options={TARGET_OPTIONS}
                        />

                        <button
                          type="button"
                          onClick={() => removePathRow(index)}
                          disabled={resourcePaths.length <= 1}
                          className="p-1 rounded-control text-muted hover:text-danger hover:bg-danger-wash disabled:opacity-30 cursor-pointer"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>

                      {rp.source && (
                        <div className="space-y-2 border-t border-rule pt-2">
                          {rp.source.type === 'api' ? (
                            <>
                              <p className="text-[11px] text-ink-2">
                                The whole destination address, not a prefix: this path is sent exactly there.
                              </p>
                              <input
                                type="text"
                                required
                                value={rp.source.url || ''}
                                onChange={(e) => updateRowSource(index, { url: e.target.value })}
                                placeholder="https://demo.pygeoapi.io/master/collections"
                                className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                              />
                            </>
                          ) : (
                            <>
                              <p className="text-[11px] text-ink-2">
                                Nothing is forwarded. The gateway answers with this body, for an
                                endpoint your server does not have.
                              </p>
                              <input
                                type="text"
                                value={rp.source.contentType || ''}
                                onChange={(e) => updateRowSource(index, { contentType: e.target.value })}
                                placeholder="application/json"
                                className="w-full bg-paper border border-rule rounded-control px-2.5 py-1 text-xs text-ink outline-none focus:border-focus font-mono"
                              />
                              <textarea
                                required
                                rows={10}
                                value={rp.source.body || ''}
                                onChange={(e) => updateRowSource(index, { body: e.target.value })}
                                placeholder={'{ "links": [] }'}
                                className="w-full min-h-[9rem] lg:min-h-[14rem] resize-y bg-paper border border-rule rounded-control px-2.5 py-2 text-[11px] leading-relaxed text-ink outline-none focus:border-focus font-mono"
                              />
                              {rp.path.replace(/\/$/, '').endsWith('/conformance') && (
                                <p className="text-[11px] text-warn">
                                  A conformance document states which classes a server implements.
                                  Writing one here declares that on its behalf, so list only what it
                                  really does.
                                </p>
                              )}
                            </>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => {
                    setIsOpen(false);
                    setEditingService(null);
                  }}
                  className="px-4 py-2 border border-rule hover:border-faint rounded-control text-ink-2 hover:bg-paper-2 transition cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="px-4 py-2 bg-accent hover:bg-accent-deep text-accent-ink font-semibold rounded-control transition cursor-pointer"
                >
                  {saveMutation.isPending ? 'Publishing...' : 'Save Service'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="space-y-4">
          {[...Array(2)].map((_, i) => (
            <SkeletonLine key={i} className="h-16" />
          ))}
        </div>
      ) : filteredServices.length === 0 ? (
        <div className="p-8 bg-paper border border-rule border-dashed rounded-surface text-center text-xs text-muted">
          {debouncedSearch || typeFilter !== 'ALL' ? (
            <>
              No services match your search &quot;{searchQuery}&quot;
              {typeFilter !== 'ALL' ? ` in type ${typeFilter}` : ''}.
            </>
          ) : (
            'No services published yet. Publish one to start routing traffic.'
          )}
        </div>
      ) : (
        <div
          ref={listScrollRef}
          className="max-h-[min(980px,70vh)] space-y-3 overflow-y-auto"
        >
          {filteredServices.map((svc) => (
            <div key={svc.id} className="bg-paper border border-rule p-4 rounded-surface shadow-sm hover:border-faint transition flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div className="flex items-start gap-3">
                <div className="p-2 rounded-control bg-paper-2 text-muted mt-1">
                  <Server className="w-5 h-5" />
                </div>
                <div>
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-title text-sm font-semibold text-ink">{svc.name}</span>
                    <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-accent-wash border border-accent-edge text-accent">
                      {svc.type}
                    </span>
                    {svc.isPublic && (
                      <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-ok-wash border border-ok-edge text-ok">
                        Public Route
                      </span>
                    )}
                    {svc.enabled === false && (
                      <span className="text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-chip bg-danger-wash border border-danger-edge text-danger">
                        Disabled
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-muted mt-1">{svc.description || 'No description.'}</p>
                  <div className="mt-2 flex gap-4 text-[10px] text-muted font-mono">
                    <span>Base: {svc.basePath}</span>
                    <span>Paths: {svc.resourcePaths?.length || 0} mapped</span>
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-2 self-end sm:self-center">
                <Link
                  href={`/admin/transforms?serviceId=${svc.id}`}
                  className="px-2.5 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition flex items-center gap-1"
                >
                  <Sliders className="w-3.5 h-3.5" /> Transforms
                </Link>
                <Link
                  href={`/admin/services/${svc.id}/spec`}
                  className="px-2.5 py-1 bg-paper border border-rule hover:border-faint text-[10px] font-semibold rounded-chip text-ink-2 hover:bg-paper-2 transition flex items-center gap-1"
                >
                  <BookOpen className="w-3.5 h-3.5" /> Spec Editor
                </Link>
                <button
                  onClick={() => {
                    setEditingService(svc);
                    setIsOpen(true);
                  }}
                  className="p-1.5 rounded-control text-muted hover:text-ink hover:bg-paper-3 transition"
                  title="Edit Service"
                >
                  <Edit className="w-4 h-4" />
                </button>
                <button
                  onClick={async () => {
                    const ok = await confirm({
                      title: `Delete service "${svc.name}"`,
                      consequences: [
                        'This cannot be undone',
                        'Its routes stop answering through the gateway immediately',
                        'Its spec and rewrite rules go with it',
                      ],
                      typeToConfirm: svc.name,
                      confirmLabel: 'Delete service',
                      danger: true,
                    });
                    if (ok) deleteMutation.mutate(svc.id);
                  }}
                  disabled={deleteMutation.isPending}
                  className="p-1.5 rounded-control text-muted hover:text-danger hover:bg-danger-wash border border-transparent hover:border-danger-edge transition"
                  title="Delete Service"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
          <InfiniteScrollSentinel
            rootRef={listScrollRef}
            hasNextPage={hasNextPage}
            isFetchingNextPage={isFetchingNextPage}
            fetchNextPage={fetchNextPage}
          />
        </div>
      )}
    </div>
  );
}
